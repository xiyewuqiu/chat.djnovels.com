#!/usr/bin/env python3
"""对比本地引擎与上游 im-server，输出跟版影响面。

背景：本仓库的历史是重写过的，与上游没有共同 commit，所以 `git diff` / `git log`
都用不上——只能做文件级对比。这个工具就是干这件事：把"我们和上游差多少"
从"据说有 31 处改动"变成一份可以核对的清单。

三类差异对应三种跟版动作：

    本地独有   自研功能，上游没有 → 跟版后必须保留，长期要自己维护
    上游独有   我们落后了 → 跟版时要补进来，否则会出现"上游修了、我们还在踩"
    内容不同   冲突面 → 跟版时必须逐个人工合并，这是主要工作量

用法：

    # 1) 先拉上游某个版本（tag 或 commit）
    git clone --depth 1 --branch v1.8.16 https://github.com/juggleim/im-server /tmp/im-server-v1.8.16

    # 2) 对比
    python3 ops/tools/upstream_diff.py \\
        --local ../chatapi.djnovels.com \\
        --upstream /tmp/im-server-v1.8.16

    # 3) 给自动化用
    python3 ops/tools/upstream_diff.py ... --json /tmp/upstream-diff.json

退出码：
    0  没有"上游独有"文件（本地没落后；内容不同仍需人工合并）
    1  存在"上游独有"文件（本地落后于该版本）
    2  路径或参数问题
"""

import argparse
import hashlib
import json
import os
import sys

# 这些目录里的东西不该进对比：构建产物、依赖、版本库元数据
SKIP_DIRS = {".git", "node_modules", "__pycache__", "build", ".idea", ".vscode"}


def file_hashes(root):
    """{相对路径: md5}；路径统一用 / 分隔，保证跨平台可比。"""
    hashes = {}
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for name in filenames:
            full = os.path.join(dirpath, name)
            rel = os.path.relpath(full, root).replace(os.sep, "/")
            try:
                with open(full, "rb") as fh:
                    hashes[rel] = hashlib.md5(fh.read()).hexdigest()
            except OSError:
                hashes[rel] = "unreadable"
    return hashes


def classify(local, upstream):
    local_only = sorted(set(local) - set(upstream))
    upstream_only = sorted(set(upstream) - set(local))
    changed = sorted(p for p in set(local) & set(upstream) if local[p] != upstream[p])
    return {
        "local_only": local_only,
        "upstream_only": upstream_only,
        "changed": changed,
    }


def group_by_top_dir(paths):
    """按顶层目录聚合，用来判断哪块改动最重。"""
    buckets = {}
    for path in paths:
        top = path.split("/", 1)[0] if "/" in path else "(root)"
        buckets[top] = buckets.get(top, 0) + 1
    return dict(sorted(buckets.items(), key=lambda kv: (-kv[1], kv[0])))


def print_section(title, paths, hint, limit):
    print("\n== %s：%d ==" % (title, len(paths)))
    if hint:
        print("   " + hint)
    for path in paths[:limit]:
        print("   " + path)
    if len(paths) > limit:
        print("   … 另有 %d 个（用 --limit 展开，或用 --json 拿全量）" % (len(paths) - limit))


def main():
    ap = argparse.ArgumentParser(description="对比本地引擎与上游 im-server 的文件级差异")
    ap.add_argument("--local", required=True, help="本地引擎仓库根目录")
    ap.add_argument("--upstream", required=True, help="上游 im-server 检出目录")
    ap.add_argument("--json", default="", help="把完整清单写入该路径")
    ap.add_argument("--limit", type=int, default=30, help="每类最多打印多少条（默认 30）")
    args = ap.parse_args()

    for path, label in ((args.local, "--local"), (args.upstream, "--upstream")):
        if not os.path.isdir(path):
            print("❌ %s 不是目录：%s" % (label, path))
            return 2

    local = file_hashes(args.local)
    upstream = file_hashes(args.upstream)
    groups = classify(local, upstream)

    print("本地文件数：%d" % len(local))
    print("上游文件数：%d" % len(upstream))
    print("本地独有：%d   上游独有：%d   内容不同：%d"
          % (len(groups["local_only"]), len(groups["upstream_only"]), len(groups["changed"])))

    print_section(
        "本地独有（自研，跟版后必须保留）",
        groups["local_only"],
        "这些是上游没有的文件，跟版时不要被覆盖。",
        args.limit,
    )
    print_section(
        "上游独有（我们落后了）",
        groups["upstream_only"],
        "跟版时要补进来；缺失可能意味着上游修过的问题我们还在踩。",
        args.limit,
    )
    print_section(
        "内容不同（冲突面，需人工合并）",
        groups["changed"],
        "这是跟版的主要工作量，逐个文件判断保留本地逻辑还是接受上游修改。",
        args.limit,
    )

    for title, key in (("本地独有", "local_only"), ("上游独有", "upstream_only"), ("内容不同", "changed")):
        buckets = group_by_top_dir(groups[key])
        if buckets:
            print("\n%s 按顶层目录：%s"
                  % (title, ", ".join("%s=%d" % kv for kv in list(buckets.items())[:8])))

    if args.json:
        payload = {
            "local_file_count": len(local),
            "upstream_file_count": len(upstream),
            "counts": {k: len(v) for k, v in groups.items()},
            **groups,
        }
        with open(args.json, "w", encoding="utf-8") as fh:
            json.dump(payload, fh, ensure_ascii=False, indent=2)
        print("\n完整清单已写入：%s" % args.json)

    print()
    if groups["upstream_only"]:
        print("⚠️  本地落后于该上游版本：有 %d 个上游文件本地不存在。" % len(groups["upstream_only"]))
        return 1
    print("✅ 没有上游独有文件；仍需人工合并 %d 个内容不同的文件。" % len(groups["changed"]))
    return 0


if __name__ == "__main__":
    sys.exit(main())
