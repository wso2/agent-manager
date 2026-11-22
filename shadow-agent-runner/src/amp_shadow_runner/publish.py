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
attributes and its evidence, with codes prefixed by the provider (``github.``). No
source code, prompt or secret is ever sent.

The body (``build_body``) is also what the results file holds (see ``results``), so the file and a direct
upload always send the same thing. The server's limits are checked first. Evidence past the 50th of an
agent is left out (the first 50 are kept, in the finding's order); a body over the agent, list or size
limits is refused rather than cut down, since a cut list would tell Agent Manager that agents were gone.

Exit codes: an expired or refused token is AUTH_ERROR; a scan that is not a source code scan of the token's
organization, or source code scans turned off (404), is CONFIG_ERROR; a scan no longer waiting for results
(409), a body over the limits, an invalid body (400, 413), a server error or a network failure is
RUNTIME_ERROR. The token never appears in a message. Messages say what went wrong, not what to do next:
the caller adds that (``PublishError.console_can_upload``).
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

_FEWER = "Scan fewer repositories in each platform."


class PublishError(Exception):
    """Publishing failed or was refused. The message is safe to show.

    ``console_can_upload`` is True when uploading the results file in the Agent Manager console, signed in
    as yourself, may still work: the token expired or was refused, or Agent Manager could not be reached.
    """

    exit_code: ExitCode
    console_can_upload: bool

    def __init__(
        self, message: str, exit_code: ExitCode = ExitCode.RUNTIME_ERROR, *, console_can_upload: bool = False
    ) -> None:
        super().__init__(message)
        self.exit_code = exit_code
        self.console_can_upload = console_can_upload


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
                f"{MAX_REPOSITORY_NAMES}, so the results cannot be uploaded. {_FEWER}"
            )
    if len(agents) > MAX_AGENTS:
        raise PublishError(
            f"The scan found {len(agents)} agents to publish, and Agent Manager takes at most {MAX_AGENTS} "
            f"in one scan, so the results cannot be uploaded. {_FEWER}"
        )
    return {"complete": report.complete, "scannedRepositories": scanned, "failedRepositories": failed, "agents": agents}


def encode_body(body: Mapping[str, Any]) -> bytes:
    """``body`` as the UTF-8 JSON the HTTP client sends. Raises PublishError when it is over 8 MiB or unencodable."""
    try:
        data = json.dumps(body, ensure_ascii=False, allow_nan=False).encode("utf-8")
    except (TypeError, ValueError):  # UnicodeEncodeError (a lone surrogate) is a ValueError.
        raise PublishError("The results cannot be written as JSON, so they cannot be uploaded.") from None
    if len(data) > MAX_BODY_BYTES:
        raise PublishError(
            f"The results are {len(data)} bytes, and Agent Manager takes at most {MAX_BODY_BYTES}, "
            f"so they cannot be uploaded. {_FEWER}"
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

    def publish(self, report: Report) -> PublishResult:
        """Send ``report``'s confirmed agents and repository lists. Raises PublishError."""
        return self.publish_body(report.scan_id, build_body(report))

    def publish_body(self, scan_id: str, body: Mapping[str, Any]) -> PublishResult:
        """Send ``body`` (from ``build_body``, or a results file's ``results``) to scan ``scan_id``.
        Raises PublishError."""
        encode_body(body)  # The size check; the client encodes it again the same way.
        if self._expires_at is not None and self._now() >= self._expires_at.timestamp():
            raise PublishError("The scan token has expired.", ExitCode.AUTH_ERROR, console_can_upload=True)
        url = self._amp_url + PUBLISH_PATH.format(scan_id=quote(scan_id, safe=""))
        try:
            # Retried on 5xx and network errors: a repeat after a lost answer is refused (409), never saved twice.
            response = self._http.post_json(url, body, headers={"Authorization": f"Bearer {self._token}"}, retry=True)
        except HttpError as exc:
            raise _error(exc) from None
        data = response.data
        status = data.get("status") if isinstance(data, dict) else None
        count = data.get("findingsCount") if isinstance(data, dict) else None
        if not isinstance(status, str) or not isinstance(count, int) or isinstance(count, bool):
            raise PublishError("Agent Manager sent an unexpected answer to the upload.")
        return PublishResult("".join(c for c in status if c.isprintable())[:40], count)


def published_line(result: PublishResult) -> str:
    """The line printed after a successful upload."""
    agents = "1 agent" if result.findings_count == 1 else f"{result.findings_count} agents"
    return f"Uploaded to Agent Manager: scan {result.status}, {agents} saved."


def report_exit_code(report: Report) -> ExitCode:
    """OK when every repository was scanned and the list was complete, else PARTIAL_FAILURE.

    Also PARTIAL_FAILURE when every repository failed: the results were still saved (and Agent Manager marks
    the scan failed once they arrive), so the command finished, but not its work.
    """
    if not report.complete or any(r.status == RepositoryStatus.FAILED for r in report.repositories):
        return ExitCode.PARTIAL_FAILURE
    return ExitCode.OK


def body_exit_code(body: Mapping[str, Any]) -> ExitCode:
    """``report_exit_code`` for a publish body: PARTIAL_FAILURE when it is incomplete or names a failed repository."""
    if body.get("complete") is not True or body.get("failedRepositories"):
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


TOKEN_EXPIRED_409 = "the scan token of this scan has expired"
"""Part of Agent Manager's 409 message for a scan older than a scan token lives: the console can still upload."""


def _error(exc: HttpError) -> PublishError:
    said = f" Agent Manager said: {exc.server_message}" if exc.server_message else ""
    status = exc.status
    if status in (401, 403):
        return PublishError(
            f"Agent Manager refused the scan token (HTTP {status}): it has expired, or it is not this scan's token.",
            ExitCode.AUTH_ERROR,
            console_can_upload=True,
        )
    if status == 404:
        return PublishError(
            "Agent Manager has no source code scan with this ID for the token's organization, or source code "
            "scans are turned off on this Agent Manager (HTTP 404). Check AMP_URL and the scan ID.",
            ExitCode.CONFIG_ERROR,
        )
    if status == 409 and TOKEN_EXPIRED_409 in (exc.server_message or ""):
        return PublishError(
            "Agent Manager says this scan's scan token has expired (HTTP 409).", console_can_upload=True
        )
    if status == 409:
        return PublishError(
            "The scan is no longer waiting for results (HTTP 409): it already has results, it was cancelled, "
            "or a newer scan of this platform uploaded first. If an earlier attempt reached Agent Manager, "
            f"the results are already saved there.{said}"
        )
    if status in (400, 413):
        return PublishError(f"Agent Manager refused the results (HTTP {status}).{said}")
    return PublishError(f"Could not upload the results ({exc}).", console_can_upload=True)
