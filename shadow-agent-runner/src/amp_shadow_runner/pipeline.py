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

"""Run a whole scan: list the repositories, confirm, fetch and scan each one, write the report, publish once.

1. ``build_scan_config`` has already checked the flags, ``AMP_URL`` and the token's lifetime. The report
   folder is checked next, before anything long.
2. The Git token is asked for only when needed (``prompts.git_token``), then, with ``--ai-review``, the LLM
   settings (``prompts.llm_settings``).
3. The repositories are listed or looked up, printed, and confirmed. Without a terminal the scan goes on
   only with ``--yes`` or ``AMP_SCAN_YES=1``, and otherwise exits CONFIG_ERROR before any listing. Declining
   publishes nothing: the pending scan expires on its own after 1 hour.
4. Each repository is downloaded and extracted into the always-deleted workspace, scanned, and deleted
   before the next one. A failure in one repository marks it failed with a short reason and the scan goes
   on. A failure that stops the Git host for every repository (a refused token, a used-up rate limit) marks
   it and every repository after it failed, so the agents found so far are still published.
5. The report is written before publishing, so ``upload`` can publish it again if publishing fails. If it
   cannot be written, the results are still published and the command exits with the report's error code.
6. The results are published once.

The whole scan must finish within the token's hour. A repository is not started with less than
TOKEN_MARGIN_SECONDS of token lifetime left: it and the rest are failed (TOKEN_RAN_OUT), and what was
finished is published. Too little time left right after confirming stops the scan before it starts.

Exit codes: OK when every repository was scanned, the list was complete and the publish succeeded;
PARTIAL_FAILURE when it published but some repositories failed or the list was incomplete, including when
every repository failed (Agent Manager then marks the scan failed); a publish error's own code otherwise.

Not-sure findings are published only after AI review confirms them. This version asks for the LLM settings
but does not review yet, so with or without ``--ai-review`` they stay in the report and are not published.
"""

import os
import sys
import time
from collections.abc import Callable, Iterable, Mapping
from dataclasses import replace
from datetime import UTC, datetime
from pathlib import Path
from typing import Protocol, TextIO

from amp_shadow_runner.config import ConfigError, ScanConfig
from amp_shadow_runner.engine.classify import Verdict
from amp_shadow_runner.engine.scan import ScanResult, scan_tree
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.fetch import ArchiveError, extract, remove_tree, workspace
from amp_shadow_runner.gitsource import GitSourceError, Repository, RepositoryError, RepositoryList
from amp_shadow_runner.gitsource.github import GitHubSource
from amp_shadow_runner.llm import LlmSettings
from amp_shadow_runner.progress import Progress
from amp_shadow_runner.prompts import git_token, llm_settings
from amp_shadow_runner.publish import (
    Publisher,
    PublishError,
    PublishResult,
    published_line,
    report_exit_code,
    reviewed_by,
)
from amp_shadow_runner.report import (
    DEFAULT_REPORT_PATH,
    EXPORT_IGNORE_NOTE,
    Report,
    ReportError,
    RepositoryReport,
    RepositoryStatus,
    check_writable,
    write_report,
)
from amp_shadow_runner.scantoken import ScanTokenError

YES_ENV = "AMP_SCAN_YES"
ANONYMOUS_WARNING = (
    "Without a GitHub token, GitHub allows about 25 repositories an hour (2 calls each). "
    "Pass a token with -e GITHUB_TOKEN for a higher limit."
)
EXPIRES_NOTE = "Nothing was scanned or published. The pending scan in Agent Manager expires on its own after 1 hour."
TOKEN_MARGIN_SECONDS = 180
"""No repository is started with less than this much token lifetime left, so the results can still be published."""
TOKEN_RAN_OUT = (
    "not scanned: the scan token's hour ran out; scan the rest with a new command (for example with --repos)"
)
_EMPTY = ScanResult((), 0, 0, 0)


class PublishInterrupted(KeyboardInterrupt):
    """Ctrl-C after publishing started: the results may already be in Agent Manager."""


class GitSource(Protocol):
    """What the pipeline needs from a Git host (``GitHubSource``)."""

    def list_all(self) -> RepositoryList: ...

    def list_named(self, names: Iterable[str]) -> RepositoryList: ...

    def resolve_commit(self, repo: Repository) -> Repository: ...

    def download_tarball(self, repo: Repository, dest_dir: Path) -> Path: ...


class ResultPublisher(Protocol):
    """What the pipeline needs from ``publish.Publisher``."""

    def publish(self, report: Report, *, report_location: str | None = None) -> PublishResult: ...


SourceFactory = Callable[[ScanConfig, str | None], GitSource]
PublisherFactory = Callable[[ScanConfig], ResultPublisher]


