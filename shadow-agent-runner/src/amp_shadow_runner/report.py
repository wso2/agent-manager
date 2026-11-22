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

"""The scan report: a JSON file for the customer, written to ``/out/report.json``.

The report says what the scan did, repository by repository, and lists every finding, proven or not sure,
including those in test code (which are never published). It holds what publishing needs, so a
saved report can be published later. It never holds source code, prompts, secrets or tokens: only the
fields below, built from findings, which carry names, paths, line numbers, short labels and counts.

With ``--ai-review``, each Not-sure finding outside test code has the attribute ``aiReview``: its outcome
and a short reason (printable, at most 200 characters, in the LLM's words when it answered, so it may
quote a few words of code; the report stays on the customer's machine), and the summary
counts the outcomes (see ``review``). A confirmed finding keeps ``reviewedBy: "ai_review"``, so ``upload``
publishes it again. Reports without them (no ``--ai-review``, or an older runner) read the same.

Inside the container ``/out`` is the folder the command mounts with ``-v "$PWD:/out"``. The file is written
to a temporary file in the same folder and then renamed, so a failed write never leaves half a report. The
results file (``results``) is written the same way, next to it.
"""

import contextlib
import json
import os
import tempfile
from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import StrEnum
from pathlib import Path
from typing import ClassVar, cast, get_args

from amp_shadow_runner._version import __version__
from amp_shadow_runner.config import Provider
from amp_shadow_runner.engine.classify import Evidence, Finding, Verdict
from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.engine.scan import ScanResult
from amp_shadow_runner.engine.version import ENGINE_VERSION, RULE_PACK_ID
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.review import ReviewOutcome, review_counts

REPORT_VERSION = 1
DEFAULT_REPORT_PATH = Path("/out/report.json")
EXPORT_IGNORE_NOTE = (
    "Repositories are downloaded as archives, so files a repository's .gitattributes marks export-ignore "
    "are not in the archive and were not scanned."
)
TEST_NOTE = "Agents in test code are listed with their tag but are not published."


class ReportError(Exception):
    """The report cannot be written or read. The message is safe to show."""

    exit_code: ClassVar[ExitCode] = ExitCode.CONFIG_ERROR


class RepositoryStatus(StrEnum):
    """What happened to one repository."""

    SCANNED = "scanned"
    FAILED = "failed"
    """It could not be listed, downloaded or scanned; ``reason`` says why."""
    GONE = "gone"
    """It no longer exists: it counts as scanned, with no agents."""
    SKIPPED = "skipped"
    """It was left out on purpose; ``reason`` says why."""


@dataclass(frozen=True)
class RepositoryReport:
    """One repository's part of the report."""

    name: str
    status: RepositoryStatus
    reason: str | None = None
    files_scanned: int = 0
    files_skipped: int = 0
    parse_errors: int = 0
    findings: tuple[Finding, ...] = ()

    @classmethod
    def scanned(cls, name: str, result: ScanResult) -> "RepositoryReport":
        return cls(
            name,
            RepositoryStatus.SCANNED,
            files_scanned=result.files_scanned,
            files_skipped=result.files_skipped,
            parse_errors=result.parse_errors,
            findings=result.findings,
        )


@dataclass(frozen=True)
class Summary:
    """The counts of a report. Agents in test code are counted apart, not as proven or not sure; agents in
    example code are published, so they count with the rest."""

    repositories: Mapping[RepositoryStatus, int]
    proven: int
    not_sure: int
    test: int
    files_scanned: int
    files_skipped: int
    parse_errors: int


