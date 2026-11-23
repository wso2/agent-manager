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
import re
import tarfile
import time
from collections.abc import Callable, Iterable, Mapping
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import pytest

from amp_shadow_runner.config import ConfigError, ScanConfig
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.gitsource import FailedRepository, GitSourceError, Repository, RepositoryError, RepositoryList
from amp_shadow_runner.llm import LlmError, LlmSettings
from amp_shadow_runner.pipeline import (
    ANONYMOUS_HINT,
    ANONYMOUS_WARNING,
    DECLINED_NOTE,
    EXPORT_IGNORE_HINT,
    JUDGE_NOT_AVAILABLE,
    TOKEN_EXPIRED_AT_START,
    UPLOAD_QUESTION,
    PublishInterrupted,
    UploadCancelled,
    assume_yes,
    run_scan,
)
from amp_shadow_runner.publish import PublishError, PublishResult, build_body
from amp_shadow_runner.report import EXPORT_IGNORE_NOTE, Report, ReportError, read_report
from amp_shadow_runner.results import RESULTS_FORMAT, parse_results, results_path
from amp_shadow_runner.terminal import Ui
from tests.test_config import _token

SCAN_ID = "3f2b6c1e-8d4a-4f5b-9c7e-1a2b3c4d5e6f"
SCAN_TOKEN = _token(time.time() + 3600)
GIT_TOKEN = "github_pat_s3cr3t-git-token"
LLM_KEY = "sk-s3cr3t-llm-key"
LLM = LlmSettings("openai", "https://api.openai.com/v1", "gpt-x", None, LLM_KEY)
SHA = "0123456789abcdef0123456789abcdef01234567"
AGENT = "from crewai import Agent\nsupport = Agent(role='support')\n"
MAYBE = "from crewai import Agent\n"  # No agent: a file that is scanned and matches nothing.
BAL_AGENT = "import ballerina/ai;\n// CODE-MARKER-THAT-MUST-STAY-LOCAL\nfinal ai:Agent {name} = check new (model);\n"
"""A Ballerina agent: always Not sure, so AI review decides."""
YES = '{"agent": true, "reason": "Creates an agent with tools."}'
NO = '{"agent": false, "reason": "Only a chat completion."}'


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

    def download_tarball(self, repo: Repository, dest_dir: Path, *, on_progress: Any = None) -> Path:
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
    """Records each upload and checks the report and the results file were already written, with this body."""

    def __init__(self, report_path: Path, *, error: BaseException | None = None) -> None:
        self.report_path = report_path
        self.error = error
        self.bodies: list[dict[str, Any]] = []
        self.files_on_disk: list[bool] = []
        self.status = "partial"

    def publish_body(self, scan_id: str, body: Mapping[str, Any]) -> PublishResult:
        assert scan_id == SCAN_ID
        self.bodies.append(dict(body))
        results = results_path(self.report_path, scan_id)
        self.files_on_disk.append(
            self.report_path.exists()
            and results.exists()
            and build_body(read_report(self.report_path)) == body
            and parse_results(json.loads(results.read_text()), results).body == body
        )
        if self.error is not None:
            raise self.error
        return PublishResult(self.status, len(body["agents"]))


class FakeLlm:
    """Answers each AI review with the answer for the file named in the request (default ``default``)."""

    def __init__(self, answers: dict[str, str | Exception] | None = None, default: str | Exception = YES) -> None:
        self.answers = answers or {}
        self.default = default
        self.users: list[str] = []

    def complete(self, system: str, user: str, *, model: str, max_tokens: int) -> str:
        self.users.append(user)
        answer = next((a for path, a in self.answers.items() if f"File: {path}\n" in user), self.default)
        if isinstance(answer, Exception):
            raise answer
        return answer


class Log(io.StringIO):
    """stderr off a terminal, in UTF-8 (as in CI)."""

    encoding = "utf-8"


