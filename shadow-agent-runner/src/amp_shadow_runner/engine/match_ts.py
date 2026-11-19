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

"""Find the uses of agent signatures in a parsed TypeScript or JavaScript file.

``new X(...)`` and ``X(...)`` targets (``Agent``, ``m.Agent``, ``Agent.create``) are resolved through the
file's ``ImportTable``, so a same-named local class, or a type-only import, never matches. A resolved
symbol is looked up as written (``@langchain/langgraph/prebuilt``) and then by its package root
(``@langchain/langgraph``), so a signature listed at the root covers every subpath of the package.

A match's name is, in order: a short string literal in the ``name:`` property of the first argument (an
options object); the variable the call is assigned to (``const support = new Agent(...)``, ``this.support =``,
a class field ``support = new Agent(...)``), seen through ``await``, parentheses, ``as`` and method chains
(``new StateGraph(s).addNode(...).compile()``); the nearest enclosing named function (a function or
method, or an arrow function or function expression named by the ``const``, property or field it is
assigned to); else ``<framework>-agent``. A signature that ``runs`` the agent (AI SDK ``generateText``)
skips the variable, which holds a result. ``Match.qualified`` places the name in the named classes and
functions that enclose it; anonymous functions (callbacks) add nothing.

Only these facts are read from the options object: ``model`` when it is a short string literal that looks
like a model name (no ``://``, ``@`` or whitespace), and tool names: the plain identifiers of a
``tools: [a, b]`` array, or the keys of a ``tools: { a, b: x }`` object (the names the model sees).
Prompts, instructions, tool schemas and every other property are never read. ``Condition.TOOL_LOOP``
reads only whether ``tools`` is present and whether ``maxSteps`` is a number literal above 1 or ``stopWhen``
is present (a ``maxSteps`` that is not a literal does not count).

An agent passed into another as its ``absorbs`` property (``new AgentExecutor({ agent, tools })``) is the
same agent, so only the outer one is reported: a matched call written in place, or the nearest earlier
assignment, in the same scope, of the variable or ``this.x`` passed.

``match_script_file`` also tells what in the file wires a graph to tools (``RuleSet.wiring``), as the
Python matcher does; only a file importing a module that ``ToolWiring.modules`` names is looked at.
"""

from collections.abc import Iterator
from dataclasses import dataclass

import tree_sitter

from amp_shadow_runner.engine.imports import ImportTable, Symbol, package_root
from amp_shadow_runner.engine.match import MAX_LITERAL_CHARS, MAX_TOOLS, Match
from amp_shadow_runner.engine.signatures import Condition, RuleSet, Shape, Signature, Wiring
from amp_shadow_runner.engine.signatures.typescript import TYPESCRIPT

_CALL_SHAPES = (Shape.CALL, Shape.FACTORY)
_CALLS = frozenset({"call_expression", "new_expression"})
_FUNCTIONS = frozenset(
    {
        "function_declaration",
        "generator_function_declaration",
        "function_expression",
        "function",
        "generator_function",
        "arrow_function",
        "method_definition",
    }
)
_CLASSES = frozenset({"class_declaration", "abstract_class_declaration", "class"})
_SCOPES = _FUNCTIONS | _CLASSES
_TRANSPARENT = frozenset(
    {"await_expression", "parenthesized_expression", "as_expression", "satisfies_expression", "non_null_expression"}
)
"""Expressions that wrap a value without changing which object it is."""
_FIELDS = frozenset({"field_definition", "public_field_definition"})
_IMPORT_PARTS = frozenset({"import_specifier", "namespace_import", "import_clause", "import_require_clause"})


@dataclass(frozen=True)
class ScriptMatches:
    """What matching one TypeScript or JavaScript file found."""

    matches: tuple[Match, ...]
    wiring: frozenset[Wiring] = frozenset()
    """What in the file wires a graph to tools (``RuleSet.wiring``); empty when nothing does."""


@dataclass(frozen=True)
class _Found:
    node: tree_sitter.Node
    match: Match
    scope: int
    variable: str | None = None


