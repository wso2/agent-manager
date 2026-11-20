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

"""Read a file's imports and resolve the names its code uses back to the modules they come from.

``extract_imports`` walks the whole syntax tree, so imports inside functions count (lazy Python imports
are common). ``ImportTable.resolve`` turns a reference as written in code (``"AE"``, ``"c.Agent"``,
``"Kernel"``) into the ``Symbol`` values it can stand for. A name no import binds resolves to ``()``, so a
local class named ``Agent`` never looks like a framework's ``Agent``. Local definitions are not tracked:
a later ``class Agent`` does not undo ``from strands import Agent``.

How a reference becomes a ``Symbol``:

- Python and C# name things with one dotted path, so the reference's path is split at its last dot:
  ``from a.b import C`` then ``C`` and ``import a.b`` then ``a.b.C`` both give ``Symbol("a.b", "C")``.
  A Python module referenced on its own (``import a.b as c`` then ``c``) gives ``Symbol("a.b", None)``.
  A relative Python import keeps its leading dots in ``Symbol.module`` (``Symbol(".tools", "y")``).
- TypeScript and JavaScript module specifiers are opaque, so ``Symbol.module`` is the specifier and
  ``Symbol.name`` the dotted member path in it: ``import * as m`` then ``m.Agent`` gives
  ``Symbol(m's module, "Agent")``, and a default import gives the member ``"default"``.

Per language:

- Python: when the same local name is imported more than once, every import is a candidate, in source
  order, since fallbacks are common (``try: from llama_index.core.agent import ReActAgent`` /
  ``except ImportError: from llama_index.agent import ReActAgent`` gives both). ``from a import *`` makes
  ``a`` a candidate only for names no other import binds. Imports inside ``if TYPE_CHECKING:`` (or
  ``if typing.TYPE_CHECKING:``) are type-only; those in its ``elif`` or ``else`` branches are not.
  ``__future__`` imports, ``importlib`` and ``__import__`` are ignored.
- TypeScript and JavaScript: ES imports, TypeScript ``import x = require("m")``, and ``require("m")`` or
  ``await import("m")`` (with a literal specifier) assigned straight to a name or a destructuring pattern.
  When the same local name is imported twice, the last import wins. ``require("m").sub``, other dynamic
  ``import()`` forms, computed destructuring keys and re-exports (``export ... from``) are ignored.
- C#: a ``using`` alias wins (the last one, if repeated); otherwise every ``using`` namespace and
  ``using static`` type gives a candidate for a single name (``Kernel``; a using does not bring in the
  namespaces under it, so ``Agents.X`` is not ``Microsoft.SemanticKernel.Agents.X`` through
  ``using Microsoft.SemanticKernel;``), and a dotted reference is taken as fully qualified
  (``Microsoft.SemanticKernel.Kernel`` with no ``using``). A member of an imported type
  (``Kernel.CreateBuilder``) is resolved by the matcher, which resolves the type first. Usings apply to the whole file, whatever
  namespace block holds them; ``global using`` is recorded in ``Import.global_`` but not shared with other
  files here. ``using`` statements and declarations (``using (var x = ...)``) are not imports.

Type-only imports (TS ``import type``, Python ``TYPE_CHECKING``) are kept in ``ImportTable.imports`` but
``resolve`` skips them, since a type is no evidence of running code.

Ballerina has no grammar, so ``extract_imports`` raises ``ValueError`` for it.
"""

import dataclasses
import functools
from collections.abc import Iterator
from dataclasses import dataclass
from enum import StrEnum

import tree_sitter

from amp_shadow_runner.engine.languages import Language


@dataclass(frozen=True)
class Symbol:
    """What a reference resolves to: a module or namespace, and a name in it (None for the module itself)."""

    module: str
    name: str | None


class ImportKind(StrEnum):
    """How an import binds its local name."""

    MODULE = "module"
    """A whole module: Python ``import a.b`` (binds ``a``), TS/JS ``import * as m``, ``m = require("m")``."""
    ALIAS = "alias"
    """A path under another name: Python ``import a.b as c``, C# ``using K = A.B.Kernel;``."""
    MEMBER = "member"
    """One member: Python ``from a import C``, TS/JS ``import { C }``, ``const { C } = require("m")``."""
    DEFAULT = "default"
    """A TS/JS default import, ``import x from "m"`` (the member ``default``)."""
    WILDCARD = "wildcard"
    """Python ``from a import *``."""
    SIDE_EFFECT = "side_effect"
    """TS/JS ``import "m"``, which binds nothing."""
    NAMESPACE = "namespace"
    """C# ``using A.B;`` (also ``global using``)."""
    STATIC = "static"
    """C# ``using static A.B;``."""


