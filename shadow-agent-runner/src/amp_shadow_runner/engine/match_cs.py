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

"""Find the uses of agent signatures in a parsed C# file.

The ``imports`` passed in are every using in scope for the file: its own, the ``global using`` directives
of its project's other files and its ``.csproj`` ``<Using>`` items (``scan`` merges them). Three shapes are
matched:

- ``new ChatCompletionAgent { ... }``, ``new ChatCompletionAgent(...)``, and a target-typed
  ``ChatCompletionAgent agent = new() { ... }`` (the declared type of the variable, field or property);
- ``OpenAIAssistantAgent.CreateAsync(...)``, a static factory (``Shape.FACTORY``);
- ``client.CreateAIAgent(...)``, an extension method (``Shape.EXTENSION``), only with its namespace in scope.

A class or factory that resolves through the imports, or a fully qualified name, proves an agent
(``Match.condition_met`` true). When nothing resolves but the name is written bare, exactly as a signature's
class (``ChatCompletionAgent``, ``OpenAIAssistantAgent.CreateAsync``), it is the lookalike case: a Match with
``condition_met`` false, which ``classify`` makes Not sure. A name qualified with another namespace is no
match.

A match's name is, in order: the ``Name = "..."`` object-initializer property or the ``name: "..."``
named argument, when a short plain string literal; the variable, field or property it is assigned to (seen
through ``await`` and parentheses; ``this.agent = ...`` gives ``agent``); the nearest enclosing method,
constructor, local function or property; the nearest enclosing class, struct, record or interface; else
``<framework>-agent``. ``Match.qualified`` places it in its classes and members, without the namespace.
No other argument, initializer or literal is read.
"""

from collections.abc import Iterator
from dataclasses import dataclass

import tree_sitter

from amp_shadow_runner.engine.imports import ImportKind, ImportTable
from amp_shadow_runner.engine.match import MAX_LITERAL_CHARS, Match
from amp_shadow_runner.engine.signatures import Condition, RuleSet, Shape, Signature
from amp_shadow_runner.engine.signatures.csharp import CSHARP

_TYPES = frozenset(
    {
        "class_declaration",
        "struct_declaration",
        "record_declaration",
        "record_struct_declaration",
        "interface_declaration",
    }
)
_MEMBERS = frozenset(
    {"method_declaration", "constructor_declaration", "local_function_statement", "property_declaration"}
)
_SCOPES = _TYPES | _MEMBERS
_TRANSPARENT = frozenset({"await_expression", "parenthesized_expression"})
_CREATIONS = frozenset({"object_creation_expression", "implicit_object_creation_expression"})


def mentions(source: bytes, rules: RuleSet = CSHARP) -> bool:
    """Whether ``source`` holds the name of any class or method in ``rules``: a file that does not can
    hold no match, so it need not be matched."""
    return any(name.encode() in source for name in _names(rules))


def _names(rules: RuleSet) -> set[str]:
    return {
        (sig.symbol.module.rsplit(".", 1)[-1] if sig.shape == Shape.FACTORY else sig.symbol.name or "")
        for sig in rules.signatures
    } - {""}


def match_csharp_file(
    path: str, tree: tree_sitter.Tree, source: bytes, imports: ImportTable, rules: RuleSet = CSHARP
) -> tuple[Match, ...]:
    """Every use of a signature in ``rules`` in the parsed C# file at ``path``, in source order."""
    reader = _Reader(source, imports, rules, _bare(rules), _extensions(rules, imports))
    found: list[Match] = []
    for node in _walk(tree.root_node):
        if node.type in _CREATIONS or node.type == "invocation_expression":
            match = reader.match(node, path)
            if match is not None:
                found.append(match)
    return tuple(found)


def _bare(rules: RuleSet) -> dict[str, Signature]:
    """The names written bare (``ChatCompletionAgent``, ``OpenAIAssistantAgent.CreateAsync``) of the
    ``Condition.NAMESPACE`` signatures, for the lookalike case."""
    bare: dict[str, Signature] = {}
    for sig in rules.signatures:
        if sig.condition != Condition.NAMESPACE or sig.symbol.name is None:
            continue
        if sig.shape == Shape.FACTORY:
            bare.setdefault(f"{sig.symbol.module.rsplit('.', 1)[-1]}.{sig.symbol.name}", sig)
        else:
            bare.setdefault(sig.symbol.name, sig)
    return bare


