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

"""Scan one local source tree: walk it, parse its files, match signatures and classify the matches.

``scan_tree`` is pure: it reads files under the root and nothing else, opens no connection and starts no
process (``tests/engine/test_purity.py`` checks it).

Python, TypeScript, JavaScript, C# and Ballerina files are matched and count as scanned. A file that cannot
be read counts as skipped, with the walker's skipped files. A file with a syntax error is still matched
(tree-sitter recovers) and counts in ``parse_errors``; Ballerina, matched as text, has none.

Every source file with an import is matched, and what wires tools in it (``PythonMatches.wiring``,
``ScriptMatches.wiring``) is kept by path: tool wiring is a fact about a file, not about one graph in it.
The repository's Python files tell which imports are local modules (``RepoModules``): for the one import
hop of ``Condition.TOOLS`` and the local ``agents`` package of ``Condition.DEPENDENCY``.

C# needs every file of a project before matching one: a ``global using`` in any ``.cs`` file applies to
the whole project, as do the ``<Using Include>`` items of its ``.csproj``. A file's project is the folder
of the nearest ``.csproj`` above it, or the repository when there is none. So C# files are parsed once for
their imports, and only those that mention a signature's name (``match_cs.mentions``) are kept and parsed
again for matching once every file has been read.

Each language is classified with its own rule set and only the manifests of its own ecosystem (see
``manifests.Ecosystem``); the other manifests still mark service folders. A manifest that cannot be read
safely (``read_packages`` gives None) counts as not read.
"""

import functools
from dataclasses import dataclass, field
from pathlib import Path, PurePosixPath

from amp_shadow_runner.engine.classify import FileMatches, Finding, classify
from amp_shadow_runner.engine.files import FileKind, RepoFile, walk
from amp_shadow_runner.engine.imports import Import, ImportTable, extract_imports
from amp_shadow_runner.engine.languages import Language, parse
from amp_shadow_runner.engine.manifests import (
    Dependencies,
    Ecosystem,
    csproj_usings,
    ecosystem,
    read_packages,
    service_folder,
)
from amp_shadow_runner.engine.match import match_python_file
from amp_shadow_runner.engine.match_bal import match_ballerina_file
from amp_shadow_runner.engine.match_cs import match_csharp_file, mentions
from amp_shadow_runner.engine.match_ts import match_script_file
from amp_shadow_runner.engine.modules import RepoModules
from amp_shadow_runner.engine.signatures import RuleSet, Wiring
from amp_shadow_runner.engine.signatures.ballerina import BALLERINA
from amp_shadow_runner.engine.signatures.csharp import CSHARP
from amp_shadow_runner.engine.signatures.python import PYTHON
from amp_shadow_runner.engine.signatures.typescript import TYPESCRIPT

_SCRIPTS = frozenset({Language.JAVASCRIPT, Language.TYPESCRIPT, Language.TSX})


@dataclass(frozen=True)
class ScanResult:
    """What scanning one repository's tree found."""

    findings: tuple[Finding, ...]
    """Ordered by path, then line. Includes test and example findings (see ``Finding.tag``)."""
    files_scanned: int
    files_skipped: int
    parse_errors: int


@dataclass
class _Scan:
    """What scanning a tree has found so far."""

    skipped: int
    scanned: int = 0
    parse_errors: int = 0


@dataclass
class _CSharp:
    """The C# files of a tree, and the project-wide usings, until every file has been read."""

    projects: tuple[str, ...]
    """The ``.csproj`` paths of the tree."""
    files: list[tuple[RepoFile, bytes, ImportTable]] = field(default_factory=list)
    """The files that mention a signature's name, with their bytes and their own imports."""
    usings: dict[str | None, list[Import]] = field(default_factory=dict)
    """By project folder (None for the repository): ``global using`` directives and ``<Using>`` items."""

    def project(self, path: str) -> str | None:
        return service_folder(path, self.projects)

    def add_csproj(self, path: str, data: bytes) -> None:
        self.usings.setdefault(str(PurePosixPath(path).parent), []).extend(csproj_usings(data))

    def add(self, file: RepoFile, state: _Scan) -> None:
        data = _read(file, state)
        tree = None if data is None else parse(Language.C_SHARP, data)
        if data is None or tree is None:
            return
        state.scanned += 1
        state.parse_errors += tree.root_node.has_error
        imports = extract_imports(Language.C_SHARP, tree, data)
        self.usings.setdefault(self.project(file.path), []).extend(i for i in imports.imports if i.global_)
        if mentions(data):
            self.files.append((file, data, imports))

    def matches(self) -> list[FileMatches]:
        found: list[FileMatches] = []
        for file, data, own in self.files:
            usings = self.usings.get(self.project(file.path), [])
            imports = ImportTable(Language.C_SHARP, tuple(dict.fromkeys([*own.imports, *usings])))
            tree = parse(Language.C_SHARP, data)
            matched = () if tree is None else match_csharp_file(file.path, tree, data, imports)
            if matched:
                found.append(FileMatches(file.path, file.tag, imports, matched))
        return found


