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

"""Run a whole scan: list the repositories, confirm, fetch and scan each one, save the results, upload them.

1. ``build_scan_config`` has already checked the flags and, when the command has them, ``AMP_URL`` and the
   scan token. The report folder is checked next, before anything long.
2. The Git token is asked for only when needed (``prompts.git_token``), then, with ``--ai-review``, the LLM
   settings (``prompts.llm_settings``).
3. The repositories are listed or looked up, printed, and confirmed. Without a terminal the scan goes on
   only with ``--yes`` or ``AMP_SHADOW_YES=1``, and otherwise exits CONFIG_ERROR before any listing.
   Declining scans nothing: the scan in Agent Manager waits for results until it is cancelled there.
4. Each repository is downloaded and extracted into the always-deleted workspace at its default branch's
   commit, scanned, and deleted before the next one. Its findings get the attribute ``branch``, that
   branch. A failure in one repository marks it failed with a short reason and the scan goes on. A failure
   that stops the Git host for every repository (a refused token, a used-up rate limit) marks it and every
   repository after it failed, so the agents found so far are still saved.
5. The report (``report.json``) and then the results file (``results-<scanId>.json``, see ``results``) are
   written, whatever the scan found. The results file holds exactly the body a direct upload sends.
6. Without a scan token the command ends there, naming both files and the next step: upload the results
   file in Agent Manager. With one it asks to upload the results now (default yes; ``--yes`` uploads
   without asking, and with no terminal and no ``--yes`` nothing is uploaded). A declined, expired or
   failed upload keeps the files and points to the console.

The scan token never limits the scan: results can always be uploaded in the console, so a scan that
outlives the token's hour finishes every repository and the upload step says the token expired.

Exit codes: OK when every repository was scanned, the list was complete and the results were saved (and
uploaded, when the upload ran); PARTIAL_FAILURE when some repositories failed or the list was incomplete,
including when every repository failed (Agent Manager then marks the scan failed); a write error's or
an upload error's own code otherwise. Not uploading by choice (declined, or no terminal), or because the
scan outlived its token, is not an error: the scan's own code stands.

Not-sure findings are published only after AI review confirms them (``--ai-review``, see ``review``). Each
repository's findings are reviewed right after its scan, while its files are still on disk; without
``--ai-review`` they stay in the report and are not in the results. The LLM judge (``--judge``) is not built
yet: the command says so once and saves the AI-enhanced detection results without it.
"""

import math
import os
import sys
import time
from collections.abc import Callable, Iterable, Mapping
from dataclasses import dataclass, replace
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, Protocol, TextIO
from urllib.parse import urlsplit

from amp_shadow_runner.config import ConfigError, ScanConfig
from amp_shadow_runner.engine.classify import Finding
from amp_shadow_runner.engine.scan import ScanResult, scan_tree
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.fetch import ArchiveError, extract, remove_tree, workspace
from amp_shadow_runner.gitsource import GitSourceError, Repository, RepositoryError, RepositoryList
from amp_shadow_runner.gitsource.github import GitHubSource
from amp_shadow_runner.httpclient import ProgressCallback
from amp_shadow_runner.keyboard import read_fresh_line
from amp_shadow_runner.llm import LlmClient, LlmSettings
from amp_shadow_runner.progress import Progress
from amp_shadow_runner.prompts import git_token, llm_settings
from amp_shadow_runner.publish import Publisher, PublishError, PublishResult, build_body, encode_body, report_exit_code
from amp_shadow_runner.report import (
    DEFAULT_REPORT_PATH,
    Report,
    ReportError,
    RepositoryReport,
    RepositoryStatus,
    check_writable,
    write_report,
)
from amp_shadow_runner.results import CONSOLE_PLACE, results_path, write_results
from amp_shadow_runner.review import Completer, Reviewer, ReviewOutcome, review_counts
from amp_shadow_runner.terminal import INDENT, LABEL_WIDTH, Mark, Ui, clean, duration, number, pad, plural, width

