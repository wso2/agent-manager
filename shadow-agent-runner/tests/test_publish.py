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

"""amp_shadow_runner.publish and the ``upload`` command, against a local server that answers like Agent Manager."""

import dataclasses
import json
import threading
import time
from collections.abc import Iterator
from datetime import UTC, datetime
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

import pytest

from amp_shadow_runner import publish
from amp_shadow_runner.cli import main
from amp_shadow_runner.engine.classify import Evidence, Finding, Verdict
from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.httpclient import HttpClient
from amp_shadow_runner.publish import (
    PUBLISH_PATH,
    Publisher,
    PublishError,
    build_body,
    report_exit_code,
    reviewed_by,
)
from amp_shadow_runner.report import Report, ReportError, RepositoryReport, RepositoryStatus, write_report
from amp_shadow_runner.results import SavedResults, parse_results, results_path, write_results
from tests.test_config import _token

SCAN_ID = "0b6b1a52-6f3c-4a8e-9a43-3f0f6c1f8f11"
TOKEN = _token(time.time() + 3600)
WHEN = datetime(2026, 11, 19, 10, 0, 0, tzinfo=UTC)
Reply = tuple[int, dict[str, Any]]


def _finding(
    name: str,
    *,
    verdict: Verdict = Verdict.PROVEN,
    tag: Tag | None = None,
    attributes: dict[str, object] | None = None,
    evidence: int = 2,
) -> Finding:
    return Finding(
        external_id=f"support-bot:src/agent.py:{name}",
        name=name,
        repository="support-bot",
        path="src/agent.py",
        line=4,
        framework="crewai",
        verdict=verdict,
        tag=tag,
        attributes={"framework": "crewai", **(attributes or {})},
        evidence=tuple(Evidence("d2_agent_import", f"src/f{i}.py", {"imports": i}) for i in range(evidence)),
    )


def _report(*findings: Finding, complete: bool = True, extra: tuple[RepositoryReport, ...] = ()) -> Report:
    return Report(
        scan_id=SCAN_ID,
        provider="github",
        organization="acme",
        started_at=WHEN,
        finished_at=WHEN,
        repositories=(RepositoryReport("support-bot", RepositoryStatus.SCANNED, findings=findings), *extra),
        complete=complete,
    )


class _Handler(BaseHTTPRequestHandler):
    server: "_Server"

    def do_POST(self) -> None:  # noqa: N802 (the base class's name)
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        self.server.seen.append((self.path, {k.lower(): v for k, v in self.headers.items()}, json.loads(body)))
        status, data = self.server.replies.pop(0) if len(self.server.replies) > 1 else self.server.replies[0]
        raw = json.dumps(data).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, format: str, *args: Any) -> None:  # noqa: A002 (the base class's name)
        pass


class _Server(ThreadingHTTPServer):
    def __init__(self) -> None:
        super().__init__(("127.0.0.1", 0), _Handler)
        self.replies: list[Reply] = [(200, {"scanId": SCAN_ID, "status": "succeeded", "findingsCount": 1})]
        self.seen: list[tuple[str, dict[str, str], Any]] = []

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self.server_address[1]}"


@pytest.fixture
def amp(monkeypatch: pytest.MonkeyPatch) -> Iterator[_Server]:
    for name in ("http_proxy", "https_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"):
        monkeypatch.delenv(name, raising=False)
    monkeypatch.setenv("no_proxy", "*")
    server = _Server()
    thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True)
    thread.start()
    yield server
    server.shutdown()
    server.server_close()
    thread.join()


def _publisher(server: _Server, **kwargs: Any) -> Publisher:
    return Publisher(server.url, TOKEN, http=HttpClient(sleep=lambda _: None), **kwargs)