def scan_tree(root: Path, *, repository: str, read_ci: bool = False) -> ScanResult:
    """Scan the tree at ``root`` (one repository, named ``repository``) and return its findings.

    Raises OSError only when ``root`` itself cannot be listed.
    """
    walked = walk(root, read_ci=read_ci)
    state = _Scan(walked.skipped)
    manifests: dict[Ecosystem, dict[str, frozenset[str]]] = {e: {} for e in Ecosystem}
    unread: set[str] = set()
    python: list[FileMatches] = []
    scripts: list[FileMatches] = []
    ballerina: list[FileMatches] = []
    wiring: dict[str, frozenset[Wiring]] = {}
    csharp = _CSharp(tuple(f.path for f in walked.files if f.kind == FileKind.MANIFEST and _csproj(f.path)))
    modules = RepoModules(f.path for f in walked.files if f.kind == FileKind.SOURCE and f.language == Language.PYTHON)
    for file in walked.files:
        if file.kind == FileKind.MANIFEST:
            kind = ecosystem(file.path)
            data = _read(file, state) if kind is not None else None
            packages = None if kind is None or data is None else read_packages(file.path, data)
            if kind is None or data is None or packages is None:
                unread.add(file.path)  # Not read; it still marks a service folder.
            else:
                manifests[kind][file.path] = packages
            if data is not None and _csproj(file.path):
                csharp.add_csproj(file.path, data)
            continue
        if file.kind != FileKind.SOURCE or file.language is None:
            continue
        if file.language == Language.PYTHON:
            _scan_python(file, state, modules, python, wiring)
        elif file.language in _SCRIPTS:
            _scan_script(file, state, scripts, wiring)
        elif file.language == Language.C_SHARP:
            csharp.add(file, state)
        elif file.language == Language.BALLERINA:
            _scan_ballerina(file, state, ballerina)
    every = {path for found in manifests.values() for path in found} | unread

    def dependencies(kind: Ecosystem) -> Dependencies:
        return Dependencies(manifests[kind], frozenset(every - manifests[kind].keys()))

    findings = [
        *_classify(python, repository, dependencies(Ecosystem.PYTHON), PYTHON, wiring),
        *_classify(scripts, repository, dependencies(Ecosystem.NPM), TYPESCRIPT, wiring),
        *_classify(csharp.matches(), repository, dependencies(Ecosystem.NUGET), CSHARP, wiring),
        *_classify(ballerina, repository, dependencies(Ecosystem.BALLERINA), BALLERINA, wiring),
    ]
    findings.sort(key=lambda f: (f.path, f.line))
    return ScanResult(tuple(findings), state.scanned, state.skipped, state.parse_errors)


def _classify(
    files: list[FileMatches],
    repository: str,
    dependencies: Dependencies,
    rules: RuleSet,
    wiring: dict[str, frozenset[Wiring]],
) -> tuple[Finding, ...]:
    if not files:
        return ()
    return classify(files, repository=repository, dependencies=dependencies, rules=rules, wiring=wiring)


def _csproj(path: str) -> bool:
    return path.lower().endswith(".csproj")


def _read(file: RepoFile, state: _Scan) -> bytes | None:
    """The file's bytes, or None (counted as skipped) when it cannot be read."""
    try:
        return file.absolute.read_bytes()
    except OSError:
        state.skipped += 1
        return None


def _scan_python(
    file: RepoFile,
    state: _Scan,
    modules: RepoModules,
    found: list[FileMatches],
    wiring: dict[str, frozenset[Wiring]],
) -> None:
    data = _read(file, state)
    tree = None if data is None else parse(Language.PYTHON, data)
    if data is None or tree is None:  # Python always has a grammar.
        return
    state.scanned += 1
    state.parse_errors += tree.root_node.has_error
    imports = extract_imports(Language.PYTHON, tree, data)
    if not imports.imports:
        return  # Nothing to match, and wiring tools needs a langchain or langgraph import.
    matched = match_python_file(file.path, tree, data, imports, is_local=functools.partial(modules.is_local, file.path))
    if matched.wiring:
        wiring[file.path] = matched.wiring
    if matched.matches:
        found.append(FileMatches(file.path, file.tag, imports, matched.matches, modules.imported(file.path, imports)))


def _scan_script(file: RepoFile, state: _Scan, found: list[FileMatches], wiring: dict[str, frozenset[Wiring]]) -> None:
    language = file.language
    data = _read(file, state)
    tree = None if data is None or language is None else parse(language, data)
    if data is None or language is None or tree is None:
        return
    state.scanned += 1
    state.parse_errors += tree.root_node.has_error
    imports = extract_imports(language, tree, data)
    if not imports.imports:
        return
    matched = match_script_file(file.path, tree, data, imports)
    if matched.wiring:
        wiring[file.path] = matched.wiring
    if matched.matches:
        found.append(FileMatches(file.path, file.tag, imports, matched.matches))


def _scan_ballerina(file: RepoFile, state: _Scan, found: list[FileMatches]) -> None:
    data = _read(file, state)
    if data is None:
        return
    state.scanned += 1
    matched, imports = match_ballerina_file(file.path, data.decode("utf-8", errors="replace"))
    if matched:
        found.append(FileMatches(file.path, file.tag, imports, matched))