YES_ENV = "AMP_SHADOW_YES"
YES_ENV_ALIASES = ("AMP_SCAN_YES",)
"""Older names of ``YES_ENV``, still read."""
TITLE = "AMP shadow agent scan"
ANONYMOUS_WARNING = "No GitHub token: about 25 repositories an hour."
ANONYMOUS_HINT = "Pass -e GITHUB_TOKEN for more."
EXPORT_IGNORE_HINT = "Files marked export-ignore in .gitattributes were not scanned."
"""The terminal's short form of ``report.EXPORT_IGNORE_NOTE``, which the report keeps."""
LIST_LIMIT = 20
"""The most repositories listed in each group before the confirm question; the rest are counted."""
DECLINED_NOTE = (
    "Nothing was scanned. The scan waits for results in Agent Manager until you cancel it "
    "(Shadow Agent Discovery, Scans, Cancel scan)."
)
UPLOAD_QUESTION = "Upload the results to Agent Manager now?"
TOKEN_EXPIRED_AT_START = "The scan token has expired, so the results will be saved but not uploaded."
JUDGE_NOT_AVAILABLE = (
    "The LLM judge is not available in this version of the runner, "
    "so AI-enhanced detection results are saved without it."
)
_EMPTY = ScanResult((), 0, 0, 0)


class PublishInterrupted(KeyboardInterrupt):
    """Ctrl-C after uploading started: the results may already be in Agent Manager.

    ``results_file`` is the saved results file's name as the user sees it, or None when it was not saved.
    """

    def __init__(self, results_file: str | None = None) -> None:
        super().__init__()
        self.results_file = results_file


class UploadCancelled(KeyboardInterrupt):
    """Ctrl-C after the files were saved, before uploading started: nothing was uploaded."""

    def __init__(self, results_file: str | None = None) -> None:
        super().__init__()
        self.results_file = results_file


class GitSource(Protocol):
    """What the pipeline needs from a Git host (``GitHubSource``)."""

    def list_all(self) -> RepositoryList: ...

    def list_named(self, names: Iterable[str]) -> RepositoryList: ...

    def resolve_commit(self, repo: Repository) -> Repository: ...

    def download_tarball(
        self, repo: Repository, dest_dir: Path, *, on_progress: ProgressCallback | None = None
    ) -> Path: ...


class ResultPublisher(Protocol):
    """What the pipeline needs from ``publish.Publisher``."""

    def publish_body(self, scan_id: str, body: Mapping[str, Any]) -> PublishResult: ...


TokenPrompt = Callable[[ScanConfig, Ui], str | None]
LlmPrompt = Callable[[ScanConfig, Ui], LlmSettings]
SourceFactory = Callable[[ScanConfig, str | None], GitSource]
PublisherFactory = Callable[[ScanConfig], ResultPublisher]


def _github_source(config: ScanConfig, token: str | None) -> GitSource:
    return GitHubSource.from_config(config, token)


def _publisher(config: ScanConfig) -> ResultPublisher:
    if config.amp_url is None or config.token is None:
        raise ConfigError("There is no scan token to upload with.")
    return Publisher(config.amp_url, config.token, expires_at=config.token_expires_at)


def assume_yes(flag: bool, env: Mapping[str, str] | None = None) -> bool:
    """True when ``--yes`` was given or ``AMP_SHADOW_YES`` (or the older ``AMP_SCAN_YES``) is ``1``, ``true``
    or ``yes``."""
    env = os.environ if env is None else env
    return flag or any(
        env.get(name, "").strip().lower() in ("1", "true", "yes") for name in (YES_ENV, *YES_ENV_ALIASES)
    )


def shown_path(path: Path) -> str:
    """``path`` as the user sees it: a file in ``/out`` is in the folder the command ran in."""
    if path.parent == DEFAULT_REPORT_PATH.parent:
        return path.name
    return clean(str(path))


