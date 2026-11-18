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

"""amp_shadow_runner.pipeline: a whole scan, with a fake Git host and a fake publisher (no network)."""

import dataclasses
import io
import json
import tarfile
import time
from collections.abc import Iterable
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import pytest

from amp_shadow_runner.config import ConfigError, ScanConfig
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.gitsource import FailedRepository, GitSourceError, Repository, RepositoryError, RepositoryList
from amp_shadow_runner.pipeline import ANONYMOUS_WARNING, EXPIRES_NOTE, assume_yes, run_scan
from amp_shadow_runner.publish import PublishError, PublishResult, build_body
from amp_shadow_runner.report import EXPORT_IGNORE_NOTE, Report, ReportError, read_report
from tests.test_config import _token

SCAN_ID = "3f2b6c1e-8d4a-4f5b-9c7e-1a2b3c4d5e6f"
SCAN_TOKEN = _token(time.time() + 3600)
GIT_TOKEN = "github_pat_s3cr3t-git-token"
SHA = "0123456789abcdef0123456789abcdef01234567"
AGENT = "from crewai import Agent\nsupport = Agent(role='support')\n"
MAYBE = "from crewai import Agent\n"  # No agent: a file that is scanned and matches nothing.


def _config(**changes: Any) -> ScanConfig:
    config = ScanConfig(
        amp_url="https://agent-manager.example.com",
        token=SCAN_TOKEN,
        token_expires_at=datetime.fromtimestamp(time.time() + 3600, UTC),
        scan_id=SCAN_ID,
        provider="github",
        organization="acme",
        all_repositories=True,
        repositories=(),
        include_private=True,
        read_ci=False,
        api_url="https://api.github.com",
        ai_review=False,
        judge=False,
    )
    return dataclasses.replace(config, **changes)


def _repo(name: str, *, branch: str | None = "main") -> Repository:
    return Repository(name, private=False, default_branch=branch)


class FakeSource:
    """Lists ``listing`` and serves each repository's files as a GitHub-style tarball."""

    def __init__(
        self,
        listing: RepositoryList,
        files: dict[str, dict[str, str]] | None = None,
        *,
        fail: dict[str, Exception] | None = None,
        links: dict[str, str] | None = None,
    ) -> None:
        self.listing = listing
        self.files = files or {}
        self.fail = fail or {}
        self.links = links or {}
        self.calls: list[str] = []

    def list_all(self) -> RepositoryList:
        self.calls.append("list_all")
        return self.listing

    def list_named(self, names: Iterable[str]) -> RepositoryList:
        self.calls.append(f"list_named:{','.join(names)}")
        return self.listing

    def resolve_commit(self, repo: Repository) -> Repository:
        self.calls.append(f"resolve:{repo.name}")
        if repo.default_branch is None:
            return dataclasses.replace(repo, empty=True)
        return dataclasses.replace(repo, commit_sha=SHA)

    def download_tarball(self, repo: Repository, dest_dir: Path) -> Path:
        self.calls.append(f"download:{repo.name}")
        if repo.name in self.fail:
            raise self.fail[repo.name]
        dest = dest_dir / f"{SHA}.tar.gz"
        with tarfile.open(dest, "w:gz") as tar:
            for path, text in self.files.get(repo.name, {}).items():
                data = text.encode()
                info = tarfile.TarInfo(f"acme-{repo.name}-{SHA[:7]}/{path}")
                info.size = len(data)
                tar.addfile(info, io.BytesIO(data))
            for path, target in self.links.items():
                info = tarfile.TarInfo(f"acme-{repo.name}-{SHA[:7]}/{path}")
                info.type, info.linkname = tarfile.SYMTYPE, target
                tar.addfile(info)
        return dest


class FakePublisher:
    """Records each publish and checks the report was already written."""

    def __init__(self, report_path: Path, *, error: PublishError | None = None) -> None:
        self.report_path = report_path
        self.error = error
        self.reports: list[Report] = []
        self.report_on_disk: list[bool] = []

    def publish(self, report: Report, *, report_location: str | None = None) -> PublishResult:
        self.reports.append(report)
        self.report_on_disk.append(self.report_path.exists() and read_report(self.report_path) == report)
        if self.error is not None:
            raise self.error
        agents = len(build_body(report)["agents"])
        return PublishResult("partial", agents, agents)