class Run:
    def __init__(
        self, tmp_path: Path, source: FakeSource, *, error: BaseException | None = None, llm: FakeLlm | None = None
    ) -> None:
        self.report_path = tmp_path / "report.json"
        self.source = source
        self.llm = FakeLlm() if llm is None else llm
        self.publisher = FakePublisher(self.report_path, error=error)
        self.out: io.StringIO = Log()
        self.asked: list[tuple[str, list[str]]] = []
        """Each prompt asked, with the Git host calls made before it."""

    def __call__(
        self,
        config: ScanConfig | None = None,
        *,
        yes: bool = True,
        tty: bool = False,
        answer: str = "",
        token: str | None = GIT_TOKEN,
        clock: Callable[[], float] = time.time,
        answer_error: BaseException | None = None,
        answers: list[str] | None = None,
        ttys: list[bool] | None = None,
    ) -> int:
        """``answers`` are the answers to each question in turn (default ``answer`` to every one); ``ttys``
        what each terminal check finds in turn (default ``tty``)."""

        def read_answer() -> str:
            if answer_error is not None:
                raise answer_error
            return answers.pop(0) if answers else answer

        def is_tty() -> bool:
            return ttys.pop(0) if ttys else tty

        def ask_token(_config: ScanConfig, _ui: Ui) -> str | None:
            self.asked.append(("ask_token", list(self.source.calls)))
            return token

        def ask_llm(_config: ScanConfig, _ui: Ui) -> LlmSettings:
            self.asked.append(("ask_llm", list(self.source.calls)))
            return LLM

        return run_scan(
            config or _config(),
            yes=yes,
            report_path=self.report_path,
            ask_token=ask_token,
            ask_llm=ask_llm,
            make_llm=lambda _settings: self.llm,
            make_source=lambda _config, _token: self.source,
            make_publisher=lambda _config: self.publisher,
            is_tty=is_tty,
            read_answer=read_answer,
            clock=clock,
            out=self.out,
        )

    @property
    def body(self) -> dict[str, Any]:
        [body] = self.publisher.bodies
        return body

    @property
    def results_path(self) -> Path:
        return results_path(self.report_path, SCAN_ID)

    @property
    def results(self) -> dict[str, Any]:
        data: dict[str, Any] = json.loads(self.results_path.read_text())
        return data

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
    assert run.publisher.files_on_disk == [True]  # Written before publishing, and published once.
    body = run.body
    assert body["scannedRepositories"] == ["support-bot"]
    assert body["failedRepositories"] == [{"name": "legacy-app", "reason": "GitHub answered HTTP 502"}]
    assert [(a["name"], a["attributes"]["reviewedBy"]) for a in body["agents"]] == [("support", "rules")]
    assert "  ✗ legacy-app    GitHub answered HTTP 502\n" in run.text
    assert run.text.endswith(f"\n! Uploaded to Agent Manager: 1 agent saved (scan partial).\n  {EXPORT_IGNORE_HINT}\n")
    assert EXPORT_IGNORE_NOTE in run.report_path.read_text()  # The report keeps the long form.
    assert f"  Report     {run.report_path}\n  Results    {run.results_path}\n" in run.text


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
    assert "  ! old-repo   not found, counted as scanned with no agents\n" in run.text


def test_listing_failures_and_an_incomplete_list_are_partial(tmp_path: Path) -> None:
    listing = RepositoryList((_repo("a"),), failed=(FailedRepository("hidden", "not found"),), complete=False)
    run = Run(tmp_path, FakeSource(listing))
    assert run() == ExitCode.PARTIAL_FAILURE
    assert run.body["complete"] is False
    assert run.body["failedRepositories"] == [{"name": "hidden", "reason": "not found"}]
    assert "  ✗ hidden   cannot be scanned: not found\n" in run.text
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
    assert "  Files      1 scanned, 1 skipped\n" in run.text


def test_declining_publishes_nothing(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run(yes=False, tty=True, answer="n\n") == ExitCode.OK
    assert run.publisher.bodies == []
    assert run.source.calls == ["list_all"]
    assert "\n? Scan 1 repository? (y/N) \n" in run.text  # Ended for the log.
    assert DECLINED_NOTE in run.text
    assert not run.report_path.exists()
    assert not run.results_path.exists()


def test_confirming_scans(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run(yes=False, tty=True, answer="Y\n") == ExitCode.OK
    assert len(run.publisher.bodies) == 1


def test_noterminal_without_yes_is_a_config_error_before_listing(tmp_path: Path) -> None:
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
    assert (f"  ! {ANONYMOUS_WARNING}\n    {ANONYMOUS_HINT}\n" in run.text) is warned


def test_an_upload_error_keeps_the_files_and_points_to_the_console(tmp_path: Path) -> None:
    error = PublishError(
        "Agent Manager refused the scan token (HTTP 401).", ExitCode.AUTH_ERROR, console_can_upload=True
    )
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}), error=error)
    assert run() == ExitCode.AUTH_ERROR
    assert read_report(run.report_path).scan_id == SCAN_ID
    assert run.results["results"] == run.body
    assert run.text.endswith(
        "\n✗ Not uploaded: Agent Manager refused the scan token (HTTP 401).\n"
        f"  Upload {run.results_path}\n"
        "  in Agent Manager: Shadow Agent Discovery, Scans, Upload results.\n"
        f"  {EXPORT_IGNORE_HINT}\n"
    )