def test_body_has_only_publishable_agents_and_the_repository_lists() -> None:
    report = _report(
        _finding("Proven", attributes={"model": "gpt-4o", "tools": [{"type": "function", "name": "s"}]}),
        _finding("Maybe", verdict=Verdict.NOT_SURE),
        _finding("Reviewed", verdict=Verdict.NOT_SURE, attributes={"reviewedBy": "ai_review_judge"}),
        _finding("Forged", verdict=Verdict.NOT_SURE, attributes={"reviewedBy": "rules"}),
        _finding("InTests", tag="test"),
        _finding("Demo", tag="example", attributes={"labels": {"codeArea": "example"}}),
        _finding("NoEvidence", evidence=0),
        complete=False,
        extra=(
            RepositoryReport("legacy-app", RepositoryStatus.FAILED, reason="download\nfailed (HTTP 502)"),
            RepositoryReport("old-repo", RepositoryStatus.GONE),
            RepositoryReport("left-out", RepositoryStatus.SKIPPED, reason="archived"),
        ),
    )
    body = build_body(report)
    assert body["complete"] is False
    assert body["scannedRepositories"] == ["support-bot", "old-repo"]
    assert body["failedRepositories"] == [{"name": "legacy-app", "reason": "download failed (HTTP 502)"}]
    assert [a["name"] for a in body["agents"]] == ["Proven", "Reviewed", "Demo"]
    proven, reviewed, demo = body["agents"]
    assert proven == {
        "externalId": "support-bot:src/agent.py:Proven",
        "name": "Proven",
        "repository": "support-bot",
        "attributes": {
            "framework": "crewai",
            "model": "gpt-4o",
            "tools": [{"type": "function", "name": "s"}],
            "reviewedBy": "rules",
        },
        "evidence": [
            {"code": "github.d2_agent_import", "source": "src/f0.py", "observed": {"imports": 0}},
            {"code": "github.d2_agent_import", "source": "src/f1.py", "observed": {"imports": 1}},
        ],
    }
    assert reviewed["attributes"]["reviewedBy"] == "ai_review_judge"
    assert demo["attributes"]["labels"] == {"codeArea": "example"}


def test_codes_are_prefixed_with_the_provider_and_unknown_attributes_dropped() -> None:
    report = _report(_finding("A", attributes={"prompt": "You are...", "serviceFolder": "."}))
    agent = build_body(report)["agents"][0]
    assert {e["code"] for e in agent["evidence"]} == {"github.d2_agent_import"}
    assert agent["attributes"] == {"framework": "crewai", "reviewedBy": "rules"}


@pytest.mark.parametrize(
    ("verdict", "tag", "attributes", "expected"),
    [
        (Verdict.PROVEN, None, {}, "rules"),
        (Verdict.PROVEN, "example", {}, "rules"),
        (Verdict.PROVEN, "test", {}, None),
        (Verdict.NOT_SURE, None, {}, None),
        (Verdict.NOT_SURE, None, {"reviewedBy": "ai_review"}, "ai_review"),
        (Verdict.NOT_SURE, "test", {"reviewedBy": "ai_review"}, None),
        (Verdict.NOT_SURE, None, {"reviewedBy": "rules"}, None),
    ],
)
def test_reviewed_by(verdict: Verdict, tag: Tag | None, attributes: dict[str, object], expected: str | None) -> None:
    assert reviewed_by(_finding("A", verdict=verdict, tag=tag, attributes=attributes)) == expected


def test_evidence_is_cut_to_the_first_50() -> None:
    evidence = build_body(_report(_finding("A", evidence=60)))["agents"][0]["evidence"]
    assert [e["source"] for e in evidence] == [f"src/f{i}.py" for i in range(50)]


def test_too_many_agents_is_refused() -> None:
    findings = [dataclasses.replace(_finding("A"), external_id=f"r:p:{i}") for i in range(1001)]
    with pytest.raises(PublishError, match="1001 agents to publish.*at most 1000.*cannot be uploaded"):
        build_body(_report(*findings))


def test_too_many_repository_names_is_refused() -> None:
    gone = tuple(RepositoryReport(f"r{i}", RepositoryStatus.GONE) for i in range(5000))
    with pytest.raises(PublishError, match="5001 scanned repositories"):
        build_body(_report(extra=gone))