class Run:
    def __init__(self, tmp_path: Path, source: FakeSource, *, error: PublishError | None = None) -> None:
        self.report_path = tmp_path / "report.json"
        self.source = source
        self.publisher = FakePublisher(self.report_path, error=error)
        self.out = io.StringIO()

    def __call__(
        self,
        config: ScanConfig | None = None,
        *,
        yes: bool = True,
        tty: bool = False,
        answer: str = "",
        token: str | None = GIT_TOKEN,
    ) -> int:
        return run_scan(
            config or _config(),
            yes=yes,
            report_path=self.report_path,
            ask_token=lambda _: token,
            make_source=lambda _config, _token: self.source,
            make_publisher=lambda _config: self.publisher,
            is_tty=lambda: tty,
            read_answer=lambda: answer,
            out=self.out,
        )

    @property
    def body(self) -> dict[str, Any]:
        [report] = self.publisher.reports
        return build_body(report)

    @property
    def text(self) -> str:
        return self.out.getvalue()


def test_one_failed_repository_gives_a_partial_failure(tmp_path: Path) -> None:
    source = FakeSource(
        RepositoryList((_repo("support-bot"), _repo("legacy-app"))),
        {"support-bot": {"src/agent.py": AGENT, "README.md": "x"}},
        fail={"legacy-app": RepositoryError("legacy-app", "GitHub answered HTTP 502")},
    )
    run = Run(tmp_path, source)
    assert run() == ExitCode.PARTIAL_FAILURE
    assert run.publisher.report_on_disk == [True]  # Written before publishing, and published once.
    body = run.body
    assert body["scannedRepositories"] == ["support-bot"]
    assert body["failedRepositories"] == [{"name": "legacy-app", "reason": "GitHub answered HTTP 502"}]
    assert [(a["name"], a["attributes"]["reviewedBy"]) for a in body["agents"]] == [("support", "rules")]
    assert "[2/2] legacy-app: failed: GitHub answered HTTP 502" in run.text
    assert "Published to Agent Manager: scan partial, 1 agent saved." in run.text
    assert EXPORT_IGNORE_NOTE in run.text
    assert f"Report: {run.report_path}" in run.text


def test_everything_scanned_is_ok_and_the_code_is_deleted(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr("tempfile.tempdir", str(tmp_path / "scratch"))
    (tmp_path / "scratch").mkdir()
    source = FakeSource(RepositoryList((_repo("a"), _repo("b"))), {"a": {"agent.py": AGENT}, "b": {"x.py": MAYBE}})
    run = Run(tmp_path, source)
    assert run() == ExitCode.OK
    assert list((tmp_path / "scratch").iterdir()) == []
    assert run.body["scannedRepositories"] == ["a", "b"]
    assert source.calls == ["list_all", "resolve:a", "download:a", "resolve:b", "download:b"]


def test_gone_repositories_count_as_scanned(tmp_path: Path) -> None:
    listing = RepositoryList((_repo("a"),), gone=("old-repo",))
    run = Run(tmp_path, FakeSource(listing, {"a": {"agent.py": AGENT}}))
    assert run(_config(all_repositories=False, repositories=("a", "old-repo"))) == ExitCode.OK
    assert run.source.calls[0] == "list_named:a,old-repo"
    assert run.body["scannedRepositories"] == ["a", "old-repo"]
    assert run.body["failedRepositories"] == []
    assert "Not found, counted as scanned with no agents (1):\n  old-repo\n" in run.text


def test_listing_failures_and_an_incomplete_list_are_partial(tmp_path: Path) -> None:
    listing = RepositoryList((_repo("a"),), failed=(FailedRepository("hidden", "not found"),), complete=False)
    run = Run(tmp_path, FakeSource(listing))
    assert run() == ExitCode.PARTIAL_FAILURE
    assert run.body["complete"] is False
    assert run.body["failedRepositories"] == [{"name": "hidden", "reason": "not found"}]
    assert "Cannot be scanned (1):\n  hidden: not found\n" in run.text
    assert "The list may be incomplete" in run.text


def test_every_repository_failing_still_publishes(tmp_path: Path) -> None:
    source = FakeSource(RepositoryList((_repo("a"),)), fail={"a": RepositoryError("a", "too large")})
    run = Run(tmp_path, source)
    assert run() == ExitCode.PARTIAL_FAILURE
    assert run.body == {
        "complete": True,
        "scannedRepositories": [],
        "failedRepositories": [{"name": "a", "reason": "too large"}],
        "agents": [],
    }


def test_a_host_failure_fails_the_rest_and_publishes_what_was_found(tmp_path: Path) -> None:
    source = FakeSource(
        RepositoryList((_repo("a"), _repo("b"), _repo("c"))),
        {"a": {"agent.py": AGENT}},
        fail={"b": GitSourceError("GitHub's API rate limit for this token is used up.")},
    )
    run = Run(tmp_path, source)
    assert run() == ExitCode.PARTIAL_FAILURE
    assert [r["name"] for r in run.body["failedRepositories"]] == ["b", "c"]
    assert {r["reason"] for r in run.body["failedRepositories"]} == {
        "GitHub's API rate limit for this token is used up."
    }
    assert "download:c" not in source.calls
    assert len(run.body["agents"]) == 1


def test_an_empty_repository_is_scanned_with_no_agents(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("empty", branch=None),))))
    assert run() == ExitCode.OK
    assert run.body["scannedRepositories"] == ["empty"]
    assert "download:empty" not in run.source.calls


