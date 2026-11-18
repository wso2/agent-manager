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
subclass; the nearest enclosing function (``def researcher(self): return Agent(...)``, the CrewAI
template); else ``<framework>-agent``. No name holds a line number, so adding code above an agent never
renames it.

``Match.qualified`` places the name in its scope: the enclosing classes and functions, then the name
(``support.agent``, ``Bot.__init__.agent``, just ``agent`` at module level). A name taken from the
enclosing function is that function's own path (``Crew.researcher``).

Only these facts are read from the arguments: ``model`` when ``model=`` or ``llm=`` is a short string
literal that looks like a model name (no ``://``, ``@`` or whitespace), and the names of the plain
identifiers listed in ``tools=[...]``. Prompts, instructions, tool schemas and every other argument are
never read; a signature's ``requires`` keyword is only checked for a literal ``False`` or ``None``.

An agent passed into another as its ``absorbs`` argument is the same agent, so only the outer one is
reported: a matched call written inline (``AgentExecutor(agent=create_react_agent(...))``), or the nearest
earlier assignment, in the same scope, of the variable or attribute passed (``agent`` or ``self.agent``).
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
_SCOPES = frozenset({"function_definition", "class_definition"})


@dataclass(frozen=True)
class Match:
    """One use of an agent signature."""

    signature: Signature
    path: str
    line: int
    """1-based line of the call or class definition."""
    name: str
    qualified: str
    """The enclosing classes and functions and the name, joined with dots (see the module docstring)."""
    model: str | None = None
    tools: tuple[str, ...] = ()

    @property
    def framework(self) -> str:
        return self.signature.framework


@dataclass(frozen=True)
class _Found:
    node: tree_sitter.Node
    match: Match
    scope: int
    """The id of the nearest enclosing function, class or module node."""
    variable: str | None = None
    """The variable or attribute chain (``agent``, ``self.agent``) the call is assigned to, if any."""


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


def _scopes(node: tree_sitter.Node) -> list[tree_sitter.Node]:
    """The functions and classes enclosing ``node`` (not ``node`` itself), outermost first."""
    scopes: list[tree_sitter.Node] = []
    parent = node.parent
    while parent is not None:
        if parent.type in _SCOPES:
            scopes.append(parent)
        parent = parent.parent
    return scopes[::-1]


def _scope_id(node: tree_sitter.Node) -> int:
    """The id of the nearest function, class or module node enclosing ``node``."""
    parent = node.parent
    while parent is not None:
        if parent.type in _SCOPES or parent.parent is None:
            return parent.id
        parent = parent.parent
    return node.id


def _absorbed(found: list[_Found], reader: "_Reader") -> set[int]:
    """The ids of matched nodes that another match takes in through its ``absorbs`` argument."""
    by_node = {f.node.id for f in found}
    absorbed: set[int] = set()
    for f in found:
        sig = f.match.signature
        if sig.absorbs is None:
            continue
        value = reader.keyword(f.node, sig.absorbs)
        if value is None and sig.absorbs_position is not None:
            value = reader.positional(f.node, sig.absorbs_position)
        if value is None:
            continue
        if value.id in by_node:
            absorbed.add(value.id)
            continue
        key = reader.reference_part(value)
        earlier = [
            g
            for g in found
            if key is not None and g.variable == key and g.scope == f.scope and g.node.start_byte < f.node.start_byte
        ]
        if earlier:
            absorbed.add(max(earlier, key=lambda g: g.node.start_byte).node.id)
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

    def scope_names(self, scopes: list[tree_sitter.Node]) -> list[str]:
        names = (s.child_by_field_name("name") for s in scopes)
        return [self.text(n) if n is not None else "_" for n in names]

    def call(self, node: tree_sitter.Node, path: str, imports: ImportTable, rules: RuleSet) -> _Found | None:
        sig = self.find(self.reference(node.child_by_field_name("function")), imports, rules, _CALL_SHAPES)
        if sig is None:
            return None
        if sig.requires is not None:
            required = self.keyword(node, sig.requires)
            if required is None or required.type in ("false", "none"):
                return None
        scopes = _scopes(node)
        path_names = self.scope_names(scopes)
        name = self.literal(self.keyword(node, "name")) or self.assigned_to(node)
        if name is not None:
            qualified = ".".join([*path_names, name])
        else:
            functions = [i for i, s in enumerate(scopes) if s.type == "function_definition"]
            if functions:
                name, qualified = path_names[functions[-1]], ".".join(path_names[: functions[-1] + 1])
            else:
                name = f"{sig.framework}-agent"
                qualified = ".".join([*path_names, name])
        model = self.model(self.keyword(node, "model")) or self.model(self.keyword(node, "llm"))
        tools = self.tool_names(self.keyword(node, "tools"))
        match = Match(sig, path, node.start_point[0] + 1, name, qualified, model, tools)
        return _Found(node, match, _scope_id(node), self.assigned_target(node))

    def subclass(self, node: tree_sitter.Node, path: str, imports: ImportTable, rules: RuleSet) -> _Found | None:
        name_node, bases = node.child_by_field_name("name"), node.child_by_field_name("superclasses")
        if name_node is None or bases is None:
            return None
        for base in bases.named_children:
            if base.type == "keyword_argument":  # metaclass=...
                continue
            sig = self.find(self.reference(base), imports, rules, _CLASS_SHAPES)
            if sig is not None:
                name = self.text(name_node)
                qualified = ".".join([*self.scope_names(_scopes(node)), name])
                return _Found(node, Match(sig, path, node.start_point[0] + 1, name, qualified), _scope_id(node))
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

    def positional(self, call: tree_sitter.Node, index: int) -> tree_sitter.Node | None:
        """The positional argument at ``index`` in a call, or None."""
        arguments = call.child_by_field_name("arguments")
        if arguments is None or arguments.type != "argument_list":
            return None
        values = [a for a in arguments.named_children if a.type not in ("keyword_argument", "comment")]
        return values[index] if index < len(values) else None

    def assigned_target(self, call: tree_sitter.Node) -> str | None:
        """What ``call`` is assigned to, as written: ``x`` or an attribute chain such as ``self.x``."""
        parent = call.parent
        if parent is None or parent.type != "assignment" or parent.child_by_field_name("right") != call:
            return None
        return self.reference_part(parent.child_by_field_name("left"))

    def assigned_to(self, call: tree_sitter.Node) -> str | None:
        """The name ``call`` is assigned to: ``x = call`` and ``self.x = call`` both give ``x``."""
        target = self.assigned_target(call)
        return None if target is None else target.rsplit(".", 1)[-1]

    def model(self, node: tree_sitter.Node | None) -> str | None:
        """A model name from a string literal; None for a URL, an address or anything with whitespace."""
        value = self.literal(node)
        if value is None or "://" in value or "@" in value or any(c.isspace() for c in value):
            return None
        return value

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
