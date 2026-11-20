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
file's ``ImportTable``, so a same-named local class, or a type-only import, never matches. Nor does a name
that a function, class, block or the module around the call binds itself (a parameter, a variable or
destructured name, a class or function declaration, a catch parameter; not ``require(...)``): that name is
not the import there. A resolved symbol is looked up as written (``@langchain/langgraph/prebuilt``) and
then by its package root (``@langchain/langgraph``), so a signature listed at the root covers every
subpath of the package.

A match's name is, in order: a short string literal in the ``name:`` property of the first argument (an
options object); the variable the call is assigned to (``const support = new Agent(...)``, ``this.support =``,
a class field ``support = new Agent(...)``, ``exports.support =``; not a bare CommonJS export such as
``module.exports =`` or ``exports.default =``), seen through ``await``, parentheses, ``as``, method chains
(``new StateGraph(s).addNode(...).compile()``) and the values of ``c ? a : b``; the nearest enclosing
named function (a function or method, or an arrow function or function expression named by the ``const``,
property or field it is assigned to); else ``<framework>-agent``. A signature that ``runs`` the agent (AI
SDK ``generateText``) skips the variable, which holds a result. ``Match.qualified`` places the name in the
named classes and functions that enclose it; anonymous functions (callbacks) add nothing.

Matches of one framework named after the same variable in one scope, in different branches of an ``if``,
``try``/``catch`` or ``?:``, are one agent: only the first is kept (see ``branches``). The same
variable assigned twice in one block, or in two ``switch`` cases (a case can fall through), is two agents.

