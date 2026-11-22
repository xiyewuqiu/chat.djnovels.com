#!/usr/bin/env python3
"""部署后验收：逐条确认 2026-11 这批改动是否真的生效。

只做只读检查，不改配置、不写业务数据。每条断言都对应一个具体提交，
失败时会说明"期望什么、实际什么"，便于直接在服务器上定位。

覆盖：
  管理台 v2 鉴权        /admingateway/v2/* 必须要求 X-Admin-Token
  内部接口凭据          /jim/internal/* 的平台密钥与租户密钥两条路径
  租户级密钥隔离        租户密钥越界作用于别的 appkey 必须被拒
  租户数据清单          /jim/internal/apps/inventory 可用

用法（在服务器上，业务层与引擎都在跑）：

    python3 ops/tools/verify_deploy.py \\
        --appkey dz \\
        --platform-key "$SERVICE_KEY" \\
        --tenant-key "$TENANT_SERVICE_KEY" \\
        --other-appkey other_tenant \\
        --admin-secret "$ADMIN_SECRET"

退出码 0 表示全部通过；1 表示有断言失败；2 表示参数或连接问题。

刻意不做的事：
  - 不触发消息发送（会在生产留数据）
  - 不改租户 webhook 配置（改配置是有副作用的写操作）
  - 不验证保留期（需要 8 天前的历史数据，改由 ops/README 的手工步骤覆盖）
"""

import argparse
import hashlib
import json
import sys
import time
import urllib.error
import urllib.request


class Report:
    def __init__(self):
        self.items = []

    def check(self, name, ok, detail=""):
        self.items.append((name, bool(ok), detail))
        mark = "✅" if ok else "❌"
        print("%s %s%s" % (mark, name, ("  [" + detail + "]") if detail else ""))

    def skip(self, name, why):
        self.items.append((name, None, why))
        print("⏭️  %s  [%s]" % (name, why))

    def failures(self):
        return [i for i in self.items if i[1] is False]

    def skipped(self):
        return [i for i in self.items if i[1] is None]


def get(url, headers=None, timeout=8):
    """发一个 GET，返回 (status, body_bytes)。4xx/5xx 不抛异常，便于断言状态码。"""
    req = urllib.request.Request(url, headers=headers or {}, method="GET")
    return _send(req, timeout)


def post_json(url, payload, headers=None, timeout=8):
    """发一个 JSON POST，返回 (status, body_bytes)。"""
    headers = dict(headers or {})
    headers.setdefault("Content-Type", "application/json")
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method="POST")
    return _send(req, timeout)


def _send(req, timeout):
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as err:
        # 4xx/5xx 的 body 仍要读出来做断言；读完后显式关闭，避免资源告警。
        try:
            return err.code, err.read()
        finally:
            err.close()
    except urllib.error.URLError as err:
        raise ConnectionError("%s -> %s" % (req.full_url, err.reason))


def as_json(raw):
    try:
        return json.loads(raw)
    except ValueError:
        return {}


def check_admin_gate(rep, args):
    if not args.admin_secret and not args.admin:
        rep.skip("管理台 v2 鉴权", "未提供 --admin")
        return
    url = args.admin.rstrip("/") + "/admingateway/v2/metrics/overview"

    # 401 = 配了 adminSecret 且凭据不对；503 = 没配 adminSecret（安全默认值，同样算"拒绝"）
    status, _ = get(url)
    rep.check("管理台 v2 无凭据必须被拒绝", status in (401, 503), "HTTP %d" % status)

    status, _ = get(url, {"X-Admin-Token": "wrong-token-for-verification"})
    rep.check("管理台 v2 错误凭据必须被拒绝", status in (401, 503), "HTTP %d" % status)

    if not args.admin_secret:
        rep.skip("管理台 v2 正确凭据放行", "未提供 --admin-secret")
        return

    status, raw = get(url, {"X-Admin-Token": args.admin_secret})
    body = as_json(raw)
    rep.check(
        "管理台 v2 正确凭据放行",
        status == 200 and body.get("code") == 0,
        "HTTP %d code=%s" % (status, body.get("code")),
    )


