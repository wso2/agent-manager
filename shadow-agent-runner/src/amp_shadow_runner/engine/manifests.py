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

"""Read the packages that dependency manifests declare.

Only package names are read, never versions, URLs or anything else. A manifest that cannot be read or
parsed declares nothing. Names are compared normalised (PEP 503: case, ``-``, ``_`` and ``.`` do not
matter), whatever the ecosystem. Each manifest belongs to one ``Ecosystem``, so a Python file is never
proven by a ``package.json`` that names a package of the same name.

- Python: ``requirements*.txt`` (and ``*.txt`` under a ``requirements/`` folder), ``pyproject.toml``
  (``[project]`` dependencies, optional dependencies and dependency groups, and Poetry dependencies and
  groups), ``Pipfile`` and ``setup.cfg``.
- npm: ``package.json`` ``dependencies``, ``devDependencies`` and ``optionalDependencies``. Not
  ``peerDependencies``: a peer dependency is one the package expects its host to install, as a plugin or
  library does, so it says the framework is used somewhere else rather than here.
- NuGet: ``*.csproj`` ``<PackageReference Include>``, ``Directory.Packages.props`` ``<PackageVersion
  Include>`` and ``packages.config`` ``<package id>``. ``csproj_usings`` also reads a ``.csproj``'s
  ``<Using Include>`` items, the project-wide usings. These XML files are parsed only when they are UTF-8
  and hold no ``<!DOCTYPE`` or ``<!ENTITY``, so entity expansion is never possible; any other file cannot
  be read (``read_packages`` gives None).
- Ballerina: ``Ballerina.toml`` ``[[dependency]]`` entries, as ``org/name``.

``service_folder`` places a file in the service it belongs to: the folder of the nearest manifest of any
kind (``package.json`` and ``*.csproj`` too) at or above it.

``project_name`` reads one more field, the project's own name, for naming agents (see ``naming``):
``pyproject.toml`` ``[project] name`` (else ``[tool.poetry] name``), ``package.json`` ``name`` without an
npm ``@scope/`` (none for a workspace root, one with a ``workspaces`` key, which names no single service),
``Ballerina.toml`` ``[package] name``, and the file name of a ``*.csproj`` without its extension (the file
is not parsed for it). The value must be a short plain string (``plain_name``).
"""

import configparser
import json
import re
import tomllib
import xml.etree.ElementTree as ElementTree
from collections.abc import Iterable, Iterator, Mapping
from dataclasses import dataclass
from enum import StrEnum
from pathlib import PurePosixPath

from amp_shadow_runner.engine.imports import Import, ImportKind

_PEP508_NAME = re.compile(r"\s*([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)")
_SEPARATORS = re.compile(r"[-_.]+")
_TOML_MANIFESTS = frozenset({"pyproject.toml", "Pipfile"})
_REQUIREMENTS_FOLDER = PurePosixPath("requirements")


_NPM_SECTIONS = ("dependencies", "devDependencies", "optionalDependencies")


_NUGET_NAMES = frozenset({"directory.packages.props", "packages.config"})
MAX_PROJECT_NAME_CHARS = 100
"""The longest project or folder name used as an agent's name; a longer one is not a name."""
_DTD = re.compile(r"<!(DOCTYPE|ENTITY)", re.IGNORECASE)
_MSBUILD_PROPERTY = re.compile(r"[$@%]\(")
"""An MSBuild property, item or metadata reference, ``$(Name)``: not a literal package name."""


class Ecosystem(StrEnum):
    """The package ecosystem of a manifest, and of the source files it declares packages for."""

    PYTHON = "python"
    NPM = "npm"
    NUGET = "nuget"
    BALLERINA = "ballerina"