def run_scan(
    config: ScanConfig,
    *,
    yes: bool = False,
    report_path: Path | None = None,
    ask_token: TokenPrompt = git_token,
    ask_llm: LlmPrompt = llm_settings,
    make_llm: Callable[[LlmSettings], Completer] = LlmClient,
    make_source: SourceFactory = _github_source,
    make_publisher: PublisherFactory = _publisher,
    is_tty: Callable[[], bool] | None = None,
    read_answer: Callable[[], str] | None = None,
    clock: Callable[[], float] = time.time,
    out: TextIO | None = None,
) -> int:
    """Run the scan ``config`` describes and return the exit code. Messages go to ``out`` (default stderr),
    in sections (see ``terminal.Ui``), with a live status line when ``out`` is a terminal.

    The report is written to ``report_path`` (default ``/out/report.json``) and the results file next to it.

    Raises ConfigError (no terminal to confirm in, or from the token or LLM prompts) and ReportError (the
    report folder is not writable) for the caller to print; every later failure is printed here. Ctrl-C
    raises KeyboardInterrupt; UploadCancelled once the files are saved; PublishInterrupted once uploading
    has started.
    """
    stream = sys.stderr if out is None else out
    report_path = DEFAULT_REPORT_PATH if report_path is None else report_path
    progress = Progress(stream)
    ui = progress.ui
    tty_now = _stdin_is_tty if is_tty is None else is_tty

    check_writable(report_path)
    ui.header(TITLE, f"{config.organization} on {_host(config.api_url)}")
    try:
        token = ask_token(config, ui)
        settings = ask_llm(config, ui) if config.ai_review else None
        if settings is not None and config.judge:
            ui.warn(JUDGE_NOT_AVAILABLE)
        if not yes and not tty_now():
            raise ConfigError(
                "The scan asks you to confirm the repositories, and there is no terminal to ask in. "
                f"Run it with docker run -it, or add --yes (or -e {YES_ENV}=1) to scan without asking."
            )
    except ConfigError:
        ui.say()  # The caller's error line, apart from the section it stopped in.
        raise

    started = datetime.fromtimestamp(clock(), UTC)
    source = make_source(config, token)
    try:
        progress.listing(config.organization)
        listing = source.list_all() if config.all_repositories else source.list_named(config.repositories)
    except GitSourceError as exc:
        _finish(ui, Mark.FAIL, str(exc))
        return exc.exit_code
    finally:
        ui.clear()
    _print_listing(ui, listing, anonymous=token is None)
    if _expired(config, clock):
        ui.warn(TOKEN_EXPIRED_AT_START, hint=f"Upload them in Agent Manager afterwards ({CONSOLE_PLACE}).")
    if not yes:
        reader = _fresh_line if read_answer is None else read_answer
        if not _confirm(ui, f"Scan {plural(len(listing.repositories), 'repository', 'repositories')}?", reader):
            ui.note(DECLINED_NOTE)
            return ExitCode.OK

    ui.section("Scanning")
    progress.begin(repo.name for repo in listing.repositories)
    try:
        reviewer = None
        if settings is not None:
            reviewer = Reviewer(make_llm(settings), settings.model, deadline=math.inf, clock=clock, notify=ui.warn)
        repositories = _scan_all(config, source, listing.repositories, progress, reviewer)
        if not listing.repositories:
            ui.note("Nothing to scan.")
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
        results_file = results_path(report_path, config.scan_id)
        saved: _Saved | None = None
        try:
            try:
                write_report(report, report_path)
            except ReportError as exc:
                report_error = exc
            saved = _save_results(report, results_file, report.finished_at)
            _print_results(ui, config, report, progress.elapsed())
            _print_saved(ui, report_path, report_error, saved, direct=config.token is not None)
            code = report_exit_code(report)
            for error in (report_error, saved.error):
                if error is not None:
                    code = error.exit_code
            upload_code = None
            if saved.body is not None and config.token is not None:
                upload_code = _upload(
                    ui,
                    progress,
                    make_publisher(config),
                    config,
                    saved,
                    yes=yes,
                    is_tty=tty_now,
                    reader=read_answer,
                    clock=clock,
                )
            ui.note(EXPORT_IGNORE_HINT)
        except PublishInterrupted:
            raise
        except KeyboardInterrupt:  # Once the results file is saved, nothing was uploaded.
            if saved is not None:
                shown = saved.shown
            elif results_file.is_file():  # Saved, and Ctrl-C came before _save_results returned.
                shown = shown_path(results_file)
            else:
                raise
            raise UploadCancelled(shown) from None
        return code if upload_code is None else upload_code

    finally:
        progress.close()  # Also on Ctrl-C, so the cursor is left on a clean line.


@dataclass(frozen=True)
class _Saved:
    """The results file as saved: the body (None when it is over Agent Manager's limits), where the file
    is (None when it was not written), and the error that stopped either."""

    body: dict[str, Any] | None
    path: Path | None
    error: PublishError | ReportError | None

    @property
    def shown(self) -> str | None:
        return None if self.path is None else shown_path(self.path)


def _save_results(report: Report, path: Path, created_at: datetime) -> _Saved:
    """Build the upload body and write the results file to ``path``."""
    try:
        body = build_body(report)
        encode_body(body)
    except PublishError as exc:
        return _Saved(None, None, exc)
    try:
        write_results(report, body, path, created_at=created_at)
    except ReportError as exc:
        return _Saved(body, None, exc)
    return _Saved(body, path, None)