@dataclass(frozen=True)
class Report:
    """A whole scan's report."""

    scan_id: str
    provider: Provider
    organization: str
    started_at: datetime
    finished_at: datetime
    repositories: tuple[RepositoryReport, ...]
    complete: bool = True
    """False when the repository list may be missing repositories."""
    runner_version: str = __version__
    engine_version: str = ENGINE_VERSION
    rule_pack_id: str = RULE_PACK_ID
    notes: tuple[str, ...] = field(default=())
    """Extra notes for the reader; ``to_json`` adds the standard ones."""

    @property
    def findings(self) -> tuple[Finding, ...]:
        return tuple(f for repo in self.repositories for f in repo.findings)

    @property
    def summary(self) -> "Summary":
        findings = self.findings
        statuses = [repo.status for repo in self.repositories]
        return Summary(
            repositories={status: statuses.count(status) for status in RepositoryStatus},
            proven=sum(f.verdict == Verdict.PROVEN and f.tag != "test" for f in findings),
            not_sure=sum(f.verdict == Verdict.NOT_SURE and f.tag != "test" for f in findings),
            test=sum(f.tag == "test" for f in findings),
            files_scanned=sum(repo.files_scanned for repo in self.repositories),
            files_skipped=sum(repo.files_skipped for repo in self.repositories),
            parse_errors=sum(repo.parse_errors for repo in self.repositories),
        )

    def to_json(self) -> dict[str, object]:
        """The report as a JSON object."""
        summary = self.summary
        notes = [EXPORT_IGNORE_NOTE]
        if summary.test:
            notes.append(TEST_NOTE)
        notes.extend(n for n in self.notes if n not in notes)
        agents: dict[str, object] = {"proven": summary.proven, "notSure": summary.not_sure, "test": summary.test}
        reviews = review_counts(self.findings)
        if any(reviews.values()):
            agents["aiReview"] = {
                "confirmed": reviews[ReviewOutcome.CONFIRMED],
                "rejected": reviews[ReviewOutcome.REJECTED],
                "failed": reviews[ReviewOutcome.FAILED],
                "notReviewed": reviews[ReviewOutcome.NOT_REVIEWED],
            }
        return {
            "reportVersion": REPORT_VERSION,
            "runnerVersion": self.runner_version,
            "engineVersion": self.engine_version,
            "rulePackId": self.rule_pack_id,
            "scanId": self.scan_id,
            "provider": self.provider,
            "organization": self.organization,
            "startedAt": _timestamp(self.started_at),
            "finishedAt": _timestamp(self.finished_at),
            "complete": self.complete,
            "summary": {
                "repositories": {status.value: count for status, count in summary.repositories.items()},
                "agents": agents,
                "filesScanned": summary.files_scanned,
                "filesSkipped": summary.files_skipped,
                "parseErrors": summary.parse_errors,
            },
            "repositories": [_repository_json(repo) for repo in self.repositories],
            "notes": notes,
        }

    @classmethod
    def from_json(cls, data: object) -> "Report":
        """Read a report written by ``to_json``. Raises ReportError when ``data`` is not one."""
        try:
            obj = _object(data)
            if obj.get("reportVersion") != REPORT_VERSION:
                raise ValueError("an unknown report version")
            provider = _str(obj, "provider")
            if provider not in get_args(Provider):
                raise ValueError("an unknown provider")
            standard = {EXPORT_IGNORE_NOTE, TEST_NOTE}
            return cls(
                scan_id=_str(obj, "scanId"),
                provider=cast(Provider, provider),
                organization=_str(obj, "organization"),
                started_at=datetime.fromisoformat(_str(obj, "startedAt")),
                finished_at=datetime.fromisoformat(_str(obj, "finishedAt")),
                repositories=tuple(_repository(r) for r in _list(obj, "repositories")),
                complete=_bool(obj, "complete"),
                runner_version=_str(obj, "runnerVersion"),
                engine_version=_str(obj, "engineVersion"),
                rule_pack_id=_str(obj, "rulePackId"),
                notes=tuple(n for n in _list(obj, "notes") if isinstance(n, str) and n not in standard),
            )
        except (KeyError, TypeError, ValueError) as exc:
            raise ReportError(f"The file is not a scan report from this runner ({_problem(exc)}).") from None


def write_report(report: Report, path: Path = DEFAULT_REPORT_PATH) -> Path:
    """Write ``report`` to ``path`` atomically and return the path.

    Raises ReportError, with a message that says how to fix the command, when the folder is not writable.
    """
    # ASCII only: a name holding a lone surrogate (from undecodable bytes) cannot be written as UTF-8.
    text = json.dumps(report.to_json(), indent=2, ensure_ascii=True) + "\n"
    return write_text_atomically(text, path, what="the scan report")


def write_text_atomically(text: str, path: Path, *, what: str) -> Path:
    """Write ``text`` (UTF-8) to ``path`` through a temporary file in the same folder, then rename it, so a
    failed write never leaves half a file. ``what`` names the file in the ReportError raised on failure."""
    try:
        fd, temporary = tempfile.mkstemp(prefix=f".{path.stem}-", suffix=".tmp", dir=path.parent)
    except OSError as exc:
        raise ReportError(_unwritable(path, exc, what)) from None
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as out:
            out.write(text)
            out.flush()
            os.fsync(out.fileno())
        os.chmod(temporary, 0o644)  # mkstemp makes it 0600; the file holds nothing secret.
        os.replace(temporary, path)
    except OSError as exc:
        with contextlib.suppress(OSError):
            os.unlink(temporary)
        raise ReportError(_unwritable(path, exc, what)) from None
    except BaseException:
        with contextlib.suppress(OSError):
            os.unlink(temporary)
        raise
    return path


def check_writable(path: Path = DEFAULT_REPORT_PATH) -> None:
    """Check, before a long scan, that the report can be written to ``path``'s folder. Raises ReportError."""
    try:
        with tempfile.NamedTemporaryFile(prefix=".report-check-", dir=path.parent):
            pass
    except OSError as exc:
        raise ReportError(_unwritable(path, exc, "the scan report")) from None