@dataclass(frozen=True)
class Import:
    """One binding made by an import statement.

    ``module`` is as written; for a relative Python import it is the part after the dots, and ``level`` is
    the number of dots. ``local`` is the name the code uses, or None when the import binds no single name
    (wildcards, side-effect imports, C# namespace and static usings). ``line`` is 1-based. ``type_only`` marks
    a TS type-only import or a Python import under ``if TYPE_CHECKING:``; ``global_`` a C# ``global using``.
    """

    module: str
    name: str | None
    local: str | None
    kind: ImportKind
    line: int
    level: int = 0
    type_only: bool = False
    global_: bool = False


@dataclass(frozen=True)
class ImportTable:
    """The imports of one file, and how to resolve the names its code uses."""

    language: Language
    imports: tuple[Import, ...]

    def resolve(self, reference: str) -> tuple[Symbol, ...]:
        """The symbols ``reference`` (a name or dotted name as written in code) can refer to through imports.

        Returns ``()`` when no import accounts for it. C# namespace usings can give several candidates.
        """
        if self.language == Language.C_SHARP:
            reference = reference.removeprefix("global::")
        parts = reference.split(".")
        if not all(parts):
            return ()
        bound = self._bindings.get(parts[0])
        if bound:
            return tuple(dict.fromkeys(self._through(imp, parts) for imp in bound))
        candidates: list[Symbol] = []
        for imp in self.imports:
            # A C# using brings in a namespace's types (or a type's members), not the namespaces under it:
            # ``using A;`` makes ``C`` stand for ``A.C`` but not ``B.C`` for ``A.B.C``.
            if imp.kind == ImportKind.WILDCARD or (
                imp.kind in (ImportKind.NAMESPACE, ImportKind.STATIC) and len(parts) == 1
            ):
                candidates.append(_split(_join(_base(imp), *parts)))
        if self.language == Language.C_SHARP and len(parts) > 1:
            candidates.append(_split(reference))
        return tuple(dict.fromkeys(candidates))

    @functools.cached_property
    def _bindings(self) -> dict[str, list[Import]]:
        """Local name to the imports that bind it: all of them for Python, the last one otherwise."""
        bindings: dict[str, list[Import]] = {}
        for imp in self.imports:
            if imp.local is None or imp.type_only:
                continue
            if self.language == Language.PYTHON:
                bindings.setdefault(imp.local, []).append(imp)
            else:
                bindings[imp.local] = [imp]
        return bindings

    def _through(self, imp: Import, parts: list[str]) -> Symbol:
        rest = parts[1:]
        if self.language in (Language.JAVASCRIPT, Language.TYPESCRIPT, Language.TSX):
            member = ".".join(rest if imp.name is None else [imp.name, *rest])
            return Symbol(imp.module, member or None)
        if self.language == Language.PYTHON:
            if imp.kind == ImportKind.MODULE:
                # ``import a.b`` binds ``a``; the reference is already a full path.
                reference = ".".join(parts)
                plain = {i.module for i in self.imports if i.kind == ImportKind.MODULE and i.local == parts[0]}
                return Symbol(reference, None) if reference in plain else _split(reference)
            if imp.kind == ImportKind.ALIAS and not rest:
                return Symbol(_base(imp), None)
        return _split(_join(_base(imp), *rest))


def package_root(specifier: str) -> str:
    """The npm package a TS/JS module specifier names: ``@scope/pkg`` for ``@scope/pkg/sub``, else the part
    before the first ``/`` (``langchain`` for ``langchain/agents``). A relative specifier gives ``.``/``..``."""
    parts = specifier.split("/")
    return "/".join(parts[:2]) if specifier.startswith("@") else parts[0]


def _base(imp: Import) -> str:
    """The dotted path an import binds: the module (with its leading dots), plus the member for a member import."""
    module = "." * imp.level + imp.module
    return module if imp.name is None else _join(module, imp.name)


def _join(*parts: str) -> str:
    """Join path parts with dots, without doubling the dot after a relative prefix (``.`` + ``x`` is ``.x``)."""
    path = ""
    for part in parts:
        path += part if not path or path.endswith(".") else f".{part}"
    return path


def _split(path: str) -> Symbol:
    """Split a dotted path at its last dot, keeping a relative prefix's dots: ``.x`` is ``Symbol(".", "x")``."""
    cut = path.rfind(".")
    if cut < 0:
        return Symbol(path, None)
    module = path[:cut]
    if not module.strip("."):
        module = path[: cut + 1]
    return Symbol(module, path[cut + 1 :])


