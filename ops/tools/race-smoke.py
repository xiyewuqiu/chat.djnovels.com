#!/usr/bin/env python3
"""并发冒烟：用 -race 编译的引擎跑真实请求，检查数据竞争。

为什么需要它：单元测试只能覆盖被测到的路径。-race 版二进制跑真实 HTTP 请求，
能覆盖 handler → 缓存 → 统计 → 存储 这一整条链路，是发现"高并发才出现"问题的
最直接手段。

用法：

    # 1) 用 race detector 编译引擎
    go build -race -o /tmp/engine-race ./launcher

    # 2) 起它，把输出重定向到文件（race 报告打到 stderr）
    /tmp/engine-race > /tmp/engine.log 2>&1 &

    # 3) 压它
    python3 ops/tools/race-smoke.py --appkey xxx --app-secret yyy

    # 4) 看结果
    grep -c "DATA RACE" /tmp/engine.log

脚本只做只读与幂等写（发消息会真的落库，指向测试租户即可）。
"""

import argparse
import hashlib
import json
import socket
import threading
import time
import urllib.error
import urllib.request

stop = threading.Event()
# timeout 单独计数：race detector 会让引擎慢 2–5 倍，压测里超时很正常，
# 把它和"真的出错"混在一起会导致误判。
stats = {"ok": 0, "fail": 0, "timeout": 0, "rate_limited": 0, "hist_ok": 0, "markread_ok": 0, "search_hits": 0}
stats_lock = threading.Lock()


def bump(key, n=1):
    with stats_lock:
        stats[key] += n


def signed_headers(appkey, app_secret):
    # 括号不能省：% 左结合，"race%d" % x % y 会先格式化再对字符串取模。
    nonce = "race%d" % (int(time.time() * 1000000) % 1000000007)
    ts = str(int(time.time() * 1000))
    sig = hashlib.sha1((app_secret + nonce + ts).encode()).hexdigest()
    return {
        "appkey": appkey,
        "nonce": nonce,
        "timestamp": ts,
        "signature": sig,
        "Content-Type": "application/json",
    }


