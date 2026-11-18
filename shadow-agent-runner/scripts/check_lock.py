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

"""Hermetic check that the committed lock agrees with requirements.in and pyproject.toml.

It reads committed files only (no PyPI), like python-instrumentation-provider's check-locks, so it
gives the same answer over time. Refreshing the lock is a human-run step (``make lock``). It fails when:

- a requirements.in line is not an exact ``name==version`` pin;
- requirements.in and ``[project].dependencies`` in pyproject.toml differ;
- a requirements.in pin is missing from the lock or locked at another version;
- a lock entry has no ``--hash`` (the image installs with ``--require-hashes``);
- a lock entry says it comes from requirements.in but is not in it, or cannot be traced (no ``# via``)
  to requirements.in or to another locked package.

Exit status: 0 pass, 1 mismatch, 2 bad input.
"""

from __future__ import annotations

import argparse
import re
import sys
import tomllib
from collections.abc import Sequence
from dataclasses import dataclass, field
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_REQUIREMENTS = ROOT / "requirements.in"
DEFAULT_LOCK = ROOT / "locks" / "requirements-linux-py312.txt"
DEFAULT_PYPROJECT = ROOT / "pyproject.toml"

_PIN = re.compile(r"^([A-Za-z0-9][A-Za-z0-9._-]*)==([A-Za-z0-9][A-Za-z0-9.+!_-]*)$")
_LOCK_ENTRY = re.compile(r"^([A-Za-z0-9][A-Za-z0-9._-]*)==([^\s;\\]+)")
_FROM_REQUIREMENTS = "-r requirements.in"


@dataclass
class LockEntry:
    version: str
    hashes: int = 0
    via: list[str] = field(default_factory=list)


class InputError(Exception):
    """A required input file is missing or malformed."""


def canonical_name(name: str) -> str:
    """Return the PEP 503 normalised project name."""
    return re.sub(r"[-_.]+", "-", name).lower()


def parse_pins(lines: Sequence[str], source: str) -> dict[str, str]:
    """Parse exact ``name==version`` pins; anything else is an input error."""
    pins: dict[str, str] = {}
    for raw in lines:
        line = raw.split("#", 1)[0].strip()
        if not line:
            continue
        match = _PIN.match(line.replace(" ", ""))
        if not match:
            raise InputError(f"{source}: {raw.strip()!r} is not an exact 'name==version' pin")
        pins[canonical_name(match.group(1))] = match.group(2)
    return pins


def parse_lock(text: str) -> dict[str, LockEntry]:
    """Parse a uv/pip-compile lock with hashes and ``# via`` annotations."""
    entries: dict[str, LockEntry] = {}
    current: LockEntry | None = None
    in_via_block = False
    for line in text.splitlines():
        match = _LOCK_ENTRY.match(line)
        if match:
            current = LockEntry(version=match.group(2))
            entries[canonical_name(match.group(1))] = current
            in_via_block = False
            continue
        stripped = line.strip()
        if current is None or not stripped:
            continue
        if stripped.startswith("--hash="):
            current.hashes += 1
        elif stripped == "# via":
            in_via_block = True
        elif stripped.startswith("# via "):
            current.via.append(stripped.removeprefix("# via ").strip())
            in_via_block = False
        elif in_via_block and stripped.startswith("#"):
            current.via.append(stripped.lstrip("#").strip())
    return entries


def compare(requirements: dict[str, str], project_deps: dict[str, str], lock: dict[str, LockEntry]) -> list[str]:
    """Return every disagreement between the three inputs."""
    problems: list[str] = []
    for name in sorted(requirements.keys() | project_deps.keys()):
        in_req, in_proj = requirements.get(name), project_deps.get(name)
        if in_req != in_proj:
            problems.append(
                f"DRIFT: {name} is {in_req or 'absent'} in requirements.in but {in_proj or 'absent'} in pyproject.toml"
            )
    for name, version in sorted(requirements.items()):
        entry = lock.get(name)
        if entry is None:
            problems.append(f"MISSING: {name}=={version} is in requirements.in but not in the lock; run `make lock`")
        elif entry.version != version:
            problems.append(f"MISMATCH: {name} is {version} in requirements.in but {entry.version} in the lock")
    for name, entry in sorted(lock.items()):
        if entry.hashes == 0:
            problems.append(f"NO HASH: {name}=={entry.version} has no --hash; run `make lock`")
        if not entry.via:
            problems.append(f"UNTRACED: {name}=={entry.version} has no '# via' annotation; run `make lock`")
            continue
        if _FROM_REQUIREMENTS in entry.via and name not in requirements:
            problems.append(f"ORPHAN: {name} is locked as coming from requirements.in, which no longer lists it")
        parents = [canonical_name(v) for v in entry.via if v != _FROM_REQUIREMENTS]
        if _FROM_REQUIREMENTS not in entry.via and not any(p in lock for p in parents):
            problems.append(f"ORPHAN: {name} is only needed by {', '.join(entry.via)}, none of which is locked")
    return problems


def run(requirements_file: Path, lock_file: Path, pyproject_file: Path) -> tuple[int, list[str]]:
    """Run the check and return (exit status, messages)."""
    try:
        requirements = parse_pins(requirements_file.read_text(encoding="utf-8").splitlines(), requirements_file.name)
        project = tomllib.loads(pyproject_file.read_text(encoding="utf-8")).get("project", {})
        project_deps = parse_pins(
            list(project.get("dependencies", [])), f"{pyproject_file.name} [project].dependencies"
        )
        lock = parse_lock(lock_file.read_text(encoding="utf-8"))
    except (OSError, tomllib.TOMLDecodeError, InputError) as exc:
        return 2, [f"ERROR: {exc}"]
    if not lock:
        return 2, [f"ERROR: lock file has no 'name==version' entries: {lock_file}"]
    problems = compare(requirements, project_deps, lock)
    if problems:
        return 1, problems
    return 0, [f"OK: {lock_file.name} matches requirements.in and pyproject.toml ({len(lock)} packages, all hashed)"]


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0] if __doc__ else None)
    parser.add_argument("--requirements", type=Path, default=DEFAULT_REQUIREMENTS)
    parser.add_argument("--lock", type=Path, default=DEFAULT_LOCK)
    parser.add_argument("--pyproject", type=Path, default=DEFAULT_PYPROJECT)
    args = parser.parse_args(argv)
    status, messages = run(args.requirements, args.lock, args.pyproject)
    stream = sys.stdout if status == 0 else sys.stderr
    for message in messages:
        print(message, file=stream)
    return status


if __name__ == "__main__":
    sys.exit(main())
