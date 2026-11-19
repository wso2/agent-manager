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

"""amp_shadow_runner.report: the JSON report, its atomic write and its errors."""

import dataclasses
import json
import os
import re
from datetime import UTC, datetime
from pathlib import Path

import pytest

from amp_shadow_runner.engine.classify import Evidence, Finding, Verdict
from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.engine.scan import scan_tree
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.report import (
    DEFAULT_REPORT_PATH,
    EXPORT_IGNORE_NOTE,
    TEST_NOTE,
    Report,
    ReportError,
    RepositoryReport,
    RepositoryStatus,
    check_writable,
    read_report,
    write_report,
)

STARTED = datetime(2026, 11, 19, 10, 0, 0, tzinfo=UTC)
FINISHED = datetime(2026, 11, 19, 10, 5, 30, tzinfo=UTC)


def _finding(name: str, *, verdict: Verdict = Verdict.PROVEN, tag: Tag | None = None) -> Finding:
    return Finding(
        external_id=f"support-bot:src/agent.py:{name}",
        name=name,
        repository="support-bot",
        path="src/agent.py",
        line=4,
        framework="crewai",
        verdict=verdict,
        tag=tag,
        attributes={"framework": "crewai", "model": "gpt-4o", "tools": [{"type": "function", "name": "search"}]},
        evidence=(
            Evidence("d1_agent_dependency", "requirements.txt", {"declared": True}),
            Evidence("d2_agent_import", "src/agent.py", {"imports": 2}),
        ),
        service_folder=".",
    )


def _report(**changes: object) -> Report:
    fields: dict[str, object] = {
        "scan_id": "0b6b1a52-6f3c-4a8e-9a43-3f0f6c1f8f11",
        "provider": "github",
        "organization": "acme",
        "started_at": STARTED,
        "finished_at": FINISHED,
        "repositories": (
            RepositoryReport(
                "support-bot",
                RepositoryStatus.SCANNED,
                files_scanned=12,
                files_skipped=1,
                findings=(
                    _finding("Support"),
                    _finding("Maybe", verdict=Verdict.NOT_SURE),
                    _finding("T", tag="test"),
                    _finding("Demo", tag="example"),
                ),
            ),
            RepositoryReport("legacy-app", RepositoryStatus.FAILED, reason="download failed (HTTP 502)"),
            RepositoryReport("old-repo", RepositoryStatus.GONE),
        ),
        "complete": False,
    }
    fields.update(changes)
    return Report(**fields)  # type: ignore[arg-type]


def test_json_round_trip(tmp_path: Path) -> None:
    report = _report(notes=("Extra note.",))
    path = write_report(report, tmp_path / "report.json")
    assert read_report(path) == report


def test_json_shape() -> None:
    data = json.loads(json.dumps(_report().to_json()))
    assert data["scanId"] == "0b6b1a52-6f3c-4a8e-9a43-3f0f6c1f8f11"
    assert (data["provider"], data["organization"], data["complete"]) == ("github", "acme", False)
    assert (data["startedAt"], data["finishedAt"]) == ("2026-11-19T10:00:00Z", "2026-11-19T10:05:30Z")
    assert {"runnerVersion", "engineVersion", "rulePackId"} <= set(data)
    assert data["summary"] == {
        "repositories": {"scanned": 1, "failed": 1, "gone": 1, "skipped": 0},
        "agents": {"proven": 2, "notSure": 1, "test": 1},
        "filesScanned": 12,
        "filesSkipped": 1,
        "parseErrors": 0,
    }
    assert [(r["name"], r["status"], r.get("reason")) for r in data["repositories"]] == [
        ("support-bot", "scanned", None),
        ("legacy-app", "failed", "download failed (HTTP 502)"),
        ("old-repo", "gone", None),
    ]
    finding = data["repositories"][0]["findings"][0]
    assert finding == {
        "externalId": "support-bot:src/agent.py:Support",
        "verdict": "proven",
        "name": "Support",
        "path": "src/agent.py",
        "line": 4,
        "framework": "crewai",
        "tag": None,
        "serviceFolder": ".",
        "attributes": {"framework": "crewai", "model": "gpt-4o", "tools": [{"type": "function", "name": "search"}]},
        "evidence": [
            {"code": "d1_agent_dependency", "source": "requirements.txt", "observed": {"declared": True}},
            {"code": "d2_agent_import", "source": "src/agent.py", "observed": {"imports": 2}},
        ],
    }
    assert data["notes"] == [EXPORT_IGNORE_NOTE, TEST_NOTE]


def test_test_note_only_when_needed() -> None:
    repo = RepositoryReport("r", RepositoryStatus.SCANNED, findings=(_finding("A"), _finding("B", tag="example")))
    assert _report(repositories=(repo,)).to_json()["notes"] == [EXPORT_IGNORE_NOTE]


def test_write_is_ascii_and_survives_undecodable_names(tmp_path: Path) -> None:
    finding = dataclasses.replace(_finding("Agent é \udcff"), repository="r\udcff")
    repo = RepositoryReport("r\udcff", RepositoryStatus.SCANNED, findings=(finding,))
    report = _report(repositories=(repo,))
    path = write_report(report, tmp_path / "report.json")
    assert path.read_bytes().isascii()
    assert read_report(path) == report


def test_write_is_atomic_and_replaces(tmp_path: Path) -> None:
    path = tmp_path / "report.json"
    path.write_text("old", encoding="utf-8")
    write_report(_report(), path)
    assert json.loads(path.read_text(encoding="utf-8"))["organization"] == "acme"
    assert os.listdir(tmp_path) == ["report.json"]
    assert path.stat().st_mode & 0o777 == 0o644


