#!/usr/bin/env python3
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

"""Map a pull request's changed lines onto coverage reports, for gate.py.

The diff is `git diff -U0 <merge-base>...HEAD`, so only lines the change
adds or rewrites count; deleted lines and pure renames add nothing.

A changed line is coverable when the coverage report counts it: inside
a Go cover profile block, or a `DA:` line of a console lcov report.
Comments, blank lines, imports, test files and excluded or generated
files are not coverable.
"""

import dataclasses
import os
import re
import subprocess

# Files vitest.coverage.ts excludes, for packages that have no report.
CONSOLE_EXCLUDED = re.compile(
    r"\.(stories|test|spec)\.|\.d\.ts$|/(test-utils|__test__|__tests__|generated)/|\.generated\.|/generated-"
)
HUNK = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@")


@dataclasses.dataclass
class ChangedResult:
    covered: int
    total: int
    uncovered: dict  # repo path -> sorted uncovered line numbers


def parse_diff(diff_text):
    """Return repo path -> added or changed line numbers in the new file."""
    changed = {}
    path = None
    for line in diff_text.splitlines():
        if line.startswith("+++ "):
            target = _unquote(line[4:].strip())
            path = target[2:] if target.startswith("b/") else None
            continue
        m = HUNK.match(line)
        if m and path:
            start = int(m.group(1))
            count = 1 if m.group(2) is None else int(m.group(2))
            if count:
                changed.setdefault(path, set()).update(range(start, start + count))
    return changed


def _unquote(path):
    """Decode a path git quoted ("b/caf\\303\\251.ts") for unusual characters."""
    if not (path.startswith('"') and path.endswith('"')):
        return path
    raw = path[1:-1].encode("latin-1", "backslashreplace").decode("unicode_escape")
    return raw.encode("latin-1").decode("utf-8", "replace")


def git_changed_lines(base, repo_root):
    """Lines changed since the merge base with `base`; None if `base` can't be resolved."""

    def git(*args):
        return subprocess.run(
            ["git", "-C", repo_root, *args], capture_output=True, text=True
        )

    mb = git("merge-base", base, "HEAD")
    if mb.returncode != 0 or not mb.stdout.strip():
        return None
    diff = git("-c", "core.quotePath=false", "diff", "-U0", "--no-color", f"{mb.stdout.strip()}...HEAD")
    if diff.returncode != 0:
        return None
    return parse_diff(diff.stdout)


def go_line_coverage(profiles, module, module_dir, excludes):
    """Repo path -> line -> covered, for every line inside a profile block.

    A line is covered when any block containing it, in any profile, was
    hit. Missing profiles are skipped; the gate checks for them itself.
    """
    prefix = module.rstrip("/") + "/"
    coverage = {}
    for path in profiles:
        if not os.path.exists(path):
            continue
        with open(path) as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("mode:"):
                    continue
                loc, _, count = line.rsplit(" ", 2)
                full, span = loc.rsplit(":", 1)
                if not full.startswith(prefix):
                    continue
                name = full[len(prefix):]
                if any(rx.search(name) for rx in excludes):
                    continue
                start, end = span.split(",")
                first, last = int(start.split(".")[0]), int(end.split(".")[0])
                lines = coverage.setdefault(f"{module_dir}/{name}", {})
                hit = int(count) > 0
                for n in range(first, last + 1):
                    lines[n] = lines.get(n, False) or hit
    return coverage


def _tally(per_file):
    """per_file: path -> {line: covered}; builds a ChangedResult."""
    covered = total = 0
    uncovered = {}
    for path in sorted(per_file):
        for n, hit in per_file[path].items():
            total += 1
            if hit:
                covered += 1
            else:
                uncovered.setdefault(path, []).append(n)
    return ChangedResult(covered, total, {p: sorted(ls) for p, ls in uncovered.items()})


def _read_source(path):
    """The file's text, or None when it doesn't exist; other read errors raise."""
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            return f.read()
    except FileNotFoundError:
        return None
    except OSError as e:
        raise OSError(f"cannot read {path}: {e}") from e


