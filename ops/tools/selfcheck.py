#!/usr/bin/env python3
"""交付前自检：一条命令跑完所有**不需要真服务**的检查。

为什么要有它：真机验证虽然最强，但每次改完都起一套数据库+服务太重。
这个脚本覆盖"改坏了立刻能发现"的那一层——编译、静态检查、单测、工具自测、
工作区整洁度、文档引用完整性。30 秒内出结果，适合每次提交前跑。

跑法（仓库根目录）：
    python3 ops/tools/selfcheck.py

退出码：
    0  全部通过
    1  有真实失败
    2  环境问题（比如不在仓库根目录、缺 go/node）
"""

import os
import re
import shutil
import subprocess
import sys

BUSINESS_REPO = "."
ENGINE_REPO = "../chatapi.djnovels.com"

# 这些包的测试需要真实 MySQL/Mongo/运行中的服务，环境不具备时必然失败。
# 它们在本轮迭代开始前就已经是失败的——不是回归，但也不会假装通过。
KNOWN_ENV_DEPENDENT = {
    "im-server/services/pushmanager/storages/dbs",
    "im-server/services/pushmanager/storages/mongodbs",
    "im-server/simulator/tests",
}

# 文档里引用的关键路径：缺任何一个都说明交接材料断了。
DOC_REFERENCES = [
    "docs/STATE-2026-11.zh-CN.md",
    "docs/client-sdk.zh-CN.md",
    "docs/message-search.zh-CN.md",
    "docs/webhook-signature.zh-CN.md",
    "docs/feature-audit-2026-11.zh-CN.md",
    "ops/README.md",
    "ops/local-verification.zh-CN.md",
    "ops/tools/verify_deploy.py",
    "ops/tools/test_verify_deploy.py",
    "ops/tools/test_upstream_diff.py",
    "ops/tools/jim-client.mjs",
    "ops/tools/test-jim-client.mjs",
    "ops/sql/tenant-inventory.sql",
]


class Result:
    def __init__(self):
        self.ok = 0
        self.failed = []
        self.notes = []

    def pass_(self, name, detail=""):
        self.ok += 1
        print("✅ %s%s" % (name, ("  [" + detail + "]") if detail else ""))

    def fail(self, name, detail=""):
        self.failed.append((name, detail))
        print("❌ %s%s" % (name, ("  [" + detail + "]") if detail else ""))

    def note(self, text):
        self.notes.append(text)
        print("ℹ️  %s" % text)


def run(cmd, cwd=None, timeout=600):
    try:
        proc = subprocess.run(
            cmd, cwd=cwd, timeout=timeout,
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        )
        return proc.returncode, proc.stdout.decode("utf-8", "replace")
    except FileNotFoundError:
        return 127, "%s: command not found" % cmd[0]
    except subprocess.TimeoutExpired:
        return 124, "%s: timeout after %ss" % (" ".join(cmd), timeout)


def check_repo_layout(res):
    if not os.path.isdir(ENGINE_REPO):
        res.fail("仓库布局", "找不到引擎仓库目录 %s" % ENGINE_REPO)
        return False
    res.pass_("仓库布局", "业务层 + 引擎仓库都在")
    return True


def check_build(res):
    for label, repo in (("业务层", BUSINESS_REPO), ("引擎", ENGINE_REPO)):
        code, out = run(["go", "build", "./..."], cwd=repo, timeout=900)
        if code == 0:
            res.pass_("%s go build" % label)
        else:
            res.fail("%s go build" % label, out.strip().splitlines()[-1] if out.strip() else "exit %d" % code)


def check_vet(res):
    code, out = run(["go", "vet", "./..."], cwd=BUSINESS_REPO, timeout=900)
    if code == 0:
        res.pass_("业务层 go vet")
    else:
        first = out.strip().splitlines()[0] if out.strip() else "exit %d" % code
        res.fail("业务层 go vet", first)


def check_tests(res):
    for label, repo in (("业务层", BUSINESS_REPO), ("引擎", ENGINE_REPO)):
        code, out = run(["go", "test", "./..."], cwd=repo, timeout=1800)
        failed_pkgs = re.findall(r"^FAIL[ \t]+(\S+)", out, re.M)
        real_failures = [p for p in failed_pkgs if p not in KNOWN_ENV_DEPENDENT]
        expected = [p for p in failed_pkgs if p in KNOWN_ENV_DEPENDENT]

        if real_failures:
            res.fail("%s go test" % label, "失败: " + ", ".join(real_failures))
        else:
            detail = ""
            if expected:
                detail = "%d 个已知环境依赖包跳过" % len(expected)
            res.pass_("%s go test" % label, detail)