def ecosystem(path: str) -> Ecosystem | None:
    """The ecosystem of the walker's manifest at ``path``, or None when this module does not read it."""
    name = PurePosixPath(path).name
    if is_python_manifest(path):
        return Ecosystem.PYTHON
    if name == "package.json":
        return Ecosystem.NPM
    if name.lower() in _NUGET_NAMES or name.lower().endswith(".csproj"):
        return Ecosystem.NUGET
    if name == "Ballerina.toml":
        return Ecosystem.BALLERINA
    return None


def read_packages(path: str, data: bytes) -> frozenset[str] | None:
    """The normalised package names the manifest at ``path`` declares, whatever its ecosystem; None when an
    XML manifest cannot be read safely (see the module docstring)."""
    kind = ecosystem(path)
    if kind == Ecosystem.NPM:
        return npm_packages(data)
    if kind == Ecosystem.NUGET:
        return nuget_packages(data)
    if kind == Ecosystem.BALLERINA:
        return ballerina_packages(data)
    return python_packages(path, data)


def _msbuild(data: bytes) -> ElementTree.Element | None:
    """The root element of an MSBuild or NuGet XML file, or None when it is not UTF-8, declares a DTD or
    entities, or does not parse."""
    try:
        text = data.decode("utf-8-sig")
    except UnicodeDecodeError:
        return None
    if _DTD.search(text):
        return None
    try:
        return ElementTree.fromstring(text)  # A str: expat ignores any encoding declaration.
    except ElementTree.ParseError:
        return None


def _elements(root: ElementTree.Element, tag: str) -> Iterator[ElementTree.Element]:
    """The elements named ``tag`` under ``root``, with or without the MSBuild XML namespace."""
    for element in root.iter():
        if isinstance(element.tag, str) and element.tag.rsplit("}", 1)[-1] == tag:
            yield element


def _includes(value: str | None) -> list[str]:
    """The literal names in an MSBuild ``Include`` value (``a;b``), without property references."""
    if value is None:
        return []
    return [v.strip() for v in value.split(";") if v.strip() and not _MSBUILD_PROPERTY.search(v)]


def nuget_packages(data: bytes) -> frozenset[str] | None:
    """The normalised package names a ``.csproj``, ``Directory.Packages.props`` or ``packages.config``
    declares, or None when it cannot be read safely."""
    root = _msbuild(data)
    if root is None:
        return None
    names = [
        *(n for e in _elements(root, "PackageReference") for n in _includes(e.get("Include"))),
        *(n for e in _elements(root, "PackageVersion") for n in _includes(e.get("Include"))),
        *(n for e in _elements(root, "package") for n in _includes(e.get("id"))),
    ]
    return frozenset(normalise(n) for n in names)


def csproj_usings(data: bytes) -> tuple[Import, ...]:
    """The project-wide usings of a ``.csproj``: its ``<Using Include="..."/>`` items, as ``global using``
    imports (``Alias`` makes an alias, ``Static="true"`` a static using). Empty when it cannot be read."""
    root = _msbuild(data)
    if root is None:
        return ()
    found: list[Import] = []
    for element in _elements(root, "Using"):
        for name in _includes(element.get("Include")):
            alias = element.get("Alias")
            if alias:
                found.append(Import(name, None, alias.strip(), ImportKind.ALIAS, 0, global_=True))
            elif (element.get("Static") or "").strip().lower() == "true":
                found.append(Import(name, None, None, ImportKind.STATIC, 0, global_=True))
            else:
                found.append(Import(name, None, None, ImportKind.NAMESPACE, 0, global_=True))
    return tuple(found)


def ballerina_packages(data: bytes) -> frozenset[str]:
    """The normalised ``org/name`` of each ``[[dependency]]`` in a ``Ballerina.toml``."""
    try:
        document = tomllib.loads(data.decode("utf-8", errors="replace"))
    except tomllib.TOMLDecodeError:
        return frozenset()
    entries = document.get("dependency")
    if not isinstance(entries, list):
        return frozenset()
    return frozenset(
        normalise(f"{e['org']}/{e['name']}")
        for e in entries
        if isinstance(e, Mapping) and isinstance(e.get("org"), str) and isinstance(e.get("name"), str)
    )