def _expired(config: ScanConfig, clock: Callable[[], float]) -> bool:
    return config.token_expires_at is not None and clock() >= config.token_expires_at.timestamp()


def _upload(
    ui: Ui,
    progress: Progress,
    publisher: ResultPublisher,
    config: ScanConfig,
    saved: _Saved,
    *,
    yes: bool,
    is_tty: Callable[[], bool],
    reader: Callable[[], str] | None,
    clock: Callable[[], float],
) -> int | None:
    """Ask to upload, and upload. Returns the error's exit code, or None when the results were uploaded, the
    user chose not to, or the scan outlived its token (the console can still take the results file)."""
    assert saved.body is not None
    if _expired(config, clock):
        _not_uploaded(ui, Mark.WARN, "Not uploaded: the scan token has expired.", saved, console=True)
        return None
    if not yes:
        if not is_tty():
            reason = (
                f"Not uploaded: there is no terminal to ask in (add --yes or -e {YES_ENV}=1 to upload without asking)."
            )
            _not_uploaded(ui, Mark.WARN, reason, saved, console=True)
            return None
        if not _confirm(ui, UPLOAD_QUESTION, _fresh_line if reader is None else reader, default=True):
            _not_uploaded(ui, Mark.WARN, "Not uploaded.", saved, console=True)
            return None
    ui.say()
    progress.publishing()
    try:
        result = publisher.publish_body(config.scan_id, saved.body)
    except PublishError as exc:
        _not_uploaded(ui, Mark.FAIL, f"Not uploaded: {exc}", saved, console=exc.console_can_upload, blank=False)
        return exc.exit_code
    except KeyboardInterrupt:
        raise PublishInterrupted(saved.shown) from None
    _finish(ui, *_published(result), blank=False)
    return None


def _scan_all(
    config: ScanConfig,
    source: GitSource,
    repos: tuple[Repository, ...],
    progress: Progress,
    reviewer: Reviewer | None = None,
) -> list[RepositoryReport]:
    """Scan each repository in turn. After a failure that stops the Git host, the rest are failed with it."""
    reports: list[RepositoryReport] = []
    total = len(repos)
    stopped: str | None = None
    with workspace() as root:
        for index, repo in enumerate(repos, 1):
            if stopped is not None:
                reports.append(RepositoryReport(repo.name, RepositoryStatus.FAILED, reason=stopped))
                continue
            try:
                result = _scan_one(config, source, repo, root / f"repo-{index}", index, total, progress, reviewer)
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
    config: ScanConfig,
    source: GitSource,
    repo: Repository,
    tree: Path,
    index: int,
    total: int,
    progress: Progress,
    reviewer: Reviewer | None = None,
) -> ScanResult:
    """Download, extract, scan, review and delete one repository. Raises what ``_scan_all`` turns into a
    failure; AI review never raises."""
    progress.downloading(index, total, repo.name)
    resolved = source.resolve_commit(repo)
    if resolved.empty:
        return _EMPTY  # No commits: scanned, with no agents.
    archive = source.download_tarball(
        resolved, tree.parent, on_progress=lambda done, length: progress.received(index, total, repo.name, done, length)
    )
    progress.extracting(index, total, repo.name)
    extracted = extract(archive, tree)  # It deletes the archive, and the tree on a failure.
    try:
        progress.scanning(index, total, repo.name)
        result = scan_tree(
            tree,
            repository=repo.name,
            read_ci=config.read_ci,
            progress=lambda done, count: progress.scanning_files(index, total, repo.name, done, count),
        )
    except OSError:
        raise
    except Exception:  # An engine bug in one repository must not stop the others; its text is not shown.
        raise RepositoryError(repo.name, "the runner could not scan it (an internal error)") from None
    else:
        if reviewer is not None:  # While the files are still on disk.
            findings = reviewer.review(
                result.findings, tree, progress=lambda n, due: progress.reviewing(index, total, repo.name, n, due)
            )
            result = replace(result, findings=findings)
    finally:
        remove_tree(tree)  # The code is deleted before the next repository is downloaded.
    return replace(
        result,
        findings=_with_branch(result.findings, resolved.default_branch),
        files_skipped=result.files_skipped + extracted.skipped,
    )