def check_tool_selftests(res):
    suites = [
        ("验收脚本自测", [sys.executable, "ops/tools/test_verify_deploy.py"]),
        ("跟版工具自测", [sys.executable, "ops/tools/test_upstream_diff.py"]),
    ]
    if shutil.which("node"):
        suites.append(("客户端 SDK 自测", ["node", "--test", "ops/tools/test-jim-client.mjs"]))
    else:
        res.note("未安装 node，跳过客户端 SDK 自测")

    for name, cmd in suites:
        code, out = run(cmd, cwd=BUSINESS_REPO, timeout=300)
        if code == 0:
            res.pass_(name)
        else:
            tail = out.strip().splitlines()[-1] if out.strip() else "exit %d" % code
            res.fail(name, tail)


def check_race(res):
    """缓存是并发密集区，且后台清理协程容易漏锁——固定用 -race 跑一遍。

    只跑 caches 这一个包：整个仓库跑 -race 要几分钟，不适合放进每次自检。
    """
    for label, repo in (("业务层", BUSINESS_REPO), ("引擎", ENGINE_REPO)):
        code, out = run(["go", "test", "-race", "./commons/caches/"], cwd=repo, timeout=600)
        if "DATA RACE" in out:
            res.fail("%s 缓存并发安全" % label, "检测到 DATA RACE")
        elif code != 0:
            tail = out.strip().splitlines()[-1] if out.strip() else "exit %d" % code
            res.fail("%s 缓存并发安全" % label, tail)
        else:
            res.pass_("%s 缓存并发安全" % label, "-race 无竞争")


def check_nginx(res):
    """Nginx 反代目标必须在白名单内：多一条 proxy_pass 就可能把管理台暴露出去。"""
    code, out = run([sys.executable, "ops/tools/check_nginx_ports.py"], cwd=BUSINESS_REPO, timeout=120)
    if code == 0:
        res.pass_("Nginx 反代目标")
    else:
        tail = [line.strip() for line in out.splitlines() if "❌" in line]
        res.fail("Nginx 反代目标", tail[0] if tail else "exit %d" % code)


def check_git_clean(res):
    for label, repo in (("业务层", BUSINESS_REPO), ("引擎", ENGINE_REPO)):
        code, out = run(["git", "status", "--porcelain"], cwd=repo, timeout=60)
        if code != 0:
            res.fail("%s 工作区状态" % label, "git 命令失败")
            continue
        dirty = [line for line in out.splitlines() if line.strip()]
        if dirty:
            # 不算失败：自检最常见的场景就是"改完还没提交先跑一遍"。
            # 但必须显式提示——交付时工作区必须干净。
            res.note("%s 有 %d 个未提交改动（提交前请确认）" % (label, len(dirty)))
        else:
            res.pass_("%s 工作区干净" % label)


def check_doc_references(res):
    missing = [p for p in DOC_REFERENCES if not os.path.exists(p)]
    if missing:
        res.fail("文档引用完整", "缺少: " + ", ".join(missing))
    else:
        res.pass_("文档引用完整", "%d 个关键路径" % len(DOC_REFERENCES))


def check_no_artifacts(res):
    """工作区里不该有测试残留（会被误认为是项目的一部分）。"""
    leftovers = []
    for name in os.listdir(BUSINESS_REPO):
        if name in ("local-verify-server.exe", "engine.exe"):
            leftovers.append(name)
    if os.path.exists(os.path.join(BUSINESS_REPO, "conf", "config.yml")):
        leftovers.append("conf/config.yml")
    if os.path.isdir(os.path.join(BUSINESS_REPO, "logs")):
        leftovers.append("logs/")
    if leftovers:
        res.fail("无测试残留", ", ".join(leftovers))
    else:
        res.pass_("无测试残留")


def main():
    if not os.path.isfile("main.go") or not os.path.isdir("ops"):
        print("❌ 请在业务层仓库根目录运行本脚本")
        return 2
    if not shutil.which("go"):
        print("❌ 找不到 go 命令")
        return 2

    print("== 交付前自检 ==\n")
    res = Result()

    if check_repo_layout(res):
        check_build(res)
        check_vet(res)
        check_tests(res)
        check_tool_selftests(res)
        check_race(res)
        check_nginx(res)
        check_git_clean(res)
        check_doc_references(res)
        check_no_artifacts(res)

    print()
    if res.failed:
        print("❌ %d 项失败，%d 项通过" % (len(res.failed), res.ok))
        for name, detail in res.failed:
            print("   - %s  [%s]" % (name, detail))
        return 1
    print("✅ 全部通过（%d 项）" % res.ok)
    if res.notes:
        print("   注：")
        for note in res.notes:
            print("   - %s" % note)
    return 0


if __name__ == "__main__":
    sys.exit(main())