def test_an_upload_error_the_console_cannot_help_with_names_no_next_step(tmp_path: Path) -> None:
    error = PublishError("The scan is no longer waiting for results (HTTP 409).")
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}), error=error)
    assert run() == ExitCode.RUNTIME_ERROR
    assert run.text.endswith(
        f"\n✗ Not uploaded: The scan is no longer waiting for results (HTTP 409).\n  {EXPORT_IGNORE_HINT}\n"
    )


def test_a_listing_error_exits_with_its_code(tmp_path: Path) -> None:
    class Refusing(FakeSource):
        def list_all(self) -> RepositoryList:
            raise GitSourceError("GitHub refused the token.", exit_code=ExitCode.CONFIG_ERROR)

    run = Run(tmp_path, Refusing(RepositoryList(())))
    assert run() == ExitCode.CONFIG_ERROR
    assert run.publisher.bodies == []
    assert run.text.endswith("\n\n✗ GitHub refused the token.\n")


def test_not_sure_agents_are_not_published_without_ai_review(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"main.bal": BAL_AGENT.format(name="x")}}))
    assert run(_config()) == ExitCode.OK
    assert "  Agents     1 not sure (published only with --ai-review)\n" in run.text
    assert run.body["agents"] == []
    assert run.llm.users == []
    assert "aiReview" not in run.report_path.read_text()


def _two_repos(tmp_path: Path, llm: FakeLlm) -> Run:
    files = {
        "a": {"main.bal": BAL_AGENT.format(name="helper"), "other.bal": BAL_AGENT.format(name="chat")},
        "b": {"agent.py": AGENT, "tests/test_x.bal": BAL_AGENT.format(name="double")},
    }
    return Run(tmp_path, FakeSource(RepositoryList((_repo("a"), _repo("b"))), files), llm=llm)


def test_ai_review_publishes_only_the_confirmed_agents(tmp_path: Path) -> None:
    run = _two_repos(tmp_path, FakeLlm({"other.bal": NO}))
    assert run(_config(ai_review=True)) == ExitCode.OK
    reviewed = {a["name"]: a["attributes"]["reviewedBy"] for a in run.body["agents"]}
    assert reviewed == {"helper": "ai_review", "support": "rules"}  # Not "chat" (rejected) or "double" (test code).
    assert len(run.llm.users) == 2
    assert all("tests/test_x.bal" not in u for u in run.llm.users)
    assert "  [1/2] a  scanning\n  [1/2] a  AI check of 2 not-sure agents\n  ✓ a   2 agents · 2 files\n" in run.text
    assert "  Agents     3: 1 proven, 1 confirmed by AI, 1 not confirmed\n" in run.text
    assert "  AI         1 rejected (not published; the report says why)\n" in run.text
    assert all("aiReview" not in a["attributes"] for a in run.body["agents"])


def test_the_report_records_the_review_without_the_code_and_upload_publishes_it_again(tmp_path: Path) -> None:
    run = _two_repos(tmp_path, FakeLlm({"other.bal": NO}))
    run(_config(ai_review=True))
    saved = run.report_path.read_text()
    assert "CODE-MARKER-THAT-MUST-STAY-LOCAL" in "".join(run.llm.users)
    assert "CODE-MARKER-THAT-MUST-STAY-LOCAL" not in saved
    assert "CODE-MARKER-THAT-MUST-STAY-LOCAL" not in run.text
    data = json.loads(saved)
    assert data["summary"]["agents"]["aiReview"] == {"confirmed": 1, "rejected": 1, "failed": 0, "notReviewed": 0}
    findings = {f["path"]: f for r in data["repositories"] for f in r["findings"]}
    assert findings["main.bal"]["attributes"]["aiReview"] == {
        "outcome": "confirmed",
        "reason": "Creates an agent with tools.",
    }
    assert findings["other.bal"]["attributes"]["aiReview"]["outcome"] == "rejected"
    assert "aiReview" not in findings["tests/test_x.bal"]["attributes"]
    assert build_body(read_report(run.report_path)) == run.body  # ``upload`` publishes the same agents.


