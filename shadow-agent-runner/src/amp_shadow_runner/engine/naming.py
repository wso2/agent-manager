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

"""The name a found agent is shown with in Agent Manager (``Finding.name``): the naming rule in one place.

Each matcher names its match from the code (``match``, ``match_ts``, ``match_cs``, ``match_bal``): a short
``name`` string literal, else the variable it is assigned to, the class, or the enclosing function, else
``<framework>-agent``. ``scan_tree`` then renames, per repository:

1. A strong name is kept: a ``name`` literal is always strong, and so is any other specific name
   (``support_agent``, ``ResearchCrew``, ``researcher``).
2. A weak name is replaced by the project's name. A name is weak when it is the ``<framework>-agent`` or
   ``<framework>-crew`` fallback (or ``ballerina-agent``), or when its core (lower case, without ``_``
   and ``-``, and without a trailing ``async`` unless it is only ``async``) has at most one character or
   is one of WEAK_NAMES (``agent``, ``graph``, ``build_agent``, ``createAgent``, ``CreateAgentAsync``,
   ``__init__``, ...). A ``name`` literal is never weak, whatever it says.
3. The project's name comes from one walk up the folders, starting at the finding's ``service_folder``
   (the folder of the nearest manifest), or at the file's own folder when no manifest is above it or when
   that folder is the repository root and its manifests state no usable name (a ``requirements.txt``, or a
   ``package.json`` workspace root), so each service of a monorepo is named after its folder. At each
   folder:

   a. the name a manifest there states (``manifests.project_name``), the first by kind in this order:
      ``pyproject.toml``, ``package.json``, ``Ballerina.toml``, ``*.csproj``;
   b. else, when the folder's own name is in GENERIC_FOLDERS or is weak (``agent``, ``app``,
      ``workflow``), the walk goes on to the folder above;
   c. else the folder's own name.

   A manifest name or folder name that is weak by the same core test counts as missing (``@acme/agent``,
   ``Agent.csproj``). At the repository root, with nothing else found, the repository's name is used,
   whatever it is.
4. Duplicates, per repository, with test findings (never published) and the others counted apart, so a
   test double never changes a published name:

   a. a renamed agent whose new name is also another agent's gets its original name in parentheses
      (``insurance-support-agent (graph)``);
   b. agents whose names still repeat, renamed or not, get instead what tells them apart: the first part
      of their path where the paths differ, after the folders they all share (``crew-ai (agent-auth-flow)``,
      ``svc-bot (a.py)``), and for agents in one file, the function or class they are built in
      (``it-agent (ask)``), or both (``bot (api, ask)``). Repeats left in one scope of one file stay.

Only the display name changes. ``Finding.external_id`` is built from ``Match.qualified`` and stays the same,
so a renamed agent is the same agent in Agent Manager, which takes the new name on the next publish. Crew
labels (``labels.crew``) are not renamed.
"""

import dataclasses
import re
from collections import Counter, defaultdict
from collections.abc import Iterable, Mapping
from pathlib import PurePosixPath

from amp_shadow_runner.engine.classify import MAX_NAME_CHARS, Finding
from amp_shadow_runner.engine.manifests import plain_name
from amp_shadow_runner.engine.match_bal import DEFAULT_NAME as BALLERINA_DEFAULT_NAME

_VERBS = ("build", "create", "make", "get", "new", "init", "setup", "run")
_NOUNS = ("agent", "graph", "workflow", "crew", "executor", "app")

WEAK_NAMES = frozenset(
    {
        "agent",
        "agents",
        "graph",
        "builder",
        "graphbuilder",
        "workflow",
        "app",
        "executor",
        "agentexecutor",
        "assistant",
        "crew",
        "main",
        "run",
        "init",
        "get",
        "post",
        "handler",
        "lambdahandler",
        *(verb + noun for verb in _VERBS for noun in _NOUNS),
    }
)
"""Names that say what the code is, not which agent, as cores: lower case without ``_`` and ``-`` (and a
trailing ``async``), so ``agent_executor``, ``AgentExecutor``, ``RunAsync`` and ``__init__`` (``init``) all
match. ``build``, ``create``, ``make``, ``get``, ``new``, ``init``, ``setup`` or ``run`` followed by
``agent``, ``graph``, ``workflow``, ``crew``, ``executor`` or ``app`` (``build_agent``, ``createAgent``) are
weak too."""

GENERIC_FOLDERS = frozenset({"src", "lib", "source", "code"})
"""Folder names (ignoring case) that do not name a project although they are not weak names."""

_SEPARATORS = re.compile(r"[_-]+")
_ROOT = PurePosixPath(".")
_PRIORITY = {"pyproject.toml": 0, "package.json": 1, "Ballerina.toml": 2}
_CSPROJ_PRIORITY = 3


def _weak_word(name: str) -> bool:
    """Whether ``name``'s core (see the module docstring) has at most one character or is in WEAK_NAMES."""
    core = _SEPARATORS.sub("", name).lower()
    if core != "async":
        core = core.removesuffix("async")
    return len(core) <= 1 or core in WEAK_NAMES


