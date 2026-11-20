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

"""Publish a scan's confirmed agents to Agent Manager (``POST .../shadow-agent-publisher/scans/{id}/results``).

Only certain agents are published: a finding the rules prove (``reviewedBy: "rules"``), or a Not-sure finding
that AI review confirmed, which the review marks with the attribute ``reviewedBy`` ``ai_review`` or
``ai_review_judge``. Findings in test code are never published. Each agent carries only allow-listed
attributes and its evidence, with codes prefixed by the provider (``github.`` or ``azure_devops.``). No
source code, prompt or secret is ever sent.

The server's limits are checked first. Evidence past the 50th of an agent is left out (the first 50 are
kept, in the finding's order); a body over the agent, list or size limits is refused rather than cut
down, since a cut list would tell Agent Manager that agents were gone. The saved report is kept either way,
so ``upload`` can publish it once the cause is fixed.

Exit codes: a refused token is AUTH_ERROR; a scan that is not a source code scan of the token's organization,
or source code scans turned off (404), is CONFIG_ERROR; a scan no longer waiting for results (409), a body
over the limits, an invalid body (400, 413), a server error or a network failure is RUNTIME_ERROR. The token
never appears in a message.
"""

import json
import time
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import datetime
from typing import Any
from urllib.parse import quote

from amp_shadow_runner.engine.classify import Finding, Verdict
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.httpclient import HttpClient, HttpError
from amp_shadow_runner.report import Report, RepositoryStatus

PUBLISH_PATH = "/api/v1/shadow-agent-publisher/scans/{scan_id}/results"
MAX_AGENTS = 1000
MAX_EVIDENCE = 50
MAX_REPOSITORY_NAMES = 5000
MAX_BODY_BYTES = 8 * 1024 * 1024
MAX_REASON_CHARS = 300
"""Agent Manager keeps this many characters of a failed repository's reason."""

REVIEWED_BY_RULES = "rules"
REVIEWED_BY_AI = frozenset({"ai_review", "ai_review_judge"})
"""The ``reviewedBy`` values AI review sets on a Not-sure finding it confirmed."""
PUBLISHED_ATTRIBUTES = frozenset({"framework", "model", "tools", "labels", "branch", "projectPath", "language"})
"""Finding attributes that are published, each only when the finding has it; ``reviewedBy`` is added here.
``branch``, ``projectPath`` and ``language`` let Agent Manager prefill deploying the agent."""

_KEEP_REPORT = "The report is saved"
_NEW_COMMAND = "Generate a new command in Agent Manager and run the scan again."


class PublishError(Exception):
    """Publishing failed or was refused. The message is safe to show."""

    exit_code: ExitCode

    def __init__(self, message: str, exit_code: ExitCode = ExitCode.RUNTIME_ERROR) -> None:
        super().__init__(message)
        self.exit_code = exit_code


@dataclass(frozen=True)
class PublishResult:
    """What Agent Manager answered."""

    status: str
    """The scan's new status, such as ``succeeded``, ``partial`` or ``failed``."""
    findings_count: int
    """The agents Agent Manager saved."""


def reviewed_by(finding: Finding) -> str | None:
    """Who confirmed ``finding`` (its ``reviewedBy``), or None when it is not published.

    A proven finding is confirmed by the rules. A Not-sure finding is published only when AI review set
    ``reviewedBy`` to ``ai_review`` or ``ai_review_judge`` on it. Test findings are never published.
    """
    if finding.tag == "test":
        return None
    if finding.verdict == Verdict.PROVEN:
        return REVIEWED_BY_RULES
    value = finding.attributes.get("reviewedBy")
    return value if isinstance(value, str) and value in REVIEWED_BY_AI else None


def build_body(report: Report) -> dict[str, Any]:
    """The publish request body for ``report``. Raises PublishError when it is over the server's limits."""
    scanned: list[str] = []
    failed: list[dict[str, str]] = []
    agents: list[dict[str, Any]] = []
    for repo in report.repositories:
        if repo.status in (RepositoryStatus.SCANNED, RepositoryStatus.GONE):
            scanned.append(repo.name)  # A gone repository counts as scanned, with no agents.
        elif repo.status == RepositoryStatus.FAILED:
            failed.append({"name": repo.name, "reason": _reason(repo.reason)})
        if repo.status != RepositoryStatus.SCANNED:
            continue
        for finding in repo.findings:
            confirmed = reviewed_by(finding)
            if confirmed is not None and finding.evidence:  # Agent Manager refuses an agent without evidence.
                agents.append(_agent(finding, confirmed, report.provider))
    for noun, names in (("scanned", scanned), ("failed", failed)):
        if len(names) > MAX_REPOSITORY_NAMES:
            raise PublishError(
                f"The scan has {len(names)} {noun} repositories, and Agent Manager takes at most "
                f"{MAX_REPOSITORY_NAMES}. Nothing was published. {_KEEP_REPORT}."
            )
    if len(agents) > MAX_AGENTS:
        raise PublishError(
            f"The scan found {len(agents)} agents to publish, and Agent Manager takes at most {MAX_AGENTS} "
            f"in one scan. Nothing was published. {_KEEP_REPORT}; scan fewer repositories in each platform."
        )
    return {"complete": report.complete, "scannedRepositories": scanned, "failedRepositories": failed, "agents": agents}