def _with_branch(findings: tuple[Finding, ...], branch: str | None) -> tuple[Finding, ...]:
    """``findings`` with the attribute ``branch``: the default branch the repository was scanned at."""
    if not branch:
        return findings
    return tuple(replace(f, attributes={**f.attributes, "branch": branch}) for f in findings)


def _host(api_url: str) -> str:
    """The Git host to name: ``github.com`` for ``https://api.github.com``, else the API address's host."""
    host = urlsplit(api_url).netloc.rpartition("@")[2]
    return host.removeprefix("api.") or api_url


def _finish(ui: Ui, kind: Mark, text: str, *, blank: bool = True) -> None:
    """A closing status line, out of any section: ``✓ Uploaded to Agent Manager: 6 agents saved.``"""
    if blank:
        ui.say()
    ui.status(kind, text, indent="")


def _published(result: PublishResult) -> tuple[Mark, str]:
    text = f"Uploaded to Agent Manager: {plural(result.findings_count, 'agent')} saved"
    if result.status == "succeeded":
        return Mark.OK, text + "."
    return Mark.WARN, f"{text} (scan {clean(result.status)})."


def _stdin_is_tty() -> bool:
    return sys.stdin is not None and sys.stdin.isatty()


def _fresh_line() -> str:
    """A line typed at stdin after the question is shown: nothing typed or pasted before can confirm a scan."""
    try:
        fd = sys.stdin.fileno()
    except (AttributeError, ValueError, OSError):
        return sys.stdin.readline()
    return read_fresh_line(fd)


def _confirm(ui: Ui, question: str, reader: Callable[[], str], *, default: bool = False) -> bool:
    """Ask ``? question (y/N)`` (``(Y/n)`` with ``default``) on a line of its own; True for y or yes, or for
    Enter alone with ``default``. Ctrl-D (the end of input) always declines; Ctrl-C reaches the caller's
    cancel path. ``reader`` should read only what is typed after the question (``_fresh_line``)."""
    ui.say()
    ui.write(ui.question(question, default="Y/n" if default else "y/N", indent=""))
    try:
        answer = reader()
    except EOFError:
        answer = ""
    if not answer.endswith("\n") or not ui.tty:  # Nothing echoed a newline after the answer.
        ui.write("\n")
    choice = answer.strip().lower()
    if default and not choice and answer.endswith("\n"):
        return True
    return choice in ("y", "yes")


def _limited(ui: Ui, names: tuple[str, ...]) -> tuple[tuple[str, ...], str]:
    """The first LIST_LIMIT of ``names``, and the note that counts the rest (or "")."""
    if len(names) <= LIST_LIMIT:
        return names, ""
    return names[:LIST_LIMIT], f"{ui.glyphs.ellipsis} and {number(len(names) - LIST_LIMIT)} more"


def _print_listing(ui: Ui, listing: RepositoryList, *, anonymous: bool) -> None:
    repos = listing.repositories
    names = [clean(r.name) for r in repos] + [clean(n) for n in listing.gone] + [clean(f.name) for f in listing.failed]
    column = min(max((width(n) for n in names), default=0), 32) + 3
    ui.section(f"Repositories ({number(len(repos))})")
    private = {r.name for r in repos if r.private}
    shown, more = _limited(ui, tuple(r.name for r in repos))
    for name in shown:
        ui.item((pad(clean(name), column) + ui.dim("private")) if name in private else clean(name))
    if more:
        ui.note(more)
    if not repos:
        ui.note("No repositories to scan.")
    shown, more = _limited(ui, listing.gone)
    for name in shown:
        ui.warn(pad(clean(name), column) + "not found, counted as scanned with no agents")
    if more:
        ui.note(more, indent="    ")
    failures = {f.name: f.reason for f in listing.failed}
    shown, more = _limited(ui, tuple(failures))
    for name in shown:
        ui.fail(pad(clean(name), column) + f"cannot be scanned: {clean(failures[name])}")
    if more:
        ui.note(more, indent="    ")
    if not listing.complete:
        ui.warn(
            "The list may be incomplete (a limit, unreadable entries, or repositories the token cannot see), "
            "so Agent Manager will not mark any agent as gone."
        )
    if anonymous and repos:
        ui.warn(ANONYMOUS_WARNING, hint=ANONYMOUS_HINT)