def test_skipped_count_includes_the_archive_entries_left_out(tmp_path: Path) -> None:
    source = FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}, links={"link.py": "/etc/passwd"})
    run = Run(tmp_path, source)
    assert run() == ExitCode.OK
    [repo] = read_report(run.report_path).repositories
    assert repo.files_skipped == 1
    assert "Files: 1 scanned, 1 skipped." in run.text


def test_declining_publishes_nothing(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run(yes=False, tty=True, answer="n\n") == ExitCode.OK
    assert run.publisher.reports == []
    assert run.source.calls == ["list_all"]
    assert "Scan 1 repository and publish the results? [y/N]" in run.text
    assert EXPIRES_NOTE in run.text
    assert not run.report_path.exists()


def test_confirming_scans(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run(yes=False, tty=True, answer="Y\n") == ExitCode.OK
    assert len(run.publisher.reports) == 1


def test_no_terminal_without_yes_is_a_config_error_before_listing(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),))))
    with pytest.raises(ConfigError, match="--yes") as raised:
        run(yes=False, tty=False)
    assert raised.value.exit_code == ExitCode.CONFIG_ERROR
    assert run.source.calls == []


def test_an_unwritable_report_folder_stops_before_listing(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),))))
    run.report_path = tmp_path / "missing" / "report.json"
    with pytest.raises(ReportError, match="Cannot write the scan report"):
        run_scan(_config(), yes=True, report_path=run.report_path, make_source=lambda c, t: run.source)
    assert run.source.calls == []


@pytest.mark.parametrize(
    ("token", "warned"),
    [(None, True), (GIT_TOKEN, False)],
)
def test_anonymous_github_warning(tmp_path: Path, token: str | None, warned: bool) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),))))
    run(_config(include_private=False), token=token)
    assert (ANONYMOUS_WARNING in run.text) is warned


def test_a_publish_error_keeps_the_report(tmp_path: Path) -> None:
    error = PublishError("Agent Manager refused the scan token (HTTP 401).", ExitCode.AUTH_ERROR)
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}), error=error)
    assert run() == ExitCode.AUTH_ERROR
    assert read_report(run.report_path).scan_id == SCAN_ID
    assert "amp-shadow-runner: Agent Manager refused the scan token (HTTP 401)." in run.text


def test_a_listing_error_exits_with_its_code(tmp_path: Path) -> None:
    class Refusing(FakeSource):
        def list_all(self) -> RepositoryList:
            raise GitSourceError("GitHub refused the token.", exit_code=ExitCode.CONFIG_ERROR)

    run = Run(tmp_path, Refusing(RepositoryList(())))
    assert run() == ExitCode.CONFIG_ERROR
    assert run.publisher.reports == []
    assert "amp-shadow-runner: GitHub refused the token." in run.text


