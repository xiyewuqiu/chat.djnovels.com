#!/usr/bin/env python3
"""upstream_diff.py 的自测：造临时目录验证三类差异的判定。

跟版工具判错的代价很直接：漏报"上游独有"会让你以为没落后，
漏报"内容不同"会让你在合并时覆盖掉自研功能。所以这些判定必须有测试。

跑法（无需仓库、无需网络）：

    python3 ops/tools/test_upstream_diff.py
"""

import os
import shutil
import tempfile
import unittest

import upstream_diff


class UpstreamDiffTest(unittest.TestCase):
    def make_tree(self, files):
        root = tempfile.mkdtemp(prefix="updiff-")
        self.addCleanup(shutil.rmtree, root, ignore_errors=True)
        for rel, content in files.items():
            full = os.path.join(root, *rel.split("/"))
            parent = os.path.dirname(full)
            if parent:
                os.makedirs(parent, exist_ok=True)
            with open(full, "w", encoding="utf-8") as fh:
                fh.write(content)
        return root

    def test_classifies_three_kinds(self):
        local = self.make_tree({"same.go": "same", "tweaked.go": "local", "mine.go": "x"})
        upstream = self.make_tree({"same.go": "same", "tweaked.go": "upstream", "theirs.go": "y"})

        groups = upstream_diff.classify(
            upstream_diff.file_hashes(local), upstream_diff.file_hashes(upstream)
        )
        self.assertEqual(["mine.go"], groups["local_only"])
        self.assertEqual(["theirs.go"], groups["upstream_only"])
        self.assertEqual(["tweaked.go"], groups["changed"])

    def test_identical_trees_have_no_diff(self):
        files = {"a/b.go": "1", "c.go": "2"}
        groups = upstream_diff.classify(
            upstream_diff.file_hashes(self.make_tree(files)),
            upstream_diff.file_hashes(self.make_tree(files)),
        )
        self.assertEqual({"local_only": [], "upstream_only": [], "changed": []}, groups)

    # .git 与 build 目录里的东西不该进对比：它们每次都变，会淹没真实差异。
    def test_skips_vcs_and_build_dirs(self):
        root = self.make_tree({".git/config": "x", "build/out.bin": "x", "keep.go": "y"})
        self.assertEqual(["keep.go"], sorted(upstream_diff.file_hashes(root)))

    def test_groups_by_top_dir(self):
        got = upstream_diff.group_by_top_dir(["a/1.go", "a/2.go", "b/1.go", "root.go"])
        self.assertEqual({"a": 2, "b": 1, "(root)": 1}, got)

    def test_top_dir_order_is_by_count_desc(self):
        got = upstream_diff.group_by_top_dir(["a/1.go", "b/1.go", "b/2.go", "b/3.go"])
        self.assertEqual(["b", "a"], list(got.keys()))


if __name__ == "__main__":
    unittest.main(verbosity=2)