def test_a_refused_llm_key_stops_review_and_still_publishes(tmp_path: Path) -> None:
    error = LlmError("The LLM at api.openai.com refused the API key (HTTP 401).", status=401)
    run = _two_repos(tmp_path, FakeLlm(default=error))
    assert run(_config(ai_review=True)) == ExitCode.OK
    assert len(run.llm.users) == 1
    assert run.text.count("AI-enhanced detection stopped") == 1
    assert "  Agents     3: 1 proven, 2 not confirmed\n" in run.text
    assert "  AI         1 failed, 1 not reviewed (not published; the report says why)\n" in run.text
    assert [a["attributes"]["reviewedBy"] for a in run.body["agents"]] == ["rules"]


def test_reviews_do_not_depend_on_the_scan_token(tmp_path: Path) -> None:
    run = _two_repos(tmp_path, FakeLlm())
    assert run(_config(ai_review=True, amp_url=None, token=None, token_expires_at=None)) == ExitCode.OK
    assert len(run.llm.users) == 2
    assert {a["attributes"]["reviewedBy"] for a in run.results["results"]["agents"]} == {"ai_review", "rules"}


def test_ai_review_with_nothing_to_review(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run(_config(ai_review=True)) == ExitCode.OK
    assert "  Agents     1 proven\n  AI         nothing to review\n" in run.text


def test_the_judge_is_not_available_yet(tmp_path: Path) -> None:
    run = _two_repos(tmp_path, FakeLlm())
    run(_config(ai_review=True, judge=True))
    assert run.text.count(JUDGE_NOT_AVAILABLE) == 1
    assert {a["attributes"]["reviewedBy"] for a in run.body["agents"]} == {"ai_review", "rules"}
    run = _two_repos(tmp_path, FakeLlm())
    run(_config(ai_review=True))
    assert JUDGE_NOT_AVAILABLE not in run.text


def test_not_sure_agents_in_test_code_are_not_counted_as_unpublished(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    from amp_shadow_runner import report as report_module
    from amp_shadow_runner.engine.classify import Verdict

    original = report_module.RepositoryReport.scanned

    def not_sure_in_tests(name: str, result: Any) -> Any:
        findings = tuple(dataclasses.replace(f, verdict=Verdict.NOT_SURE, tag="test") for f in result.findings)
        return original(name, dataclasses.replace(result, findings=findings))

    monkeypatch.setattr(report_module.RepositoryReport, "scanned", staticmethod(not_sure_in_tests))
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run(_config()) == ExitCode.OK
    assert "not-sure agent" not in run.text.lower()
    assert "  Agents     none found\n  Test code  1 more, not published\n" in run.text


def test_secrets_are_never_printed_or_saved(tmp_path: Path) -> None:
    source = FakeSource(
        RepositoryList((_repo("a"), _repo("b"))),
        {"a": {"agent.py": AGENT + f"key = '{GIT_TOKEN}'\n"}},
        fail={"b": RepositoryError("b", "GitHub answered HTTP 500")},
    )
    run = Run(tmp_path, source)
    run(_config(ai_review=True, judge=True))
    assert [name for name, _ in run.asked] == ["ask_token", "ask_llm"]
    saved = run.report_path.read_text() + run.results_path.read_text()
    for secret in (GIT_TOKEN, SCAN_TOKEN, LLM_KEY):
        assert secret not in run.text
        assert secret not in saved
        assert secret not in json.dumps(run.body)


@pytest.mark.parametrize(
    ("flag", "env", "expected"),
    [
        (True, {}, True),
        (False, {}, False),
        (False, {"AMP_SHADOW_YES": "1"}, True),
        (False, {"AMP_SHADOW_YES": "yes"}, True),
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
        def download_tarball(self, repo: Repository, dest_dir: Path, *, on_progress: Any = None) -> Path:
            if repo.name != "evil":
                return super().download_tarball(repo, dest_dir)
            dest = dest_dir / f"{SHA}.tar.gz"
            with tarfile.open(dest, "w:gz") as tar:
                info = tarfile.TarInfo("top/../../escape.py")
                tar.addfile(info, io.BytesIO(b""))
            return dest

    import amp_shadow_runner.pipeline as pipeline
    from amp_shadow_runner.engine.scan import scan_tree as real_scan

    def scan_tree(root: Path, *, repository: str, read_ci: bool = False, progress: Any = None) -> Any:
        if repository == "buggy":
            raise RecursionError("source text that must not be shown")
        return real_scan(root, repository=repository, read_ci=read_ci, progress=progress)

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


def test_an_unsaved_report_still_saves_and_uploads_the_results(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    import amp_shadow_runner.pipeline as pipeline

    def refuse(report: Report, path: Path) -> Path:
        raise ReportError("Cannot write the scan report to /out (No space left on device).")

    monkeypatch.setattr(pipeline, "write_report", refuse)
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run() == ExitCode.CONFIG_ERROR
    assert run.results["results"]["scannedRepositories"] == ["a"]
    assert "  Report     not saved\n" in run.text
    assert "  ! Cannot write the scan report to /out (No space left on device).\n" in run.text
    assert "Uploaded to Agent Manager" in run.text


T0 = 1_800_000_000.0


class SlowSource(FakeSource):
    """Each download takes 500 seconds on ``now``."""

    def __init__(self, *args: Any, now: list[float], **kwargs: Any) -> None:
        super().__init__(*args, **kwargs)
        self.now = now

    def download_tarball(self, repo: Repository, dest_dir: Path, *, on_progress: Any = None) -> Path:
        self.now[0] += 500
        return super().download_tarball(repo, dest_dir)


def test_a_scan_that_outlasts_the_token_scans_everything_and_points_to_the_console(tmp_path: Path) -> None:
    now = [T0]
    repos = RepositoryList((_repo("a"), _repo("b"), _repo("c"), _repo("d")))
    run = Run(tmp_path, SlowSource(repos, {"a": {"agent.py": AGENT}}, now=now))
    config = _config(token_expires_at=datetime.fromtimestamp(T0 + 1000, UTC))
    assert run(config, clock=lambda: now[0]) == ExitCode.OK  # The console can take the file: not an error.
    assert run.publisher.bodies == []
    assert run.results["results"]["scannedRepositories"] == ["a", "b", "c", "d"]
    assert run.results["summary"] == {
        "organization": "acme",
        "repositoriesScanned": 4,
        "repositoriesFailed": 0,
        "agents": 1,
    }
    assert run.text.endswith(
        "\n! Not uploaded: the scan token has expired.\n"
        f"  Upload {run.results_path}\n"
        "  in Agent Manager: Shadow Agent Discovery, Scans, Upload results.\n"
        f"  {EXPORT_IGNORE_HINT}\n"
    )


def test_a_token_expired_at_the_start_warns_and_still_scans(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    config = _config(token_expires_at=datetime.fromtimestamp(T0 - 1, UTC))
    assert run(config, yes=False, tty=True, answer="y\n", clock=lambda: T0) == ExitCode.OK
    assert f"  ! {TOKEN_EXPIRED_AT_START}\n" in run.text
    assert UPLOAD_QUESTION not in run.text  # Not asked: it could only fail.
    assert run.publisher.bodies == []
    assert run.results["results"]["scannedRepositories"] == ["a"]


def test_a_partial_scan_that_outlasts_the_token_keeps_its_own_code(tmp_path: Path) -> None:
    now = [T0]
    repos = RepositoryList((_repo("a"), _repo("b")), failed=(FailedRepository("c", "private"),))
    run = Run(tmp_path, SlowSource(repos, {"a": {"agent.py": AGENT}}, now=now))
    config = _config(token_expires_at=datetime.fromtimestamp(T0 + 600, UTC))
    assert run(config, clock=lambda: now[0]) == ExitCode.PARTIAL_FAILURE
    assert run.publisher.bodies == []
    assert "! Not uploaded: the scan token has expired.\n" in run.text


def test_ctrl_c_right_after_the_files_are_saved_says_nothing_was_uploaded(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    import amp_shadow_runner.pipeline as pipeline

    real = pipeline._save_results

    def save_then_interrupt(*args: Any, **kwargs: Any) -> Any:
        real(*args, **kwargs)
        raise KeyboardInterrupt  # After the file is written, before _save_results returns.

    monkeypatch.setattr(pipeline, "_save_results", save_then_interrupt)
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    with pytest.raises(UploadCancelled) as raised:
        run()
    assert raised.value.results_file == str(run.results_path)
    assert run.results_path.exists()
    assert run.publisher.bodies == []


def test_ctrl_c_while_saving_the_report_is_not_called_saved(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    import amp_shadow_runner.pipeline as pipeline

    def interrupt(report: Report, path: Path) -> Path:
        raise KeyboardInterrupt

    monkeypatch.setattr(pipeline, "write_report", interrupt)
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),))))
    with pytest.raises(KeyboardInterrupt) as raised:
        run()
    assert not isinstance(raised.value, PublishInterrupted | UploadCancelled)
    assert not run.results_path.exists()


def test_ctrl_c_at_the_prompt_cancels_instead_of_declining(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),))))
    with pytest.raises(KeyboardInterrupt) as raised:
        run(yes=False, tty=True, answer_error=KeyboardInterrupt())
    assert not isinstance(raised.value, PublishInterrupted | UploadCancelled)
    assert DECLINED_NOTE not in run.text
    assert run.publisher.bodies == []


