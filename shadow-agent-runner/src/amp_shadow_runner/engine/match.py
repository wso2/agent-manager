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
renames it. ``Match.named`` tells a ``name=`` literal apart; ``naming`` decides the name the agent is shown
with, which replaces a weak name (``agent``, ``graph``, ``build_agent``) by the project's name.

``Match.qualified`` places the name in its scope: the enclosing classes and functions, then the name
(``support.agent``, ``Bot.__init__.agent``, just ``agent`` at module level). A name taken from the
enclosing function is that function's own path (``Crew.researcher``).

Only these facts are read from the arguments: ``model`` when ``model=`` or ``llm=`` is a short string
literal that looks like a model name (no ``://``, ``@`` or whitespace), and the names of the plain
identifiers listed in ``tools=[...]``. Prompts, instructions, tool schemas and every other argument are
never read; a signature's ``requires`` keyword is only checked for a literal ``False`` or ``None``.

An agent passed into another as its ``absorbs`` argument is the same agent, so only the outer one is
reported: a matched call written inline (``AgentExecutor(agent=create_react_agent(...))``), or the nearest
earlier assignment of the variable or attribute passed (``agent`` or ``self.agent``) in the same scope, else
in the nearest enclosing function or the module that has one.

Conditional signatures (see ``Condition``) still give a ``Match``, with the signature's condition, and
``classify`` decides the verdict, except for two things decided here:

- ``Condition.DEPENDENCY``: when ``is_local`` says the symbol's module, or a module above it, is a module
  of the repository (a local ``agents`` package, or ``agents/agent.py`` for ``agents.agent``), the call is
  no match at all.
- ``Condition.CREW``: a crew links the agents in its ``agents=`` list, or in the list a variable of the same
  scope was last assigned before the crew (``team = [a, b]``), and its ``manager_agent=``, that are matches
  of the same file: a matched call written in place, a variable (its nearest earlier assignment in the
  crew's scope, else a module-level one), ``self.x`` (assigned in the same class) or ``self.m()`` (an agent
  in method ``m`` of the same class, decorated with an agent decorator); and ``agents=self.agents`` in a
  project class (``@CrewBase``) links every agent in its agent-decorated methods. Each linked agent gets
  ``Match.crew``, the crew's name (the first crew, in source order, when several link it); a crew that
  links an agent is not a match itself, and one that links none is. A crew's name is its ``name=``
  literal, else the variable it is assigned to, else the class when it is built in a method (``ResearchCrew``
  for the template's ``@crew def crew(self)``), else the enclosing function, else the enclosing class,
  else ``<framework>-crew``.

``match_python_file`` also tells what in the file wires a graph to tools (``RuleSet.wiring``), which
``Condition.TOOLS`` needs from the file and from the files it imports. That is a fact about the file, not
about one graph: every graph of a file that wires tools counts as wired. A file that imports no module
``ToolWiring.modules`` names is not looked at.
"""

import dataclasses
from collections.abc import Callable, Iterator
from dataclasses import dataclass

import tree_sitter

from amp_shadow_runner.engine.imports import ImportKind, ImportTable, Symbol
from amp_shadow_runner.engine.signatures import Condition, RuleSet, Shape, Signature, Wiring
from amp_shadow_runner.engine.signatures.python import PYTHON

MAX_LITERAL_CHARS = 100
"""The longest string literal read as a name or model; a longer one is not a name."""
MAX_TOOLS = 50
"""The most tool names kept for one agent (Agent Manager's limit on a list attribute)."""
MAX_REFERENCE_PARTS = 64
"""The most parts a dotted name (``a.b.C``) may have; a longer chain is no reference (and is read without
recursion, so a generated chain of thousands of parts cannot exhaust the stack)."""

_CALL_SHAPES = (Shape.CALL, Shape.FACTORY)
_CLASS_SHAPES = (Shape.SUBCLASS,)
_SCOPES = frozenset({"function_definition", "class_definition"})
_IMPORT_PARTS = frozenset({"dotted_name", "aliased_import"})
"""Parents of the identifiers an import statement names; such an identifier is no reference."""


def plain_literal(value: str) -> bool:
    """Whether a string literal's value can be a name or model: 1 to ``MAX_LITERAL_CHARS`` printable
    characters, with no backslash and no U+FFFD (bytes that were not UTF-8)."""
    return 0 < len(value) <= MAX_LITERAL_CHARS and "\\" not in value and "\ufffd" not in value and value.isprintable()


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
    crew: str | None = None
    """The name of the crew that links this agent (see the module docstring)."""
    condition_met: bool | None = None
    """For a condition the matcher checks in the code it matched (``Condition.TOOL_LOOP``,
    ``Condition.NAMESPACE``), whether it holds; None for every other match."""
    named: bool = False
    """True when ``name`` is the agent's own name literal (``name=`` and the like), which ``naming`` keeps."""

    @property
    def framework(self) -> str:
        return self.signature.framework


@dataclass(frozen=True)
class PythonMatches:
    """What matching one Python file found."""

    matches: tuple[Match, ...]
    wiring: frozenset[Wiring] = frozenset()
    """What in the file wires a graph to tools (``RuleSet.wiring``); empty when nothing does."""


@dataclass(frozen=True)
class _Found:
    node: tree_sitter.Node
    match: Match
    scope: int
    """The id of the nearest enclosing function, class or module node."""
    variable: str | None = None
    """The variable or attribute chain (``agent``, ``self.agent``) the call is assigned to, if any."""


def match_python_file(
    path: str,
    tree: tree_sitter.Tree,
    source: bytes,
    imports: ImportTable,
    rules: RuleSet = PYTHON,
    *,
    is_local: Callable[[str], bool] | None = None,
) -> PythonMatches:
    """Every use of a signature in ``rules`` in the parsed Python file at ``path``, in source order, and what
    in the file wires tools.

    ``is_local`` tells whether an absolute module name is a module of the repository, as this file imports it
    (``RepoModules.is_local``); without it no module is.
    """
    reader = _Reader(source)
    found: list[_Found] = []
    live = [imp for imp in imports.imports if not imp.type_only]
    look = any(not imp.level and imp.module.startswith(rules.wiring.modules) for imp in live)
    wiring: set[Wiring] = set()
    references = look and bool(rules.wiring.references)
    bound = {imp.local for imp in live if imp.local is not None}
    wildcard = any(imp.kind == ImportKind.WILDCARD for imp in live)
    for node in _walk(tree.root_node):
        if node.type == "call":
            item = reader.call(node, path, imports, rules, is_local)
            if look and (kind := reader.wires(node, imports, rules)) is not None:
                wiring.add(kind)
        elif node.type == "class_definition":
            item = reader.subclass(node, path, imports, rules)
        else:
            if (
                references
                and Wiring.TOOLS_CONDITION not in wiring
                and node.type in ("identifier", "attribute")
                and reader.wiring_reference(node, imports, rules, bound, wildcard)
            ):
                wiring.add(Wiring.TOOLS_CONDITION)
            continue
        if item is not None:
            found.append(item)
    absorbed = _absorbed(found, reader)
    kept = [f for f in found if f.node.id not in absorbed]
    return PythonMatches(tuple(f.match for f in _link_crews(kept, reader, imports, rules)), frozenset(wiring))


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
        for scope in _enclosing_scopes(f.node) if key is not None else ():
            earlier = [
                g for g in found if g.variable == key and g.scope == scope and g.node.start_byte < f.node.start_byte
            ]
            if earlier:
                absorbed.add(max(earlier, key=lambda g: g.node.start_byte).node.id)
                break
    return absorbed


def _enclosing_scopes(node: tree_sitter.Node) -> list[int]:
    """The ids of the scopes whose variables ``node`` can see, innermost first: its own scope, then the
    functions around it and the module. A class body around a function is skipped: its names are not
    variables of its methods."""
    ids = [_scope_id(node)]
    parent = node.parent
    while parent is not None:
        if (parent.type == "function_definition" or parent.parent is None) and parent.id not in ids:
            ids.append(parent.id)
        parent = parent.parent
    return ids


def _innermost(node: tree_sitter.Node, kind: str) -> tree_sitter.Node | None:
    """The nearest ``kind`` node (a function or class definition) enclosing ``node``, or None."""
    parent = node.parent
    while parent is not None:
        if parent.type == kind:
            return parent
        parent = parent.parent
    return None


def _owner(function: tree_sitter.Node) -> tree_sitter.Node | None:
    """The class a function is a method of (defined straight in its body), or None."""
    parent = function.parent
    if parent is not None and parent.type == "decorated_definition":
        parent = parent.parent
    if parent is not None and parent.type == "block":
        parent = parent.parent
    return parent if parent is not None and parent.type == "class_definition" else None


def _link_crews(found: list[_Found], reader: "_Reader", imports: ImportTable, rules: RuleSet) -> list[_Found]:
    """``found`` with each linked agent given its crew, and the crews that link an agent left out."""
    crews = [f for f in found if f.match.signature.condition == Condition.CREW]
    if not crews:
        return found
    agents = [f for f in found if f.match.signature.condition != Condition.CREW]
    labels: dict[int, str] = {}
    linking: set[int] = set()
    for crew in crews:
        members = _Crew(reader, imports, rules, agents, crew).members()
        if members:
            linking.add(crew.node.id)
            for member in members:
                labels.setdefault(member.node.id, crew.match.name)
    return [
        dataclasses.replace(f, match=dataclasses.replace(f.match, crew=labels[f.node.id])) if f.node.id in labels else f
        for f in found
        if f.node.id not in linking
    ]


@dataclass(frozen=True)
class _Crew:
    """Finds the agents one crew links (see the module docstring)."""

    reader: "_Reader"
    imports: ImportTable
    rules: RuleSet
    agents: list[_Found]
    crew: _Found

    def members(self) -> list[_Found]:
        linked: dict[int, _Found] = {}
        value = self.reader.keyword(self.crew.node, "agents")
        if value is not None and value.type == "identifier":
            value = self.assigned_list(self.reader.text(value)) or value
        if value is not None and value.type in ("list", "tuple"):
            for element in value.named_children:
                for agent in self.element(element):
                    linked.setdefault(agent.node.id, agent)
        elif value is not None and self.reader.reference_part(value) == "self.agents":
            owner = _innermost(self.crew.node, "class_definition")
            if owner is not None and self.is_project(owner):
                linked.update((a.node.id, a) for a in self.agents if self.agent_method(a, owner) is not None)
        manager = self.reader.keyword(self.crew.node, "manager_agent")
        if manager is not None:
            for agent in self.element(manager):
                linked.setdefault(agent.node.id, agent)
        return list(linked.values())

    def assigned_list(self, name: str) -> tree_sitter.Node | None:
        """The list or tuple that ``name`` was last assigned, in the crew's own scope, before the crew."""
        scope = self.crew.node.parent
        while scope is not None and scope.type not in _SCOPES and scope.parent is not None:
            scope = scope.parent
        if scope is None:
            return None
        latest: tree_sitter.Node | None = None
        stack = list(scope.children)
        while stack:
            node = stack.pop()
            if node.start_byte >= self.crew.node.start_byte or node.type in _SCOPES:
                continue  # Later code, or a nested function or class: another scope.
            if node.type == "assignment":
                left, right = node.child_by_field_name("left"), node.child_by_field_name("right")
                if (
                    left is not None
                    and left.type == "identifier"
                    and self.reader.text(left) == name
                    and (latest is None or node.start_byte > latest.start_byte)
                ):
                    latest = right
            stack.extend(node.children)
        return latest if latest is not None and latest.type in ("list", "tuple") else None

    def element(self, element: tree_sitter.Node) -> list[_Found]:
        """The agents one element of the ``agents=`` list, or the ``manager_agent=`` value, refers to."""
        inline = [a for a in self.agents if a.node.id == element.id]
        if inline:
            return inline
        owner = _innermost(self.crew.node, "class_definition")
        if element.type == "call":
            method = self.reader.reference_part(element.child_by_field_name("function"))
            if owner is None or method is None or not method.startswith("self.") or method.count(".") != 1:
                return []
            name = method.removeprefix("self.")
            return [a for a in self.agents if self.agent_method(a, owner) == name]
        reference = self.reader.reference_part(element)
        if reference is None:
            return []
        if reference.startswith("self.") and reference.count(".") == 1:
            if owner is None:
                return []
            return [
                a for a in self.agents if a.variable == reference and _innermost(a.node, "class_definition") == owner
            ]
        if "." in reference:
            return []
        named = [a for a in self.agents if a.variable == reference]
        earlier = [a for a in named if a.scope == self.crew.scope and a.node.start_byte < self.crew.node.start_byte]
        if earlier:
            return [max(earlier, key=lambda a: a.node.start_byte)]
        root = self.crew.node
        while root.parent is not None:
            root = root.parent
        module = [a for a in named if a.scope == root.id]
        return [max(module, key=lambda a: a.node.start_byte)] if module else []

    def agent_method(self, agent: _Found, owner: tree_sitter.Node) -> str | None:
        """The name of the agent-decorated method of ``owner`` that ``agent`` is in, or None."""
        function = _innermost(agent.node, "function_definition")
        if (
            function is None
            or _owner(function) != owner
            or not self.decorated(function, self.rules.crews.agent_decorators)
        ):
            return None
        name = function.child_by_field_name("name")
        return None if name is None else self.reader.text(name)

    def is_project(self, owner: tree_sitter.Node) -> bool:
        return self.decorated(owner, self.rules.crews.project_decorators)

    def decorated(self, definition: tree_sitter.Node, decorators: frozenset[Symbol]) -> bool:
        """Whether a function or class definition has a decorator that resolves to one of ``decorators``."""
        parent = definition.parent
        if parent is None or parent.type != "decorated_definition":
            return False
        for decorator in parent.named_children:
            if decorator.type != "decorator" or not decorator.named_children:
                continue
            expression = decorator.named_children[0]
            if expression.type == "call":  # ``@agent()``
                expression = expression.child_by_field_name("function") or expression
            reference = self.reader.reference_part(expression)
            if reference is not None and any(s in decorators for s in self.imports.resolve(reference)):
                return True
        return False


@dataclass(frozen=True)
class _Reader:
    source: bytes

    def text(self, node: tree_sitter.Node) -> str:
        return self.source[node.start_byte : node.end_byte].decode("utf-8", errors="replace")

    def reference(self, node: tree_sitter.Node | None) -> str | None:
        """A name or dotted name (``a.b.C``), seen through a subscript (``Agent[Deps]``); None for anything else,
        or for a name of more than ``MAX_REFERENCE_PARTS`` parts."""
        if node is not None and node.type == "subscript":
            node = node.child_by_field_name("value")
        return self.reference_part(node)

    def reference_part(self, node: tree_sitter.Node | None) -> str | None:
        """Like ``reference`` but without the subscript: ``a[0].B`` is not a dotted name."""
        parts: list[str] = []
        while node is not None and node.type == "attribute":
            attr = node.child_by_field_name("attribute")
            if attr is None or len(parts) >= MAX_REFERENCE_PARTS - 1:
                return None
            parts.append(self.text(attr))
            node = node.child_by_field_name("object")
        if node is None or node.type != "identifier":
            return None
        parts.append(self.text(node))
        return ".".join(reversed(parts))

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

    def call(
        self,
        node: tree_sitter.Node,
        path: str,
        imports: ImportTable,
        rules: RuleSet,
        is_local: Callable[[str], bool] | None = None,
    ) -> _Found | None:
        sig = self.find(self.reference(node.child_by_field_name("function")), imports, rules, _CALL_SHAPES)
        if sig is None:
            return None
        if sig.condition == Condition.DEPENDENCY and is_local is not None:
            parts = sig.symbol.module.split(".")
            if any(is_local(".".join(parts[:n])) for n in range(1, len(parts) + 1)):
                return None
        if sig.requires is not None:
            required = self.keyword(node, sig.requires)
            if required is None or required.type in ("false", "none"):
                return None
        scopes = _scopes(node)
        path_names = self.scope_names(scopes)
        literal = self.literal(self.keyword(node, "name"))
        name = literal or self.assigned_to(node)
        if name is not None:
            qualified = ".".join([*path_names, name])
        else:
            functions = [i for i, s in enumerate(scopes) if s.type == "function_definition"]
            crew = sig.condition == Condition.CREW
            method_of = _owner(scopes[functions[-1]]) if crew and functions else None
            if method_of is not None:  # A crew built in a method is named after its class.
                at = scopes.index(method_of)
                name, qualified = path_names[at], ".".join(path_names[: at + 1])
            elif functions:
                name, qualified = path_names[functions[-1]], ".".join(path_names[: functions[-1] + 1])
            elif crew and scopes:
                name, qualified = path_names[-1], ".".join(path_names)
            else:
                name = f"{sig.framework}-{'crew' if crew else 'agent'}"
                qualified = ".".join([*path_names, name])
        model = self.model(self.keyword(node, "model")) or self.model(self.keyword(node, "llm"))
        tools = self.tool_names(self.keyword(node, "tools"))
        match = Match(sig, path, node.start_point[0] + 1, name, qualified, model, tools, named=literal is not None)
        return _Found(node, match, _scope_id(node), self.assigned_target(node))

    def wires(self, call: tree_sitter.Node, imports: ImportTable, rules: RuleSet) -> Wiring | None:
        """What ``call`` wires tools as: a call of a ``wiring.calls`` symbol or of a ``wiring.methods`` method."""
        function = call.child_by_field_name("function")
        if function is None:
            return None
        if function.type == "attribute":
            attribute = function.child_by_field_name("attribute")
            if attribute is not None and self.text(attribute) in rules.wiring.methods:
                return Wiring.BIND_TOOLS
        reference = self.reference(function)
        if reference is not None and any(s in rules.wiring.calls for s in imports.resolve(reference)):
            return Wiring.TOOL_NODE
        return None

    def wiring_reference(
        self, node: tree_sitter.Node, imports: ImportTable, rules: RuleSet, bound: set[str], wildcard: bool
    ) -> bool:
        """Whether a whole name or dotted name (not part of a longer one, nor in an import) refers to a
        ``wiring.references`` symbol. Only names an import binds, or any with a wildcard import, are resolved."""
        parent = node.parent
        if parent is not None and parent.type in ("attribute", *_IMPORT_PARTS):
            return False
        leading = node
        while leading.type == "attribute" and (inner := leading.child_by_field_name("object")) is not None:
            leading = inner
        if leading.type != "identifier" or (not wildcard and self.text(leading) not in bound):
            return False
        reference = self.reference_part(node)
        return reference is not None and any(s in rules.wiring.references for s in imports.resolve(reference))

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
        if not plain_literal(value):
            return None
        return value

    def tool_names(self, node: tree_sitter.Node | None) -> tuple[str, ...]:
        """The plain identifiers in a ``tools=[...]`` list, in order, without repeats."""
        if node is None or node.type != "list":
            return ()
        names = dict.fromkeys(self.text(c) for c in node.named_children if c.type == "identifier")
        return tuple(names)[:MAX_TOOLS]
