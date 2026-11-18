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

"""Find the uses of agent signatures in a parsed Python file.

Call targets (``Agent``, ``crewai.Agent``, ``ReActAgent.from_tools``, ``Agent[Deps]``) and base classes are
resolved through the file's ``ImportTable``, so a same-named local class never matches. Each matched node
gives one ``Match``, even when fallback imports make it resolve to several symbols.

A match's name is, in order: a short string literal passed as ``name=``; the variable the call is assigned
to (``support = Agent(...)``, or the attribute in ``self.support = Agent(...)``); the class name for a
subclass; else ``<framework>-agent@<line>``.

Only these facts are read from the arguments: ``model`` when ``model=`` or ``llm=`` is a short string
literal, and the names of the plain identifiers listed in ``tools=[...]``. Prompts, instructions, tool
schemas and every other argument are never read.

An agent passed into another as ``absorbs`` says (``AgentExecutor(agent=create_react_agent(...))``, or a
variable assigned in the same file) is the same agent, so only the outer one is reported.
"""

from collections.abc import Iterator
from dataclasses import dataclass

import tree_sitter

from amp_shadow_runner.engine.imports import ImportTable
from amp_shadow_runner.engine.signatures import RuleSet, Shape, Signature
from amp_shadow_runner.engine.signatures.python import PYTHON

MAX_LITERAL_CHARS = 100
"""The longest string literal read as a name or model; a longer one is not a name."""
MAX_TOOLS = 50
"""The most tool names kept for one agent (Agent Manager's limit on a list attribute)."""

_CALL_SHAPES = (Shape.CALL, Shape.FACTORY)
_CLASS_SHAPES = (Shape.SUBCLASS,)


@dataclass(frozen=True)
class Match:
    """One use of an agent signature."""

    signature: Signature
    path: str
    line: int
    """1-based line of the call or class definition."""
    name: str
    model: str | None = None
    tools: tuple[str, ...] = ()

    @property
    def framework(self) -> str:
        return self.signature.framework


@dataclass(frozen=True)
class _Found:
    node: tree_sitter.Node
    match: Match
    variable: str | None
    """The plain variable the call is assigned to, if any."""


def match_python(
    path: str, tree: tree_sitter.Tree, source: bytes, imports: ImportTable, rules: RuleSet = PYTHON
) -> tuple[Match, ...]:
    """Every use of a signature in ``rules`` in the parsed Python file at ``path``, in source order."""
    reader = _Reader(source)
    found: list[_Found] = []
    for node in _walk(tree.root_node):
        if node.type == "call":
            item = reader.call(node, path, imports, rules)
        elif node.type == "class_definition":
            item = reader.subclass(node, path, imports, rules)
        else:
            continue
        if item is not None:
            found.append(item)
    absorbed = _absorbed(found, reader)
    return tuple(f.match for f in found if f.node.id not in absorbed)


def _walk(root: tree_sitter.Node) -> Iterator[tree_sitter.Node]:
    """Every node under ``root``, in source order, without recursion."""
    stack = [root]
    while stack:
        node = stack.pop()
        yield node
        stack.extend(reversed(node.children))


def _absorbed(found: list[_Found], reader: "_Reader") -> set[int]:
    """The ids of matched nodes that another match takes in through its ``absorbs`` keyword."""
    by_node = {f.node.id for f in found}
    by_variable = {f.variable: f.node.id for f in found if f.variable is not None}
    absorbed: set[int] = set()
    for f in found:
        keyword = f.match.signature.absorbs
        value = reader.keyword(f.node, keyword) if keyword is not None else None
        if value is None:
            continue
        if value.id in by_node:
            absorbed.add(value.id)
        elif value.type == "identifier" and (target := by_variable.get(reader.text(value))) is not None:
            absorbed.add(target)
    return absorbed


