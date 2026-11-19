#!/usr/bin/env python3
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

"""Licence and denylist gate for the runner's runtime dependencies, and the notice-file generator.

The lock file is the exact set of packages the image ships. For each locked package this script
reads the licence from the installed distribution metadata (install the lock first) and fails when:

- the package is on the denylist (no exception can override it);
- its licence is not MIT or Apache-2.0 and its exact ``name==version`` is not a reviewed exception
  in ``licenses/exceptions.txt`` (GPL-family licences can never be excepted);
- it is not installed, or is installed at a different version than the lock pins;
- ``licenses/THIRD_PARTY_LICENSES.md`` differs from what the lock produces (stale notice).

An exception the lock does not need is also an error, so the file never keeps stale entries.

Usage:
    python scripts/check_deps.py            # check (CI)
    python scripts/check_deps.py --write    # regenerate the notice file, then check

Exit status: 0 pass, 1 gate failed, 2 bad input (missing or malformed file).
"""

from __future__ import annotations

import argparse
import importlib.metadata
import re
import sys
from collections.abc import Callable, Iterable, Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_LOCK = ROOT / "locks" / "requirements-linux-py312.txt"
DEFAULT_EXCEPTIONS = ROOT / "licenses" / "exceptions.txt"
DEFAULT_NOTICE = ROOT / "licenses" / "THIRD_PARTY_LICENSES.md"

ALLOWED_LICENSES = frozenset({"MIT", "Apache-2.0"})

# Canonical (PEP 503) name -> why it is denied.
DENYLIST: Mapping[str, str] = {
    "tree-sitter-language-pack": "bundles GPL-licensed and unlicensed grammars",
}

# Strong copyleft: never acceptable through licenses/exceptions.txt.
_COPYLEFT = re.compile(r"(?<![A-Za-z])(?:A|L)?GPL|SSPL|General Public License", re.IGNORECASE)

# Free-text License fields and trove classifiers mapped to SPDX identifiers.
_LICENSE_ALIASES: Mapping[str, str] = {
    "mit": "MIT",
    "mit license": "MIT",
    "the mit license": "MIT",
    "apache-2.0": "Apache-2.0",
    "apache 2.0": "Apache-2.0",
    "apache license 2.0": "Apache-2.0",
    "apache license, version 2.0": "Apache-2.0",
    "apache software license": "Apache-2.0",
    "apache software license 2.0": "Apache-2.0",
}
_CLASSIFIER_ALIASES: Mapping[str, str] = {
    "License :: OSI Approved :: MIT License": "MIT",
    "License :: OSI Approved :: Apache Software License": "Apache-2.0",
    "License :: OSI Approved :: Python Software Foundation License": "PSF-2.0",
}

_LOCK_ENTRY = re.compile(r"^([A-Za-z0-9][A-Za-z0-9._-]*)(?:\[[^\]]*\])?==([^\s;\\]+)")
_PREFERRED_URL_LABELS = ("source", "source code", "repository", "homepage", "home")


class Metadata(Protocol):
    """The part of ``importlib.metadata.PackageMetadata`` this script reads."""

    def get_all(self, name: str, failobj: None = None) -> list[Any] | None: ...


@dataclass(frozen=True)
class PackageInfo:
    """One locked package as it appears in the notice file."""

    name: str
    version: str
    license: str
    url: str


Lookup = Callable[[str], PackageInfo | None]
"""Return the installed package's notice row, or None when it is not installed."""


class InputError(Exception):
    """A required input file is missing or malformed."""


def canonical_name(name: str) -> str:
    """Return the PEP 503 normalised project name."""
    return re.sub(r"[-_.]+", "-", name).lower()


def parse_lock(text: str) -> dict[str, str]:
    """Return {canonical name: version} for every ``name==version`` entry in a pip-style lock."""
    locked: dict[str, str] = {}
    for line in text.splitlines():
        match = _LOCK_ENTRY.match(line)
        if match:
            locked[canonical_name(match.group(1))] = match.group(2)
    return locked


def parse_exceptions(text: str) -> set[str]:
    """Return the reviewed ``name==version`` exceptions; ``#`` starts a comment."""
    entries: set[str] = set()
    for number, raw in enumerate(text.splitlines(), start=1):
        line = raw.split("#", 1)[0].strip()
        if not line:
            continue
        name, sep, version = line.partition("==")
        if not sep or not name.strip() or not version.strip() or " " in version.strip():
            raise InputError(f"exceptions line {number}: expected 'name==version', got {raw.strip()!r}")
        entries.add(f"{canonical_name(name.strip())}=={version.strip()}")
    return entries


def _first(meta: Metadata, field: str) -> str | None:
    values = meta.get_all(field)
    if not values:
        return None
    value = str(values[0]).strip()
    return value or None


def detect_license(meta: Metadata) -> str:
    """Return an SPDX-style licence expression for a distribution, or ``UNKNOWN``.

    Order: the PEP 639 ``License-Expression`` field, then licence classifiers (several mean all must
    be allowed), then a short free-text ``License`` field.
    """
    expression = _first(meta, "License-Expression")
    if expression:
        return expression
    classifiers = [str(c) for c in meta.get_all("Classifier") or [] if str(c).startswith("License ::")]
    if classifiers:
        mapped = sorted({_CLASSIFIER_ALIASES.get(c, c.rsplit("::", 1)[-1].strip()) for c in classifiers})
        return " AND ".join(mapped)
    free_text = _first(meta, "License")
    if free_text and "\n" not in free_text and len(free_text) <= 64:
        return _LICENSE_ALIASES.get(free_text.lower(), free_text)
    return "UNKNOWN"