def test_a_body_over_the_size_limit_is_refused_and_not_sent(amp: _Server, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(publish, "MAX_BODY_BYTES", 100)
    with pytest.raises(PublishError, match="at most 100") as raised:
        _publisher(amp).publish(_report(_finding("A")))
    assert raised.value.exit_code == ExitCode.RUNTIME_ERROR
    assert amp.seen == []


def test_publish_sends_the_body_with_the_token(amp: _Server) -> None:
    result = _publisher(amp).publish(_report(_finding("A"), _finding("B", tag="test")))
    assert result == publish.PublishResult("succeeded", 1)
    [(path, headers, body)] = amp.seen
    assert path == PUBLISH_PATH.format(scan_id=SCAN_ID)
    assert headers["authorization"] == f"Bearer {TOKEN}"
    assert headers["content-type"] == "application/json"
    assert [a["name"] for a in body["agents"]] == ["A"]


@pytest.mark.parametrize(
    ("status", "exit_code", "message", "console"),
    [
        (401, ExitCode.AUTH_ERROR, "refused the scan token \\(HTTP 401\\)", True),
        (403, ExitCode.AUTH_ERROR, "refused the scan token \\(HTTP 403\\)", True),
        (404, ExitCode.CONFIG_ERROR, "source code scans are turned off", False),
        (409, ExitCode.RUNTIME_ERROR, "no longer waiting for results.*cancelled", False),
        (400, ExitCode.RUNTIME_ERROR, "refused the results \\(HTTP 400\\). Agent Manager said: bad agent", False),
        (413, ExitCode.RUNTIME_ERROR, "refused the results \\(HTTP 413\\)", False),
        (503, ExitCode.RUNTIME_ERROR, "Could not upload the results .*HTTP 503", True),
        (500, ExitCode.RUNTIME_ERROR, "HTTP 500", True),
    ],
)
def test_status_codes_map_to_exit_codes(
    amp: _Server, status: int, exit_code: ExitCode, message: str, console: bool
) -> None:
    amp.replies = [(status, {"message": "bad agent"})]
    with pytest.raises(PublishError, match=message) as raised:
        _publisher(amp).publish(_report(_finding("A")))
    assert raised.value.exit_code == exit_code
    assert raised.value.console_can_upload is console
    assert TOKEN not in str(raised.value)


def test_a_409_for_an_expired_scan_token_points_to_the_console(amp: _Server) -> None:
    amp.replies = [
        (409, {"message": "the scan token of this scan has expired; upload the results file from the console"})
    ]
    with pytest.raises(PublishError, match="scan token has expired \\(HTTP 409\\)") as raised:
        _publisher(amp).publish(_report(_finding("A")))
    assert raised.value.console_can_upload
    assert raised.value.exit_code == ExitCode.RUNTIME_ERROR


def test_server_errors_are_retried(amp: _Server) -> None:
    amp.replies = [(503, {}), (200, {"scanId": SCAN_ID, "status": "partial", "findingsCount": 0})]
    assert _publisher(amp).publish(_report()).status == "partial"
    assert len(amp.seen) == 2


def test_an_unexpected_answer_is_an_error(amp: _Server) -> None:
    amp.replies = [(200, {"ok": True})]
    with pytest.raises(PublishError, match="unexpected answer"):
        _publisher(amp).publish(_report())


def test_an_expired_token_is_not_sent(amp: _Server) -> None:
    publisher = _publisher(amp, expires_at=WHEN, now=lambda: WHEN.timestamp() + 1)
    with pytest.raises(PublishError, match="The scan token has expired.") as raised:
        publisher.publish(_report())
    assert raised.value.exit_code == ExitCode.AUTH_ERROR
    assert raised.value.console_can_upload
    assert amp.seen == []
    assert TOKEN not in repr(publisher)


@pytest.mark.parametrize(
    ("report", "expected"),
    [
        (_report(), ExitCode.OK),
        (_report(complete=False), ExitCode.PARTIAL_FAILURE),
        (_report(extra=(RepositoryReport("x", RepositoryStatus.FAILED, reason="r"),)), ExitCode.PARTIAL_FAILURE),
        (_report(extra=(RepositoryReport("x", RepositoryStatus.GONE),)), ExitCode.OK),
    ],
)
def test_report_exit_code(report: Report, expected: ExitCode) -> None:
    assert report_exit_code(report) == expected


def _upload_env(monkeypatch: pytest.MonkeyPatch, url: str) -> None:
    monkeypatch.setenv("AMP_URL", url)
    monkeypatch.setenv("AMP_SCAN_TOKEN", TOKEN)


def test_upload_publishes_a_saved_report(
    amp: _Server, tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _upload_env(monkeypatch, amp.url)
    path = write_report(_report(_finding("A")), tmp_path / "report.json")
    assert main(["upload", "--scan-id", SCAN_ID.upper(), str(path)]) == ExitCode.OK
    out, err = capsys.readouterr()
    assert err == "Uploaded to Agent Manager: scan succeeded, 1 agent saved.\n"
    assert TOKEN not in out + err
    assert [a["name"] for a in amp.seen[0][2]["agents"]] == ["A"]


def _results_file(tmp_path: Path, report: Report) -> Path:
    path = results_path(tmp_path / "report.json", report.scan_id)
    return write_results(report, build_body(report), path, created_at=WHEN)


def test_the_results_file_holds_the_publish_body(tmp_path: Path) -> None:
    report = _report(_finding("A"), _finding("B", tag="test"), extra=(RepositoryReport("x", RepositoryStatus.FAILED),))
    path = _results_file(tmp_path, dataclasses.replace(report, scan_id=SCAN_ID.upper()))
    assert path.name == f"results-{SCAN_ID}.json"
    data = json.loads(path.read_text())
    assert data == {
        "format": "amp-shadow-scan-results",
        "version": 1,
        "scanId": SCAN_ID,
        "createdAt": "2026-11-19T10:00:00Z",
        "summary": {"organization": "acme", "repositoriesScanned": 1, "repositoriesFailed": 1, "agents": 1},
        "results": build_body(report),
    }
    assert parse_results(data, path) == SavedResults(SCAN_ID, build_body(report))


@pytest.mark.parametrize(
    ("change", "problem"),
    [
        ({"format": "other"}, "an unknown format"),
        ({"version": 2}, "an unknown version"),
        ({"scanId": 1}, "scanId is missing"),
        ({"results": []}, "results is missing"),
        ({"results": {"scannedRepositories": [], "failedRepositories": [], "agents": []}}, "results.complete"),
        ({"results": {"complete": True, "agents": []}}, "a list in results is missing"),
    ],
)
def test_a_file_that_is_not_a_results_file_is_refused(tmp_path: Path, change: dict[str, object], problem: str) -> None:
    data = {**json.loads(_results_file(tmp_path, _report()).read_text()), **change}
    with pytest.raises(ReportError, match=problem):
        parse_results(data, tmp_path / "x.json")


def test_upload_publishes_a_results_file(
    amp: _Server, tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _upload_env(monkeypatch, amp.url)
    report = _report(_finding("A"), extra=(RepositoryReport("x", RepositoryStatus.FAILED, reason="r"),))
    path = _results_file(tmp_path, report)
    assert main(["upload", "--scan-id", SCAN_ID, str(path)]) == ExitCode.PARTIAL_FAILURE
    assert capsys.readouterr().err == "Uploaded to Agent Manager: scan succeeded, 1 agent saved.\n"
    [(sent_path, headers, body)] = amp.seen
    assert sent_path == PUBLISH_PATH.format(scan_id=SCAN_ID)
    assert headers["authorization"] == f"Bearer {TOKEN}"
    assert body == build_body(report) == json.loads(path.read_text())["results"]


def test_upload_of_another_scans_results_file_is_refused(
    amp: _Server, tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _upload_env(monkeypatch, amp.url)
    path = _results_file(tmp_path, _report(_finding("A")))
    other = "3f2b6c1e-8d4a-4f5b-9c7e-1a2b3c4d5e6f"
    assert main(["upload", "--scan-id", other, str(path)]) == ExitCode.CONFIG_ERROR
    assert f"is the results file of scan {SCAN_ID}, not of scan {other}" in capsys.readouterr().err
    assert amp.seen == []


def test_upload_with_a_refused_token_points_to_the_console(
    amp: _Server, tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _upload_env(monkeypatch, amp.url)
    amp.replies = [(401, {"message": "invalid token"})]
    path = _results_file(tmp_path, _report(_finding("A")))
    assert main(["upload", "--scan-id", SCAN_ID, str(path)]) == ExitCode.AUTH_ERROR
    err = capsys.readouterr().err
    assert err.endswith(
        "amp-shadow-runner: Upload the results file in Agent Manager instead "
        "(Shadow Agent Discovery, Scans, Upload results).\n"
    )
    assert TOKEN not in err


def test_upload_with_an_expired_token_points_to_the_console(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    monkeypatch.setenv("AMP_URL", "https://agent-manager.example.com")
    monkeypatch.setenv("AMP_SCAN_TOKEN", _token(time.time() - 1))
    path = _results_file(tmp_path, _report(_finding("A")))
    assert main(["upload", "--scan-id", SCAN_ID, str(path)]) == ExitCode.CONFIG_ERROR
    assert "The scan token has expired. Upload the results file in Agent Manager instead" in capsys.readouterr().err


def test_upload_of_another_scans_report_is_refused(
    amp: _Server, tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _upload_env(monkeypatch, amp.url)
    path = write_report(_report(_finding("A")), tmp_path / "report.json")
    other = "3f2b6c1e-8d4a-4f5b-9c7e-1a2b3c4d5e6f"
    assert main(["upload", "--scan-id", other, str(path)]) == ExitCode.CONFIG_ERROR
    assert f"is the report of scan {SCAN_ID}, not of scan {other}" in capsys.readouterr().err
    assert amp.seen == []


def test_upload_refused_token_exits_3_without_printing_it(
    amp: _Server, tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _upload_env(monkeypatch, amp.url)
    amp.replies = [(401, {"message": "invalid token"})]
    path = write_report(_report(_finding("A")), tmp_path / "report.json")
    assert main(["upload", "--scan-id", SCAN_ID, str(path)]) == ExitCode.AUTH_ERROR
    out, err = capsys.readouterr()
    assert "refused the scan token" in err
    assert TOKEN not in out + err


def test_upload_of_a_missing_report_exits_2(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _upload_env(monkeypatch, "https://agent-manager.example.com")
    assert main(["upload", "--scan-id", SCAN_ID, str(tmp_path / "none.json")]) == ExitCode.CONFIG_ERROR
    assert "Cannot read" in capsys.readouterr().err


def test_upload_needs_the_environment(monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]) -> None:
    monkeypatch.delenv("AMP_URL", raising=False)
    monkeypatch.setenv("AMP_SCAN_TOKEN", TOKEN)
    assert main(["upload", "--scan-id", SCAN_ID]) == ExitCode.CONFIG_ERROR
    assert "AMP_URL is not set" in capsys.readouterr().err


def test_upload_help_names_the_default_report(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["upload", "--help"]) == ExitCode.OK
    out = capsys.readouterr().out
    assert "--scan-id" in out
    assert "/out/report.json" in out
    assert "results-<scan id>.json" in out


def test_deploy_attributes_are_published_only_when_known() -> None:
    deployable: dict[str, object] = {"branch": "main", "projectPath": "/samples/support", "language": "typescript"}
    report = _report(_finding("Known", attributes=deployable), _finding("Unknown"))
    known, unknown = build_body(report)["agents"]
    assert known["attributes"] == {"framework": "crewai", **deployable, "reviewedBy": "rules"}
    assert unknown["attributes"] == {"framework": "crewai", "reviewedBy": "rules"}