def extract_imports(language: Language, tree: tree_sitter.Tree, source: bytes) -> ImportTable:
    """Every import in a parsed file, in source order. Raises ValueError for Ballerina, which has no grammar."""
    if language == Language.BALLERINA:
        raise ValueError("Ballerina has no grammar, so its imports cannot be read")
    reader = _Reader(source)
    if language == Language.PYTHON:
        found = reader.python(tree.root_node)
    elif language == Language.C_SHARP:
        found = reader.c_sharp(tree.root_node)
    else:
        found = reader.script(tree.root_node)
    return ImportTable(language, tuple(found))


def _nodes(root: tree_sitter.Node, types: frozenset[str]) -> Iterator[tree_sitter.Node]:
    """The nodes of the given types under ``root``, in source order."""
    stack = [root]
    while stack:
        node = stack.pop()
        if node.type in types:
            yield node
        stack.extend(reversed(node.children))


def _line(node: tree_sitter.Node) -> int:
    return node.start_point[0] + 1


def _has_keyword(node: tree_sitter.Node, keyword: str) -> bool:
    return any(not child.is_named and child.type == keyword for child in node.children)


_CS_TARGETS = frozenset({"identifier", "qualified_name", "generic_name", "alias_qualified_name"})
"""The node types a C# ``using`` can name: a namespace or type, never a tuple or array type."""


