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

Only Python files are matched so far, so only they count as scanned; other languages come in later steps.
A file that cannot be read counts as skipped, with the walker's skipped files. A file with a syntax error
is still matched (tree-sitter recovers) and counts in ``parse_errors``.
"""

from dataclasses import dataclass
from pathlib import Path

from amp_shadow_runner.engine.classify import FileMatches, Finding, classify
from amp_shadow_runner.engine.files import FileKind, walk
from amp_shadow_runner.engine.imports import extract_imports
from amp_shadow_runner.engine.languages import Language, parse
from amp_shadow_runner.engine.manifests import Dependencies, is_python_manifest, python_packages
from amp_shadow_runner.engine.match import match_python
from amp_shadow_runner.engine.signatures.python import PYTHON


@dataclass(frozen=True)
class ScanResult:
    """What scanning one repository's tree found."""

    findings: tuple[Finding, ...]
    """Ordered by path, then line. Includes test and example findings (see ``Finding.tag``)."""
    files_scanned: int
    files_skipped: int
    parse_errors: int


def scan_tree(root: Path, *, repository: str, read_ci: bool = False) -> ScanResult:
    """Scan the tree at ``root`` (one repository, named ``repository``) and return its findings.

    Raises OSError only when ``root`` itself cannot be listed.
    """
    walked = walk(root, read_ci=read_ci)
    skipped = walked.skipped
    manifests: dict[str, frozenset[str]] = {}
    files: list[FileMatches] = []
    scanned = parse_errors = 0
    for file in walked.files:
        is_python = file.kind == FileKind.SOURCE and file.language == Language.PYTHON
        if not is_python and not (file.kind == FileKind.MANIFEST and is_python_manifest(file.path)):
            continue
        try:
            data = file.absolute.read_bytes()
        except OSError:
            skipped += 1
            continue
        if not is_python:
            manifests[file.path] = python_packages(file.path, data)
            continue
        tree = parse(Language.PYTHON, data)
        if tree is None:  # Python always has a grammar.
            continue
        scanned += 1
        if tree.root_node.has_error:
            parse_errors += 1
        imports = extract_imports(Language.PYTHON, tree, data)
        if imports.imports:
            matches = match_python(file.path, tree, data, imports)
            if matches:
                files.append(FileMatches(file.path, file.tag, imports, matches))
    findings = classify(files, repository=repository, dependencies=Dependencies(manifests), rules=PYTHON)
    return ScanResult(findings, scanned, skipped, parse_errors)