def match_script_file(
    path: str, tree: tree_sitter.Tree, source: bytes, imports: ImportTable, rules: RuleSet = TYPESCRIPT
) -> ScriptMatches:
    """Every use of a signature in ``rules`` in the parsed TS/JS file at ``path``, in source order, and what in
    the file wires tools."""
    reader = _Reader(source, imports, rules)
    look = any(not imp.type_only and imp.module.startswith(rules.wiring.modules) for imp in imports.imports)
    references = look and bool(rules.wiring.references)
    found: list[_Found] = []
    wiring: set[Wiring] = set()
    for node in _walk(tree.root_node):
        if node.type in _CALLS:
            item = reader.call(node, path)
            if item is not None:
                found.append(item)
            if look and (kind := reader.wires(node)) is not None:
                wiring.add(kind)
        elif (
            references
            and Wiring.TOOLS_CONDITION not in wiring
            and node.type in ("identifier", "member_expression")
            and reader.wiring_reference(node)
        ):
            wiring.add(Wiring.TOOLS_CONDITION)
    absorbed = _absorbed(found, reader)
    return ScriptMatches(tuple(f.match for f in found if f.node.id not in absorbed), frozenset(wiring))


def _walk(root: tree_sitter.Node) -> Iterator[tree_sitter.Node]:
    """Every node under ``root``, in source order, without recursion."""
    stack = [root]
    while stack:
        node = stack.pop()
        yield node
        stack.extend(reversed(node.children))


def _scopes(node: tree_sitter.Node) -> list[tree_sitter.Node]:
    """The functions and classes enclosing ``node``, outermost first."""
    scopes: list[tree_sitter.Node] = []
    parent = node.parent
    while parent is not None:
        if parent.type in _SCOPES:
            scopes.append(parent)
        parent = parent.parent
    return scopes[::-1]


def _scope_id(node: tree_sitter.Node) -> int:
    parent = node.parent
    while parent is not None:
        if parent.type in _SCOPES or parent.parent is None:
            return parent.id
        parent = parent.parent
    return node.id


def _outer(node: tree_sitter.Node) -> tree_sitter.Node:
    """The outermost expression that is still ``node``'s object: through ``await``, parentheses, ``as`` and
    method chains (``new StateGraph(s).addNode(...).compile()``)."""
    while (parent := node.parent) is not None:
        if parent.type in _TRANSPARENT:
            node = parent
        elif (
            parent.type == "member_expression"
            and parent.child_by_field_name("object") == node
            and (call := parent.parent) is not None
            and call.type == "call_expression"
            and call.child_by_field_name("function") == parent
        ):
            node = call
        else:
            return node
    return node


def _inner(node: tree_sitter.Node) -> tree_sitter.Node:
    """``node`` without the ``await``, parentheses or ``as`` around it."""
    while node.type in _TRANSPARENT:
        value = next((c for c in node.named_children if c.type != "comment"), None)
        if value is None:
            return node
        node = value
    return node