def _github_source(config: ScanConfig, token: str | None) -> GitSource:
    return GitHubSource.from_config(config, token)


def _publisher(config: ScanConfig) -> ResultPublisher:
    return Publisher(config.amp_url, config.token, expires_at=config.token_expires_at)


def assume_yes(flag: bool, env: Mapping[str, str] | None = None) -> bool:
    """True when ``--yes`` was given or ``AMP_SCAN_YES`` is ``1``, ``true`` or ``yes``."""
    value = (os.environ if env is None else env).get(YES_ENV, "")
    return flag or value.strip().lower() in ("1", "true", "yes")


def run_scan(
    config: ScanConfig,
    *,
    yes: bool = False,
    report_path: Path | None = None,
    ask_token: Callable[[ScanConfig], str | None] = git_token,
    ask_llm: Callable[[ScanConfig], LlmSettings] = llm_settings,
    make_source: SourceFactory = _github_source,
    make_publisher: PublisherFactory = _publisher,
    is_tty: Callable[[], bool] | None = None,
    read_answer: Callable[[], str] | None = None,
    clock: Callable[[], float] = time.time,
    out: TextIO | None = None,
) -> int:
    """Run the scan ``config`` describes and return the exit code. Messages go to ``out`` (default stderr).

    The report is written to ``report_path`` (default ``/out/report.json``).

    Raises ConfigError (no terminal to confirm in, or from the token or LLM prompts), ReportError (the report
    folder is not writable) and ScanTokenError (too little token lifetime left after confirming) for the
    caller to print; every later failure is printed here. Ctrl-C raises KeyboardInterrupt, or
    PublishInterrupted once publishing has started.
    """
    stream = sys.stderr if out is None else out
    report_path = DEFAULT_REPORT_PATH if report_path is None else report_path

    def say(line: str = "") -> None:
        print(line, file=stream, flush=True)

    if config.provider != "github":
        say("amp-shadow-runner: Azure DevOps scans are not available in this version of the runner yet.")
        return ExitCode.RUNTIME_ERROR
    check_writable(report_path)
    token = ask_token(config)
    _llm = ask_llm(config) if config.ai_review else None  # AI review (a later step) uses these settings.
    tty = is_tty() if is_tty is not None else sys.stdin is not None and sys.stdin.isatty()
    if not yes and not tty:
        raise ConfigError(
            "The scan asks you to confirm the repositories, and there is no terminal to ask in. "
            f"Run it with docker run -it, or add --yes (or -e {YES_ENV}=1) to scan without asking."
        )

    started = datetime.fromtimestamp(clock(), UTC)
    source = make_source(config, token)
    try:
        listing = source.list_all() if config.all_repositories else source.list_named(config.repositories)
    except GitSourceError as exc:
        say(f"amp-shadow-runner: {exc}")
        return exc.exit_code
    _print_listing(say, config, listing, anonymous=token is None)
    if not yes:
        say(f"Scan {_plural(len(listing.repositories), 'repository', 'repositories')} and publish the results? [y/N]")
        reader = sys.stdin.readline if read_answer is None else read_answer
        try:
            answer = reader()
        except EOFError:  # Ctrl-D declines; Ctrl-C reaches the caller's cancel path.
            answer = ""
        if answer.strip().lower() not in ("y", "yes"):
            say(EXPIRES_NOTE)
            return ExitCode.OK

    deadline = config.token_expires_at.timestamp() - TOKEN_MARGIN_SECONDS
    if listing.repositories and clock() >= deadline:
        raise ScanTokenError(
            "The scan token has too little time left to scan and publish. "
            "Generate a new command in Agent Manager and run it again."
        )
    repositories = _scan_all(config, source, listing.repositories, Progress(stream), deadline, clock)
    repositories += [RepositoryReport(name, RepositoryStatus.GONE) for name in listing.gone]
    repositories += [RepositoryReport(f.name, RepositoryStatus.FAILED, reason=f.reason) for f in listing.failed]
    report = Report(
        scan_id=config.scan_id,
        provider=config.provider,
        organization=config.organization,
        started_at=started,
        finished_at=datetime.fromtimestamp(clock(), UTC),
        repositories=tuple(repositories),
        complete=listing.complete,
    )

    report_error: ReportError | None = None
    try:
        write_report(report, report_path)
    except ReportError as exc:
        report_error = exc
        say(f"amp-shadow-runner: {exc} The results are published anyway.")
    Progress(stream).summary(report, str(report_path) if report_error is None else "not saved")
    _print_unpublished(say, config, report)
    say(EXPORT_IGNORE_NOTE)

    try:
        result = make_publisher(config).publish(
            report, report_location=str(report_path) if report_error is None else None
        )
        say(published_line(result))
    except PublishError as exc:
        say(f"amp-shadow-runner: {exc}")
        return exc.exit_code
    except KeyboardInterrupt:
        raise PublishInterrupted from None
    if report_error is not None:
        return report_error.exit_code
    return report_exit_code(report)