def project_url(name: str, meta: Metadata) -> str:
    """Return the package's source or home page, falling back to its PyPI page."""
    labelled: dict[str, str] = {}
    for entry in meta.get_all("Project-URL") or []:
        label, _, url = str(entry).partition(",")
        if url.strip():
            labelled.setdefault(label.strip().lower(), url.strip())
    for label in _PREFERRED_URL_LABELS:
        if label in labelled:
            return labelled[label]
    return _first(meta, "Home-page") or f"https://pypi.org/project/{name}/"


def installed_package(name: str) -> PackageInfo | None:
    """Look a package up in the running environment's distribution metadata."""
    try:
        dist = importlib.metadata.distribution(name)
    except importlib.metadata.PackageNotFoundError:
        return None
    meta = dist.metadata
    return PackageInfo(name=name, version=dist.version, license=detect_license(meta), url=project_url(name, meta))


def license_allowed(expression: str) -> bool:
    """True when some ``OR`` alternative of the expression consists only of allowed licences."""
    if "(" in expression or ")" in expression or " WITH " in expression:
        return False
    for alternative in re.split(r"\s+OR\s+", expression.strip()):
        terms = [term.strip() for term in re.split(r"\s+AND\s+", alternative)]
        if terms and all(term in ALLOWED_LICENSES for term in terms):
            return True
    return False


def evaluate(
    locked: Mapping[str, str],
    lookup: Lookup,
    exceptions: Iterable[str] = (),
) -> tuple[list[PackageInfo], list[str]]:
    """Check every locked package. Return the notice rows and the list of problems found."""
    exception_set = set(exceptions)
    used_exceptions: set[str] = set()
    rows: list[PackageInfo] = []
    problems: list[str] = []
    for name in sorted(locked):
        version = locked[name]
        pin = f"{name}=={version}"
        if name in DENYLIST:
            problems.append(f"DENIED: {pin} is on the denylist ({DENYLIST[name]}); remove it from the lock")
            continue
        info = lookup(name)
        if info is None:
            problems.append(f"MISSING: {pin} is locked but not installed; install the lock first")
            continue
        if info.version != version:
            problems.append(f"MISMATCH: {name} is locked at {version} but {info.version} is installed")
            continue
        rows.append(info)
        if license_allowed(info.license):
            continue
        if pin in exception_set and not _COPYLEFT.search(info.license):
            used_exceptions.add(pin)
            continue
        reason = (
            "copyleft licences cannot be excepted"
            if _COPYLEFT.search(info.license)
            else "it has no reviewed exception in licenses/exceptions.txt"
        )
        problems.append(f"LICENCE: {pin} is licensed {info.license!r}, not MIT or Apache-2.0; {reason}")
    for unused in sorted(exception_set - used_exceptions):
        problems.append(f"STALE EXCEPTION: {unused} in licenses/exceptions.txt is not needed by the lock; remove it")
    return rows, problems


def render_notice(rows: Sequence[PackageInfo], lock_name: str) -> str:
    """Render THIRD_PARTY_LICENSES.md, in the table shape of agent-manager-service's notice file."""
    lines = [
        "# Third-Party Licenses for shadow-agent-runner",
        "",
        f"<!-- Generated from locks/{lock_name} by `make licenses` (scripts/check_deps.py --write). Do not edit. -->",
        "",
        "Runtime dependencies shipped in the `amp-shadow-runner` image.",
        "Development tools are not shipped and are not listed.",
        "",
        "| Library | Version | License | Project |",
        "|---|---|---|---|",
    ]
    lines += [f"| {r.name} | {r.version} | {r.license} | {r.url} |" for r in sorted(rows, key=lambda r: r.name)]
    return "\n".join(lines) + "\n"


def _read(path: Path, what: str) -> str:
    try:
        return path.read_text(encoding="utf-8")
    except FileNotFoundError as exc:
        raise InputError(f"{what} not found: {path}") from exc


def run(
    lock: Path,
    exceptions_file: Path,
    notice: Path,
    write: bool,
    lookup: Lookup = installed_package,
) -> tuple[int, list[str]]:
    """Run the gate and return (exit status, messages)."""
    try:
        locked = parse_lock(_read(lock, "lock file"))
        if not locked:
            raise InputError(f"lock file has no 'name==version' entries: {lock}")
        exceptions = parse_exceptions(_read(exceptions_file, "exceptions file"))
    except InputError as exc:
        return 2, [f"ERROR: {exc}"]

    rows, problems = evaluate(locked, lookup, exceptions)
    expected = render_notice(rows, lock.name)
    if write and not problems:
        notice.write_text(expected, encoding="utf-8")
    current = notice.read_text(encoding="utf-8") if notice.exists() else None
    if current != expected:
        problems.append(f"STALE NOTICE: {notice.name} is stale; run `make licenses` and commit the result")
    if problems:
        return 1, problems
    return 0, [f"OK: {len(rows)} runtime packages, all MIT or Apache-2.0 (or reviewed); {notice.name} is current"]


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0] if __doc__ else None)
    parser.add_argument("--lock", type=Path, default=DEFAULT_LOCK, help="hash-pinned lock to check")
    parser.add_argument("--exceptions", type=Path, default=DEFAULT_EXCEPTIONS, help="reviewed exceptions file")
    parser.add_argument("--notice", type=Path, default=DEFAULT_NOTICE, help="THIRD_PARTY_LICENSES.md path")
    parser.add_argument("--write", action="store_true", help="regenerate the notice file before checking it")
    args = parser.parse_args(argv)
    status, messages = run(args.lock, args.exceptions, args.notice, args.write)
    stream = sys.stdout if status == 0 else sys.stderr
    for message in messages:
        print(message, file=stream)
    return status


if __name__ == "__main__":
    sys.exit(main())
