# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

import io
import os
import re
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from changed_lines import (  # noqa: E402
    ChangedResult,
    console_line_coverage,
    evaluate_console,
    evaluate_go,
    go_line_coverage,
    parse_diff,
)

MOD = "github.com/acme/svc"


def write(dirpath, name, text):
    path = os.path.join(dirpath, name)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        f.write(text)
    return path


class TestParseDiff(unittest.TestCase):
    def test_added_and_changed_lines(self):
        diff = (
            "diff --git a/svc/a.go b/svc/a.go\n"
            "--- a/svc/a.go\n"
            "+++ b/svc/a.go\n"
            "@@ -3 +3 @@ func a() {\n"
            "-\told()\n"
            "+\tnew()\n"
            "@@ -10,0 +11,3 @@\n"
            "+x\n+y\n+z\n"
        )
        self.assertEqual(parse_diff(diff), {"svc/a.go": {3, 11, 12, 13}})

    def test_deleted_only_hunk_adds_nothing(self):
        diff = "--- a/svc/a.go\n+++ b/svc/a.go\n@@ -4,2 +3,0 @@\n-a\n-b\n"
        self.assertEqual(parse_diff(diff), {})

    def test_new_file(self):
        diff = "--- /dev/null\n+++ b/svc/new.go\n@@ -0,0 +1,2 @@\n+a\n+b\n"
        self.assertEqual(parse_diff(diff), {"svc/new.go": {1, 2}})

    def test_deleted_file_skipped(self):
        diff = "--- a/svc/old.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-a\n-b\n"
        self.assertEqual(parse_diff(diff), {})

    def test_rename_with_edits(self):
        diff = (
            "diff --git a/svc/old.go b/svc/new.go\n"
            "similarity index 90%\n"
            "rename from svc/old.go\n"
            "rename to svc/new.go\n"
            "--- a/svc/old.go\n"
            "+++ b/svc/new.go\n"
            "@@ -5 +5 @@\n-a\n+b\n"
        )
        self.assertEqual(parse_diff(diff), {"svc/new.go": {5}})

    def test_quoted_path_decoded(self):
        diff = '--- a/x\n+++ "b/console/src/caf\\303\\251\\"q.ts"\n@@ -1 +1 @@\n-a\n+b\n'
        self.assertEqual(parse_diff(diff), {'console/src/café"q.ts': {1}})

    def test_pure_rename_has_no_lines(self):
        diff = (
            "diff --git a/svc/old.go b/svc/new.go\n"
            "similarity index 100%\n"
            "rename from svc/old.go\n"
            "rename to svc/new.go\n"
        )
        self.assertEqual(parse_diff(diff), {})