@pytest.mark.parametrize(
    ("ai_review", "message"),
    [
        (False, "1 not-sure agent is not published"),
        (True, "AI review is not available in this version of the runner, so 1 not-sure agent is not published."),
    ],
)
def test_not_sure_agents_are_not_published(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, ai_review: bool, message: str
) -> None:
    from amp_shadow_runner import report as report_module
    from amp_shadow_runner.engine.classify import Verdict

    original = report_module.RepositoryReport.scanned

    def not_sure(name: str, result: Any) -> Any:
        findings = tuple(dataclasses.replace(f, verdict=Verdict.NOT_SURE) for f in result.findings)
        return original(name, dataclasses.replace(result, findings=findings))

    monkeypatch.setattr(report_module.RepositoryReport, "scanned", staticmethod(not_sure))
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run(_config(ai_review=ai_review)) == ExitCode.OK
    assert message.lower() in run.text.lower()
    assert run.body["agents"] == []


def test_secrets_are_never_printed_or_saved(tmp_path: Path) -> None:
    source = FakeSource(
        RepositoryList((_repo("a"), _repo("b"))),
        {"a": {"agent.py": AGENT + f"key = '{GIT_TOKEN}'\n"}},
        fail={"b": RepositoryError("b", "GitHub answered HTTP 500")},
    )
    run = Run(tmp_path, source)
    run()
    saved = run.report_path.read_text()
    for secret in (GIT_TOKEN, SCAN_TOKEN):
        assert secret not in run.text
        assert secret not in saved
        assert secret not in json.dumps(run.body)


@pytest.mark.parametrize(
    ("flag", "env", "expected"),
    [
        (True, {}, True),
        (False, {}, False),
        (False, {"AMP_SCAN_YES": "1"}, True),
        (False, {"AMP_SCAN_YES": "true"}, True),
        (False, {"AMP_SCAN_YES": "0"}, False),
    ],
)
def test_assume_yes(flag: bool, env: dict[str, str], expected: bool) -> None:
    assert assume_yes(flag, env) is expected


def test_a_refused_archive_or_an_engine_bug_fails_only_that_repository(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    class Hostile(FakeSource):
        def download_tarball(self, repo: Repository, dest_dir: Path) -> Path:
            if repo.name != "evil":
                return super().download_tarball(repo, dest_dir)
            dest = dest_dir / f"{SHA}.tar.gz"
            with tarfile.open(dest, "w:gz") as tar:
                info = tarfile.TarInfo("top/../../escape.py")
                tar.addfile(info, io.BytesIO(b""))
            return dest

    import amp_shadow_runner.pipeline as pipeline
    from amp_shadow_runner.engine.scan import scan_tree as real_scan

    def scan_tree(root: Path, *, repository: str, read_ci: bool = False) -> Any:
        if repository == "buggy":
            raise RecursionError("source text that must not be shown")
        return real_scan(root, repository=repository, read_ci=read_ci)

    monkeypatch.setattr(pipeline, "scan_tree", scan_tree)
    source = Hostile(RepositoryList((_repo("evil"), _repo("buggy"), _repo("a"))), {"a": {"agent.py": AGENT}})
    run = Run(tmp_path, source)
    assert run() == ExitCode.PARTIAL_FAILURE
    assert run.body["failedRepositories"] == [
        {"name": "evil", "reason": "the archive has a member path with '..'"},
        {"name": "buggy", "reason": "the runner could not scan it (an internal error)"},
    ]
    assert run.body["scannedRepositories"] == ["a"]
    assert "must not be shown" not in run.text


def test_an_unsaved_report_is_still_published(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    import amp_shadow_runner.pipeline as pipeline

    def refuse(report: Report, path: Path) -> Path:
        raise ReportError("Cannot write the scan report to /out (No space left on device).")

    monkeypatch.setattr(pipeline, "write_report", refuse)
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run() == ExitCode.CONFIG_ERROR
    assert len(run.publisher.reports) == 1
    assert "No space left on device). The results are published anyway." in run.text
    assert "Report: not saved" in run.text