def _absorbed(found: list[_Found], reader: "_Reader") -> set[int]:
    """The ids of matched nodes that another match takes in through its ``absorbs`` property."""
    by_node = {f.node.id for f in found}
    absorbed: set[int] = set()
    for f in found:
        sig = f.match.signature
        if sig.absorbs is None:
            continue
        value = reader.property(reader.options(f.node), sig.absorbs)
        if value is None:
            continue
        value = _inner(value)
        if value.id in by_node:
            absorbed.add(value.id)
            continue
        key = reader.reference(value)
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
    imports: ImportTable
    rules: RuleSet

    def text(self, node: tree_sitter.Node) -> str:
        return self.source[node.start_byte : node.end_byte].decode("utf-8", errors="replace")

    def reference(self, node: tree_sitter.Node | None) -> str | None:
        """A name or dotted name (``a.b.C``, ``this.x``); None for anything else."""
        if node is None:
            return None
        if node.type in ("identifier", "shorthand_property_identifier", "this"):
            return self.text(node)
        if node.type == "member_expression":
            obj = self.reference(node.child_by_field_name("object"))
            prop = node.child_by_field_name("property")
            if obj is not None and prop is not None and prop.type == "property_identifier":
                return f"{obj}.{self.text(prop)}"
        return None

    def symbols(self, reference: str | None) -> Iterator[Symbol]:
        """The symbols ``reference`` resolves to, each as written and then by its package root."""
        if reference is None or reference.startswith("this."):
            return
        for symbol in self.imports.resolve(reference):
            yield symbol
            root = package_root(symbol.module)
            if root != symbol.module:
                yield Symbol(root, symbol.name)

    def find(self, reference: str | None) -> Signature | None:
        for symbol in self.symbols(reference):
            sig = self.rules.find(symbol, _CALL_SHAPES)
            if sig is not None:
                return sig
        return None

    def target(self, call: tree_sitter.Node) -> tree_sitter.Node | None:
        return call.child_by_field_name("constructor" if call.type == "new_expression" else "function")

    def options(self, call: tree_sitter.Node) -> tree_sitter.Node | None:
        """The first argument of a call when it is an object literal, or None."""
        arguments = call.child_by_field_name("arguments")
        if arguments is None:
            return None
        first = next((a for a in arguments.named_children if a.type != "comment"), None)
        return first if first is not None and first.type == "object" else None

    def property(self, options: tree_sitter.Node | None, name: str) -> tree_sitter.Node | None:
        """The value of property ``name`` in an object literal (the identifier itself for ``{ name }``), or None."""
        if options is None:
            return None
        for prop in options.named_children:
            if prop.type == "shorthand_property_identifier" and self.text(prop) == name:
                return prop
            if prop.type == "pair":
                key = prop.child_by_field_name("key")
                if key is not None and self.key(key) == name:
                    return prop.child_by_field_name("value")
        return None

    def key(self, key: tree_sitter.Node) -> str | None:
        """An object key written as an identifier or a plain string."""
        if key.type == "property_identifier":
            return self.text(key)
        return self.literal(key) if key.type == "string" else None

    def call(self, node: tree_sitter.Node, path: str) -> _Found | None:
        sig = self.find(self.reference(self.target(node)))
        if sig is None:
            return None
        options = self.options(node)
        met: bool | None = None
        if sig.condition == Condition.TOOL_LOOP:
            if self.property(options, "tools") is None:
                return None
            met = self.loops(options)
        scopes = _scopes(node)
        names = [self.scope_name(s) for s in scopes]
        path_names = [n for n in names if n is not None]
        variable = self.assigned_target(node)
        assigned = None if sig.runs or variable is None else variable.rsplit(".", 1)[-1]
        literal = self.literal(self.property(options, "name"))
        name = literal or assigned
        if name is not None:
            qualified = ".".join([*path_names, name])
        else:
            functions = [i for i, s in enumerate(scopes) if s.type in _FUNCTIONS and names[i] is not None]
            if functions:
                at = functions[-1]
                name = names[at] or ""
                qualified = ".".join(n for n in names[: at + 1] if n is not None)
            else:
                name = f"{sig.framework}-agent"
                qualified = ".".join([*path_names, name])
        model = self.model(self.property(options, "model"))
        tools = self.tool_names(self.property(options, "tools"))
        named = literal is not None
        match = Match(sig, path, node.start_point[0] + 1, name, qualified, model, tools, condition_met=met, named=named)
        return _Found(node, match, _scope_id(node), variable)

    def loops(self, options: tree_sitter.Node | None) -> bool:
        """Whether a model call takes more than one step: ``maxSteps`` above 1, or any ``stopWhen``."""
        if self.property(options, "stopWhen") is not None:
            return True
        steps = self.property(options, "maxSteps")
        if steps is None or steps.type != "number":
            return False
        try:
            return float(self.text(steps)) > 1
        except ValueError:
            return False  # Hex, octal or BigInt literals: not worth reading.

    def scope_name(self, scope: tree_sitter.Node) -> str | None:
        """A function's or class's name, or for an anonymous function the name it is assigned to; else None."""
        name = scope.child_by_field_name("name")
        if name is not None:
            return self.text(name)
        if scope.type not in _FUNCTIONS:
            return None
        parent = scope.parent
        if parent is None:
            return None
        if parent.type == "variable_declarator" and parent.child_by_field_name("value") == scope:
            target = parent.child_by_field_name("name")
            return self.text(target) if target is not None and target.type == "identifier" else None
        if parent.type == "pair" and parent.child_by_field_name("value") == scope:
            key = parent.child_by_field_name("key")
            return None if key is None else self.key(key)
        if parent.type in _FIELDS and parent.child_by_field_name("value") == scope:
            field = parent.child_by_field_name("property") or parent.child_by_field_name("name")
            return None if field is None else self.text(field)
        if parent.type == "assignment_expression" and parent.child_by_field_name("right") == scope:
            left = self.reference(parent.child_by_field_name("left"))
            return None if left is None else left.rsplit(".", 1)[-1]
        return None

    def assigned_target(self, call: tree_sitter.Node) -> str | None:
        """What ``call`` is assigned to, as written: ``x``, ``this.x``, or a class field's name."""
        node = _outer(call)
        parent = node.parent
        if parent is None:
            return None
        if parent.type == "variable_declarator" and parent.child_by_field_name("value") == node:
            target = parent.child_by_field_name("name")
            return self.text(target) if target is not None and target.type == "identifier" else None
        if parent.type == "assignment_expression" and parent.child_by_field_name("right") == node:
            return self.reference(parent.child_by_field_name("left"))
        if parent.type in _FIELDS and parent.child_by_field_name("value") == node:
            field = parent.child_by_field_name("property") or parent.child_by_field_name("name")
            return self.text(field) if field is not None and field.type == "property_identifier" else None
        return None

    def wires(self, call: tree_sitter.Node) -> Wiring | None:
        """What ``call`` wires tools as: a ``wiring.calls`` symbol, or a call of a ``wiring.methods`` method."""
        function = self.target(call)
        if function is None:
            return None
        if call.type == "call_expression" and function.type == "member_expression":
            prop = function.child_by_field_name("property")
            if prop is not None and self.text(prop) in self.rules.wiring.methods:
                return Wiring.BIND_TOOLS
        if any(s in self.rules.wiring.calls for s in self.symbols(self.reference(function))):
            return Wiring.TOOL_NODE
        return None

    def wiring_reference(self, node: tree_sitter.Node) -> bool:
        """Whether a whole name or dotted name (not part of a longer one, nor in an import) refers to a
        ``wiring.references`` symbol."""
        parent = node.parent
        if parent is not None and (parent.type == "member_expression" or parent.type in _IMPORT_PARTS):
            return False
        return any(s in self.rules.wiring.references for s in self.symbols(self.reference(node)))

    def model(self, node: tree_sitter.Node | None) -> str | None:
        """A model name from a string literal; None for a URL, an address or anything with whitespace."""
        value = self.literal(node)
        if value is None or "://" in value or "@" in value or any(c.isspace() for c in value):
            return None
        return value

    def literal(self, node: tree_sitter.Node | None) -> str | None:
        """The value of a short, single-line, plain string literal (no escapes), or None for anything else."""
        if node is None or node.type != "string":
            return None
        if any(c.type != "string_fragment" for c in node.named_children):
            return None
        value = "".join(self.text(c) for c in node.named_children).strip()
        if not value or len(value) > MAX_LITERAL_CHARS or "\\" in value or not value.isprintable():
            return None
        return value

    def tool_names(self, node: tree_sitter.Node | None) -> tuple[str, ...]:
        """The plain identifiers of a ``tools: [...]`` array, or the keys of a ``tools: {...}`` object."""
        if node is None:
            return ()
        if node.type == "array":
            found = [self.text(c) for c in node.named_children if c.type == "identifier"]
        elif node.type == "object":
            found = []
            for prop in node.named_children:
                if prop.type == "shorthand_property_identifier":
                    found.append(self.text(prop))
                elif prop.type == "pair" and (key := prop.child_by_field_name("key")) is not None:
                    if key.type == "property_identifier":
                        found.append(self.text(key))
        else:
            return ()
        return tuple(dict.fromkeys(found))[:MAX_TOOLS]