class TestGoLineCoverage(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()

    def profile(self, name, *lines):
        return write(self.tmp, name, "mode: set\n" + "".join(l + "\n" for l in lines))

    def test_multiline_block_marks_each_line(self):
        p = self.profile("p.out", f"{MOD}/pkg/a.go:3.10,6.2 2 1", f"{MOD}/pkg/a.go:8.2,9.3 1 0")
        cov = go_line_coverage([p], MOD, "svc", [])
        self.assertEqual(
            cov,
            {"svc/pkg/a.go": {3: True, 4: True, 5: True, 6: True, 8: False, 9: False}},
        )

    def test_hit_in_any_profile_wins(self):
        p1 = self.profile("u.out", f"{MOD}/a.go:3.1,4.2 1 0")
        p2 = self.profile("i.out", f"{MOD}/a.go:3.1,4.2 1 3")
        missing = os.path.join(self.tmp, "absent.out")
        cov = go_line_coverage([p1, missing, p2], MOD, "svc", [])
        self.assertEqual(cov, {"svc/a.go": {3: True, 4: True}})

    def test_overlapping_hit_and_miss_is_covered(self):
        p = self.profile("p.out", f"{MOD}/a.go:3.1,5.2 1 0", f"{MOD}/a.go:5.3,7.2 1 1")
        cov = go_line_coverage([p], MOD, "svc", [])
        self.assertEqual(cov["svc/a.go"], {3: False, 4: False, 5: True, 6: True, 7: True})

    def test_excluded_file_dropped(self):
        p = self.profile("p.out", f"{MOD}/spec/gen.go:1.1,2.2 1 0", f"{MOD}/a.go:1.1,1.9 1 1")
        cov = go_line_coverage([p], MOD, "svc", [re.compile(r"^spec/")])
        self.assertEqual(cov, {"svc/a.go": {1: True}})

    def test_other_module_ignored(self):
        p = self.profile("p.out", "github.com/other/x/a.go:1.1,1.9 1 1")
        self.assertEqual(go_line_coverage([p], MOD, "svc", []), {})


class TestEvaluateGo(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp()
        for name in ("svc/a.go", "svc/b.go", "svc/a_test.go", "svc/spec/gen.go"):
            write(self.root, name, "package x\nfunc f() {\n" + "\tg()\n" * 10 + "}\n")
        write(self.root, "svc/types.go", "package x\n\ntype T struct{}\n\nvar Err = 1\n")

    def evaluate(self, changed, coverage, excludes=()):
        return evaluate_go(changed, coverage, "svc", list(excludes), repo_root=self.root)

    def test_test_files_and_non_go_ignored(self):
        changed = {"svc/a_test.go": {1}, "svc/README.md": {1}, "cli/x.go": {1}, "svc/a.go": {2}}
        res = self.evaluate(changed, {"svc/a.go": {2: True}})
        self.assertEqual(res, ChangedResult(covered=1, total=1, uncovered={}))

    def test_lines_outside_blocks_not_coverable(self):
        res = self.evaluate({"svc/a.go": {1, 2, 3}}, {"svc/a.go": {2: False}})
        self.assertEqual(res, ChangedResult(covered=0, total=1, uncovered={"svc/a.go": [2]}))

    def test_file_missing_from_profile_not_coverable(self):
        # The coverage runs compile every package (go test ./..., -coverpkg=./...),
        # so a file missing from the profile was not built (build tag, other
        # platform, wireinject) or has no statements (only types, vars, consts).
        write(self.root, "svc/linux.go", "//go:build linux\n\npackage x\n\nfunc L() {}\n")
        write(self.root, "svc/lit.go", "package x\n\nvar hs = []func() int{\n\tfunc() int { return 1 },\n}\n")
        write(self.root, "svc/new/n.go", "package n\n\nfunc F() {\n\tg()\n}\n")
        changed = {"svc/linux.go": {5}, "svc/lit.go": {3, 4}, "svc/new/n.go": {3, 4}, "svc/gone.go": {1}}
        res = self.evaluate(changed, {"svc/a.go": {2: True}})
        self.assertEqual(res, ChangedResult(covered=0, total=0, uncovered={}))

    def test_comment_lines_inside_a_block_not_coverable(self):
        write(self.root, "svc/c.go", "package x\n\nfunc f() {\n\t// why\n\n\t/* more\n\t   detail */ g()\n}\n")
        cov = {"svc/c.go": {n: False for n in range(3, 9)}}
        res = self.evaluate({"svc/c.go": {4, 5, 6, 7}}, cov)
        self.assertEqual(res, ChangedResult(covered=0, total=1, uncovered={"svc/c.go": [7]}))

    def test_comment_markers_inside_go_strings_are_code(self):
        write(self.root, "svc/s.go", 'package x\n\nfunc f() {\n\tr := "/api/*"\n\tu := `http://x/*`\n\tg(r, u)\n}\n')
        cov = {"svc/s.go": {n: False for n in range(3, 8)}}
        res = self.evaluate({"svc/s.go": {4, 5, 6}}, cov)
        self.assertEqual(res, ChangedResult(covered=0, total=3, uncovered={"svc/s.go": [4, 5, 6]}))

    def test_matching_build_tag_file_in_profile_is_counted(self):
        write(self.root, "svc/l.go", "//go:build linux\n\npackage x\n\nfunc L() {\n\tg()\n}\n")
        res = self.evaluate({"svc/l.go": {6}}, {"svc/l.go": {5: False, 6: False, 7: False}})
        self.assertEqual(res, ChangedResult(covered=0, total=1, uncovered={"svc/l.go": [6]}))

    def test_read_error_names_the_file(self):
        class Broken(io.StringIO):
            def read(self, *args):
                raise OSError(5, "Input/output error")

        with mock.patch("changed_lines.open", return_value=Broken(), create=True):
            with self.assertRaises(OSError) as ctx:
                self.evaluate({"svc/a.go": {2}}, {"svc/a.go": {2: True}})
        self.assertIn("a.go", str(ctx.exception))

    def test_uncovered_lines_listed_sorted(self):
        cov = {"svc/a.go": {9: False, 3: False, 5: True}}
        res = self.evaluate({"svc/a.go": {9, 3, 5}}, cov)
        self.assertEqual(res, ChangedResult(covered=1, total=3, uncovered={"svc/a.go": [3, 9]}))


class ConsoleTree(unittest.TestCase):
    """A repo with console/ packages: views (reported), fresh (no report), types."""

    def setUp(self):
        self.root = tempfile.mkdtemp()
        self.console = os.path.join(self.root, "console")
        for pkg in ("workspaces/libs/views", "workspaces/libs/fresh", "workspaces/libs/types"):
            write(self.console, f"{pkg}/package.json", "{}")
        views = os.path.join(self.console, "workspaces/libs/views")
        write(
            views,
            "coverage/lcov.info",
            "TN:\nSF:src/a.ts\nDA:1,1\nDA:2,0\nend_of_record\n"
            f"SF:{views}/src/b.tsx\nDA:5,3\nend_of_record\n",
        )
        # A report inside node_modules is a symlinked copy and is never read.
        write(
            self.console,
            "node_modules/views/coverage/lcov.info",
            "SF:src/z.ts\nDA:1,0\nend_of_record\n",
        )


class TestConsoleLineCoverage(ConsoleTree):
    def test_relative_and_absolute_sf_resolve_alike(self):
        cov, _ = console_line_coverage(self.console, self.root)
        self.assertEqual(cov["console/workspaces/libs/views/src/a.ts"], {1: True, 2: False})
        self.assertEqual(cov["console/workspaces/libs/views/src/b.tsx"], {5: True})

    def test_node_modules_skipped(self):
        cov, _ = console_line_coverage(self.console, self.root)
        self.assertFalse(any("node_modules" in p for p in cov))
        self.assertNotIn("console/node_modules/views/src/z.ts", cov)

    def test_reported_packages_listed(self):
        _, reported = console_line_coverage(self.console, self.root)
        self.assertEqual(reported, {"console/workspaces/libs/views"})


class TestEvaluateConsole(ConsoleTree):
    def evaluate(self, changed, no_coverage=()):
        cov, reported = console_line_coverage(self.console, self.root)
        return evaluate_console(changed, cov, reported, list(no_coverage), self.root)

    def test_file_absent_from_report_not_coverable(self):
        changed = {
            "console/workspaces/libs/views/src/a.ts": {1, 2, 9},
            "console/workspaces/libs/views/src/a.stories.tsx": {1},
        }
        res = self.evaluate(changed)
        self.assertEqual(
            res,
            ChangedResult(covered=1, total=2, uncovered={"console/workspaces/libs/views/src/a.ts": [2]}),
        )

    def test_package_without_report_is_reported(self):
        changed = {
            "console/workspaces/libs/fresh/src/x.tsx": {1, 3},
            "console/workspaces/libs/fresh/src/y.ts": {2},
        }
        self.assertEqual(
            self.evaluate(changed),
            ChangedResult(covered=0, total=0, uncovered={}, unreported=["console/workspaces/libs/fresh"]),
        )

    def test_package_without_report_skips_tests_and_stories(self):
        changed = {
            "console/workspaces/libs/fresh/src/x.test.tsx": {1},
            "console/workspaces/libs/fresh/src/x.stories.tsx": {1},
            "console/workspaces/libs/fresh/src/env.d.ts": {1},
            "console/workspaces/libs/fresh/src/__tests__/h.ts": {1},
        }
        self.assertEqual(self.evaluate(changed), ChangedResult(covered=0, total=0, uncovered={}))

    def test_no_coverage_package_ignored(self):
        res = self.evaluate({"console/workspaces/libs/types/src/t.ts": {1}}, ["workspaces/libs/types"])
        self.assertEqual(res, ChangedResult(covered=0, total=0, uncovered={}))

    def test_files_outside_src_ignored(self):
        changed = {
            "console/workspaces/libs/fresh/vite.config.ts": {1},
            "console/workspaces/libs/fresh/src/styles.css": {1},
            "console/scripts/build.ts": {1},
            "agent-manager-service/a.go": {1},
        }
        self.assertEqual(self.evaluate(changed), ChangedResult(covered=0, total=0, uncovered={}))


if __name__ == "__main__":
    unittest.main()
