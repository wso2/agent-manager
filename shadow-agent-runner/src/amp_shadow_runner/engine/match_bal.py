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

"""Find Ballerina agents by text: Ballerina has no grammar, so every match is Not sure (``Condition.TEXT``).

A file counts only when it imports a signature's module (``import ballerina/ai;``, or with ``as p``), and
then only these clear patterns with that module's prefix (``ai``, ``agent`` for ``ai.agent``, or the alias):

- a typed declaration: ``final ai:Agent support = check new (...)``, which gives the name ``support``;
- a constructor: ``new ai:Agent(...)``, named after the variable it is assigned to (``support = new ...``,
  ``var support = check new ...``), else ``ballerina-agent``.

Lines that start with ``//`` are comments and are skipped. The name is the whole qualified name: the
match is at module level for its external ID, since finding the enclosing function by text is unreliable.
Nothing else in the file is read.
"""

import re

from amp_shadow_runner.engine.imports import Import, ImportKind, ImportTable
from amp_shadow_runner.engine.languages import Language
from amp_shadow_runner.engine.match import Match
from amp_shadow_runner.engine.signatures import RuleSet
from amp_shadow_runner.engine.signatures.ballerina import BALLERINA

DEFAULT_NAME = "ballerina-agent"

_IMPORT = re.compile(r"^[ \t]*import\s+([A-Za-z_]\w*)/([A-Za-z_][\w.]*)(?:\s+as\s+([A-Za-z_]\w*))?\s*;", re.MULTILINE)
_IDENTIFIER = r"[A-Za-z_]\w*"


def ballerina_imports(text: str) -> ImportTable:
    """The ``import org/name [as prefix];`` statements of a Ballerina file, each binding its prefix."""
    found: list[Import] = []
    for match in _IMPORT.finditer(text):
        org, name, alias = match.groups()
        prefix = alias or name.rsplit(".", 1)[-1]
        line = text.count("\n", 0, match.start()) + 1
        found.append(Import(f"{org}/{name}", None, prefix, ImportKind.MODULE, line))
    return ImportTable(Language.BALLERINA, tuple(found))


def match_ballerina_file(path: str, text: str, rules: RuleSet = BALLERINA) -> tuple[tuple[Match, ...], ImportTable]:
    """The agent matches in the Ballerina file at ``path`` (whose ``text`` is given), in source order, and its
    imports."""
    imports = ballerina_imports(text)
    code = "\n".join("" if line.lstrip().startswith("//") else line for line in text.split("\n"))
    found: dict[int, Match] = {}
    for imp in imports.imports:
        signatures = {s.symbol.name: s for s in rules.signatures if s.symbol.module == imp.module and s.symbol.name}
        if not signatures or imp.local is None:
            continue
        prefix = re.escape(imp.local)
        classes = "|".join(re.escape(name) for name in sorted(signatures))
        pattern = re.compile(
            rf"\b{prefix}:(?P<declared>{classes})\s+(?P<variable>{_IDENTIFIER})\s*=\s*(?:check\s+)?new\b"
            rf"|(?:\b(?P<target>{_IDENTIFIER})\s*=\s*(?:check\s+)?)?\bnew\s+{prefix}:(?P<created>{classes})\b"
        )
        for match in pattern.finditer(code):
            cls = match.group("declared") or match.group("created")
            name = match.group("variable") or match.group("target") or DEFAULT_NAME
            line = code.count("\n", 0, match.start()) + 1
            found.setdefault(match.start(), Match(signatures[cls], path, line, name, name))
    return tuple(found[k] for k in sorted(found)), imports
