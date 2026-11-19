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

"""amp_shadow_runner.progress: progress lines on stderr."""

import io
from datetime import UTC, datetime

import pytest

from amp_shadow_runner.engine.classify import Evidence, Finding, Verdict
from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.engine.scan import ScanResult
from amp_shadow_runner.progress import Progress
from amp_shadow_runner.report import Report, RepositoryReport, RepositoryStatus

NOW = datetime(2026, 11, 19, tzinfo=UTC)


def _finding(name: str, tag: Tag | None = None, verdict: Verdict = Verdict.PROVEN) -> Finding:
    return Finding(
        f"r:a.py:{name}",
        name,
        "r",
        "a.py",
        1,
        "crewai",
        verdict,
        tag,
        {"framework": "crewai"},
        (Evidence("d2_agent_import", "a.py", {"imports": 1}),),
    )


def _lines(write: object) -> list[str]:
    out = io.StringIO()
    assert callable(write)
    write(Progress(out))
    return out.getvalue().splitlines()


def test_repository_lines() -> None:
    result = ScanResult((_finding("a"), _finding("b", tag="example"), _finding("c", tag="test")), 120, 2, 0)

    def write(p: Progress) -> None:
        p.downloading(1, 3, "support-bot")
        p.scanning(1, 3, "support-bot")
        p.scanned(1, 3, "support-bot", result)
        p.scanned(2, 3, "tiny", ScanResult((_finding("x"),), 1, 0, 0))
        p.failed(3, 3, "legacy\x1b[31m", "download failed (HTTP 502)")

    assert _lines(write) == [
        "[1/3] support-bot: downloading",
        "[1/3] support-bot: scanning",
        "[1/3] support-bot: done, 2 agents (1 more in test code), 120 files scanned, 2 skipped",
        "[2/3] tiny: done, 1 agent, 1 file scanned",
        "[3/3] legacy?[31m: failed: download failed (HTTP 502)",
    ]


@pytest.mark.parametrize(
    ("repositories", "expected"),
    [
        (
            (
                RepositoryReport(
                    "a",
                    RepositoryStatus.SCANNED,
                    files_scanned=10,
                    files_skipped=1,
                    findings=(
                        _finding("x"),
                        _finding("y", verdict=Verdict.NOT_SURE),
                        _finding("e", tag="example"),
                        _finding("z", tag="test"),
                    ),
                ),
                RepositoryReport("b", RepositoryStatus.GONE),
                RepositoryReport("c", RepositoryStatus.FAILED, reason="r"),
            ),
            [
                "Scanned 2 repositories (1 not found, 1 failed).",
                "Found 3 agents: 2 proven, 1 not sure; 1 more in test code, not published.",
                "Files: 10 scanned, 1 skipped.",
                "Report: report.json in the current folder",
            ],
        ),
        (
            (RepositoryReport("a", RepositoryStatus.SCANNED, files_scanned=1),),
            [
                "Scanned 1 repository.",
                "Found 0 agents: 0 proven, 0 not sure.",
                "Files: 1 scanned, 0 skipped.",
                "Report: report.json in the current folder",
            ],
        ),
    ],
)
def test_summary(repositories: tuple[RepositoryReport, ...], expected: list[str]) -> None:
    report = Report("id", "github", "acme", NOW, NOW, repositories)
    assert _lines(lambda p: p.summary(report, "report.json in the current folder")) == expected


def test_default_stream_is_stderr(capsys: pytest.CaptureFixture[str]) -> None:
    Progress().scanning(1, 1, "r")
    captured = capsys.readouterr()
    assert (captured.out, captured.err) == ("", "[1/1] r: scanning\n")
