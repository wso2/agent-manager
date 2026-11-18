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

Findings in test code (``RepoFile.tag`` ``test``) are still returned, with their tag, but are never
published: the report lists them apart, since test doubles are not running agents. Findings in example code
(tag ``example``) are published like any other, with the attribute ``labels: {"codeArea": "example"}``.

``external_id`` is ``<repository>:<path>:<qualified name>`` (see ``Match.qualified``), so it holds no line
number and code added elsewhere never changes it. Only a real repeat of the same qualified name (two
unnamed agents in one scope) gets ``@2``, ``@3`` and so on, numbered in source order.
"""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from enum import StrEnum

from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.engine.imports import ImportTable
from amp_shadow_runner.engine.manifests import Dependencies, service_folder
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
    """``test`` or ``example`` for a finding in such code. Test findings are reported but never published."""
    attributes: Mapping[str, object]
    """Attributes to publish: ``framework``; ``model`` and ``tools`` when the code states them; and
    ``labels`` ``{"codeArea": "example"}`` for example code."""
    evidence: tuple[Evidence, ...]
    service_folder: str | None = None
    """The folder of the nearest manifest above the file (``.`` for the root), reported, not published."""


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
    counts: dict[str, int] = {}
    used: set[str] = set()
    for file in sorted(files, key=lambda f: f.path):
        for match in sorted(file.matches, key=lambda m: m.line):
            framework = rules.framework(match.framework)
            name = match.name.strip()[:MAX_NAME_CHARS] or f"{framework.id}-agent"
            evidence = [Evidence(D2_AGENT_IMPORT, file.path, {"imports": _import_count(file.imports, framework)})]
            manifest = dependencies.declaring(framework.packages, file.path)
            if manifest is not None:
                evidence.insert(0, Evidence(D1_AGENT_DEPENDENCY, manifest, {"declared": True}))
            findings.append(
                Finding(
                    external_id=_unique(f"{repository}:{file.path}:{match.qualified}", counts, used),
                    name=name,
                    repository=repository,
                    path=file.path,
                    line=match.line,
                    framework=framework.id,
                    verdict=Verdict.PROVEN,
                    tag=file.tag,
                    attributes=_attributes(match, file.tag),
                    evidence=tuple(evidence),
                    service_folder=service_folder(file.path, dependencies.paths),
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


def _attributes(match: Match, tag: Tag | None) -> dict[str, object]:
    attributes: dict[str, object] = {"framework": match.framework}
    if match.model is not None:
        attributes["model"] = match.model
    if match.tools:
        attributes["tools"] = [{"type": "function", "name": name} for name in match.tools]
    if tag == "example":
        attributes["labels"] = {"codeArea": "example"}
    return attributes


def _unique(external_id: str, counts: dict[str, int], used: set[str]) -> str:
    """``external_id``, or for its n-th use ``external_id@n`` (or the next free number)."""
    n = counts.get(external_id, 1)
    candidate = external_id
    while candidate in used:
        n += 1
        candidate = f"{external_id}@{n}"
    counts[external_id] = n
    used.add(candidate)
    return candidate
