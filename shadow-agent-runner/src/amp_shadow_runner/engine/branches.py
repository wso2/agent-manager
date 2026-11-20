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

"""Tell one agent written once in each branch of a conditional from two agents.

``if fast: graph = create_agent(...)`` with ``else: graph = create_agent(...)`` builds one agent, whichever
branch runs. Each tree-sitter matcher passes its matches to ``repeats`` with a key (framework, scope and
qualified name) only for a match named after the variable it is assigned to, and drops a match that sits
in another branch (``exclusive``) than each match already merged into an earlier one with the same key. A
variable assigned twice in one block builds two agents, so both are kept.
"""

from collections import Counter
from collections.abc import Callable, Hashable, Mapping, Sequence

import tree_sitter

MAX_REPEATS = 32
"""The most matches of one key in a file that are merged; with more, none of them is (a duplicate row is
the safe side, and merging stays cheap)."""

Branch = Callable[[tree_sitter.Node, tree_sitter.Node], bool]
"""Whether a child of a node is one of its branches: code that runs only when its other branches do not."""


def branch_table(table: Mapping[str, frozenset[str]]) -> Branch:
    """A ``Branch`` from each branching node type to the field names or node types of its branches (never
    its condition, nor a part that always runs, such as ``finally``)."""

    def branch(parent: tree_sitter.Node, child: tree_sitter.Node) -> bool:
        selectors = table.get(parent.type)
        if selectors is None:
            return False
        return child.type in selectors or any(
            c.id == child.id for field in selectors for c in parent.children_by_field_name(field)
        )

    return branch


def exclusive(a: tree_sitter.Node, b: tree_sitter.Node, branch: Branch) -> bool:
    """Whether ``a`` and ``b`` sit in different branches of their nearest common ancestor."""
    toward_a: dict[int, tree_sitter.Node] = {}
    child, parent = a, a.parent
    while parent is not None:
        toward_a[parent.id] = child
        child, parent = parent, parent.parent
    child, parent = b, b.parent
    while parent is not None:
        other = toward_a.get(parent.id)
        if other is not None:
            return other.id != child.id and branch(parent, other) and branch(parent, child)
        child, parent = parent, parent.parent
    return False


def repeats(found: Sequence[tuple[tree_sitter.Node, Hashable | None]], branch: Branch) -> set[int]:
    """The ids of the nodes to drop. ``found`` is every match in source order with its key (None for one
    never dropped). A kept node stands for one node per path through the code: a later node of its key is
    merged into it (dropped) when it is ``exclusive`` with every node merged into it so far."""
    counts = Counter(key for _, key in found if key is not None)
    groups: dict[Hashable, list[list[tree_sitter.Node]]] = {}
    dropped: set[int] = set()
    for node, key in found:
        if key is None or counts[key] > MAX_REPEATS:
            continue
        kept = groups.setdefault(key, [])
        group = next((g for g in kept if all(exclusive(m, node, branch) for m in g)), None)
        if group is None:
            kept.append([node])
        else:
            group.append(node)
            dropped.add(node.id)
    return dropped