def test_ctrl_c_while_publishing_is_told_apart(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),))), error=KeyboardInterrupt())
    with pytest.raises(PublishInterrupted) as raised:
        run()
    assert raised.value.results_file == str(run.results_path)


def test_ctrl_c_at_the_upload_question_says_the_files_are_saved(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),))))
    answers = ["y\n"]

    def reader() -> str:
        if answers:
            return answers.pop(0)
        raise KeyboardInterrupt

    with pytest.raises(UploadCancelled) as raised:
        run_scan(
            _config(),
            report_path=run.report_path,
            ask_token=lambda c, u: None,
            make_source=lambda c, t: run.source,
            make_publisher=lambda c: run.publisher,
            is_tty=lambda: True,
            read_answer=reader,
            out=run.out,
        )
    assert raised.value.results_file == str(run.results_path)
    assert run.results_path.exists()
    assert run.publisher.bodies == []


@pytest.mark.parametrize("ai_review", [True, False])
def test_llm_settings_are_asked_after_the_token_and_before_listing(tmp_path: Path, ai_review: bool) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run(_config(ai_review=ai_review)) == ExitCode.OK
    expected: list[tuple[str, list[str]]] = [("ask_token", []), ("ask_llm", [])] if ai_review else [("ask_token", [])]
    assert run.asked == expected
    assert run.source.calls[0] == "list_all"


