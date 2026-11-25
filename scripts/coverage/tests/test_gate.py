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

import contextlib
import io
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from changed_lines import ChangedResult  # noqa: E402
from gate import check_changed, check_minimums, main  # noqa: E402

CONFIG = {
    "overall_min": 80,
    "area_min": 70,
    "area_exceptions": {"server": 12.7},
    "changed_min": 80,
    "changed_min_lines": 10,
}


def area(covered, total):
    return {"covered": covered, "total": total}


class TestCheckMinimums(unittest.TestCase):
    def test_overall_below_min_fails(self):
        failures, _, _ = check_minimums({"a": area(79, 100)}, CONFIG)
        self.assertEqual(len(failures), 1)
        self.assertTrue(any("overall" in f for f in failures))

    def test_area_below_min_fails_including_new_area(self):
        areas = {"a": area(950, 1000), "brand/new": area(43, 100)}
        failures, _, rows = check_minimums(areas, CONFIG)
        self.assertEqual(failures, ["`brand/new` 43.0% is below its minimum 70.0%"])
        self.assertIn(("brand/new", "43.0%", "70.0%", "**below**"), rows)

    def test_exception_floor_used(self):
        areas = {"a": area(990, 1000), "server": area(13, 100)}
        failures, warnings, rows = check_minimums(areas, CONFIG)
        self.assertEqual((failures, warnings), ([], []))
        self.assertIn(("server", "13.0%", "12.7%", "ok"), rows)
        failures, _, _ = check_minimums({"a": area(990, 1000), "server": area(12, 100)}, CONFIG)
        self.assertEqual(failures, ["`server` 12.0% is below its minimum 12.7%"])

    def test_stale_exception_warns(self):
        config = dict(CONFIG, area_exceptions={"server": 12.7, "gone": 30})
        areas = {"a": area(990, 1000), "server": area(75, 100)}
        failures, warnings, _ = check_minimums(areas, config)
        self.assertEqual(failures, [])
        self.assertEqual(
            warnings,
            [
                "exception `gone` names an area that no longer exists; remove it",
                "`server` is at 75.0%, above the area minimum 70.0%; remove its exception",
            ],
        )

    def test_zero_total_area_passes(self):
        failures, _, _ = check_minimums({"a": area(90, 100), "empty": area(0, 0)}, CONFIG)
        self.assertEqual(failures, [])


class TestCheckChanged(unittest.TestCase):
    def test_below_min_fails_with_ranges(self):
        res = ChangedResult(covered=10, total=20, uncovered={"svc/a.go": [12, 13, 14, 15, 40], "svc/b.go": [1]})
        failures, report = check_changed(res, CONFIG, exempt=False)
        self.assertEqual(failures, ["changed lines 50.0% covered (10/20), below 80%"])
        self.assertIn("- `svc/a.go`: 12-15, 40", report)
        self.assertIn("- `svc/b.go`: 1", report)

    def test_small_diff_reports_only(self):
        res = ChangedResult(covered=1, total=3, uncovered={"svc/a.go": [1, 2]})
        failures, report = check_changed(res, CONFIG, exempt=False)
        self.assertEqual(failures, [])
        self.assertTrue(any("fewer than 10" in l for l in report))

    def test_exempt_reports_only(self):
        res = ChangedResult(covered=0, total=50, uncovered={"svc/a.go": list(range(1, 51))})
        failures, report = check_changed(res, CONFIG, exempt=True)
        self.assertEqual(failures, [])
        self.assertTrue(any("coverage-exception label set" in l for l in report))

    def test_skipped_when_no_base(self):
        failures, report = check_changed(None, CONFIG, exempt=False)
        self.assertEqual(failures, [])
        self.assertTrue(any("changed-line check skipped (base not found)" in l for l in report))

    def test_no_coverable_changes_passes(self):
        failures, _ = check_changed(ChangedResult(0, 0, {}), CONFIG, exempt=False)
        self.assertEqual(failures, [])


class TestMain(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        os.environ.pop("GITHUB_STEP_SUMMARY", None)

    def file(self, name, data):
        path = os.path.join(self.tmp, name)
        with open(path, "w") as f:
            f.write(data if isinstance(data, str) else json.dumps(data))
        return path

    def run_main(self, *argv):
        out = io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(out):
            code = main(list(argv))
        return code, out.getvalue()

    def go_args(self, *profiles):
        args = []
        for p in profiles:
            args += ["--go-profile", p]
        excludes = self.file("ex.txt", "^spec/\n")
        return args + ["--module", "github.com/acme/svc", "--module-dir", "svc", "--exclude-file", excludes]

    def test_missing_required_profile_fails(self):
        summary = self.file("s.json", {"areas": {"a": area(90, 100)}})
        cfg = self.file("c.json", CONFIG)
        present = self.file("u.out", "mode: set\n")
        absent = os.path.join(self.tmp, "sqlite.out")
        code, out = self.run_main(
            summary, cfg, "--title", "svc", "--repo-root", self.tmp, "--base", "nope",
            *self.go_args(present, absent), "--require-profiles",
        )
        self.assertEqual(code, 1)
        self.assertIn(f"coverage profile missing: {absent}", out)

    def test_report_only_exits_zero(self):
        summary = self.file("s.json", {"areas": {"a": area(10, 100)}})
        cfg = self.file("c.json", CONFIG)
        code, out = self.run_main(
            summary, cfg, "--title", "svc", "--repo-root", self.tmp, "--base", "nope",
            "--console-root", self.tmp, "--report-only",
        )
        self.assertEqual(code, 0)
        self.assertIn("below its minimum", out)

    def test_unknown_base_still_enforces_minimums(self):
        summary = self.file("s.json", {"areas": {"a": area(10, 100)}})
        cfg = self.file("c.json", CONFIG)
        code, out = self.run_main(
            summary, cfg, "--title", "svc", "--repo-root", self.tmp, "--base", "nope",
            *self.go_args(self.file("u.out", "mode: set\n")),
        )
        self.assertEqual(code, 1)
        self.assertIn("changed-line check skipped (base not found)", out)
        self.assertIn("statements", out)

    def test_writes_step_summary(self):
        summary = self.file("s.json", {"areas": {"a": area(90, 100)}})
        cfg = self.file("c.json", CONFIG)
        step = os.path.join(self.tmp, "step.md")
        os.environ["GITHUB_STEP_SUMMARY"] = step
        try:
            code, _ = self.run_main(
                summary, cfg, "--title", "console", "--repo-root", self.tmp, "--base", "nope",
                "--console-root", self.tmp,
            )
        finally:
            del os.environ["GITHUB_STEP_SUMMARY"]
        self.assertEqual(code, 0)
        with open(step) as f:
            text = f.read()
        self.assertIn("### console", text)
        self.assertIn("lines", text)


if __name__ == "__main__":
    unittest.main()
