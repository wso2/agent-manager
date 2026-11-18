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

"""Find the files of a source tree the engine scans, without reading them.

Three kinds of file are kept: source files (by extension, see ``languages``),
dependency manifests, and, only with ``read_ci``, CI pipeline definitions
(``.github/workflows/*.yml`` at the root, and ``azure-pipelines*.yml``
anywhere or any ``*.yml`` at any depth under a root ``.azure-pipelines/``
folder).

Never kept: ``.env`` files in any form, minified bundles, links and other
non-regular files, and files over the size cap. Lock files are not kept
either, since no rule matches them. Folders of dependencies, build output,
caches and virtualenvs are not entered (by name, see ``PRUNED_FOLDERS``), nor
are hidden folders other than ``.github`` and ``.azure-pipelines``: they hold
tool state (``.yarn``, ``.terraform``, ``.cache``), not an application's code.
``bin`` is entered: in a repository it holds hand-written scripts and CLI
entry points, since .NET build output is not committed.

Only what the scan would have read counts as skipped: links (to files or
folders), other non-regular entries with a name a rule matches, files over
the cap, and folders that cannot be listed. Files no rule matches, and
everything in a pruned folder, are ignored, not skipped.
"""

import os
import re
from collections import Counter
from collections.abc import Mapping
from dataclasses import dataclass
from enum import StrEnum
from pathlib import Path
from typing import Literal

from amp_shadow_runner.engine.languages import Language, language_for

DEFAULT_MAX_FILE_BYTES = 1024 * 1024
"""The largest file kept (1 MiB). Hand-written source is far smaller; bigger files are generated or data."""

Tag = Literal["test", "example"]

PRUNED_FOLDERS = frozenset(
    {
        "node_modules",
        "bower_components",
        "vendor",
        "venv",
        "site-packages",
        "__pycache__",
        "dist",
        "build",
        "out",
        "target",
        "obj",
    }
)
"""Folder names (case-insensitive) never entered: dependencies, build output and caches. Hidden folders are
pruned separately."""
_VISIBLE_DOT_FOLDERS = frozenset({".github", ".azure-pipelines"})
_TEST_FOLDERS = frozenset({"__tests__", "spec", "specs", "testing"})
_TEST_FOLDER = re.compile(r"(^|[._-])(unit|integration)?tests?$")
"""Lower-case folder names such as ``tests``, ``Agents.Tests``, ``MyApp.UnitTests`` and ``integration_tests``."""
_EXAMPLE_FOLDERS = frozenset({"example", "examples", "sample", "samples", "demo", "demos"})
_TEST_FILE = re.compile(
    r"^test_.*\.py$|_test\.py$|^(tests?|conftest)\.py$|\.(test|spec)\.(js|jsx|mjs|cjs|ts|tsx|mts|cts)$|Tests?\.cs$"
)
_MINIFIED = re.compile(r"\.min\.(js|mjs|cjs)$|\.bundle\.js$")
_MANIFEST_NAMES = frozenset(
    {
        "pyproject.toml",
        "Pipfile",
        "setup.cfg",
        "package.json",
        "Ballerina.toml",
    }
)
_DOTNET_MANIFEST_NAMES = frozenset({"directory.packages.props", "packages.config"})
"""Compared in lower case, like ``*.csproj``: .NET tooling ignores case."""
_REQUIREMENTS = re.compile(r"^requirements.*\.txt$")
_YAML = (".yml", ".yaml")


class FileKind(StrEnum):
    """Why a file is kept."""

    SOURCE = "source"
    MANIFEST = "manifest"
    CI = "ci"


class SkipReason(StrEnum):
    """Why a file the scan would have read was not kept."""

    TOO_LARGE = "too_large"
    NOT_REGULAR = "not_regular"
    """A link (never followed), device, FIFO or socket."""
    UNREADABLE = "unreadable"
    """A folder that could not be listed, or a file that could not be stat'ed."""