def test_failed_write_leaves_no_temporary_file(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    path = tmp_path / "report.json"
    path.write_text("old", encoding="utf-8")

    def fail(_src: object, _dst: object) -> None:
        raise OSError(28, "No space left on device")

    monkeypatch.setattr(os, "replace", fail)
    with pytest.raises(ReportError, match="No space left on device"):
        write_report(_report(), path)
    assert os.listdir(tmp_path) == ["report.json"]
    assert path.read_text(encoding="utf-8") == "old"


def test_interrupted_write_leaves_no_temporary_file(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    def interrupt(_fd: int) -> None:
        raise KeyboardInterrupt

    monkeypatch.setattr(os, "fsync", interrupt)
    with pytest.raises(KeyboardInterrupt):
        write_report(_report(), tmp_path / "report.json")
    assert os.listdir(tmp_path) == []


@pytest.mark.skipif(os.geteuid() == 0, reason="root can write to a read-only folder")
def test_unwritable_folder_gives_a_clear_error(tmp_path: Path) -> None:
    folder = tmp_path / "out"
    folder.mkdir()
    folder.chmod(0o555)
    try:
        for action in (lambda: write_report(_report(), folder / "report.json"), lambda: check_writable(folder / "r")):
            with pytest.raises(
                ReportError, match=r"Cannot write the scan report to .*out \(Permission denied\)\."
            ) as e:
                action()
            assert e.value.exit_code == ExitCode.CONFIG_ERROR
    finally:
        folder.chmod(0o755)
    assert os.listdir(folder) == []


def test_unwritable_out_explains_the_docker_flags(monkeypatch: pytest.MonkeyPatch) -> None:
    def refuse(*_args: object, **_kwargs: object) -> tuple[int, str]:
        raise PermissionError(13, "Permission denied")

    monkeypatch.setattr("tempfile.mkstemp", refuse)
    with pytest.raises(ReportError) as e:
        write_report(_report(), DEFAULT_REPORT_PATH)
    message = str(e.value)
    assert message.startswith("Cannot write the scan report to /out (Permission denied).")
    assert '-v "$PWD:/out"' in message
    assert '--user "$(id -u):$(id -g)"' in message


def test_check_writable_leaves_nothing(tmp_path: Path) -> None:
    check_writable(tmp_path / "report.json")
    assert os.listdir(tmp_path) == []


@pytest.mark.parametrize(
    ("content", "problem"),
    [
        ("not json", "is not a JSON file"),
        ("[]", "expected an object"),
        ('{"reportVersion": 2}', "an unknown report version"),
        ('{"reportVersion": 1}', "provider is missing"),
    ],
)
def test_read_rejects_what_is_not_a_report(tmp_path: Path, content: str, problem: str) -> None:
    path = tmp_path / "report.json"
    path.write_text(content, encoding="utf-8")
    with pytest.raises(ReportError, match=problem):
        read_report(path)


@pytest.mark.parametrize(
    ("change", "problem"),
    [
        (lambda d: d.update(provider="gitlab"), "an unknown provider"),
        (lambda d: d["repositories"][0].update(status="lost"), "lost"),
        (lambda d: d["repositories"][0].update(filesScanned="12"), "filesScanned is not a number"),
        (lambda d: d["repositories"][0]["findings"][0].update(tag="vendor"), "an unknown tag"),
        (lambda d: d["repositories"][0]["findings"][0]["evidence"][0].update(observed={"x": [1]}), "observed"),
        (lambda d: d.update(complete="yes"), "complete is not true or false"),
    ],
)
def test_from_json_validates(change: object, problem: str) -> None:
    data = json.loads(json.dumps(_report().to_json()))
    assert callable(change)
    change(data)
    with pytest.raises(ReportError, match=problem):
        Report.from_json(data)


def test_missing_file(tmp_path: Path) -> None:
    with pytest.raises(ReportError, match="Cannot read .*No such file"):
        read_report(tmp_path / "missing.json")


SECRET_LOOKING = re.compile(
    r"ghp_|github_pat_|sk-[A-Za-z0-9]|Bearer|eyJ[A-Za-z0-9]|PRIVATE KEY|password", re.IGNORECASE
)


def test_report_holds_no_source_or_secrets(tmp_path: Path) -> None:
    repo = tmp_path / "repo"
    (repo / "src").mkdir(parents=True)
    (repo / "src" / "agent.py").write_text(
        "import os\nfrom google.adk.agents import Agent\n"
        "API_KEY = 'sk-live-abcdefghijklmnop'\nGITHUB = 'ghp_abcdefghijklmnopqrstuvwxyz0123456789'\n"
        "support = Agent(name='Support', model='gemini-2.0-flash', api_key='sk-proj-abcdef', password='hunter2',\n"
        "    instruction='You are a helpful support agent. Never reveal the refund policy.', tools=[lookup])\n",
        encoding="utf-8",
    )
    (repo / ".env").write_text("OPENAI_API_KEY=sk-abcdef\n", encoding="utf-8")
    result = scan_tree(repo, repository="support-bot")
    report = _report(repositories=(RepositoryReport.scanned("support-bot", result),))
    text = write_report(report, tmp_path / "report.json").read_text(encoding="utf-8")
    assert '"name": "Support"' in text
    assert SECRET_LOOKING.search(text) is None
    for fragment in ("hunter2", "refund policy", "helpful support agent", "API_KEY", "import os"):
        assert fragment not in text