def test_noterminal_is_found_only_after_the_llm_settings_are_asked(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),))))
    with pytest.raises(ConfigError, match="no terminal"):
        run(_config(ai_review=True), yes=False, tty=False)
    assert run.asked == [("ask_token", []), ("ask_llm", [])]
    assert run.source.calls == []


class _Terminal(io.StringIO):
    encoding = "utf-8"

    def isatty(self) -> bool:
        return True


def _clean_lines(text: str) -> bool:
    """Every permanent line starts on a clean line: a live line drawn before it was erased first."""
    return all("\r" not in line or line.rsplit("\r", 1)[1].startswith("\x1b[K") for line in text.split("\n"))


def test_on_aterminal_every_message_clears_the_live_line_first(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("TERM", "xterm")
    monkeypatch.setenv("NO_COLOR", "1")
    error = LlmError("The LLM at api.openai.com refused the API key (HTTP 401).", status=401)
    run = _two_repos(tmp_path, FakeLlm(default=error))
    run.out = terminal = _Terminal()
    assert run(_config(ai_review=True)) == ExitCode.OK
    text = terminal.getvalue()
    assert "\r  [1/2] a  scanning" in text and "AI-enhanced detection" in text and "Uploading to Agent Manager…" in text
    assert "\r\x1b[K  ! AI-enhanced detection stopped: " in text  # Printed over the live line, not into it.
    assert re.search(r"\n  ✓ a   2 agents · 2 files · [0-9.]+s\n", text)
    assert "  Scanned    2 repositories in " in text
    assert "AI check of" not in text
    assert _clean_lines(text)
    assert text.endswith("\n")


def test_ctrl_c_on_aterminal_leaves_a_clean_line(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("TERM", "xterm")
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),))), error=KeyboardInterrupt())
    run.out = terminal = _Terminal()
    with pytest.raises(PublishInterrupted):
        run()
    assert terminal.getvalue().endswith("Uploading to Agent Manager…\x1b[K\r\x1b[K")


def test_published_agents_carry_branch_project_path_and_language(tmp_path: Path) -> None:
    files = {
        "a": {"agent.py": AGENT},
        "b": {"services/support/pyproject.toml": '[project]\nname = "support"\n', "services/support/bot.py": AGENT},
    }
    listing = RepositoryList((_repo("a"), _repo("b", branch="release/2.0")))
    run = Run(tmp_path, FakeSource(listing, files))
    assert run(_config()) == ExitCode.OK
    attributes = {a["repository"]: a["attributes"] for a in run.body["agents"]}
    assert attributes == {
        "a": {"framework": "crewai", "language": "python", "branch": "main", "reviewedBy": "rules"},
        "b": {
            "framework": "crewai",
            "language": "python",
            "projectPath": "/services/support",
            "branch": "release/2.0",
            "reviewedBy": "rules",
        },
    }
    saved = json.loads(run.report_path.read_text())
    assert [f["attributes"].get("branch") for r in saved["repositories"] for f in r["findings"]] == [
        "main",
        "release/2.0",
    ]
    assert build_body(read_report(run.report_path)) == run.body  # ``upload`` publishes the same attributes.


