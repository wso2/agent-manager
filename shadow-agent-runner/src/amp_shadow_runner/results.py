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

"""The results file, ``/out/results-<scanId>.json``: what Agent Manager receives, saved for upload.

Every scan writes it next to the report, also when no agent was found. The user uploads it in the Agent
Manager console (Shadow Agent Discovery, Scans, Upload results), or the runner uploads it directly with a
scan token. ``results`` is exactly the body ``publish.Publisher`` sends (``publish.build_body``): confirmed
agents and the repository lists, no Not-sure findings, review reasons, code or secrets. ``summary`` is for
display only; Agent Manager never trusts it::

    {"format": "amp-shadow-scan-results", "version": 1, "scanId": "<uuid>", "createdAt": "<RFC 3339 UTC>",
     "summary": {"organization": "acme", "repositoriesScanned": 12, "repositoriesFailed": 1, "agents": 7},
     "results": {"complete": true, "scannedRepositories": [...], "failedRepositories": [...], "agents": [...]}}

It is written atomically, like the report.
"""

import json
from collections.abc import Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, cast

from amp_shadow_runner.report import Report, ReportError, write_text_atomically

RESULTS_FORMAT = "amp-shadow-scan-results"
RESULTS_VERSION = 1
CONSOLE_PLACE = "Shadow Agent Discovery, Scans, Upload results"
"""Where the console takes a results file."""


@dataclass(frozen=True)
class SavedResults:
    """A results file read back: its scan and the body to send."""

    scan_id: str
    body: dict[str, Any]


def results_path(report_path: Path, scan_id: str) -> Path:
    """The results file next to ``report_path``: ``results-<scan id in lowercase>.json``."""
    return report_path.parent / f"results-{scan_id.lower()}.json"


def results_json(report: Report, body: Mapping[str, Any], *, created_at: datetime) -> dict[str, object]:
    """The results file's JSON for ``report``, whose publish body is ``body``."""
    return {
        "format": RESULTS_FORMAT,
        "version": RESULTS_VERSION,
        "scanId": report.scan_id.lower(),
        "createdAt": created_at.astimezone(UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
        "summary": {
            "organization": report.organization,
            "repositoriesScanned": len(body["scannedRepositories"]),
            "repositoriesFailed": len(body["failedRepositories"]),
            "agents": len(body["agents"]),
        },
        "results": dict(body),
    }


def write_results(report: Report, body: Mapping[str, Any], path: Path, *, created_at: datetime) -> Path:
    """Write the results file atomically and return ``path``. ``body`` must have passed
    ``publish.encode_body``, so it encodes as UTF-8. Raises ReportError when the folder is not writable."""
    text = json.dumps(results_json(report, body, created_at=created_at), indent=2, ensure_ascii=False) + "\n"
    return write_text_atomically(text, path, what="the results file")


def is_results(data: object) -> bool:
    """True when ``data`` claims to be a results file (it has ``format``); a report has none."""
    return isinstance(data, dict) and "format" in data


def parse_results(data: object, path: Path) -> SavedResults:
    """Read a results file's JSON. Raises ReportError when it is not one this runner can upload.

    Only the shape is checked here; Agent Manager validates the body itself.
    """
    obj = cast(dict[str, object], data) if isinstance(data, dict) else {}
    problem = None
    if obj.get("format") != RESULTS_FORMAT:
        problem = "an unknown format"
    elif obj.get("version") != RESULTS_VERSION:
        problem = "an unknown version"
    elif not isinstance(obj.get("scanId"), str):
        problem = "scanId is missing"
    else:
        body = obj.get("results")
        if not isinstance(body, dict):
            problem = "results is missing"
        elif not isinstance(body.get("complete"), bool):
            problem = "results.complete is missing"
        elif not all(isinstance(body.get(k), list) for k in ("scannedRepositories", "failedRepositories", "agents")):
            problem = "a list in results is missing"
    if problem is not None:
        raise ReportError(f"{path} is not a results file from this runner ({problem}).")
    return SavedResults(cast(str, obj["scanId"]), cast(dict[str, Any], obj["results"]))