Only these facts are read from the options object: ``model`` when it is a short string literal that looks
like a model name (no ``://``, ``@`` or whitespace), and tool names: the plain identifiers of a
``tools: [a, b]`` array, or the keys of a ``tools: { a, b: x }`` object (the names the model sees).
Prompts, instructions, tool schemas and every other property are never read. ``Condition.TOOL_LOOP``
reads only whether ``tools`` is present (not ``undefined``, ``null``, ``{}`` or ``[]``) and whether
``maxSteps`` is an integer literal of 2 or more or ``stopWhen`` is present (not ``undefined``, ``null`` or
``stepCountIs(n)`` with a literal ``n`` below 2). A ``maxSteps`` that is not a literal does not count.

An agent passed into another as its ``absorbs`` property (``new AgentExecutor({ agent, tools })``) is the
same agent, so only the outer one is reported: a matched call written in place, or the nearest earlier
assignment of the variable or ``this.x`` passed, in the same scope, else in the nearest enclosing function
or the module that has one.

``match_script_file`` also tells what in the file wires a graph to tools (``RuleSet.wiring``), as the
Python matcher does; only a file importing a module that ``ToolWiring.modules`` names is looked at.
"""

import dataclasses
import re
from collections.abc import Iterator
from dataclasses import dataclass

import tree_sitter

from amp_shadow_runner.engine.branches import branch_table, repeats
from amp_shadow_runner.engine.imports import ImportTable, Symbol, package_root
from amp_shadow_runner.engine.match import MAX_REFERENCE_PARTS, MAX_TOOLS, Match, plain_literal
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
_DECLARATIONS = frozenset(
    {
        "function_declaration",
        "generator_function_declaration",
        "class_declaration",
        "abstract_class_declaration",
        "enum_declaration",
    }
)
"""Statements that bind their ``name`` in the scope they are written in."""
_EMPTY = frozenset({"undefined", "null"})
_EXPORTS = frozenset({"exports", "module.exports", "exports.default", "module.exports.default"})
"""CommonJS export targets: what a module exports, not a name."""
_INTEGER = re.compile(r"[0-9][0-9_]*")
_BRANCHES = branch_table(
    {
        "if_statement": frozenset({"consequence", "alternative"}),
        "try_statement": frozenset({"body", "handler"}),
        "ternary_expression": frozenset({"consequence", "alternative"}),
    }
)
"""A ``switch`` is not listed: a case without ``break`` falls through into the next."""


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
    repeated = repeats([(f.node, _repeat_key(f)) for f in found], _BRANCHES)
    found = [f for f in found if f.node.id not in repeated]
    absorbed = _absorbed(found, reader)
    return ScriptMatches(tuple(f.match for f in found if f.node.id not in absorbed), frozenset(wiring))


def _repeat_key(f: _Found) -> tuple[str, int, str] | None:
    """What a match named after the variable it is assigned to must share with another to be its repeat."""
    if f.match.named or f.variable is None or f.match.signature.runs:
        return None
    return f.match.framework, f.scope, f.match.qualified


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
    """The outermost expression that is still ``node``'s object: through ``await``, parentheses, ``as``, method
    chains (``new StateGraph(s).addNode(...).compile()``) and the values of ``c ? a : b``."""
    while (parent := node.parent) is not None:
        if parent.type in _TRANSPARENT or (parent.type == "ternary_expression" and _BRANCHES(parent, node)):
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
    functions around it and the module. A class body around a method is skipped: its fields are not
    variables of its methods."""
    ids = [_scope_id(node)]
    parent = node.parent
    while parent is not None:
        if (parent.type in _FUNCTIONS or parent.parent is None) and parent.id not in ids:
            ids.append(parent.id)
        parent = parent.parent
    return ids


def _is_import(declarator: tree_sitter.Node) -> bool:
    """Whether a ``variable_declarator``'s value is ``require(...)`` or ``await import(...)``: the binding is an
    import, which the ``ImportTable`` already accounts for."""
    value = declarator.child_by_field_name("value")
    if value is not None and value.type == "await_expression":
        value = next((c for c in value.named_children if c.type != "comment"), None)
    if value is None or value.type != "call_expression" or (function := value.child_by_field_name("function")) is None:
        return False
    return function.type == "import" or (function.type == "identifier" and function.text == b"require")


@dataclass(frozen=True)
class _Reader:
    source: bytes
    imports: ImportTable
    rules: RuleSet
    bindings: dict[int, frozenset[str]] = dataclasses.field(default_factory=dict)
    """The names each function, class or module binds itself (see ``bound``), by node id, once read."""

    def text(self, node: tree_sitter.Node) -> str:
        return self.source[node.start_byte : node.end_byte].decode("utf-8", errors="replace")

    def reference(self, node: tree_sitter.Node | None) -> str | None:
        """A name or dotted name (``a.b.C``, ``this.x``); None for anything else, or for a name of more than
        ``MAX_REFERENCE_PARTS`` parts."""
        parts: list[str] = []
        while node is not None and node.type == "member_expression":
            prop = node.child_by_field_name("property")
            if prop is None or prop.type != "property_identifier" or len(parts) >= MAX_REFERENCE_PARTS - 1:
                return None
            parts.append(self.text(prop))
            node = node.child_by_field_name("object")
        if node is None or node.type not in ("identifier", "shorthand_property_identifier", "this"):
            return None
        parts.append(self.text(node))
        return ".".join(reversed(parts))

    def shadowed(self, node: tree_sitter.Node, name: str) -> bool:
        """Whether a function, class or module around ``node`` binds ``name`` itself (a parameter, variable,
        class, function or catch parameter), other than by an import: then ``name`` there is not the import."""
        parent = node.parent
        while parent is not None:
            if (parent.type in _SCOPES or parent.parent is None) and name in self.bound(parent):
                return True
            parent = parent.parent
        return False

    def bound(self, scope: tree_sitter.Node) -> frozenset[str]:
        """The names ``scope`` binds: its own name (not a method's), its parameters, and the variables, classes,
        functions, enums and catch parameters declared anywhere in it outside nested functions and classes.
        Over-counting a block-scoped name only loses a match."""
        cached = self.bindings.get(scope.id)
        if cached is not None:
            return cached
        names: set[str] = set()
        patterns: list[tree_sitter.Node | None] = []
        if scope.type != "method_definition" and (own := scope.child_by_field_name("name")) is not None:
            names.add(self.text(own))
        patterns.append(scope.child_by_field_name("parameter"))
        parameters = scope.child_by_field_name("parameters")
        patterns.extend(parameters.named_children if parameters is not None else ())
        stack = list(scope.children)
        while stack:
            node = stack.pop()
            if node.type in _DECLARATIONS and (declared := node.child_by_field_name("name")) is not None:
                names.add(self.text(declared))
            if node.type in _SCOPES:
                continue  # A nested function or class: its own bindings are its own.
            if node.type == "variable_declarator" and not _is_import(node):
                patterns.append(node.child_by_field_name("name"))
            elif node.type == "catch_clause":
                patterns.append(node.child_by_field_name("parameter"))
            elif node.type == "for_in_statement":
                patterns.append(node.child_by_field_name("left"))
            stack.extend(node.children)
        while patterns:
            pattern = patterns.pop()
            if pattern is None:
                continue
            if pattern.type in ("identifier", "shorthand_property_identifier_pattern"):
                names.add(self.text(pattern))
            elif pattern.type in ("object_pattern", "array_pattern", "rest_pattern"):
                patterns.extend(pattern.named_children)
            elif pattern.type == "pair_pattern":
                patterns.append(pattern.child_by_field_name("value"))
            elif pattern.type in ("assignment_pattern", "object_assignment_pattern"):
                patterns.append(pattern.child_by_field_name("left"))
            elif pattern.type in ("required_parameter", "optional_parameter"):
                patterns.append(pattern.child_by_field_name("pattern"))
        found = frozenset(names)
        self.bindings[scope.id] = found
        return found

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
        reference = self.reference(self.target(node))
        sig = self.find(reference)
        if sig is None or reference is None or self.shadowed(node, reference.split(".", 1)[0]):
            return None
        options = self.options(node)
        met: bool | None = None
        if sig.condition == Condition.TOOL_LOOP:
            if not self.given(self.property(options, "tools")):
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

    def given(self, value: tree_sitter.Node | None) -> bool:
        """Whether a property value is given: not missing, ``undefined``, ``null``, ``{}`` or ``[]``."""
        if value is None or value.type in _EMPTY:
            return False
        if value.type in ("object", "array"):
            return any(c.type != "comment" for c in value.named_children)
        return True

    def loops(self, options: tree_sitter.Node | None) -> bool:
        """Whether a model call takes more than one step: ``maxSteps`` an integer literal of 2 or more, or a
        ``stopWhen`` other than ``undefined``, ``null`` or ``stepCountIs(n)`` with a literal ``n`` below 2."""
        stop = self.property(options, "stopWhen")
        if stop is not None and stop.type not in _EMPTY and not self.one_step(stop):
            return True
        steps = self.property(options, "maxSteps")
        return steps is not None and (count := self.integer(steps)) is not None and count >= 2

    def one_step(self, stop: tree_sitter.Node) -> bool:
        """Whether a ``stopWhen`` is ``stepCountIs(n)`` with an integer literal ``n`` below 2."""
        function = stop.child_by_field_name("function") if stop.type == "call_expression" else None
        reference = self.reference(function)
        if reference is None or reference.rsplit(".", 1)[-1] != "stepCountIs":
            return False
        arguments = stop.child_by_field_name("arguments")
        args = [a for a in arguments.named_children if a.type != "comment"] if arguments is not None else []
        count = self.integer(args[0]) if len(args) == 1 else None
        return count is not None and count < 2

    def integer(self, node: tree_sitter.Node) -> int | None:
        """The value of a plain decimal integer literal (``5``, ``1_000``), or None for anything else."""
        text = self.text(node)
        return int(text.replace("_", "")) if node.type == "number" and _INTEGER.fullmatch(text) else None

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
            assigned = self.reference(parent.child_by_field_name("left"))
            return None if assigned in _EXPORTS else assigned
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
        return value if plain_literal(value) else None

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
