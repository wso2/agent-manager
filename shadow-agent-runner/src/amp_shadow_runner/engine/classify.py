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

A finding's verdict is ``proven`` or ``not_sure``. Every always-proven signature gives ``proven``. A
conditional signature (see ``Condition``) gives ``proven`` only when its condition holds:

- ``TOOLS``: the file wires tools (any ``Wiring``), or a repository file it imports
  (``FileMatches.imported``) has a ``ToolNode(...)`` call or a ``tools_condition`` reference; a
  ``.bind_tools(...)`` there does not count. The first such file, the match's own file before the
  others, is ``d7_tool_calling`` evidence. Wiring is a fact about a file, not about one graph: every graph
  in a file that wires tools, or that imports one that does, counts as wired.
- ``DEPENDENCY``: the nearest manifest above the file declares the framework's package, or, with no
  manifest above it, one in a root ``requirements/`` folder does (``Dependencies.declaring`` with
  ``strict``). That manifest is also the finding's ``d1_agent_dependency`` evidence; every other finding
  takes that evidence from the lenient lookup.
- ``CREW``: never; a crew is a finding only when it links no agent, and is then ``not_sure``.

Evidence codes here carry no provider prefix: the publish step adds ``github.`` or ``azure_devops.``.

- ``d2_agent_import``: the file that uses the signature, with ``observed.imports`` the number of the
  framework's imports in it.
- ``d1_agent_dependency``: a dependency manifest that declares the framework's package (see
  ``Dependencies.declaring``), with ``observed.declared`` true.
- ``d7_tool_calling``: for a graph wired to tools, the file that wires it, with a true flag for each thing
  that wired it there: ``toolNode``, ``toolsCondition`` or ``bindTools``.

Findings in test code (``RepoFile.tag`` ``test``) are still returned, with their tag, but are never
published: the report lists them apart, since test doubles are not running agents. Findings in example code
(tag ``example``) are published like any other, with the attribute ``labels: {"codeArea": "example"}``. An
agent a crew links also has ``labels: {"crew": <crew name>}``, merged with any other label.

``external_id`` is ``<repository>:<path>:<qualified name>`` (see ``Match.qualified``), so it holds no line
number and code added elsewhere never changes it. Only a real repeat of the same qualified name (two
unnamed agents in one scope) gets ``@2``, ``@3`` and so on, numbered in source order. An ID over
MAX_EXTERNAL_ID_CHARS keeps a readable prefix and ends with ``#`` and 12 hex characters of the full ID's
SHA-256, so it stays stable and unique within Agent Manager's limit.
"""

import hashlib
from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from enum import StrEnum

from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.engine.imports import ImportTable
from amp_shadow_runner.engine.manifests import Dependencies, service_folder
from amp_shadow_runner.engine.match import Match
from amp_shadow_runner.engine.signatures import Condition, Framework, RuleSet, Wiring

MAX_NAME_CHARS = 200
"""The longest agent name kept (Agent Manager's limit)."""
MAX_EXTERNAL_ID_CHARS = 1000
"""The longest external ID published (Agent Manager keys agents on the first 1024 characters)."""

D1_AGENT_DEPENDENCY = "d1_agent_dependency"
D2_AGENT_IMPORT = "d2_agent_import"
D7_TOOL_CALLING = "d7_tool_calling"

Observed = Mapping[str, bool | int | str]

_HOP_WIRING = frozenset({Wiring.TOOL_NODE, Wiring.TOOLS_CONDITION})
"""What wires a graph to tools one import hop away (see ``Wiring.BIND_TOOLS``)."""


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
    ``labels``: ``{"codeArea": "example"}`` for example code, ``{"crew": <name>}`` for an agent in a crew."""
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
    imported: tuple[str, ...] = ()
    """The repository files this file imports (``RepoModules.imported``), for ``Condition.TOOLS``."""


def classify(
    files: Sequence[FileMatches],
    *,
    repository: str,
    dependencies: Dependencies,
    rules: RuleSet,
    wiring: Mapping[str, frozenset[Wiring]] | None = None,
) -> tuple[Finding, ...]:
    """The findings for every match of one repository, ordered by path and then line. ``wiring`` holds, by
    path, what wires tools in each file of the repository that does (``PythonMatches.wiring``)."""
    wiring = wiring or {}
    findings: list[Finding] = []
    counts: dict[str, int] = {}
    used: set[str] = set()
    for file in sorted(files, key=lambda f: f.path):
        for match in sorted(file.matches, key=lambda m: m.line):
            framework = rules.framework(match.framework)
            name = match.name.strip()[:MAX_NAME_CHARS] or f"{framework.id}-agent"
            evidence = [Evidence(D2_AGENT_IMPORT, file.path, {"imports": _import_count(file.imports, framework)})]
            condition = match.signature.condition
            strict = condition == Condition.DEPENDENCY
            manifest = dependencies.declaring(framework.packages, file.path, strict=strict)
            if manifest is not None:
                evidence.insert(0, Evidence(D1_AGENT_DEPENDENCY, manifest, {"declared": True}))
            verdict = Verdict.PROVEN
            if condition == Condition.TOOLS:
                wired = _wired(file, wiring)
                if wired is None:
                    verdict = Verdict.NOT_SURE
                else:
                    evidence.append(Evidence(D7_TOOL_CALLING, wired[0], {k.value: True for k in sorted(wired[1])}))
            elif (condition == Condition.DEPENDENCY and manifest is None) or condition == Condition.CREW:
                verdict = Verdict.NOT_SURE
            findings.append(
                Finding(
                    external_id=_unique(f"{repository}:{file.path}:{match.qualified}", counts, used),
                    name=name,
                    repository=repository,
                    path=file.path,
                    line=match.line,
                    framework=framework.id,
                    verdict=verdict,
                    tag=file.tag,
                    attributes=_attributes(match, file.tag),
                    evidence=tuple(evidence),
                    service_folder=service_folder(file.path, dependencies.paths),
                )
            )
    return tuple(findings)


def _wired(file: FileMatches, wiring: Mapping[str, frozenset[Wiring]]) -> tuple[str, frozenset[Wiring]] | None:
    """The first file that wires ``file``'s graphs to tools, and what wires them there, or None."""
    own = wiring.get(file.path, frozenset())
    if own:
        return file.path, own
    for path in file.imported:
        kinds = wiring.get(path, frozenset()) & _HOP_WIRING
        if kinds:
            return path, kinds
    return None


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
    labels: dict[str, str] = {}
    if tag == "example":
        labels["codeArea"] = "example"
    if match.crew is not None:
        labels["crew"] = match.crew
    if labels:
        attributes["labels"] = labels
    return attributes


def cap_external_id(external_id: str) -> str:
    """``external_id``, or when it is over MAX_EXTERNAL_ID_CHARS, a prefix, ``#`` and a 12-hex SHA-256 of it."""
    if len(external_id) <= MAX_EXTERNAL_ID_CHARS:
        return external_id
    digest = hashlib.sha256(external_id.encode("utf-8", "surrogatepass")).hexdigest()[:12]
    return f"{external_id[: MAX_EXTERNAL_ID_CHARS - 13]}#{digest}"


def _unique(external_id: str, counts: dict[str, int], used: set[str]) -> str:
    """``external_id``, or for its n-th use ``external_id@n`` (or the next free number), capped in length."""
    n = counts.get(external_id, 1)
    candidate = cap_external_id(external_id)
    while candidate in used:
        n += 1
        candidate = cap_external_id(f"{external_id}@{n}")
    counts[external_id] = n
    used.add(candidate)
    return candidate
