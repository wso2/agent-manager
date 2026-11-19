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

"""Short, human-readable progress lines on stderr while a scan runs.

Lines are plain text with no colours or carriage returns, so they read the same in a terminal and in a CI
log; they are printed whether or not stderr is a terminal (the runner has no quiet mode). They hold only
repository names, counts and safe reasons, never a secret. Control characters in names are replaced, so a
name cannot move the cursor or recolour the terminal.
"""

import sys
from typing import TextIO

from amp_shadow_runner.engine.scan import ScanResult
from amp_shadow_runner.report import Report, RepositoryStatus


def _clean(text: str) -> str:
    return "".join(c if c.isprintable() else "?" for c in text)


def _plural(count: int, noun: str, plural: str | None = None) -> str:
    return f"{count} {noun}" if count == 1 else f"{count} {plural or noun + 's'}"


class Progress:
    """Writes progress lines to ``out`` (default ``sys.stderr``)."""

    def __init__(self, out: TextIO | None = None) -> None:
        self._out = out

    def _print(self, line: str) -> None:
        print(line, file=sys.stderr if self._out is None else self._out, flush=True)

    def _repo(self, index: int, total: int, name: str, text: str) -> None:
        self._print(f"[{index}/{total}] {_clean(name)}: {text}")

    def downloading(self, index: int, total: int, name: str) -> None:
        self._repo(index, total, name, "downloading")

    def scanning(self, index: int, total: int, name: str) -> None:
        self._repo(index, total, name, "scanning")

    def scanned(self, index: int, total: int, name: str, result: ScanResult) -> None:
        agents = sum(f.tag != "test" for f in result.findings)
        text = f"done, {_plural(agents, 'agent')}, {_plural(result.files_scanned, 'file')} scanned"
        if result.files_skipped:
            text += f", {result.files_skipped} skipped"
        tagged = len(result.findings) - agents
        if tagged:
            text += f" ({tagged} more in test code)"
        self._repo(index, total, name, text)

    def failed(self, index: int, total: int, name: str, reason: str) -> None:
        self._repo(index, total, name, f"failed: {_clean(reason)}")

    def summary(self, report: Report, location: str) -> None:
        """The closing lines: repositories by status, agents found, files, and where the report is."""
        summary = report.summary
        repos = summary.repositories
        scanned = repos[RepositoryStatus.SCANNED] + repos[RepositoryStatus.GONE]
        extra = [
            f"{repos[status]} {label}"
            for status, label in (
                (RepositoryStatus.GONE, "not found"),
                (RepositoryStatus.FAILED, "failed"),
                (RepositoryStatus.SKIPPED, "skipped"),
            )
            if repos[status]
        ]
        line = f"Scanned {_plural(scanned, 'repository', 'repositories')}"
        self._print(line + (f" ({', '.join(extra)})." if extra else "."))
        line = f"Found {_plural(summary.proven + summary.not_sure, 'agent')}: "
        line += f"{summary.proven} proven, {summary.not_sure} not sure"
        if summary.test:
            line += f"; {summary.test} more in test code, not published"
        self._print(line + ".")
        self._print(f"Files: {summary.files_scanned} scanned, {summary.files_skipped} skipped.")
        self._print(f"Report: {_clean(location)}")