def is_weak(finding: Finding) -> bool:
    """Whether ``finding``'s name is weak (see the module docstring); a ``name`` literal never is."""
    if finding.named:
        return False
    if finding.name in (f"{finding.framework}-agent", f"{finding.framework}-crew", BALLERINA_DEFAULT_NAME):
        return True
    return _weak_word(finding.name)


def display_names(findings: Iterable[Finding], *, repository: str, names: Mapping[str, str]) -> tuple[Finding, ...]:
    """``findings`` (of one repository, in order) with each weak name replaced by its project's name.

    ``names`` holds, by manifest path, the project name each manifest states (``manifests.project_name``).
    """
    by_folder = _names_by_folder(names)
    found = list(findings)
    projects = {i: _project(f, by_folder, repository) for i, f in enumerate(found) if is_weak(f)}
    shown = [projects[i][0] if i in projects else f.name for i, f in enumerate(found)]

    def repeated() -> Counter[tuple[bool, str]]:
        return Counter((f.tag == "test", name) for f, name in zip(found, shown, strict=True))

    counts = repeated()
    for i in projects:
        if counts[(found[i].tag == "test", shown[i])] > 1:
            shown[i] = f"{projects[i][0]} ({found[i].name})"
    counts = repeated()
    clashes: dict[tuple[bool, str], list[int]] = defaultdict(list)
    for i, f in enumerate(found):
        if counts[(f.tag == "test", shown[i])] > 1:
            clashes[(f.tag == "test", shown[i])].append(i)
    for clash in clashes.values():
        for i, label in _tell_apart([found[i] for i in clash], repository).items():
            if label:
                base = projects[clash[i]][0] if clash[i] in projects else found[clash[i]].name
                shown[clash[i]] = f"{base} ({label})"
    return tuple(
        dataclasses.replace(f, name=shown[i][:MAX_NAME_CHARS]) if shown[i] != f.name else f for i, f in enumerate(found)
    )


def _tell_apart(clash: list[Finding], repository: str) -> dict[int, str]:
    """By position in ``clash`` (findings shown with one name), what tells each apart from the others: the
    first part of its path that differs from every other path, after the folders all the paths share, and
    the scope it is built in when another of them is in the same file. Empty when nothing does."""
    parts = [PurePosixPath(f.path).parts for f in clash]
    shared = 0
    while all(len(p) > shared + 1 and p[shared] == parts[0][shared] for p in parts):
        shared += 1
    scopes = [_scope(f, repository) for f in clash]
    labels: dict[int, str] = {}
    for i, path in enumerate(parts):
        others = [p for p in parts if p != path]
        label = []
        if others:
            k = shared + 1
            while k < len(path) and any(p[:k] == path[:k] for p in others):
                k += 1
            label.append("/".join(path[shared:k]))
        siblings = [scopes[j] for j, p in enumerate(parts) if p == path and j != i]
        if siblings and scopes[i] and scopes[i] not in siblings:
            label.append(scopes[i])
        labels[i] = ", ".join(label)
    return labels


def _scope(finding: Finding, repository: str) -> str:
    """The function or class ``finding`` is built in, from its external ID (``<repo>:<path>:<qualified>``);
    empty at module level."""
    prefix = f"{repository}:{finding.path}:"
    if not finding.external_id.startswith(prefix):
        return ""  # A long ID is cut and hashed.
    qualified = finding.external_id.removeprefix(prefix)
    head, at, count = qualified.rpartition("@")
    if at and count.isdigit():
        qualified = head
    if finding.named:  # The last part is the name literal, which may hold dots.
        return qualified.removesuffix(finding.name).removesuffix(".") if qualified.endswith(finding.name) else ""
    return qualified.rpartition(".")[0]


def _names_by_folder(names: Mapping[str, str]) -> dict[str, str]:
    """By folder, the first name that is not weak its manifests state, in the order of the module docstring."""
    best: dict[str, tuple[int, str, str]] = {}
    for path, name in names.items():
        if _weak_word(name):
            continue
        manifest = PurePosixPath(path)
        rank = _PRIORITY.get(manifest.name, _CSPROJ_PRIORITY)
        folder = str(manifest.parent)
        if folder not in best or (rank, path) < best[folder][:2]:
            best[folder] = (rank, path, name)
    return {folder: name for folder, (_, _, name) in best.items()}


def _project(finding: Finding, by_folder: Mapping[str, str], repository: str) -> tuple[str, PurePosixPath]:
    """The name of the project ``finding`` is in, and the folder it names (see the module docstring)."""
    start = finding.service_folder
    folder = PurePosixPath(start) if start is not None else PurePosixPath(finding.path).parent
    if folder == _ROOT and str(_ROOT) not in by_folder:
        folder = PurePosixPath(finding.path).parent  # A root manifest that names nothing places nothing.
    while True:
        if str(folder) in by_folder:
            return by_folder[str(folder)], folder
        if folder == _ROOT:
            break
        name = plain_name(folder.name)
        if folder.name.lower() not in GENERIC_FOLDERS and name is not None and not _weak_word(name):
            return name, folder
        folder = folder.parent
    return plain_name(repository.rstrip("/").rsplit("/", 1)[-1]) or finding.name, _ROOT