def npm_packages(data: bytes) -> frozenset[str]:
    """The normalised package names a ``package.json`` declares (see the module docstring)."""
    try:
        document = json.loads(data.decode("utf-8", errors="replace"))
    except (ValueError, RecursionError):  # Invalid JSON, or nested too deeply to parse.
        return frozenset()
    if not isinstance(document, Mapping):
        return frozenset()
    return frozenset(normalise(name) for section in _NPM_SECTIONS for name in _keys(document.get(section)))


def project_name(path: str, data: bytes) -> str | None:
    """The project's own name that the manifest at ``path`` states (see the module docstring), or None."""
    name = PurePosixPath(path).name
    if name.lower().endswith(".csproj"):
        return plain_name(name[: -len(".csproj")])
    if name not in ("pyproject.toml", "package.json", "Ballerina.toml"):
        return None
    text = data.decode("utf-8", errors="replace")
    try:
        document = json.loads(text) if name == "package.json" else tomllib.loads(text)
    except (ValueError, RecursionError):  # TOMLDecodeError is a ValueError.
        return None
    if not isinstance(document, Mapping):
        return None
    if name == "package.json":
        if "workspaces" in document:  # A monorepo root: each workspace is its own service.
            return None
        value = document.get("name")
        if isinstance(value, str) and value.startswith("@"):  # ``@scope/pkg``: the package is ``pkg``.
            value = value.partition("/")[2]
        return plain_name(value)
    if name == "Ballerina.toml":
        return plain_name(_table(document, "package").get("name"))
    tool = _table(document, "tool")
    return plain_name(_table(document, "project").get("name")) or plain_name(_table(tool, "poetry").get("name"))


def plain_name(value: object) -> str | None:
    """``value`` stripped, when it is a printable string of 1 to MAX_PROJECT_NAME_CHARS characters with no
    backslash; None for anything else."""
    if not isinstance(value, str):
        return None
    value = value.strip()
    if not value or len(value) > MAX_PROJECT_NAME_CHARS or "\\" in value or not value.isprintable():
        return None
    return value


def _table(document: Mapping[str, object], key: str) -> Mapping[str, object]:
    value = document.get(key)
    return value if isinstance(value, Mapping) else {}


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

    def declaring(self, packages: Iterable[str], path: str, *, strict: bool = False) -> str | None:
        """The manifest that declares one of ``packages`` for the file at ``path``, or None.

        When a read manifest is in a folder above the file, only the nearest such folder's manifests count:
        the file belongs to that project. Only a file with no manifest above it falls back to the first
        declaring manifest by path, since a repository may declare its dependencies away from its code.
        With ``strict`` (proof that a symbol is the framework's), that fallback takes only a manifest in a
        root ``requirements/`` folder (one at the root is always above the file), never another service's.
        A package ending in ``.*`` (``Microsoft.SemanticKernel.*``) stands for every package under it.
        """
        wanted = {normalise(p) for p in packages}
        exact = {w for w in wanted if not w.endswith("*")}
        prefixes = tuple(w[:-1] for w in wanted if w.endswith("*"))
        folders = PurePosixPath(path).parents
        above = [m for m in self.manifests if PurePosixPath(m).parent in folders]
        if above:
            depth = max(len(PurePosixPath(m).parent.parts) for m in above)
            candidates = [m for m in above if len(PurePosixPath(m).parent.parts) == depth]
        elif strict:
            candidates = [m for m in self.manifests if PurePosixPath(m).parent == _REQUIREMENTS_FOLDER]
        else:
            candidates = list(self.manifests)
        declaring = sorted(
            m
            for m in candidates
            if self.manifests[m] & exact or (prefixes and any(p.startswith(prefixes) for p in self.manifests[m]))
        )
        return declaring[0] if declaring else None