def encode_body(body: Mapping[str, Any]) -> bytes:
    """``body`` as the UTF-8 JSON the HTTP client sends. Raises PublishError when it is over 8 MiB or unencodable."""
    try:
        data = json.dumps(body, ensure_ascii=False, allow_nan=False).encode("utf-8")
    except (TypeError, ValueError):  # UnicodeEncodeError (a lone surrogate) is a ValueError.
        raise PublishError(f"The results cannot be sent as JSON. Nothing was published. {_KEEP_REPORT}.") from None
    if len(data) > MAX_BODY_BYTES:
        raise PublishError(
            f"The results are {len(data)} bytes, and Agent Manager takes at most {MAX_BODY_BYTES}. "
            f"Nothing was published. {_KEEP_REPORT}; scan fewer repositories in each platform."
        )
    return data


class Publisher:
    """Publishes a report to one scan with the scan token, sent only as ``Authorization: Bearer``."""

    def __init__(
        self,
        amp_url: str,
        token: str,
        *,
        expires_at: datetime | None = None,
        http: HttpClient | None = None,
        now: Callable[[], float] = time.time,
    ) -> None:
        self._amp_url = amp_url.rstrip("/")
        self._token = token
        self._expires_at = expires_at
        self._http = HttpClient() if http is None else http
        self._now = now

    def __repr__(self) -> str:
        return f"Publisher({self._amp_url!r})"  # Never the token.

    def publish(self, report: Report, *, report_location: str | None = None) -> PublishResult:
        """Send ``report``'s confirmed agents and repository lists. Raises PublishError.

        ``report_location`` is named in error messages so the reader knows what ``upload`` needs.
        """
        body = build_body(report)
        encode_body(body)  # The size check; the client encodes it again the same way.
        saved = f"{_KEEP_REPORT} at {report_location}" if report_location else _KEEP_REPORT
        if self._expires_at is not None and self._now() >= self._expires_at.timestamp():
            raise PublishError(
                f"The scan token expired before the results could be published. {saved}, "
                f"but a scan accepts results only within its hour. {_NEW_COMMAND}",
                ExitCode.AUTH_ERROR,
            )
        url = self._amp_url + PUBLISH_PATH.format(scan_id=quote(report.scan_id, safe=""))
        try:
            # Retried on 5xx and network errors: a repeat after a lost answer is refused (409), never saved twice.
            response = self._http.post_json(url, body, headers={"Authorization": f"Bearer {self._token}"}, retry=True)
        except HttpError as exc:
            raise _error(exc, saved, report.scan_id) from None
        data = response.data
        status = data.get("status") if isinstance(data, dict) else None
        count = data.get("findingsCount") if isinstance(data, dict) else None
        if not isinstance(status, str) or not isinstance(count, int) or isinstance(count, bool):
            raise PublishError(f"Agent Manager sent an unexpected answer to the publish. {saved}.")
        return PublishResult("".join(c for c in status if c.isprintable())[:40], count)


def published_line(result: PublishResult) -> str:
    """The line printed after a successful publish."""
    agents = "1 agent" if result.findings_count == 1 else f"{result.findings_count} agents"
    return f"Published to Agent Manager: scan {result.status}, {agents} saved."


def report_exit_code(report: Report) -> ExitCode:
    """OK when every repository was scanned and the list was complete, else PARTIAL_FAILURE.

    Also PARTIAL_FAILURE when every repository failed: the results were still published (Agent Manager marks
    the scan failed), so the command finished, but not its work.
    """
    if not report.complete or any(r.status == RepositoryStatus.FAILED for r in report.repositories):
        return ExitCode.PARTIAL_FAILURE
    return ExitCode.OK


def _agent(finding: Finding, confirmed: str, provider: str) -> dict[str, Any]:
    attributes = {k: v for k, v in finding.attributes.items() if k in PUBLISHED_ATTRIBUTES}
    attributes["reviewedBy"] = confirmed
    return {
        "externalId": finding.external_id,
        "name": finding.name,
        "repository": finding.repository,
        "attributes": attributes,
        "evidence": [
            {"code": f"{provider}.{e.code}", "source": e.source, "observed": dict(e.observed)}
            for e in finding.evidence[:MAX_EVIDENCE]
        ],
    }


def _reason(reason: str | None) -> str:
    """A failed repository's reason, printable, whitespace collapsed, at most MAX_REASON_CHARS characters."""
    text = " ".join("".join(c if c.isprintable() else " " for c in reason or "").split()) or "could not be scanned"
    return text if len(text) <= MAX_REASON_CHARS else text[: MAX_REASON_CHARS - 3] + "..."


def _error(exc: HttpError, saved: str, scan_id: str) -> PublishError:
    said = f" Agent Manager said: {exc.server_message}" if exc.server_message else ""
    status = exc.status
    if status in (401, 403):
        return PublishError(
            f"Agent Manager refused the scan token (HTTP {status}): it has expired, or it is not this scan's "
            f"token. {saved}, but a scan accepts results only within its hour. {_NEW_COMMAND}",
            ExitCode.AUTH_ERROR,
        )
    if status == 404:
        return PublishError(
            "Agent Manager has no source code scan with this ID for the token's organization, or source code "
            f"scans are turned off on this Agent Manager (HTTP 404). Check AMP_URL and the scan ID. {saved}.",
            ExitCode.CONFIG_ERROR,
        )
    if status == 409:
        return PublishError(
            "The scan is no longer waiting for results (HTTP 409): it was already published, it expired after "
            "1 hour, or a newer scan of this platform published first. If an earlier attempt reached Agent "
            f"Manager, the results are already saved.{said} {saved}."
        )
    if status in (400, 413):
        return PublishError(f"Agent Manager refused the results (HTTP {status}).{said} {saved}.")
    return PublishError(
        f"Could not publish the results ({exc}). {saved}; publish it within the scan's hour with:\n"
        f'  docker run --rm -it --user "$(id -u):$(id -g)" -v "$PWD:/out" -e AMP_URL=... -e AMP_SCAN_TOKEN=... '
        f"<image> upload --scan-id {scan_id}"
    )