def _ts_code_lines(text, lines):
    """The given line numbers of a TypeScript file that hold code.

    Blank lines, comments and import statements are not code. A rough
    scan, not a parser: enough to keep comment and import edits in a
    package with no coverage run from counting as untested.
    """
    code, in_block, in_import = set(), False, False
    for n, raw in enumerate(text.splitlines(), 1):
        line = raw.strip()
        if in_block:
            if "*/" in line:
                in_block = False
            continue
        bare = line.split("//", 1)[0].rstrip()
        opened = bare.rfind("/*")
        if opened >= 0 and "*/" not in bare[opened:]:
            in_block = True
            bare = bare[:opened].rstrip()
        if in_import:
            in_import = not bare.startswith("}")
            continue
        if bare.startswith("import "):
            in_import = bare.endswith("{")
            continue
        if bare and not bare.startswith("/*") and n in lines:
            code.add(n)
    return code


def evaluate_go(changed, coverage, module_dir, excludes):
    """Covered and total coverable changed lines of one Go module."""
    prefix = module_dir.rstrip("/") + "/"
    per_file = {}
    for path, lines in changed.items():
        if not path.startswith(prefix) or not path.endswith(".go") or path.endswith("_test.go"):
            continue
        if any(rx.search(path[len(prefix):]) for rx in excludes):
            continue
        # The coverage runs compile every package (go test ./..., and
        # -coverpkg=./... for the API's integration runs), tested or not. A
        # file missing from the profile was not built (a build tag, another
        # platform, wireinject) or has no statements: nothing to cover.
        if path in coverage:
            per_file[path] = {n: coverage[path][n] for n in lines if n in coverage[path]}
    return _tally(per_file)


def console_line_coverage(console_root, repo_root):
    """Repo path -> line -> covered from every package's coverage/lcov.info.

    Also returns the package dirs (repo-relative) that have a report.
    node_modules is never walked: pnpm symlinks packages into it, and
    following those links stalls the walk.
    """
    repo_root = os.path.abspath(repo_root)
    coverage, reported = {}, set()
    for dirpath, dirnames, filenames in os.walk(os.path.abspath(console_root)):
        dirnames[:] = [d for d in dirnames if d != "node_modules"]
        if os.path.basename(dirpath) != "coverage":
            continue
        dirnames[:] = []
        if "lcov.info" not in filenames:
            continue
        package = os.path.dirname(dirpath)
        reported.add(os.path.relpath(package, repo_root).replace(os.sep, "/"))
        lines = None
        with open(os.path.join(dirpath, "lcov.info")) as f:
            for line in f:
                line = line.strip()
                if line.startswith("SF:"):
                    src = os.path.join(package, line[3:])  # absolute SF wins
                    rel = os.path.relpath(os.path.normpath(src), repo_root)
                    lines = coverage.setdefault(rel.replace(os.sep, "/"), {})
                elif line.startswith("DA:") and lines is not None:
                    n, hits = line[3:].split(",")[:2]
                    lines[int(n)] = lines.get(int(n), False) or int(hits) > 0
                elif line == "end_of_record":
                    lines = None
    return coverage, reported


def _console_package(path, repo_root):
    """The nearest ancestor dir of a repo path that has a package.json."""
    d = os.path.dirname(path)
    while d.startswith("console"):
        if os.path.isfile(os.path.join(repo_root, d, "package.json")):
            return d
        d = os.path.dirname(d)
    return None


def evaluate_console(changed, coverage, reported_packages, no_coverage_packages, repo_root):
    """Covered and total coverable changed lines under console/."""
    skip = {"console/" + p.strip("/") for p in no_coverage_packages}
    per_file = {}
    for path, lines in changed.items():
        if not path.startswith("console/") or not path.endswith((".ts", ".tsx")):
            continue
        package = _console_package(path, repo_root)
        if package is None or not path.startswith(package + "/src/"):
            continue
        if package in reported_packages:
            # Vitest includes every src file, so one missing from the report
            # was excluded on purpose (stories, tests, pure barrels).
            if path in coverage:
                per_file[path] = {n: coverage[path][n] for n in lines if n in coverage[path]}
        elif package not in skip and not CONSOLE_EXCLUDED.search(path):
            # A package that runs no coverage at all: its changed code is untested.
            text = _read_source(os.path.join(repo_root, path))
            if text is not None:
                per_file[path] = {n: False for n in _ts_code_lines(text, lines)}
    return _tally(per_file)
