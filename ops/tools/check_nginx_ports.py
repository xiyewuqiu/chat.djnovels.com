#!/usr/bin/env python3
"""静态检查 Nginx 配置的反代目标是否越界。

为什么需要：公网该暴露的只有两个端口——65110（业务层）与 65103（引擎长连接）。
65101（引擎 REST API）、65102（Navigator）、65109（管理台）、3306（MySQL）
一律不出宿主机回环。有人为了让某个接口"能访问"，顺手加一条 proxy_pass
是很容易发生的事，而这类改动在 code review 里极不显眼。

这个脚本不连网、不需要服务在跑，纯文本解析，可以放进每次自检。

跑法：
    python3 ops/tools/check_nginx_ports.py [配置文件...]

退出码：0 通过、1 有问题、2 找不到配置文件。
"""

import os
import re
import sys

# 允许被反代的端口：业务层 API 与引擎长连接。
ALLOWED_PORTS = {65110, 65103}

# 明确不该出现在 proxy_pass 里的端口。留空表示"任何未在白名单里的都报"，
# 但把它们单独列出来，是为了在报错信息里直接说明它是什么。
FORBIDDEN_PORTS = {
    65101: "引擎 REST API（服务端到服务端，靠签名鉴权，不该对外）",
    65102: "引擎 Navigator（内部寻址）",
    65109: "管理台（只该监听回环，靠 X-Admin-Token）",
    3306: "MySQL",
}

# 这些路径必须被显式拒绝，否则内部接口会暴露到公网。
REQUIRED_DENY_PREFIXES = ["/jim/internal/"]

PROXY_PASS_RE = re.compile(r"proxy_pass\s+https?://([^/:\s]+):(\d+)")
LOCATION_RE = re.compile(r"location\s+([^\s{]+)\s*\{")


def parse_file(path):
    with open(path, "r", encoding="utf-8") as handle:
        return handle.read()


def check_config(path):
    """返回 (问题列表, 说明列表)。"""
    problems = []
    notes = []
    try:
        text = parse_file(path)
    except OSError as err:
        return ["读取失败: %s" % err], []

    for match in PROXY_PASS_RE.finditer(text):
        host, port_str = match.group(1), match.group(2)
        port = int(port_str)
        if port in FORBIDDEN_PORTS:
            problems.append(
                "第 %d 行 proxy_pass 指向 %s:%d —— 这是%s，不该被反代"
                % (text[: match.start()].count("\n") + 1, host, port, FORBIDDEN_PORTS[port])
            )
        elif port not in ALLOWED_PORTS:
            problems.append(
                "第 %d 行 proxy_pass 指向 %s:%d —— 不在白名单 %s 内，请确认是否该暴露"
                % (text[: match.start()].count("\n") + 1, host, port, sorted(ALLOWED_PORTS))
            )

    for prefix in REQUIRED_DENY_PREFIXES:
        if prefix not in text:
            problems.append("缺少对 %s 的显式拒绝（应 return 404），内部接口会暴露到公网" % prefix)

    locations = LOCATION_RE.findall(text)
    notes.append("location 共 %d 条: %s" % (len(locations), ", ".join(sorted(set(locations)))))
    notes.append("proxy_pass 目标端口: %s"
                 % ", ".join(sorted({m.group(2) for m in PROXY_PASS_RE.finditer(text)})))
    return problems, notes


def main(argv):
    configs = argv[1:]
    if not configs:
        default = os.path.join("ops", "nginx-reference.conf")
        if not os.path.exists(default):
            print("❌ 找不到 %s，也没在命令行指定配置文件" % default)
            return 2
        configs = [default]

    total_problems = 0
    for path in configs:
        print("== %s ==" % path)
        problems, notes = check_config(path)
        for note in notes:
            print("  ℹ️  %s" % note)
        for problem in problems:
            print("  ❌ %s" % problem)
        total_problems += len(problems)
        if not problems:
            print("  ✅ 反代目标都在白名单内")

    print()
    if total_problems:
        print("❌ %d 个问题" % total_problems)
        return 1
    print("✅ 通过")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