def _scan_all(
    config: ScanConfig,
    source: GitSource,
    repos: tuple[Repository, ...],
    progress: Progress,
    deadline: float,
    clock: Callable[[], float],
) -> list[RepositoryReport]:
    """Scan each repository in turn. None is started after ``deadline``: it and the rest are failed."""
    reports: list[RepositoryReport] = []
    total = len(repos)
    stopped: str | None = None
    with workspace() as root:
        for index, repo in enumerate(repos, 1):
            if stopped is None and clock() >= deadline:
                stopped = TOKEN_RAN_OUT
                progress.failed(index, total, repo.name, stopped)
            if stopped is not None:
                reports.append(RepositoryReport(repo.name, RepositoryStatus.FAILED, reason=stopped))
                continue
            try:
                result = _scan_one(config, source, repo, root / f"repo-{index}", index, total, progress)
            except RepositoryError as exc:
                reason = exc.reason
            except GitSourceError as exc:  # The host refuses every repository now, so stop asking it.
                reason = stopped = str(exc)
            except ArchiveError as exc:
                reason = str(exc)
            except OSError as exc:
                reason = f"could not write it to the scan folder ({exc.strerror or 'input/output error'})"
            else:
                progress.scanned(index, total, repo.name, result)
                reports.append(RepositoryReport.scanned(repo.name, result))
                continue
            progress.failed(index, total, repo.name, reason)
            reports.append(RepositoryReport(repo.name, RepositoryStatus.FAILED, reason=reason))
    return reports


def _scan_one(
    config: ScanConfig, source: GitSource, repo: Repository, tree: Path, index: int, total: int, progress: Progress
) -> ScanResult:
    """Download, extract, scan and delete one repository. Raises what ``_scan_all`` turns into a failure."""
    progress.downloading(index, total, repo.name)
    resolved = source.resolve_commit(repo)
    if resolved.empty:
        return _EMPTY  # No commits: scanned, with no agents.
    archive = source.download_tarball(resolved, tree.parent)
    extracted = extract(archive, tree, kind="tar.gz")  # It deletes the archive, and the tree on a failure.
    try:
        progress.scanning(index, total, repo.name)
        result = scan_tree(tree, repository=repo.name, read_ci=config.read_ci)
    except OSError:
        raise
    except Exception:  # An engine bug in one repository must not stop the others; its text is not shown.
        raise RepositoryError(repo.name, "the runner could not scan it (an internal error)") from None
    finally:
        remove_tree(tree)  # The code is deleted before the next repository is downloaded.
    return replace(result, files_skipped=result.files_skipped + extracted.skipped)


def _print_listing(say: Callable[[str], None], config: ScanConfig, listing: RepositoryList, *, anonymous: bool) -> None:
    repos = listing.repositories
    say(f"{_plural(len(repos), 'repository', 'repositories')} to scan in {_clean(config.organization)}:")
    for repo in repos:
        say(f"  {_clean(repo.name)}{' (private)' if repo.private else ''}")
    if listing.gone:
        say(f"Not found, counted as scanned with no agents ({len(listing.gone)}):")
        for name in listing.gone:
            say(f"  {_clean(name)}")
    if listing.failed:
        say(f"Cannot be scanned ({len(listing.failed)}):")
        for failed in listing.failed:
            say(f"  {_clean(failed.name)}: {_clean(failed.reason)}")
    if not listing.complete:
        say(
            "The list may be incomplete (a limit, unreadable entries, or repositories the token cannot see), "
            "so Agent Manager will not mark any agent as gone."
        )
    if anonymous and repos:
        say(ANONYMOUS_WARNING)


def _print_unpublished(say: Callable[[str], None], config: ScanConfig, report: Report) -> None:
    # Test code is never published, so its not-sure agents are not counted here (as in the summary).
    unconfirmed = sum(
        f.verdict == Verdict.NOT_SURE and f.tag != "test" and reviewed_by(f) is None for f in report.findings
    )
    if not unconfirmed:
        return
    count = f"{_plural(unconfirmed, 'not-sure agent')} {'is' if unconfirmed == 1 else 'are'} not published"
    if config.ai_review:
        say(f"AI review is not available in this version of the runner, so {count}.")
    else:
        say(f"{count[0].upper()}{count[1:]}: without --ai-review, only agents the rules prove are published.")


def _plural(count: int, noun: str, plural: str | None = None) -> str:
    return f"{count} {noun}" if count == 1 else f"{count} {plural or noun + 's'}"


def _clean(text: str) -> str:
    return "".join(c if c.isprintable() else "?" for c in text)