@dataclass(frozen=True)
class RepoFile:
    """A kept file. Its contents are not read here."""

    path: str
    """Relative to the walked root, with ``/`` separators."""
    absolute: Path
    kind: FileKind
    language: Language | None
    """Set for source files only."""
    tag: Tag | None
    """``test`` or ``example`` when the file is in such a folder (or, for tests, named like one)."""


@dataclass(frozen=True)
class WalkResult:
    """What ``walk`` found."""

    files: tuple[RepoFile, ...]
    """Kept files, sorted by ``path``."""
    skipped_by_reason: Mapping[SkipReason, int]

    @property
    def skipped(self) -> int:
        return sum(self.skipped_by_reason.values())


def walk(root: Path, *, read_ci: bool = False, max_file_bytes: int = DEFAULT_MAX_FILE_BYTES) -> WalkResult:
    """List the files under ``root`` that the scan reads. Links are never followed.

    The walk is iterative, so a deep tree cannot exhaust the stack. An error
    listing ``root`` itself is raised; errors below it are counted as skipped.
    """
    root = Path(os.path.abspath(root))
    kept: list[RepoFile] = []
    skipped: Counter[SkipReason] = Counter()
    pending: list[tuple[str, ...]] = [()]
    while pending:
        folder = pending.pop()
        try:
            with os.scandir(root.joinpath(*folder)) as it:
                entries = list(it)
        except OSError:
            if not folder:
                raise
            skipped[SkipReason.UNREADABLE] += 1
            continue
        for entry in entries:
            parts = (*folder, entry.name)
            try:
                if entry.is_symlink():
                    skipped[SkipReason.NOT_REGULAR] += 1
                elif entry.is_dir(follow_symlinks=False):
                    if not _pruned(entry.name):
                        pending.append(parts)
                elif (kind := _kind(parts, read_ci)) is None:
                    continue
                elif not entry.is_file(follow_symlinks=False):
                    skipped[SkipReason.NOT_REGULAR] += 1
                elif entry.stat(follow_symlinks=False).st_size > max_file_bytes:
                    skipped[SkipReason.TOO_LARGE] += 1
                else:
                    language = language_for(entry.name) if kind is FileKind.SOURCE else None
                    kept.append(RepoFile("/".join(parts), Path(entry.path), kind, language, _tag(parts)))
            except OSError:
                skipped[SkipReason.UNREADABLE] += 1
    kept.sort(key=lambda f: f.path)
    return WalkResult(tuple(kept), dict(skipped))


def _pruned(name: str) -> bool:
    return (name.startswith(".") and name not in _VISIBLE_DOT_FOLDERS) or name.lower() in PRUNED_FOLDERS


def _kind(parts: tuple[str, ...], read_ci: bool) -> FileKind | None:
    """What the file at ``parts`` is kept as, or None when it is not kept."""
    name = parts[-1]
    lower = name.lower()
    if lower == ".env" or lower.startswith(".env.") or lower.endswith(".env"):
        return None
    if language_for(name) is not None:
        return None if _MINIFIED.search(lower) else FileKind.SOURCE
    if (
        name in _MANIFEST_NAMES
        or _REQUIREMENTS.match(name)
        or lower in _DOTNET_MANIFEST_NAMES
        or lower.endswith(".csproj")
        or (len(parts) > 1 and parts[-2] == "requirements" and lower.endswith(".txt"))
    ):
        return FileKind.MANIFEST
    if read_ci and lower.endswith(_YAML):
        top = parts[:-1]
        if top == (".github", "workflows") or top[:1] == (".azure-pipelines",) or lower.startswith("azure-pipelines"):
            return FileKind.CI
    return None


def _tag(parts: tuple[str, ...]) -> Tag | None:
    folders = {p.lower() for p in parts[:-1]}
    if folders & _TEST_FOLDERS or any(_TEST_FOLDER.search(f) for f in folders) or _TEST_FILE.search(parts[-1]):
        return "test"
    if folders & _EXAMPLE_FOLDERS:
        return "example"
    return None