def _extensions(rules: RuleSet, imports: ImportTable) -> dict[str, Signature]:
    """The extension-method signatures whose namespace (or one under it) a namespace using imports, by name."""
    namespaces = {imp.module for imp in imports.imports if imp.kind == ImportKind.NAMESPACE and not imp.type_only}
    found: dict[str, Signature] = {}
    for sig in rules.signatures:
        if sig.shape != Shape.EXTENSION or sig.symbol.name is None:
            continue
        root = sig.symbol.module
        if any(ns == root or ns.startswith(f"{root}.") for ns in namespaces):
            found.setdefault(sig.symbol.name, sig)
    return found


def _walk(root: tree_sitter.Node) -> Iterator[tree_sitter.Node]:
    stack = [root]
    while stack:
        node = stack.pop()
        yield node
        stack.extend(reversed(node.children))


def _scopes(node: tree_sitter.Node) -> list[tree_sitter.Node]:
    """The types and members enclosing ``node``, outermost first."""
    scopes: list[tree_sitter.Node] = []
    parent = node.parent
    while parent is not None:
        if parent.type in _SCOPES:
            scopes.append(parent)
        parent = parent.parent
    return scopes[::-1]


def _outer(node: tree_sitter.Node) -> tree_sitter.Node:
    """``node`` with the ``await`` and parentheses around it."""
    while node.parent is not None and node.parent.type in _TRANSPARENT:
        node = node.parent
    return node