def get(url, headers, timeout=15):
    req = urllib.request.Request(url, headers=headers, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as err:
        try:
            return json.loads(err.read())
        finally:
            err.close()
    except (socket.timeout, TimeoutError):
        return {"code": -2}
    except Exception:
        return {"code": -1}


def post(url, body, headers, timeout=15):
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as err:
        try:
            return json.loads(err.read())
        finally:
            err.close()
    except (socket.timeout, TimeoutError):
        return {"code": -2}  # 超时（race 版慢，属预期）
    except Exception:
        return {"code": -1}  # 其他错误


def worker(index, args):
    base = args.engine.rstrip("/")
    round_no = 0
    while not stop.is_set():
        round_no += 1
        headers = signed_headers(args.appkey, args.app_secret)
        body = {
            "sender_id": "race-u%d" % index,
            "target_id": "race-peer",
            "channel_type": 1,
            "msg_type": "jg:text",
            "msg_content": json.dumps({"content": "并发冒烟 %d 书单" % index}, ensure_ascii=False),
        }
        resp = post(base + "/apigateway/messages/private/send", body, headers)
        code = resp.get("code")
        last_msg_id = ""
        if code == 0:
            bump("ok")
            # 发送响应形如 {"data":[{"target_id":"...","msg_id":"..."}]}
            data = resp.get("data")
            if isinstance(data, list) and data:
                last_msg_id = data[0].get("msg_id", "") or ""
        elif code == -2:
            bump("timeout")
        else:
            bump("fail")

        headers = signed_headers(args.appkey, args.app_secret)
        # 拉历史：走 historymsg 的**读**路径（发送走的是写路径），
        # 两条路径用的缓存与存储方法不同，都要压。
        if round_no % 5 == 0:
            ts = int(time.time() * 1000)
            qurl = ("%s/apigateway/hismsgs/query?channel_type=1&from_id=race-u%d&target_id=race-peer&start=%d&count=20"
                    % (base, index, ts))
            hresp = get(qurl, headers)
            hcode = hresp.get("code")
            if hcode == 0:
                bump("hist_ok")
            elif hcode == -2:
                bump("timeout")
            else:
                bump("fail")

        # 已读上报：走 read receipt 的写路径（异步 RPC + 更新 is_read）。
        # msg_id 用不存在的值没关系——压的是代码路径与锁，不是数据效果。
        if round_no % 10 == 0 and last_msg_id:
            # 用刚发出去的真实 msg_id：这样才会走到真正的 UPDATE，
            # 假 id 只会命中"影响 0 行"的路径，压不到实际更新。
            mr = post(base + "/apigateway/messages/markread", {
                "user_id": "race-u%d" % index,
                "target_id": "race-peer",
                "channel_type": 1,
                "msg_ids": [last_msg_id],
            }, headers)
            mr_code = mr.get("code")
            if mr_code == 0:
                bump("markread_ok")
            elif mr_code == -2:
                bump("timeout")
            else:
                bump("fail")

        # 搜索按人 20 次/分钟限流，且会话 ID 由双方 user_id 决定——
        # 换 user_id 等于换会话，根本搜不到刚发的消息。
        # 所以保持同一会话，只把搜索频率降到不触发限流的节奏
        # （每 30 次发送搜 1 次，30 秒内每人约 14 次）。
        if round_no % 30 != 0:
            continue
        sresp = post(base + "/apigateway/hismsgs/search", {
            "user_id": "race-u%d" % index,
            "target_id": "race-peer",
            "channel_type": 1,
            "keyword": "书单",
            "limit": 5,
        }, headers)
        scode = sresp.get("code")
        if scode == 0:
            bump("search_hits", (sresp.get("data") or {}).get("count", 0))
        elif scode in (17019, 10012):
            # 搜索按人 20 次/分钟限流；压测超过这个节奏必然触发，属预期。
            bump("rate_limited")
        elif scode == -2:
            bump("timeout")
        else:
            bump("fail")


def main():
    ap = argparse.ArgumentParser(description="用 -race 版引擎做并发冒烟")
    ap.add_argument("--engine", default="http://127.0.0.1:65101", help="引擎 API 地址")
    ap.add_argument("--appkey", required=True)
    ap.add_argument("--app-secret", required=True)
    ap.add_argument("--threads", type=int, default=8)
    ap.add_argument("--seconds", type=float, default=5.0)
    args = ap.parse_args()

    print("并发 %d 线程，持续 %.1f 秒…" % (args.threads, args.seconds))
    threads = [threading.Thread(target=worker, args=(i, args), daemon=True) for i in range(args.threads)]
    for t in threads:
        t.start()
    time.sleep(args.seconds)
    stop.set()
    for t in threads:
        t.join(timeout=10)

    total = (stats["ok"] + stats["fail"] + stats["timeout"] + stats["rate_limited"]
             + stats["hist_ok"] + stats["markread_ok"])
    print("请求 %d 次：发送 %d、拉历史 %d、已读 %d、业务失败 %d、超时 %d、限流 %d，命中 %d 条"
          % (total, stats["ok"], stats["hist_ok"], stats["markread_ok"], stats["fail"],
             stats["timeout"], stats["rate_limited"], stats["search_hits"]))
    if total == 0:
        print("❌ 一次都没成功——引擎没起，或 appkey/secret 不对")
        return 2
    # 注意：这个脚本只负责压。是否发生数据竞争要看引擎日志里的 "DATA RACE"，
    # 脚本本身读不到引擎的 stderr。
    print("下一步：在引擎日志里数 'DATA RACE' 出现的次数（应为 0）")
    # 只对"业务失败"判红：超时是 race 版的性能开销造成的，不代表逻辑错误。
    return 0 if stats["fail"] == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
