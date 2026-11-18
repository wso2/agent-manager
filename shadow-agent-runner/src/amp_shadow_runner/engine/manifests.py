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

"""Read the packages that Python dependency manifests declare.

Only package names are read, never versions, URLs or anything else. Supported: ``requirements*.txt`` (and
``*.txt`` under a ``requirements/`` folder), ``pyproject.toml`` (``[project]`` dependencies, optional
dependencies and dependency groups, and Poetry dependencies and groups), ``Pipfile`` and ``setup.cfg``. A
manifest that cannot be read or parsed declares nothing. Names are compared normalised (PEP 503: case,
``-``, ``_`` and ``.`` do not matter).

``service_folder`` places a file in the service it belongs to: the folder of the nearest manifest of any
kind (``package.json`` and ``*.csproj`` too) at or above it.
"""

import configparser
import re
import tomllib
from collections.abc import Iterable, Iterator, Mapping
from dataclasses import dataclass
from pathlib import PurePosixPath

_PEP508_NAME = re.compile(r"\s*([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)")
_SEPARATORS = re.compile(r"[-_.]+")
_TOML_MANIFESTS = frozenset({"pyproject.toml", "Pipfile"})


def normalise(name: str) -> str:
    """A package name in PEP 503 form: lower case, runs of ``-``, ``_`` and ``.`` as one ``-``."""
    return _SEPARATORS.sub("-", name).lower()


def is_python_manifest(path: str) -> bool:
    """Whether the walker's manifest at ``path`` is one this module reads."""
    name = PurePosixPath(path).name
    return name in _TOML_MANIFESTS or name == "setup.cfg" or name.endswith(".txt")


def python_packages(path: str, data: bytes) -> frozenset[str]:
    """The normalised package names the manifest at ``path`` (a walker manifest) declares."""
    name = PurePosixPath(path).name
    text = data.decode("utf-8", errors="replace")
    try:
        if name == "pyproject.toml":
            found: Iterable[str] = _pyproject(tomllib.loads(text))
        elif name == "Pipfile":
            document = tomllib.loads(text)
            found = [*_keys(document.get("packages")), *_keys(document.get("dev-packages"))]
        elif name == "setup.cfg":
            found = _setup_cfg(text)
        elif name.endswith(".txt"):
            found = _requirement_names(text.splitlines())
        else:
            return frozenset()
        return frozenset(normalise(n) for n in found)
    except (tomllib.TOMLDecodeError, configparser.Error):
        return frozenset()


def _requirement_name(line: str) -> str | None:
    """The package name a PEP 508 requirement (or a requirements.txt line) starts with, or None."""
    line = line.split("#", 1)[0].strip()
    if not line or line.startswith("-"):
        return None  # Options such as ``-r other.txt`` and ``-e path``.
    match = _PEP508_NAME.match(line)
    return match.group(1) if match else None


def _requirement_names(lines: Iterable[object]) -> Iterator[str]:
    for line in lines:
        if isinstance(line, str) and (name := _requirement_name(line)) is not None:
            yield name


def _keys(table: object) -> list[str]:
    return [k for k in table if isinstance(k, str)] if isinstance(table, Mapping) else []


def _lists(table: object) -> Iterator[object]:
    if isinstance(table, Mapping):
        for value in table.values():
            if isinstance(value, list):
                yield from value


def _pyproject(document: Mapping[str, object]) -> Iterator[str]:
    project = document.get("project")
    if isinstance(project, Mapping):
        dependencies = project.get("dependencies")
        if isinstance(dependencies, list):
            yield from _requirement_names(dependencies)
        yield from _requirement_names(_lists(project.get("optional-dependencies")))
    yield from _requirement_names(_lists(document.get("dependency-groups")))
    tool = document.get("tool")
    poetry = tool.get("poetry") if isinstance(tool, Mapping) else None
    if isinstance(poetry, Mapping):
        yield from (k for k in _keys(poetry.get("dependencies")) if k.lower() != "python")
        yield from _keys(poetry.get("dev-dependencies"))
        groups = poetry.get("group")
        if isinstance(groups, Mapping):
            for group in groups.values():
                if isinstance(group, Mapping):
                    yield from _keys(group.get("dependencies"))


def _setup_cfg(text: str) -> list[str]:
    parser = configparser.ConfigParser(interpolation=None)
    parser.read_string(text)
    values: list[str] = []
    if parser.has_option("options", "install_requires"):
        values.append(parser.get("options", "install_requires"))
    if parser.has_section("options.extras_require"):
        values.extend(value for _, value in parser.items("options.extras_require"))
    return list(_requirement_names(line for value in values for line in value.splitlines()))


def service_folder(path: str, manifests: Iterable[str]) -> str | None:
    """The folder of the nearest manifest at or above the file at ``path``: ``.`` for the repository root,
    or None when no manifest is above it. In a monorepo it tells which service a file belongs to."""
    folders = PurePosixPath(path).parents
    above = [PurePosixPath(m).parent for m in manifests if PurePosixPath(m).parent in folders]
    return str(max(above, key=lambda f: len(f.parts))) if above else None


@dataclass(frozen=True)
class Dependencies:
    """The packages each manifest of a repository declares, by manifest path."""

    manifests: Mapping[str, frozenset[str]]
    """The manifests that were read."""
    unread: frozenset[str] = frozenset()
    """Manifests not read (another language's, or unreadable): they still mark a service folder."""

    @property
    def paths(self) -> tuple[str, ...]:
        """Every manifest path, read or not."""
        return (*self.manifests, *sorted(self.unread))

    def declaring(self, packages: Iterable[str], path: str) -> str | None:
        """The manifest that declares one of ``packages`` for the file at ``path``, or None.

        When a read manifest is in a folder above the file, only the nearest such folder's manifests count:
        the file belongs to that project. Only a file with no manifest above it falls back to the first
        declaring manifest by path, since a repository may declare its dependencies away from its code.
        """
        wanted = {normalise(p) for p in packages}
        folders = PurePosixPath(path).parents
        above = [m for m in self.manifests if PurePosixPath(m).parent in folders]
        if above:
            depth = max(len(PurePosixPath(m).parent.parts) for m in above)
            candidates = [m for m in above if len(PurePosixPath(m).parent.parts) == depth]
        else:
            candidates = list(self.manifests)
        declaring = sorted(m for m in candidates if self.manifests[m] & wanted)
        return declaring[0] if declaring else None
