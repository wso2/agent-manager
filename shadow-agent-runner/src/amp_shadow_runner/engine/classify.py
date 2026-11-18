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

"""Turn signature matches into findings: what the runner reports and, when certain, publishes.

A finding's verdict is ``proven`` or ``not_sure``. Every always-proven signature gives ``proven``; the
conditional rules and the Ballerina text match add ``not_sure`` later.

Evidence codes here carry no provider prefix: the publish step adds ``github.`` or ``azure_devops.``.

- ``d2_agent_import``: the file that uses the signature, with ``observed.imports`` the number of the
  framework's imports in it.
- ``d1_agent_dependency``: a dependency manifest that declares the framework's package (see
  ``Dependencies.declaring``), with ``observed.declared`` true.

Findings in test or example code (``RepoFile.tag``) are still returned, with their tag. They are not
published: the report lists them as skipped, test or example code, since samples and test doubles are not
running agents.

``external_id`` is ``<repository>:<path>:<name>``. When that repeats in one scan, the later finding (by
path, then line) gets ``@<line>`` appended, and a number after that if it still repeats, so the same code
always gives the same ids.
"""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from enum import StrEnum

from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.engine.imports import ImportTable
from amp_shadow_runner.engine.manifests import Dependencies
from amp_shadow_runner.engine.match import Match
from amp_shadow_runner.engine.signatures import Framework, RuleSet

MAX_NAME_CHARS = 200
"""The longest agent name kept (Agent Manager's limit)."""

D1_AGENT_DEPENDENCY = "d1_agent_dependency"
D2_AGENT_IMPORT = "d2_agent_import"

Observed = Mapping[str, bool | int | str]


class Verdict(StrEnum):
    """How certain a finding is."""

    PROVEN = "proven"
    """The rules prove an agent; it is published."""
    NOT_SURE = "not_sure"
    """It may be an agent; it is published only when AI review confirms it."""


@dataclass(frozen=True)
class Evidence:
    """Why a finding is an agent. ``observed`` holds only flags, counts and short labels."""

    code: str
    source: str
    """The repository path the evidence was seen in."""
    observed: Observed


@dataclass(frozen=True)
class Finding:
    """An agent found in a repository."""

    external_id: str
    name: str
    repository: str
    path: str
    line: int
    framework: str
    verdict: Verdict
    tag: Tag | None
    """``test`` or ``example`` for a finding in such code, which is reported but not published."""
    attributes: Mapping[str, object]
    """Attributes to publish: ``framework``, and ``model`` and ``tools`` when the code states them."""
    evidence: tuple[Evidence, ...]


@dataclass(frozen=True)
class FileMatches:
    """The matches in one source file, with what classifying them needs from the file."""

    path: str
    tag: Tag | None
    imports: ImportTable
    matches: tuple[Match, ...] = field(default=())


def classify(
    files: Sequence[FileMatches], *, repository: str, dependencies: Dependencies, rules: RuleSet
) -> tuple[Finding, ...]:
    """The findings for every match of one repository, ordered by path and then line."""
    findings: list[Finding] = []
    taken: set[str] = set()
    for file in sorted(files, key=lambda f: f.path):
        for match in sorted(file.matches, key=lambda m: m.line):
            framework = rules.framework(match.framework)
            name = match.name.strip()[:MAX_NAME_CHARS] or f"{framework.id}-agent@{match.line}"
            evidence = [Evidence(D2_AGENT_IMPORT, file.path, {"imports": _import_count(file.imports, framework)})]
            manifest = dependencies.declaring(framework.packages, file.path)
            if manifest is not None:
                evidence.insert(0, Evidence(D1_AGENT_DEPENDENCY, manifest, {"declared": True}))
            findings.append(
                Finding(
                    external_id=_unique(f"{repository}:{file.path}:{name}", match.line, taken),
                    name=name,
                    repository=repository,
                    path=file.path,
                    line=match.line,
                    framework=framework.id,
                    verdict=Verdict.PROVEN,
                    tag=file.tag,
                    attributes=_attributes(match),
                    evidence=tuple(evidence),
                )
            )
    return tuple(findings)


def _import_count(imports: ImportTable, framework: Framework) -> int:
    """How many non-type-only, absolute imports in the file come from the framework's modules."""
    count = 0
    for imp in imports.imports:
        if imp.type_only or imp.level:
            continue
        path = imp.module if imp.name is None else f"{imp.module}.{imp.name}"
        if any(path == root or path.startswith(f"{root}.") for root in framework.modules):
            count += 1
    return count


def _attributes(match: Match) -> dict[str, object]:
    attributes: dict[str, object] = {"framework": match.framework}
    if match.model is not None:
        attributes["model"] = match.model
    if match.tools:
        attributes["tools"] = [{"type": "function", "name": name} for name in match.tools]
    return attributes


def _unique(external_id: str, line: int, taken: set[str]) -> str:
    candidate = external_id
    if candidate in taken:
        candidate = f"{external_id}@{line}"
        n = 2
        while candidate in taken:
            candidate = f"{external_id}@{line}.{n}"
            n += 1
    taken.add(candidate)
    return candidate