def read_json(path: Path) -> object:
    """Read a JSON file. Raises ReportError when it cannot be read or is not JSON."""
    try:
        return cast(object, json.loads(path.read_text(encoding="utf-8")))
    except OSError as exc:
        raise ReportError(f"Cannot read {path}: {exc.strerror or 'unknown error'}.") from None
    except (UnicodeDecodeError, json.JSONDecodeError, RecursionError):
        raise ReportError(f"{path} is not a JSON file.") from None


def read_report(path: Path) -> Report:
    """Read a report file. Raises ReportError when it cannot be read or is not a report."""
    return Report.from_json(read_json(path))


def _unwritable(path: Path, exc: OSError, what: str) -> str:
    folder = path.parent
    message = f"Cannot write {what} to {folder} ({exc.strerror or 'unknown error'})."
    if folder == DEFAULT_REPORT_PATH.parent:
        message += (
            ' Run the command exactly as Agent Manager shows it: -v "$PWD:/out" mounts the current folder at /out,'
            ' and --user "$(id -u):$(id -g)" lets the scan write there as you. Run it from a folder you can write to.'
        )
    return message


def _timestamp(value: datetime) -> str:
    return value.astimezone(UTC).isoformat().replace("+00:00", "Z")


def _repository_json(repo: RepositoryReport) -> dict[str, object]:
    data: dict[str, object] = {"name": repo.name, "status": repo.status.value}
    if repo.reason is not None:
        data["reason"] = repo.reason
    data.update(
        filesScanned=repo.files_scanned,
        filesSkipped=repo.files_skipped,
        parseErrors=repo.parse_errors,
        findings=[_finding_json(f) for f in repo.findings],
    )
    return data


def _finding_json(finding: Finding) -> dict[str, object]:
    return {
        "externalId": finding.external_id,
        "verdict": finding.verdict.value,
        "name": finding.name,
        "path": finding.path,
        "line": finding.line,
        "framework": finding.framework,
        "tag": finding.tag,
        "serviceFolder": finding.service_folder,
        "attributes": dict(finding.attributes),
        "evidence": [{"code": e.code, "source": e.source, "observed": dict(e.observed)} for e in finding.evidence],
    }


def _repository(data: object) -> RepositoryReport:
    obj = _object(data)
    name = _str(obj, "name")
    reason = obj.get("reason")
    if reason is not None and not isinstance(reason, str):
        raise TypeError("reason is not a string")
    return RepositoryReport(
        name=name,
        status=RepositoryStatus(_str(obj, "status")),
        reason=reason,
        files_scanned=_int(obj, "filesScanned"),
        files_skipped=_int(obj, "filesSkipped"),
        parse_errors=_int(obj, "parseErrors"),
        findings=tuple(_finding(f, name) for f in _list(obj, "findings")),
    )


def _finding(data: object, repository: str) -> Finding:
    obj = _object(data)
    tag = obj.get("tag")
    if tag is not None and tag not in get_args(Tag):
        raise ValueError("an unknown tag")
    folder = obj.get("serviceFolder")
    if folder is not None and not isinstance(folder, str):
        raise TypeError("serviceFolder is not a string")
    return Finding(
        external_id=_str(obj, "externalId"),
        name=_str(obj, "name"),
        repository=repository,
        path=_str(obj, "path"),
        line=_int(obj, "line"),
        framework=_str(obj, "framework"),
        verdict=Verdict(_str(obj, "verdict")),
        tag=cast(Tag | None, tag),
        attributes=_object(obj["attributes"]),
        evidence=tuple(_evidence(e) for e in _list(obj, "evidence")),
        service_folder=folder,
    )


def _evidence(data: object) -> Evidence:
    obj = _object(data)
    observed = _object(obj["observed"])
    if not all(isinstance(v, bool | int | str) for v in observed.values()):
        raise TypeError("observed holds a value that is not a flag, count or label")
    return Evidence(_str(obj, "code"), _str(obj, "source"), cast(Mapping[str, bool | int | str], observed))


def _object(data: object) -> dict[str, object]:
    if not isinstance(data, dict):
        raise TypeError("expected an object")
    return cast(dict[str, object], data)


def _list(obj: Mapping[str, object], key: str) -> Sequence[object]:
    value = obj[key]
    if not isinstance(value, list):
        raise TypeError(f"{key} is not a list")
    return value


def _str(obj: Mapping[str, object], key: str) -> str:
    value = obj[key]
    if not isinstance(value, str):
        raise TypeError(f"{key} is not a string")
    return value


def _int(obj: Mapping[str, object], key: str) -> int:
    value = obj[key]
    if not isinstance(value, int) or isinstance(value, bool):
        raise TypeError(f"{key} is not a number")
    return value


def _bool(obj: Mapping[str, object], key: str) -> bool:
    value = obj[key]
    if not isinstance(value, bool):
        raise TypeError(f"{key} is not true or false")
    return value


def _problem(exc: Exception) -> str:
    return f"{exc.args[0]} is missing" if isinstance(exc, KeyError) else str(exc)