RESULTS_NAME = f"results-{SCAN_ID}.json"
OUTPUT_HEAD = (
    "AMP shadow agent scan · acme on github.com\n"
    "\n"
    "◆ Repositories (2)\n"
    "  agent-manager\n"
    "  docs            private\n"
    f"  ! {ANONYMOUS_WARNING}\n"
    f"    {ANONYMOUS_HINT}\n"
    "\n"
    "? Scan 2 repositories? (y/N) \n"
    "\n"
    "◆ Scanning\n"
    "  [1/2] agent-manager  downloading\n"
    "  [1/2] agent-manager  scanning\n"
    "  ✓ agent-manager   1 agent · 1 in test code · 2 files\n"
    "  [2/2] docs  downloading\n"
    "  [2/2] docs  scanning\n"
    "  ✓ docs            no agents · 1 file\n"
    "\n"
    "◆ Results\n"
    "  Scanned    2 repositories\n"
    "  Agents     1 proven\n"
    "  Test code  1 more, not published\n"
    "  Files      3 scanned\n"
    "\n"
    "◆ Done\n"
    "  Report     report.json in this folder\n"
    f"  Results    {RESULTS_NAME} in this folder\n"
)


def _whole_run(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Run:
    """Two repositories, with the report in the command's folder (``/out``)."""
    monkeypatch.setattr("amp_shadow_runner.pipeline.DEFAULT_REPORT_PATH", tmp_path / "report.json")
    files = {"agent-manager": {"agent.py": AGENT, "tests/test_a.py": AGENT}, "docs": {"x.py": MAYBE}}
    listing = RepositoryList((_repo("agent-manager"), dataclasses.replace(_repo("docs"), private=True)))
    run = Run(tmp_path, FakeSource(listing, files))
    run.publisher.status = "succeeded"
    return run


def test_the_whole_output_without_a_scan_token(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    run = _whole_run(tmp_path, monkeypatch)
    config = _config(include_private=False, amp_url=None, token=None, token_expires_at=None)
    assert run(config, token=None, yes=False, tty=True, answer="y\n") == ExitCode.OK
    assert run.publisher.bodies == []  # No network: nothing to upload with.
    assert run.text == OUTPUT_HEAD + (
        f"  Next       Upload {RESULTS_NAME}\n"
        "             in Agent Manager: Shadow Agent Discovery, Scans, Upload results.\n"
        f"  {EXPORT_IGNORE_HINT}\n"
    )
    assert read_report(run.report_path).scan_id == SCAN_ID
    assert run.results["format"] == RESULTS_FORMAT
    assert run.results["results"] == build_body(read_report(run.report_path))


def test_the_whole_output_asking_to_upload(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    run = _whole_run(tmp_path, monkeypatch)
    assert run(_config(include_private=False), token=None, yes=False, tty=True, answers=["y\n", "\n"]) == ExitCode.OK
    assert run.publisher.files_on_disk == [True]  # Enter takes the default, yes.
    assert run.text == OUTPUT_HEAD + (
        f"\n? {UPLOAD_QUESTION} (Y/n) \n\n✓ Uploaded to Agent Manager: 1 agent saved.\n  {EXPORT_IGNORE_HINT}\n"
    )


@pytest.mark.parametrize("answer", ["n\n", "no\n", ""])  # "" is Ctrl-D: the end of input declines.
def test_declining_the_upload_keeps_the_files(tmp_path: Path, monkeypatch: pytest.MonkeyPatch, answer: str) -> None:
    run = _whole_run(tmp_path, monkeypatch)
    assert run(_config(include_private=False), token=None, yes=False, tty=True, answers=["y\n", answer]) == ExitCode.OK
    assert run.publisher.bodies == []
    assert run.text.endswith(
        f"? {UPLOAD_QUESTION} (Y/n) \n"
        "\n"
        "! Not uploaded.\n"
        f"  Upload {RESULTS_NAME}\n"
        "  in Agent Manager: Shadow Agent Discovery, Scans, Upload results.\n"
        f"  {EXPORT_IGNORE_HINT}\n"
    )
    assert run.results_path.exists() and run.report_path.exists()


def test_yes_uploads_without_asking(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    run = _whole_run(tmp_path, monkeypatch)
    assert run(_config(include_private=False), token=None, yes=True, tty=False) == ExitCode.OK
    assert run.publisher.files_on_disk == [True]
    assert "?" not in run.text.replace(f"{ANONYMOUS_WARNING}", "")
    assert run.text.endswith(
        f"  Results    {RESULTS_NAME} in this folder\n\n✓ Uploaded to Agent Manager: 1 agent saved.\n"
        f"  {EXPORT_IGNORE_HINT}\n"
    )


def test_no_terminal_at_the_upload_question_does_not_upload(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    run = _whole_run(tmp_path, monkeypatch)
    # A terminal to confirm the repositories in, gone by the time the upload is asked.
    assert run(_config(include_private=False), token=None, yes=False, ttys=[True, False], answer="y\n") == ExitCode.OK
    assert run.publisher.bodies == []
    assert UPLOAD_QUESTION not in run.text
    assert run.text.endswith(
        "! Not uploaded: there is no terminal to ask in (add --yes or -e AMP_SHADOW_YES=1 to upload without asking).\n"
        f"  Upload {RESULTS_NAME}\n"
        "  in Agent Manager: Shadow Agent Discovery, Scans, Upload results.\n"
        f"  {EXPORT_IGNORE_HINT}\n"
    )


def test_the_results_file_is_saved_when_no_agent_was_found(tmp_path: Path) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"x.py": MAYBE}}))
    assert run(_config(amp_url=None, token=None, token_expires_at=None)) == ExitCode.OK
    data = run.results
    assert set(data) == {"format", "version", "scanId", "createdAt", "summary", "results"}
    assert (data["format"], data["version"], data["scanId"]) == (RESULTS_FORMAT, 1, SCAN_ID)
    assert re.fullmatch(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ", data["createdAt"])
    assert data["summary"] == {"organization": "acme", "repositoriesScanned": 1, "repositoriesFailed": 0, "agents": 0}
    assert data["results"] == {"complete": True, "scannedRepositories": ["a"], "failedRepositories": [], "agents": []}
    assert [p.name for p in tmp_path.iterdir() if p.name.startswith(".")] == []  # No temporary file left.


def test_results_over_the_limits_are_not_saved_but_the_report_is(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr("amp_shadow_runner.publish.MAX_AGENTS", 0)
    run = Run(tmp_path, FakeSource(RepositoryList((_repo("a"),)), {"a": {"agent.py": AGENT}}))
    assert run() == ExitCode.RUNTIME_ERROR
    assert run.publisher.bodies == []
    assert read_report(run.report_path).scan_id == SCAN_ID
    assert not run.results_path.exists()
    assert "  Results    not saved\n" in run.text
    assert "  ! The scan found 1 agents to publish, and Agent Manager takes at most 0" in run.text
    assert UPLOAD_QUESTION not in run.text and "Uploaded" not in run.text


@pytest.mark.parametrize(
    ("api_url", "host"),
    [
        ("https://api.github.com", "github.com"),
        ("https://api.acme.ghe.com", "acme.ghe.com"),
        ("http://ghe.internal:8080/api/v3", "ghe.internal:8080"),
    ],
)
def test_the_header_names_the_organization_and_the_host(tmp_path: Path, api_url: str, host: str) -> None:
    run = Run(tmp_path, FakeSource(RepositoryList(())))
    run(_config(api_url=api_url, organization="wso2"))
    assert run.text.startswith(f"AMP shadow agent scan · wso2 on {host}\n")


def test_a_long_list_shows_the_first_twenty(tmp_path: Path) -> None:
    listing = RepositoryList(tuple(_repo(f"r{i:02d}") for i in range(25)), gone=tuple(f"g{i:02d}" for i in range(22)))
    run = Run(tmp_path, FakeSource(listing))
    run()
    text = run.text.split("◆ Scanning")[0]
    assert "◆ Repositories (25)\n" in text
    assert "  r19\n" in text and "  r20\n" not in text and "  … and 5 more\n" in text
    assert "  ! g19 " in text and "  ! g20 " not in text and "    … and 2 more\n" in text


def test_the_results_name_the_default_folder_as_this_folder() -> None:
    from amp_shadow_runner.pipeline import shown_path
    from amp_shadow_runner.report import DEFAULT_REPORT_PATH

    assert shown_path(DEFAULT_REPORT_PATH) == "report.json"
    assert shown_path(results_path(DEFAULT_REPORT_PATH, SCAN_ID.upper())) == f"results-{SCAN_ID}.json"
    assert shown_path(Path("/elsewhere/report.json")) == "/elsewhere/report.json"