@dataclass(frozen=True)
class _Reader:
    source: bytes
    imports: ImportTable
    rules: RuleSet
    bare: dict[str, Signature]
    extensions: dict[str, Signature]

    def text(self, node: tree_sitter.Node) -> str:
        return self.source[node.start_byte : node.end_byte].decode("utf-8", errors="replace")

    def reference(self, node: tree_sitter.Node | None) -> str | None:
        """A type or member name as a dotted path without generic arguments or ``global::``; else None."""
        if node is None:
            return None
        if node.type == "identifier":
            return self.text(node)
        if node.type == "generic_name":
            name = next((c for c in node.named_children if c.type == "identifier"), None)
            return None if name is None else self.text(name)
        if node.type == "nullable_type":
            return self.reference(node.child_by_field_name("type"))
        if node.type == "alias_qualified_name":
            alias = node.child_by_field_name("alias")
            if alias is None or self.text(alias) != "global":
                return None  # An extern alias: not a namespace path.
            return self.reference(node.child_by_field_name("name"))
        if node.type in ("qualified_name", "member_access_expression"):
            left = node.child_by_field_name("qualifier" if node.type == "qualified_name" else "expression")
            head, tail = self.reference(left), self.reference(node.child_by_field_name("name"))
            return None if head is None or tail is None else f"{head}.{tail}"
        return None

    def find(self, reference: str | None, shapes: tuple[Shape, ...]) -> tuple[Signature, bool] | None:
        """The signature ``reference`` names, and whether it resolved (True) or only matched bare (False)."""
        if reference is None:
            return None
        for symbol in self.imports.resolve(reference):
            sig = self.rules.find(symbol, shapes)
            if sig is not None:
                return sig, True
        sig = self.bare.get(reference)
        return (sig, False) if sig is not None and sig.shape in shapes else None

    def match(self, node: tree_sitter.Node, path: str) -> Match | None:
        if node.type == "object_creation_expression":
            found = self.find(self.reference(node.child_by_field_name("type")), (Shape.CALL,))
        elif node.type == "implicit_object_creation_expression":
            found = self.find(self.reference(self.declared_type(node)), (Shape.CALL,))
        else:
            found = self.invocation(node)
        if found is None:
            return None
        sig, resolved = found
        met = resolved if sig.condition == Condition.NAMESPACE else None
        scopes = _scopes(node)
        if scopes and scopes[-1].type == "property_declaration" and _outer(node).parent == scopes[-1]:
            scopes.pop()  # A property's initializer: the property is the name, not a scope.
        names = [self.scope_name(s) for s in scopes]
        literal = self.literal_name(node)
        name = literal or self.assigned_to(node)
        if name is not None:
            qualified = ".".join([*names, name])
        else:
            members = [i for i, s in enumerate(scopes) if s.type in _MEMBERS]
            types = [i for i, s in enumerate(scopes) if s.type in _TYPES]
            if members or types:
                at = (members or types)[-1]
                name, qualified = names[at], ".".join(names[: at + 1])
            else:
                name = qualified = f"{sig.framework}-agent"
        return Match(sig, path, node.start_point[0] + 1, name, qualified, condition_met=met, named=literal is not None)

    def invocation(self, node: tree_sitter.Node) -> tuple[Signature, bool] | None:
        function = node.child_by_field_name("function")
        if function is None:
            return None
        if function.type == "member_access_expression":
            method = self.reference(function.child_by_field_name("name"))
            sig = self.extensions.get(method or "")
            if sig is not None:
                return sig, True
        return self.find(self.reference(function), (Shape.FACTORY,))

    def declared_type(self, node: tree_sitter.Node) -> tree_sitter.Node | None:
        """The type a target-typed ``new()`` takes from the variable, field or property it initialises."""
        parent = node.parent
        if parent is not None and parent.type == "variable_declarator":
            declaration = parent.parent
            return None if declaration is None else declaration.child_by_field_name("type")
        if parent is not None and parent.type == "property_declaration":
            return parent.child_by_field_name("type")
        return None

    def scope_name(self, scope: tree_sitter.Node) -> str:
        name = scope.child_by_field_name("name")
        return "_" if name is None else self.text(name)

    def literal_name(self, node: tree_sitter.Node) -> str | None:
        """The ``Name = "..."`` initializer property or the ``name: "..."`` named argument, as a literal."""
        initializer = next((c for c in node.named_children if c.type == "initializer_expression"), None)
        for item in initializer.named_children if initializer is not None else ():
            left = item.child_by_field_name("left")
            if item.type == "assignment_expression" and left is not None and self.text(left) == "Name":
                return self.literal(item.child_by_field_name("right"))
        arguments = node.child_by_field_name("arguments")
        if arguments is None:
            arguments = next((c for c in node.named_children if c.type == "argument_list"), None)
        for argument in arguments.named_children if arguments is not None else ():
            key = argument.child_by_field_name("name")
            if argument.type == "argument" and key is not None and self.text(key) == "name":
                value = next((c for c in reversed(argument.named_children) if c != key), None)
                return self.literal(value)
        return None

    def assigned_to(self, node: tree_sitter.Node) -> str | None:
        """The variable, field, property or member (``this.agent``) ``node`` is assigned to."""
        node = _outer(node)
        parent = node.parent
        if parent is None:
            return None
        if parent.type in ("variable_declarator", "property_declaration"):
            name = parent.child_by_field_name("name")
            return None if name is None else self.text(name)
        if parent.type == "assignment_expression" and parent.child_by_field_name("right") == node:
            target = self.reference(parent.child_by_field_name("left"))
            if target is None:
                left = parent.child_by_field_name("left")
                inner = left.child_by_field_name("name") if left is not None else None
                target = self.reference(inner)  # ``this.agent``: ``this`` is no name.
            return None if target is None else target.rsplit(".", 1)[-1]
        return None

    def literal(self, node: tree_sitter.Node | None) -> str | None:
        """The value of a short, single-line, plain string literal (no escapes, not verbatim), or None."""
        if node is None or node.type != "string_literal":
            return None
        if any(c.type != "string_literal_content" for c in node.named_children):
            return None
        value = "".join(self.text(c) for c in node.named_children).strip()
        if not value or len(value) > MAX_LITERAL_CHARS or "\\" in value or not value.isprintable():
            return None
        return value