def check_internal_credentials(rep, args):
    if not args.platform_key and not args.tenant_key:
        rep.skip("内部接口凭据", "未提供 --platform-key 或 --tenant-key")
        return

    info_url = args.business.rstrip("/") + "/jim/internal/apps/info?app_key=" + args.appkey

    # 不带任何密钥：必须拒绝。这道断言防的是"忘了配密钥等于全开"。
    _, raw = get(info_url, {"appkey": args.appkey})
    body = as_json(raw)
    rep.check(
        "内部接口无凭据必须被拒绝",
        body.get("code") not in (0, None),
        "code=%s" % body.get("code"),
    )

    if args.platform_key:
        _, raw = get(info_url, {"X-Service-Key": args.platform_key, "appkey": args.appkey})
        body = as_json(raw)
        rep.check(
            "平台密钥可作用于任意租户",
            body.get("code") == 0,
            "code=%s" % body.get("code"),
        )
    else:
        rep.skip("平台密钥可作用于任意租户", "未提供 --platform-key")

    if not args.tenant_key:
        rep.skip("租户密钥可作用于本租户", "未提供 --tenant-key")
        rep.skip("租户密钥越界必须被拒绝", "未提供 --tenant-key / --other-appkey")
        return

    _, raw = get(info_url, {"X-Service-Key": args.tenant_key, "appkey": args.appkey})
    body = as_json(raw)
    rep.check(
        "租户密钥可作用于本租户",
        body.get("code") == 0,
        "code=%s —— 若不是 0，多半是该租户还没执行 scope=service_key 补发"
        % body.get("code"),
    )

    if not args.other_appkey:
        rep.skip("租户密钥越界必须被拒绝", "未提供 --other-appkey")
        return

    other_url = (
        args.business.rstrip("/")
        + "/jim/internal/apps/info?app_key="
        + args.other_appkey
    )
    _, raw = get(
        other_url, {"X-Service-Key": args.tenant_key, "appkey": args.other_appkey}
    )
    body = as_json(raw)
    rep.check(
        "租户密钥越界必须被拒绝（多租户隔离的底线）",
        body.get("code") not in (0, None),
        "code=%s —— 若为 0，说明隔离没生效，立即检查 apis/serviceauth.go 是否已部署"
        % body.get("code"),
    )


def check_inventory(rep, args):
    if not args.platform_key:
        rep.skip("租户数据清单可用", "未提供 --platform-key")
        return
    url = (
        args.business.rstrip("/")
        + "/jim/internal/apps/inventory?app_key="
        + args.appkey
    )
    status, raw = get(url, {"X-Service-Key": args.platform_key, "appkey": args.appkey})
    body = as_json(raw)
    data = body.get("data") or {}
    rep.check(
        "租户数据清单可用（干跑）",
        status == 200 and body.get("code") == 0 and "total_rows" in data,
        "code=%s total_rows=%s tables=%d"
        % (body.get("code"), data.get("total_rows"), len(data.get("tables") or [])),
    )


def engine_signed_headers(appkey, app_secret, nonce=None, timestamp=None):
    """按引擎 API 网关的签名规则造请求头。

    规则见 chatapi/services/apigateway/apis/signature.go：
        signature = hex(sha1(app_secret + nonce + timestamp))
    """
    nonce = nonce or ("verify%d" % int(time.time() * 1000))
    timestamp = timestamp or str(int(time.time() * 1000))
    raw = (app_secret + nonce + timestamp).encode("utf-8")
    return {
        "appkey": appkey,
        "nonce": nonce,
        "timestamp": timestamp,
        "signature": hashlib.sha1(raw).hexdigest(),
    }