def _print_results(ui: Ui, config: ScanConfig, report: Report, elapsed: float | None) -> None:
    """The Results section: repositories (when more than one), agents, AI, test code, files."""
    summary = report.summary
    ui.section("Results")
    repos = summary.repositories
    extra = [
        f"{number(repos[status])} {label}"
        for status, label in (
            (RepositoryStatus.GONE, "not found"),
            (RepositoryStatus.FAILED, "failed"),
            (RepositoryStatus.SKIPPED, "skipped"),
        )
        if repos[status]
    ]
    if len(report.repositories) > 1 or extra:
        scanned = repos[RepositoryStatus.SCANNED] + repos[RepositoryStatus.GONE]
        text = plural(scanned, "repository", "repositories")
        if elapsed is not None:
            text += f" in {duration(elapsed)}"
        ui.row("Scanned", text + (f" ({', '.join(extra)})" if extra else ""))

    ui.row("Agents", _agents(summary.proven, summary.not_sure, report, ai_review=config.ai_review))
    if config.ai_review:
        counts = review_counts(report.findings)
        outcomes = [
            f"{number(counts[outcome])} {label}"
            for outcome, label in (
                (ReviewOutcome.REJECTED, "rejected"),
                (ReviewOutcome.FAILED, "failed"),
                (ReviewOutcome.NOT_REVIEWED, "not reviewed"),
            )
            if counts[outcome]
        ]
        if outcomes:
            ui.row("AI", ", ".join(outcomes) + ui.dim(" (not published; the report says why)"))
        elif not summary.not_sure:
            ui.row("AI", ui.dim("nothing to review"))
    if summary.test:
        ui.row("Test code", f"{number(summary.test)} more, not published")
    files = f"{number(summary.files_scanned)} scanned"
    if summary.files_skipped:
        files += f", {number(summary.files_skipped)} skipped"
    ui.row("Files", files)


def _saved_row(ui: Ui, label: str, path: Path | None) -> None:
    if path is None:
        ui.row(label, ui.dim("not saved"))
    elif path.parent == DEFAULT_REPORT_PATH.parent:
        ui.row(label, f"{path.name} in this folder")  # The command mounts its folder at /out.
    else:
        ui.row(label, clean(str(path)))


def _print_saved(ui: Ui, report_path: Path, report_error: ReportError | None, saved: _Saved, *, direct: bool) -> None:
    """The Done section: the two files and, without a scan token, the next step."""
    ui.section("Done")
    _saved_row(ui, "Report", None if report_error is not None else report_path)
    _saved_row(ui, "Results", saved.path)
    if report_error is not None:
        ui.warn(str(report_error))
    if saved.error is not None:
        ui.warn(str(saved.error))
    if saved.path is not None and not direct:
        _next_step(ui, saved.path, row=True)


def _next_step(ui: Ui, path: Path, *, row: bool = False) -> None:
    """``Upload results-<id>.json`` / ``in Agent Manager: Shadow Agent Discovery, Scans, Upload results.``,
    as a ``Next`` row or as two lines under a closing status line."""
    lead = INDENT + " " * LABEL_WIDTH if row else INDENT
    first = f"Upload {shown_path(path)}"
    if row:
        ui.row("Next", first)
    else:
        ui.say(lead + first)
    ui.say(f"{lead}in Agent Manager: {CONSOLE_PLACE}.")


def _not_uploaded(ui: Ui, kind: Mark, text: str, saved: _Saved, *, console: bool, blank: bool = True) -> None:
    """The closing lines when the results were not uploaded: why, and, when the console can still take the
    results file, how to upload it there."""
    _finish(ui, kind, text, blank=blank)
    if console and saved.path is not None:
        _next_step(ui, saved.path)


def _agents(proven: int, not_sure: int, report: Report, *, ai_review: bool) -> str:
    """``6: 5 proven, 1 confirmed by AI``. Agents in test code are counted apart."""
    if not proven + not_sure:
        return "none found"
    parts = [f"{number(proven)} proven"] if proven else []
    if ai_review:
        confirmed = review_counts(report.findings)[ReviewOutcome.CONFIRMED]
        if confirmed:
            parts.append(f"{number(confirmed)} confirmed by AI")
        if not_sure - confirmed:
            parts.append(f"{number(not_sure - confirmed)} not confirmed")
    elif not_sure:
        parts.append(f"{number(not_sure)} not sure (published only with --ai-review)")
    if len(parts) == 1:
        return parts[0]
    return f"{number(proven + not_sure)}: {', '.join(parts)}"