@dataclass(frozen=True)
class _Reader:
    source: bytes

    def text(self, node: tree_sitter.Node) -> str:
        return self.source[node.start_byte : node.end_byte].decode("utf-8", errors="replace")

    def reference(self, node: tree_sitter.Node | None) -> str | None:
        """A name or dotted name (``a.b.C``), seen through a subscript (``Agent[Deps]``); None for anything else."""
        if node is not None and node.type == "subscript":
            node = node.child_by_field_name("value")
        if node is None:
            return None
        if node.type == "identifier":
            return self.text(node)
        if node.type == "attribute":
            obj = self.reference_part(node.child_by_field_name("object"))
            attr = node.child_by_field_name("attribute")
            if obj is not None and attr is not None:
                return f"{obj}.{self.text(attr)}"
        return None

    def reference_part(self, node: tree_sitter.Node | None) -> str | None:
        """Like ``reference`` but without the subscript: ``a[0].B`` is not a dotted name."""
        if node is None or node.type == "subscript":
            return None
        return self.reference(node)

    def find(
        self, reference: str | None, imports: ImportTable, rules: RuleSet, shapes: tuple[Shape, ...]
    ) -> Signature | None:
        if reference is None:
            return None
        for symbol in imports.resolve(reference):
            sig = rules.find(symbol, shapes)
            if sig is not None:
                return sig
        return None

    def call(self, node: tree_sitter.Node, path: str, imports: ImportTable, rules: RuleSet) -> _Found | None:
        sig = self.find(self.reference(node.child_by_field_name("function")), imports, rules, _CALL_SHAPES)
        if sig is None:
            return None
        line = node.start_point[0] + 1
        name = self.literal(self.keyword(node, "name")) or self.assigned_to(node) or f"{sig.framework}-agent@{line}"
        model = self.literal(self.keyword(node, "model")) or self.literal(self.keyword(node, "llm"))
        match = Match(sig, path, line, name, model, self.tool_names(self.keyword(node, "tools")))
        return _Found(node, match, self.assigned_to(node, attribute=False))

    def subclass(self, node: tree_sitter.Node, path: str, imports: ImportTable, rules: RuleSet) -> _Found | None:
        name_node, bases = node.child_by_field_name("name"), node.child_by_field_name("superclasses")
        if name_node is None or bases is None:
            return None
        for base in bases.named_children:
            if base.type == "keyword_argument":  # metaclass=...
                continue
            sig = self.find(self.reference(base), imports, rules, _CLASS_SHAPES)
            if sig is not None:
                return _Found(node, Match(sig, path, node.start_point[0] + 1, self.text(name_node)), None)
        return None

    def keyword(self, call: tree_sitter.Node, name: str) -> tree_sitter.Node | None:
        """The value of keyword argument ``name`` in a call, or None."""
        arguments = call.child_by_field_name("arguments")
        if arguments is None or arguments.type != "argument_list":
            return None
        for argument in arguments.named_children:
            if argument.type == "keyword_argument":
                key = argument.child_by_field_name("name")
                if key is not None and self.text(key) == name:
                    return argument.child_by_field_name("value")
        return None

    def assigned_to(self, call: tree_sitter.Node, *, attribute: bool = True) -> str | None:
        """The name ``call`` is assigned to: ``x = call`` gives ``x``, and (with ``attribute``) ``self.x = call``
        gives ``x`` too."""
        parent = call.parent
        if parent is None or parent.type != "assignment" or parent.child_by_field_name("right") != call:
            return None
        left = parent.child_by_field_name("left")
        if attribute and left is not None and left.type == "attribute":
            left = left.child_by_field_name("attribute")
        return self.text(left) if left is not None and left.type == "identifier" else None

    def literal(self, node: tree_sitter.Node | None) -> str | None:
        """The value of a short, single-line, plain string literal, or None for anything else."""
        if node is None or node.type != "string" or not node.children:
            return None
        prefix = self.text(node.children[0]).lower()
        if "b" in prefix or any(c.type == "interpolation" for c in node.named_children):
            return None
        value = "".join(self.text(c) for c in node.named_children if c.type == "string_content").strip()
        if not value or len(value) > MAX_LITERAL_CHARS or "\\" in value or not value.isprintable():
            return None
        return value

    def tool_names(self, node: tree_sitter.Node | None) -> tuple[str, ...]:
        """The plain identifiers in a ``tools=[...]`` list, in order, without repeats."""
        if node is None or node.type != "list":
            return ()
        names = dict.fromkeys(self.text(c) for c in node.named_children if c.type == "identifier")
        return tuple(names)[:MAX_TOOLS]