def check_search(rep, args):
    """搜索接口只读检查：签名、参数校验、正常返回形状。

    刻意不验证"能不能搜到某条具体消息"——那需要库里恰好有匹配内容，
    属于人工测试。这里只确认接口是活的、闸门是紧的、响应形状没变。
    """
    if not args.app_secret:
        rep.skip("消息搜索接口", "未提供 --app-secret")
        return
    if not args.im_api:
        rep.skip("消息搜索接口", "未提供 --im-api")
        return

    url = args.im_api.rstrip("/") + "/apigateway/hismsgs/search"
    base_body = {
        "user_id": args.search_user or "verify-user",
        "target_id": args.search_target or "verify-target",
        "channel_type": int(args.search_channel),
        "keyword": "验证",
    }

    # 1) 完全不签名：必须在网关层被拒，不能进业务逻辑。
    body = dict(base_body, keyword="验证")
    status, raw = post_json(url, body, {"appkey": args.appkey})
    payload = as_json(raw)
    rep.check(
        "搜索接口无签名必须被拒绝",
        payload.get("code") not in (0, None),
        "code=%s" % payload.get("code"),
    )

    headers = engine_signed_headers(args.appkey, args.app_secret)

    # 2) 关键词只有 1 个字：必须被参数校验挡住（12004），而不是去扫 500 条。
    short_body = dict(base_body, keyword="验")
    _, raw = post_json(url, short_body, headers)
    payload = as_json(raw)
    rep.check(
        "搜索关键词过短必须被拒绝",
        payload.get("code") == 12004,
        "code=%s（期望 12004 参数非法）" % payload.get("code"),
    )

    # 3) 合法请求：接口活着，且响应形状是 hits/count/scanned。
    headers = engine_signed_headers(args.appkey, args.app_secret)
    _, raw = post_json(url, body, headers)
    payload = as_json(raw)
    data = payload.get("data") or {}
    shape_ok = all(key in data for key in ("hits", "count", "scanned"))
    rep.check(
        "搜索接口返回预期形状",
        payload.get("code") == 0 and shape_ok,
        "code=%s keys=%s" % (payload.get("code"), sorted(data.keys())),
    )
    if payload.get("code") == 0:
        print(
            "   扫描 %s 条候选，命中 %s 条（候选上限 %s）"
            % (data.get("scanned"), data.get("count"), data.get("candidate_limit"))
        )


def main():
    ap = argparse.ArgumentParser(
        description="部署后只读验收：管理台鉴权 / 内部接口凭据 / 租户隔离 / 数据清单 / 消息搜索"
    )
    ap.add_argument("--business", default="http://127.0.0.1:65110",
                    help="业务层回环地址（默认 http://127.0.0.1:65110）")
    ap.add_argument("--admin", default="http://127.0.0.1:65109",
                    help="管理台回环地址（默认 http://127.0.0.1:65109）")
    ap.add_argument("--im-api", default="http://127.0.0.1:65101",
                    help="引擎 API 网关回环地址（默认 http://127.0.0.1:65101）")
    ap.add_argument("--appkey", required=True, help="用于验收的租户 app_key")
    ap.add_argument("--app-secret", default="", help="该租户的 app_secret，用于给引擎 API 签名")
    ap.add_argument("--platform-key", default="", help="平台密钥（conf/config.yml 的 serviceKey）")
    ap.add_argument("--tenant-key", default="", help="该租户的 service_key")
    ap.add_argument("--other-appkey", default="", help="另一个租户，用于验证越界拒绝")
    ap.add_argument("--admin-secret", default="", help="管理台 adminSecret")
    ap.add_argument("--search-user", default="", help="搜索用的 user_id（缺省用占位值）")
    ap.add_argument("--search-target", default="", help="搜索用的 target_id（缺省用占位值）")
    ap.add_argument("--search-channel", default="2",
                    help="搜索的 channel_type：1 单聊 / 2 群聊（默认 2）")
    args = ap.parse_args()

    rep = Report()
    try:
        print("== 管理台 v2 鉴权 ==")
        check_admin_gate(rep, args)
        print("\n== 内部接口凭据与租户隔离 ==")
        check_internal_credentials(rep, args)
        print("\n== 租户数据清单 ==")
        check_inventory(rep, args)
        print("\n== 消息搜索接口 ==")
        check_search(rep, args)
    except ConnectionError as err:
        print("\n❌ 连接失败：%s" % err)
        print("   确认业务层/引擎进程在跑，且地址与端口正确。")
        return 2

    print()
    failures = rep.failures()
    skipped = rep.skipped()
    if failures:
        print("❌ %d 项失败，%d 项跳过" % (len(failures), len(skipped)))
        for name, _, detail in failures:
            print("   - %s  [%s]" % (name, detail))
        return 1
    if skipped:
        print("✅ 已执行的断言全部通过；%d 项因缺参数跳过（补上参数可覆盖）" % len(skipped))
        return 0
    print("✅ 全部通过")
    return 0


if __name__ == "__main__":
    sys.exit(main())