@dataclass(frozen=True)
class _Reader:
    source: bytes

    def text(self, node: tree_sitter.Node) -> str:
        return self.source[node.start_byte : node.end_byte].decode("utf-8", errors="replace")

    def dotted(self, node: tree_sitter.Node) -> str:
        """A Python ``dotted_name`` as ``a.b``, whatever spacing the source has around the dots."""
        return ".".join(self.text(child) for child in node.named_children if child.type == "identifier")

    # Python

    def python(self, root: tree_sitter.Node) -> Iterator[Import]:
        for statement in _nodes(root, frozenset({"import_statement", "import_from_statement"})):
            found = (
                self._python_import(statement) if statement.type == "import_statement" else self._python_from(statement)
            )
            if self._under_type_checking(statement):
                found = (dataclasses.replace(imp, type_only=True) for imp in found)
            yield from found

    def _under_type_checking(self, node: tree_sitter.Node) -> bool:
        """Whether ``node`` is in the body of ``if TYPE_CHECKING:`` (or ``typing.TYPE_CHECKING``), not elif or else."""
        child, parent = node, node.parent
        while parent is not None:
            if parent.type == "if_statement" and parent.child_by_field_name("consequence") == child:
                condition = parent.child_by_field_name("condition")
                if condition is not None and condition.type == "attribute":
                    condition = condition.child_by_field_name("attribute")
                if condition is not None and condition.type == "identifier" and self.text(condition) == "TYPE_CHECKING":
                    return True
            child, parent = parent, parent.parent
        return False

    def _python_import(self, statement: tree_sitter.Node) -> Iterator[Import]:
        for item in statement.children_by_field_name("name"):
            if item.type == "dotted_name":
                module = self.dotted(item)
                yield Import(module, None, module.split(".")[0], ImportKind.MODULE, _line(item))
            elif item.type == "aliased_import":
                name, alias = item.child_by_field_name("name"), item.child_by_field_name("alias")
                if name is not None and alias is not None:
                    yield Import(self.dotted(name), None, self.text(alias), ImportKind.ALIAS, _line(item))

    def _python_from(self, statement: tree_sitter.Node) -> Iterator[Import]:
        source = statement.child_by_field_name("module_name")
        if source is None:
            return
        level, module = 0, self.dotted(source)
        if source.type == "relative_import":
            for child in source.named_children:
                if child.type == "import_prefix":
                    level = self.text(child).count(".")
                elif child.type == "dotted_name":
                    module = self.dotted(child)
        for child in statement.named_children:
            if child.type == "wildcard_import":
                yield Import(module, None, None, ImportKind.WILDCARD, _line(child), level)
        for item in statement.children_by_field_name("name"):
            name_node: tree_sitter.Node | None = item
            alias: tree_sitter.Node | None = None
            if item.type == "aliased_import":
                name_node, alias = item.child_by_field_name("name"), item.child_by_field_name("alias")
            if name_node is None:
                continue
            name = self.dotted(name_node)
            local = name if alias is None else self.text(alias)
            yield Import(module, name, local, ImportKind.MEMBER, _line(item), level)

    # TypeScript and JavaScript

    def script(self, root: tree_sitter.Node) -> Iterator[Import]:
        for node in _nodes(root, frozenset({"import_statement", "variable_declarator"})):
            if node.type == "import_statement":
                yield from self._es_import(node)
            else:
                yield from self._require(node)

    def _string(self, node: tree_sitter.Node | None) -> str | None:
        """The value of a string literal (or a template literal with no substitutions), or None."""
        if node is None or node.type not in ("string", "template_string"):
            return None
        if any(child.type == "template_substitution" for child in node.named_children):
            return None
        return self.text(node)[1:-1]

    def _name(self, node: tree_sitter.Node) -> str | None:
        """An imported or destructured member name, written as an identifier or a string (``"an-agent"``)."""
        return self._string(node) if node.type == "string" else self.text(node)

    def _es_import(self, statement: tree_sitter.Node) -> Iterator[Import]:
        type_only = _has_keyword(statement, "type")
        module = self._string(statement.child_by_field_name("source"))
        for child in statement.named_children:
            if child.type == "import_require_clause":
                module = self._string(child.child_by_field_name("source"))
                local = next((c for c in child.named_children if c.type == "identifier"), None)
                if module is not None and local is not None:
                    yield Import(module, None, self.text(local), ImportKind.MODULE, _line(child), type_only=type_only)
                return
        if module is None:
            return
        clause = next((c for c in statement.named_children if c.type == "import_clause"), None)
        if clause is None:
            yield Import(module, None, None, ImportKind.SIDE_EFFECT, _line(statement))
            return
        for part in clause.named_children:
            if part.type == "identifier":
                yield Import(module, "default", self.text(part), ImportKind.DEFAULT, _line(part), type_only=type_only)
            elif part.type == "namespace_import":
                local = next((c for c in part.named_children if c.type == "identifier"), None)
                if local is not None:
                    yield Import(module, None, self.text(local), ImportKind.MODULE, _line(part), type_only=type_only)
            elif part.type == "named_imports":
                for spec in part.named_children:
                    if spec.type != "import_specifier":
                        continue
                    name_node = spec.child_by_field_name("name")
                    if name_node is None:
                        continue
                    alias = spec.child_by_field_name("alias")
                    name = self._name(name_node)
                    if name is None:
                        continue
                    bound = name if alias is None else self.text(alias)
                    spec_type_only = type_only or _has_keyword(spec, "type")
                    yield Import(module, name, bound, ImportKind.MEMBER, _line(spec), type_only=spec_type_only)

    def _require(self, declarator: tree_sitter.Node) -> Iterator[Import]:
        target, value = declarator.child_by_field_name("name"), declarator.child_by_field_name("value")
        awaited = value is not None and value.type == "await_expression"
        if value is not None and awaited:
            value = next((c for c in value.named_children if c.type != "comment"), None)
        if target is None or value is None or value.type != "call_expression":
            return
        function, arguments = value.child_by_field_name("function"), value.child_by_field_name("arguments")
        if function is None or arguments is None:
            return
        # ``import("m")`` gives a promise, so only an awaited one binds the module.
        dynamic = awaited and function.type == "import"
        if not dynamic and not (function.type == "identifier" and self.text(function) == "require"):
            return
        args = [a for a in arguments.named_children if a.type != "comment"]
        module = self._string(args[0]) if len(args) == 1 else None
        if module is None:
            return
        if target.type == "identifier":
            yield Import(module, None, self.text(target), ImportKind.MODULE, _line(target))
        elif target.type == "object_pattern":
            for prop in target.named_children:
                if prop.type == "shorthand_property_identifier_pattern":
                    name = self.text(prop)
                    yield Import(module, name, name, ImportKind.MEMBER, _line(prop))
                elif prop.type == "pair_pattern":
                    key, local = prop.child_by_field_name("key"), prop.child_by_field_name("value")
                    if (
                        key is None
                        or key.type == "computed_property_name"
                        or local is None
                        or local.type != "identifier"
                    ):
                        continue
                    member = self._name(key)
                    if member is not None:
                        yield Import(module, member, self.text(local), ImportKind.MEMBER, _line(prop))

    # C#

    def c_sharp(self, root: tree_sitter.Node) -> Iterator[Import]:
        for directive in _nodes(root, frozenset({"using_directive"})):
            alias = directive.child_by_field_name("name")
            targets = [c for c in directive.named_children if c.type != "comment" and c != alias]
            if not targets or targets[-1].type not in _CS_TARGETS:
                continue  # e.g. a tuple alias, ``using P = (int, int);``
            path = "".join(self.text(targets[-1]).split()).removeprefix("global::").split("<", 1)[0]
            is_global = _has_keyword(directive, "global")
            if alias is not None:
                yield Import(path, None, self.text(alias), ImportKind.ALIAS, _line(directive), global_=is_global)
            elif _has_keyword(directive, "static"):
                yield Import(path, None, None, ImportKind.STATIC, _line(directive), global_=is_global)
            else:
                yield Import(path, None, None, ImportKind.NAMESPACE, _line(directive), global_=is_global)
