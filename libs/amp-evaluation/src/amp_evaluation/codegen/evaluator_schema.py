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

"""
Schema export for evaluator editor UI.

Introspects trace model dataclasses and produces structured data consumed
by the console's Monaco editor (completions, hover docs) and the data model
reference drawer (tree views).

Type lists, union members, and expandable classes are **derived dynamically**
from the SDK source (type aliases, ``TYPE_TO_LEVEL``, dataclass field hints)
rather than hardcoded, so adding a new span/step/message type in the SDK is
automatically reflected in the generated schema.
"""

from __future__ import annotations

import dataclasses
import enum as _enum
import inspect
import re
import sys
import warnings
from typing import Any, Dict, List, Optional, Set, get_type_hints, Union, get_args, get_origin

from amp_evaluation.models import EvalResult
from amp_evaluation.evaluators.params import _ParamDescriptor
from amp_evaluation.evaluators.base import TYPE_TO_LEVEL
from amp_evaluation.trace import models as _trace_models
from amp_evaluation.trace.models import Message, Span, AgentStep


# ============================================================================
# DYNAMIC TYPE DISCOVERY
# ============================================================================


def _get_union_members(type_alias: Any) -> List[type]:
    """Extract concrete types from a Union type alias (e.g. Message, Span, AgentStep)."""
    args = get_args(type_alias)
    return [t for t in args if isinstance(t, type)]


# Union member lists — extracted from the SDK type aliases
_STEP_TYPES = _get_union_members(AgentStep)
_MESSAGE_TYPES = _get_union_members(Message)
_SPAN_TYPES = _get_union_members(Span)

# Top-level evaluator input types derived from TYPE_TO_LEVEL mapping
_TOP_LEVEL_TYPES: Dict[str, type] = {}
for _name, _level in TYPE_TO_LEVEL.items():
    _cls = getattr(_trace_models, _name, None)
    if _cls is not None:
        _TOP_LEVEL_TYPES[_level.value] = _cls


def _collect_field_dataclasses(cls: type, visited: Optional[Set[type]] = None) -> Set[type]:
    """Recursively collect all dataclass types referenced in fields of *cls*."""
    if visited is None:
        visited = set()
    if cls in visited or not dataclasses.is_dataclass(cls):
        return visited
    visited.add(cls)

    hints = get_type_hints(cls)
    for f in dataclasses.fields(cls):
        th = hints.get(f.name, f.type)
        for inner in _flatten_type(th):
            if isinstance(inner, type) and dataclasses.is_dataclass(inner) and inner not in visited:
                _collect_field_dataclasses(inner, visited)
    return visited


def _flatten_type(th: Any) -> List[Any]:
    """Flatten a (possibly nested) generic type into its leaf types."""
    origin = get_origin(th)
    args = get_args(th)
    if origin is Union or origin is list or origin is List:
        result: List[Any] = []
        for a in args or []:
            result.extend(_flatten_type(a))
        return result
    return [th]


def _discover_expandable_classes() -> Set[type]:
    """Discover all dataclass types that should be expandable in the tree view.

    Walks fields of top-level types and union member types to find nested
    dataclass types (metrics, messages, tool calls, etc.).
    """
    root_types = list(_TOP_LEVEL_TYPES.values()) + _STEP_TYPES + _MESSAGE_TYPES + _SPAN_TYPES
    all_classes: Set[type] = set()
    for cls in root_types:
        _collect_field_dataclasses(cls, all_classes)
    # Remove the root types themselves — they're handled separately
    all_classes -= set(root_types)
    return all_classes


_EXPANDABLE_CLASSES = _discover_expandable_classes()

_TYPE_TO_CLASS: Dict[str, type] = {cls.__name__: cls for cls in _EXPANDABLE_CLASSES}


def _discover_union_fields(cls: type) -> Dict[str, List[type]]:
    """Find fields on *cls* whose type is List[SomeUnion] with multiple concrete members.

    Returns a dict mapping field name to the list of concrete union member types.
    """
    result: Dict[str, List[type]] = {}
    hints = get_type_hints(cls)
    for f in dataclasses.fields(cls):
        th = hints.get(f.name, f.type)
        origin = get_origin(th)
        if origin is not list and origin is not List:
            continue
        args = get_args(th)
        if not args:
            continue
        inner = args[0]
        # Check if the inner type is a Union with multiple concrete types
        inner_origin = get_origin(inner)
        inner_args = get_args(inner)
        if inner_origin is Union and inner_args:
            members = [t for t in inner_args if isinstance(t, type)]
            if len(members) > 1:
                result[f.name] = members
                continue
        # Also check known type aliases (Message, Span, AgentStep) which may
        # resolve differently depending on Python version
        if inner is Message:
            result[f.name] = _MESSAGE_TYPES
        elif inner is Span:
            result[f.name] = _SPAN_TYPES
        elif inner is AgentStep:
            result[f.name] = _STEP_TYPES
    return result


# All classes whose public API is introspected by this codegen (for drift detection)
_INTROSPECTED_CLASSES: List[type] = sorted(
    set(list(_TOP_LEVEL_TYPES.values()) + _STEP_TYPES + _MESSAGE_TYPES + _SPAN_TYPES + list(_EXPANDABLE_CLASSES)),
    key=lambda c: c.__name__,
)


# ============================================================================
# TYPE FORMATTING
# ============================================================================


def _format_type(type_hint: Any) -> str:
    """Format a Python type hint as a readable string."""
    origin = getattr(type_hint, "__origin__", None)
    args = getattr(type_hint, "__args__", None)

    if origin is list or origin is List:
        if args:
            return f"List[{_format_type(args[0])}]"
        return "List"

    if origin is dict or origin is Dict:
        if args and len(args) == 2:
            return f"Dict[{_format_type(args[0])}, {_format_type(args[1])}]"
        return "Dict"

    if origin is Union:
        if args and len(args) == 2 and type(None) in args:
            non_none = [a for a in args if a is not type(None)]
            if non_none:
                return f"{_format_type(non_none[0])} | None"
        filtered = [_format_type(a) for a in (args or []) if a is not type(None)]
        return " | ".join(filtered)

    if origin is Optional:
        if args:
            return f"{_format_type(args[0])} | None"

    if hasattr(type_hint, "__name__"):
        return type_hint.__name__

    return str(type_hint).replace("typing.", "")


# ============================================================================
# INTROSPECTION HELPERS
# ============================================================================


def _is_internal(f: dataclasses.Field) -> bool:
    """Check if a field is marked as internal via metadata."""
    return bool(f.metadata.get("internal", False)) if f.metadata else False


def _get_field_description(f: dataclasses.Field) -> str:
    """Get the description from a field's metadata."""
    return f.metadata.get("description", "") if f.metadata else ""


def _class_summary(cls: type) -> str:
    """First line of a class docstring, or class name as fallback."""
    return (cls.__doc__ or cls.__name__).strip().split("\n")[0]


def _method_summary(method: Any) -> str:
    """First line of a method/function docstring."""
    return (method.__doc__ or "").strip().split("\n")[0]


def _parse_docstring_args(func: Any) -> Dict[str, str]:
    """Parse 'Args:' section from a function/method docstring.

    Returns a dict mapping parameter names to their description strings.
    Supports Google-style docstrings::

        Args:
            score: Evaluation score between 0.0 and 1.0
            explanation: Human-readable explanation of the result
    """
    doc = inspect.getdoc(func)
    if not doc:
        return {}

    result: Dict[str, str] = {}
    in_args = False
    current_param: Optional[str] = None
    current_desc: List[str] = []

    for line in doc.split("\n"):
        stripped = line.strip()

        # Detect the Args: section header
        if stripped == "Args:":
            in_args = True
            continue

        if not in_args:
            continue

        # Blank line ends the Args section
        if not stripped:
            if current_param:
                result[current_param] = " ".join(current_desc).strip()
                current_param = None
                current_desc = []
            in_args = False
            continue

        # End of Args section: another top-level header (Returns:, Raises:, etc.)
        # or any non-indented line
        if not line.startswith("    "):
            if current_param:
                result[current_param] = " ".join(current_desc).strip()
            in_args = False
            continue

        # Match "param_name: description" or "param_name (type): description"
        m = re.match(r"^\s{4,}(\w+)(?:\s*\([^)]*\))?\s*:\s*(.*)$", line)
        if m:
            # Save previous param
            if current_param:
                result[current_param] = " ".join(current_desc).strip()
            current_param = m.group(1)
            current_desc = [m.group(2)] if m.group(2) else []
        elif current_param and stripped:
            # Continuation line for the current parameter
            current_desc.append(stripped)

    # Don't forget the last param
    if current_param:
        result[current_param] = " ".join(current_desc).strip()

    return result


def _get_param_supported_types() -> List[str]:
    """Derive the supported Param types from _ParamDescriptor.to_schema's type_map.

    Instantiates dummy descriptors for each candidate type and checks whether
    to_schema() produces a known type string. Also checks Enum support.
    Returns a list of type name strings (e.g. ["float", "int", "str", ...]).
    """
    supported: List[str] = []
    candidate_types = [float, int, str, bool, list, dict]

    for t in candidate_types:
        desc = _ParamDescriptor(default=t(), description="")
        desc._attr_name = "_probe"
        desc.type = t
        try:
            schema = desc.to_schema()
            if schema.get("type") not in (None, "string"):
                # to_schema mapped it to something meaningful
                supported.append(t.__name__)
            elif t is str:
                # str maps to "string" which is the default fallback too,
                # but str is definitely supported
                supported.append(t.__name__)
        except Exception:
            pass

    # Check Enum support: _ParamDescriptor has explicit issubclass(base_type, Enum) handling
    class _ProbeEnum(_enum.Enum):
        A = "a"

    desc = _ParamDescriptor(default=_ProbeEnum.A, description="")
    desc._attr_name = "_probe"
    desc.type = _ProbeEnum
    try:
        schema = desc.to_schema()
        if "enum_values" in schema:
            supported.append("Enum")
    except Exception:
        pass

    return supported


def _resolve_list_inner(type_hint: Any) -> Optional[type]:
    """If type_hint is List[X], return X."""
    origin = getattr(type_hint, "__origin__", None)
    args = getattr(type_hint, "__args__", None)
    if (origin is list or origin is List) and args:
        inner = args[0]
        if isinstance(inner, type):
            return inner
    return None


def _discover_methods(cls: type) -> List[Dict[str, Any]]:
    """Discover public methods with return type annotations and docstrings."""
    nodes = []
    for name, method in inspect.getmembers(cls, predicate=inspect.isfunction):
        if name.startswith("_"):
            continue
        hints = get_type_hints(method)
        ret = hints.get("return")
        if ret is None:
            continue
        doc = _method_summary(method)
        node: Dict[str, Any] = {
            "name": f"{name}()",
            "type": _format_type(ret),
            "description": doc,
            "isMethod": True,
        }
        # Expand children if return type is List[SomeDataclass]
        inner = _resolve_list_inner(ret)
        if inner and dataclasses.is_dataclass(inner):
            children = _build_field_tree(inner)
            if children:
                node["children"] = children
        nodes.append(node)
    return nodes


def _discover_properties(cls: type) -> List[Dict[str, Any]]:
    """Discover @property members with return type annotations."""
    nodes = []
    for name in dir(cls):
        if name.startswith("_"):
            continue
        attr = getattr(cls, name, None)
        if not isinstance(attr, property):
            continue
        if attr.fget is None:
            continue
        hints = get_type_hints(attr.fget)
        ret = hints.get("return")
        if ret is None:
            continue
        doc = _method_summary(attr.fget)
        nodes.append(
            {
                "name": name,
                "type": _format_type(ret),
                "description": doc,
            }
        )
    return nodes


def _discover_classmethods(cls: type) -> List[Dict[str, Any]]:
    """Discover public @classmethod members with return type annotations."""
    nodes = []
    for name in dir(cls):
        if name.startswith("_"):
            continue
        attr = getattr(cls, name, None)
        if not inspect.ismethod(attr):
            continue
        hints = get_type_hints(attr)
        ret = hints.get("return")
        if ret is None:
            continue
        # Build parameter signature
        sig = inspect.signature(attr)
        params = [p for p in sig.parameters if p != "cls"]
        param_str = ", ".join(params) if params else ""
        doc = _method_summary(attr)
        nodes.append(
            {
                "name": f"{name}({param_str})",
                "type": _format_type(ret),
                "description": doc,
                "isMethod": True,
            }
        )
    return nodes


# ============================================================================
# TREE BUILDER — dataclass introspection for reference drawer
# ============================================================================


def _resolve_inner_type(type_hint: Any) -> Optional[type]:
    """If type_hint is List[X] or Optional[X], return X if it's expandable."""
    origin = getattr(type_hint, "__origin__", None)
    args = getattr(type_hint, "__args__", None)

    if origin is list or origin is List:
        if args:
            return _resolve_concrete(args[0])
    if origin is Union:
        if args:
            for a in args:
                if a is not type(None):
                    resolved = _resolve_concrete(a)
                    if resolved:
                        return resolved
    return _resolve_concrete(type_hint)


def _resolve_concrete(t: Any) -> Optional[type]:
    """Return t if it's an expandable dataclass, else None."""
    if isinstance(t, type) and dataclasses.is_dataclass(t) and t in _EXPANDABLE_CLASSES:
        return t
    name = getattr(t, "__name__", None) or str(t)
    cls = _TYPE_TO_CLASS.get(name)
    if cls and dataclasses.is_dataclass(cls):
        return cls
    return None


def _build_field_tree(cls: type, visited: Optional[set] = None) -> List[Dict[str, Any]]:
    """Build a tree of FieldNode dicts from a dataclass, skipping internal fields."""
    if visited is None:
        visited = set()
    if cls in visited:
        return []
    visited.add(cls)

    nodes = []
    hints = get_type_hints(cls)

    for f in dataclasses.fields(cls):
        if _is_internal(f):
            continue

        type_hint = hints.get(f.name, f.type)
        type_str = _format_type(type_hint)
        desc = _get_field_description(f)

        node: Dict[str, Any] = {
            "name": f.name,
            "type": type_str,
            "description": desc,
        }

        # Check if we should expand children
        inner = _resolve_inner_type(type_hint)
        if inner and inner != cls:
            children = _build_field_tree(inner, visited.copy())
            if children:
                node["children"] = children

        nodes.append(node)

    return nodes


def _build_union_children(
    types: List[type],
    include_properties: bool = False,
) -> List[Dict[str, Any]]:
    """Build children for a union type field, using class docstrings for descriptions."""
    children = []
    for cls in types:
        cls_children = _build_field_tree(cls)
        if include_properties:
            cls_children.extend(_discover_properties(cls))
        node: Dict[str, Any] = {
            "name": cls.__name__,
            "type": "",
            "description": _class_summary(cls),
            "children": cls_children,
        }
        children.append(node)
    return children


# Known union type aliases — used for matching field types
_UNION_ALIASES: Dict[Any, List[type]] = {
    Span: _SPAN_TYPES,
    AgentStep: _STEP_TYPES,
    Message: _MESSAGE_TYPES,
}

# Display names for union type aliases
_UNION_DISPLAY_NAMES: Dict[Any, str] = {
    Span: "Span",
    AgentStep: "AgentStep",
    Message: "Message",
}


def _get_union_alias_for_field(cls: type, field_name: str) -> Optional[Any]:
    """Check if a field's inner List type matches a known union alias."""
    hints = get_type_hints(cls)
    f_hint = hints.get(field_name)
    if f_hint is None:
        return None
    origin = get_origin(f_hint)
    if origin is not list and origin is not List:
        return None
    args = get_args(f_hint)
    if not args:
        return None
    inner = args[0]
    # Check against known union aliases
    for alias in _UNION_ALIASES:
        if inner is alias:
            return alias
    # Also check if the inner type is a Union whose members match
    inner_args = get_args(inner)
    if inner_args:
        inner_members = set(t for t in inner_args if isinstance(t, type))
        for alias, members in _UNION_ALIASES.items():
            if inner_members == set(members):
                return alias
    return None


# ============================================================================
# MODEL TREE SCHEMA
# ============================================================================


def _build_eval_result_tree() -> List[Dict[str, Any]]:
    """Build tree nodes for EvalResult by introspecting __init__ + classmethods."""
    sig = inspect.signature(EvalResult.__init__)
    hints = get_type_hints(EvalResult.__init__)
    arg_docs = _parse_docstring_args(EvalResult.__init__)

    nodes = []
    for name, param in sig.parameters.items():
        if name == "self":
            continue
        type_str = _format_type(hints.get(name, Any))
        desc = arg_docs.get(name, "")
        if not desc:
            warnings.warn(
                f"EvalResult.__init__ parameter '{name}' has no docstring description. "
                f"Add it to the Args: section of EvalResult.__init__'s docstring.",
                stacklevel=2,
            )
        nodes.append(
            {
                "name": name,
                "type": type_str,
                "description": desc,
            }
        )

    # Add classmethods (e.g. skip)
    nodes.extend(_discover_classmethods(EvalResult))
    return nodes


def _build_param_tree() -> List[Dict[str, Any]]:
    """Build tree nodes for Param by introspecting _ParamDescriptor.__init__."""
    sig = inspect.signature(_ParamDescriptor.__init__)
    hints = get_type_hints(_ParamDescriptor.__init__)
    arg_docs = _parse_docstring_args(_ParamDescriptor.__init__)

    nodes = []
    for name, param in sig.parameters.items():
        if name == "self":
            continue
        type_str = _format_type(hints.get(name, Any))
        desc = arg_docs.get(name, "")
        if not desc:
            warnings.warn(
                f"_ParamDescriptor.__init__ parameter '{name}' has no docstring description. "
                f"Add it to the Args: section of _ParamDescriptor.__init__'s docstring.",
                stacklevel=2,
            )
        nodes.append(
            {
                "name": name,
                "type": type_str,
                "description": desc,
            }
        )
    return nodes


def _build_type_tree_with_unions(cls: type, include_properties: bool = False) -> List[Dict[str, Any]]:
    """Build field tree for a type, auto-expanding any List[Union] fields."""
    nodes = _build_field_tree(cls)

    # Find and replace union fields with expanded versions
    for i, node in enumerate(nodes):
        alias = _get_union_alias_for_field(cls, node["name"])
        if alias is not None:
            members = _UNION_ALIASES[alias]
            display_name = _UNION_DISPLAY_NAMES[alias]
            desc = (
                _get_field_description(next(f for f in dataclasses.fields(cls) if f.name == node["name"]))
                + ", one of the following types:"
            )
            nodes[i] = {
                "name": node["name"],
                "type": f"List[{display_name}]",
                "description": desc,
                "children": _build_union_children(members, include_properties=include_properties),
            }

    return nodes


def get_model_tree_schema() -> Dict[str, Any]:
    """Return the model tree schema for all evaluator-facing types."""
    result: Dict[str, Any] = {}

    for level, cls in _TOP_LEVEL_TYPES.items():
        nodes = _build_type_tree_with_unions(cls, include_properties=True)
        # Add methods and properties
        nodes.extend(_discover_methods(cls))
        # Add properties if not already present (union expansion may include them)
        existing_names = {n["name"] for n in nodes}
        for prop in _discover_properties(cls):
            if prop["name"] not in existing_names:
                nodes.append(prop)

        result[level] = {
            "className": cls.__name__,
            "module": "amp_evaluation.trace.models",
            "description": _class_summary(cls),
            "nodes": nodes,
        }

    # SDK utility types
    result["eval_result"] = {
        "className": "EvalResult",
        "module": "amp_evaluation",
        "description": _class_summary(EvalResult),
        "nodes": _build_eval_result_tree(),
    }
    result["param"] = {
        "className": "Param",
        "module": "amp_evaluation",
        "description": _class_summary(_ParamDescriptor),
        "nodes": _build_param_tree(),
    }

    return result


# ============================================================================
# COMPLETION SCHEMA
# ============================================================================


def _field_completions(
    cls: type,
    var_name: str,
    sort_base: str = "1",
    priority_fields: Optional[set] = None,
) -> List[Dict[str, Any]]:
    """Generate completion items for a dataclass's non-internal fields."""
    items = []
    hints = get_type_hints(cls)

    for f in dataclasses.fields(cls):
        if _is_internal(f):
            continue
        type_hint = hints.get(f.name, f.type)
        type_str = _format_type(type_hint)
        desc = _get_field_description(f)

        sort = sort_base
        if priority_fields and f.name in priority_fields:
            sort = str(int(sort_base) - 1) if sort_base.isdigit() else sort_base

        items.append(
            {
                "label": f"{var_name}.{f.name}",
                "kind": "Property",
                "insertText": f"{var_name}.{f.name}",
                "detail": type_str,
                "documentation": desc,
                "sortText": sort,
            }
        )

    return items


def _method_completions(cls: type, var_name: str, sort_base: str = "3") -> List[Dict[str, Any]]:
    """Generate completion items from discovered methods."""
    items = []
    for node in _discover_methods(cls):
        label = f"{var_name}.{node['name']}"
        items.append(
            {
                "label": label,
                "kind": "Method",
                "insertText": label,
                "detail": node["type"],
                "documentation": node["description"],
                "sortText": sort_base,
            }
        )
    return items


def _property_completions(cls: type, var_name: str, sort_base: str = "2") -> List[Dict[str, Any]]:
    """Generate completion items from discovered properties."""
    items = []
    for node in _discover_properties(cls):
        label = f"{var_name}.{node['name']}"
        items.append(
            {
                "label": label,
                "kind": "Property",
                "insertText": label,
                "detail": node["type"],
                "documentation": node["description"],
                "sortText": sort_base,
            }
        )
    return items


def _class_completion(cls: type, module: str = "amp_evaluation.trace.models") -> Dict[str, Any]:
    """Generate a Class completion item from a dataclass."""
    return {
        "label": cls.__name__,
        "kind": "Class",
        "insertText": cls.__name__,
        "detail": f"{module}.{cls.__name__}",
        "documentation": _class_summary(cls),
        "sortText": "3",
    }


def _generate_isinstance_snippet(
    var_name: str,
    iter_field: str,
    item_var: str,
    union_members: List[type],
) -> Dict[str, Any]:
    """Generate a workflow snippet that iterates a union field with isinstance checks.

    Produces code like::

        for step in agent_trace.steps:
            if isinstance(step, LLMReasoningStep):
                field1 = step.field1
            elif isinstance(step, ToolExecutionStep):
                ...
    """
    lines = [f"for {item_var} in {var_name}.{iter_field}:"]
    for idx, cls in enumerate(union_members):
        keyword = "if" if idx == 0 else "elif"
        lines.append(f"    {keyword} isinstance({item_var}, {cls.__name__}):")
        # Show key (non-internal) fields for this type
        hints = get_type_hints(cls)
        shown = 0
        for f in dataclasses.fields(cls):
            if _is_internal(f) or shown >= 3:
                break
            type_str = _format_type(hints.get(f.name, f.type))
            lines.append(f"        {f.name} = {item_var}.{f.name}  # {type_str}")
            shown += 1
        if shown == 0:
            lines.append("        pass")

    # Build display name from the union members
    type_names = " | ".join(cls.__name__ for cls in union_members)
    return {
        "label": f"Iterate {iter_field} by type",
        "kind": "Snippet",
        "insertText": "\n".join(lines),
        "detail": f"Workflow: walk {iter_field} ({type_names})",
        "documentation": f"Iterate {iter_field} and handle each type with isinstance checks.",
        "sortText": "5",
    }


# Variable name conventions for each evaluator level
_LEVEL_VAR_NAMES: Dict[str, str] = {
    "trace": "trace",
    "agent": "agent_trace",
    "llm": "llm_span",
}

# Item variable names for union field iteration
_FIELD_ITEM_VARS: Dict[str, str] = {
    "spans": "span",
    "steps": "step",
    "messages": "msg",
}


def get_completion_schema() -> Dict[str, Any]:
    """Return Monaco completion items for each evaluator level."""

    # --- Common completions (EvalResult, Param) — curated snippets ---
    common = [
        {
            "label": "EvalResult",
            "kind": "Snippet",
            "insertText": 'EvalResult(score=${1:1.0}, passed=${2:True}, explanation=${3:""})',
            "snippet": True,
            "detail": "amp_evaluation.EvalResult",
            "documentation": _class_summary(EvalResult),
        },
        {
            "label": "EvalResult.skip",
            "kind": "Snippet",
            "insertText": 'EvalResult.skip("${1:reason}")',
            "snippet": True,
            "detail": "Skip this evaluation",
            "documentation": _method_summary(EvalResult.skip),
        },
        {
            "label": "Param",
            "kind": "Snippet",
            "insertText": 'Param(default=${1:None}, description="${2:}")',
            "snippet": True,
            "detail": "amp_evaluation.Param",
            "documentation": _class_summary(_ParamDescriptor),
        },
    ]

    completions: Dict[str, List[Dict[str, Any]]] = {}

    for level, cls in _TOP_LEVEL_TYPES.items():
        var_name = _LEVEL_VAR_NAMES.get(level, level)
        items: List[Dict[str, Any]] = [_class_completion(cls)]
        items.extend(_field_completions(cls, var_name, sort_base="1"))
        items.extend(_method_completions(cls, var_name, sort_base="3"))
        items.extend(_property_completions(cls, var_name, sort_base="2"))

        # Add completions for union member types
        union_fields = _discover_union_fields(cls)
        for field_name, members in union_fields.items():
            for member_cls in members:
                items.append(_class_completion(member_cls))
            item_var = _FIELD_ITEM_VARS.get(field_name, field_name.rstrip("s"))
            for member_cls in members:
                items.extend(_field_completions(member_cls, item_var, sort_base="4"))
                items.extend(_property_completions(member_cls, item_var, sort_base="4"))
            # Auto-generate isinstance iteration snippet
            items.append(_generate_isinstance_snippet(var_name, field_name, item_var, members))

        # Add completions for nested dataclass field types (e.g. ToolCall)
        hints = get_type_hints(cls)
        for f in dataclasses.fields(cls):
            th = hints.get(f.name, f.type)
            inner = _resolve_list_inner(th)
            if inner and dataclasses.is_dataclass(inner) and inner in _EXPANDABLE_CLASSES:
                item_var = f.name.rstrip("s")
                items.append(_class_completion(inner))
                items.extend(_field_completions(inner, item_var, sort_base="4"))

        # Also add nested types from union members' fields (e.g. ToolCall in AssistantMessage)
        for _field_name, members in union_fields.items():
            for member_cls in members:
                member_hints = get_type_hints(member_cls)
                for mf in dataclasses.fields(member_cls):
                    mth = member_hints.get(mf.name, mf.type)
                    m_inner = _resolve_list_inner(mth)
                    if m_inner and dataclasses.is_dataclass(m_inner) and m_inner in _EXPANDABLE_CLASSES:
                        item_var = mf.name.rstrip("s")
                        # Avoid duplicate class completions
                        existing_labels = {it["label"] for it in items}
                        if m_inner.__name__ not in existing_labels:
                            items.append(_class_completion(m_inner))
                        items.extend(_field_completions(m_inner, item_var, sort_base="4"))

        # isinstance snippet for union types with multiple members
        for _field_name, members in union_fields.items():
            if len(members) > 1:
                item_var = _FIELD_ITEM_VARS.get(_field_name, _field_name.rstrip("s"))
                choices = ",".join(c.__name__ for c in members)
                insert_text = "isinstance(${1:" + item_var + "}, ${2|" + choices + "|})"
                items.append(
                    {
                        "label": f"isinstance — filter {_field_name} by type",
                        "kind": "Snippet",
                        "insertText": insert_text,
                        "snippet": True,
                        "detail": "Type check",
                        "documentation": f"Filter {_field_name} by type using isinstance.",
                        "sortText": "5",
                    }
                )

        completions[level] = items

    return {
        "common": common,
        **completions,
    }


# ============================================================================
# HOVER DOCS
# ============================================================================


def _class_hover_doc(cls: type) -> Dict[str, str]:
    """Build hover doc for a class from its fields, methods, and properties."""
    lines = [_class_summary(cls), ""]
    lines.append("**Properties:**")
    hints = get_type_hints(cls)
    for f in dataclasses.fields(cls):
        if _is_internal(f):
            continue
        type_str = _format_type(hints.get(f.name, f.type))
        desc = _get_field_description(f)
        desc_part = f" — {desc}" if desc else ""
        lines.append(f"- `{f.name}: {type_str}`{desc_part}")

    # Discover methods
    methods = _discover_methods(cls)
    if methods:
        lines.append("")
        lines.append("**Methods:**")
        for m in methods:
            lines.append(f"- `{m['name']} → {m['type']}`")

    # Discover properties
    props = _discover_properties(cls)
    if props:
        lines.append("")
        lines.append("**Computed properties:**")
        for p in props:
            desc_part = f" — {p['description']}" if p["description"] else ""
            lines.append(f"- `{p['name']}: {p['type']}`{desc_part}")

    return {"type": f"class {cls.__name__}", "doc": "\n".join(lines)}


def _eval_result_hover_doc() -> Dict[str, str]:
    """Build hover doc for EvalResult by introspecting __init__ + classmethods."""
    lines = [_class_summary(EvalResult), ""]
    sig = inspect.signature(EvalResult.__init__)
    hints = get_type_hints(EvalResult.__init__)
    arg_docs = _parse_docstring_args(EvalResult.__init__)
    lines.append("**Constructor parameters:**")
    for name, param in sig.parameters.items():
        if name == "self":
            continue
        type_str = _format_type(hints.get(name, Any))
        desc = arg_docs.get(name, "")
        desc_part = f" — {desc}" if desc else ""
        lines.append(f"- `{name}: {type_str}`{desc_part}")

    classmethods = _discover_classmethods(EvalResult)
    if classmethods:
        lines.append("")
        lines.append("**Class methods:**")
        for m in classmethods:
            desc_part = f" — {m['description']}" if m["description"] else ""
            lines.append(f"- `{m['name']} → {m['type']}`{desc_part}")

    return {"type": "class EvalResult", "doc": "\n".join(lines)}


def _param_hover_doc() -> Dict[str, str]:
    """Build hover doc for Param by introspecting _ParamDescriptor.__init__."""
    lines = [_class_summary(_ParamDescriptor), ""]
    sig = inspect.signature(_ParamDescriptor.__init__)
    hints = get_type_hints(_ParamDescriptor.__init__)
    arg_docs = _parse_docstring_args(_ParamDescriptor.__init__)
    lines.append("**Parameters:**")
    for name, param in sig.parameters.items():
        if name == "self":
            continue
        type_str = _format_type(hints.get(name, Any))
        desc = arg_docs.get(name, "")
        desc_part = f" — {desc}" if desc else ""
        lines.append(f"- `{name}: {type_str}`{desc_part}")
    lines.append("")
    supported = _get_param_supported_types()
    lines.append(f"**Supported types:** {', '.join(supported)}")

    return {"type": "class Param", "doc": "\n".join(lines)}


def get_hover_docs() -> Dict[str, Dict[str, str]]:
    """Return hover docs for all known identifiers."""
    docs: Dict[str, Dict[str, str]] = {}

    # Main evaluator types
    for cls in _TOP_LEVEL_TYPES.values():
        docs[cls.__name__] = _class_hover_doc(cls)

    # Supporting classes — all union members + expandable types
    supporting = set(_STEP_TYPES + _MESSAGE_TYPES + _SPAN_TYPES) | _EXPANDABLE_CLASSES
    # Remove top-level types (already handled above)
    supporting -= set(_TOP_LEVEL_TYPES.values())
    for cls in sorted(supporting, key=lambda c: c.__name__):
        if dataclasses.is_dataclass(cls):
            docs[cls.__name__] = _class_hover_doc(cls)

    # SDK utilities
    docs["EvalResult"] = _eval_result_hover_doc()
    docs["Param"] = _param_hover_doc()

    return docs


# ============================================================================
# CODE & LLM JUDGE TEMPLATES (curated — not derivable from introspection)
# ============================================================================


def get_code_templates() -> Dict[str, str]:
    """Return starter code templates per evaluator level."""
    supported_types = ", ".join(_get_param_supported_types())
    return {
        "trace": f'''from amp_evaluation import EvalResult, Param
from amp_evaluation.trace.models import Trace


def my_evaluator(
    trace: Trace,
    # Configurable parameters — these become UI fields when the evaluator is used.
    # Supported types: {supported_types}. Use Param() for constraints.
    threshold: float = Param(default=0.5, description="Pass threshold"),
) -> EvalResult:
    """Evaluate a complete trace (called once per trace)."""

    user_input = trace.input or ""
    agent_output = trace.output or ""

    # Example: check that the agent produced a non-empty response
    if not agent_output.strip():
        return EvalResult.skip("No output to evaluate")

    # Your evaluation logic
    score = 1.0
    passed = score >= threshold

    return EvalResult(
        score=score,
        passed=passed,
        explanation="Evaluation explanation here",
    )
''',
        "agent": f'''from amp_evaluation import EvalResult, Param
from amp_evaluation.trace.models import AgentTrace


def my_evaluator(
    agent_trace: AgentTrace,
    # Configurable parameters — these become UI fields when the evaluator is used.
    # Supported types: {supported_types}. Use Param() for constraints.
    threshold: float = Param(default=0.5, description="Pass threshold"),
) -> EvalResult:
    """Evaluate an agent span (called once per agent in the trace)."""

    agent_input = agent_trace.input or ""
    agent_output = agent_trace.output or ""
    tools_used = [s.tool_name for s in agent_trace.get_tool_steps()]

    # Example: check tool usage
    if not tools_used:
        return EvalResult(score=0.5, explanation="Agent did not use any tools")

    score = 1.0
    passed = score >= threshold

    return EvalResult(
        score=score,
        passed=passed,
        explanation=f"Agent used {{len(tools_used)}} tool(s): {{', '.join(tools_used)}}",
    )
''',
        "llm": f'''from amp_evaluation import EvalResult, Param
from amp_evaluation.trace.models import LLMSpan


def my_evaluator(
    llm_span: LLMSpan,
    # Configurable parameters — these become UI fields when the evaluator is used.
    # Supported types: {supported_types}. Use Param() for constraints.
    threshold: float = Param(default=0.5, description="Pass threshold"),
) -> EvalResult:
    """Evaluate an LLM call (called once per LLM invocation)."""

    output = llm_span.output or ""
    model = llm_span.model or ""

    # Example: check output is non-empty
    if not output.strip():
        return EvalResult.skip("Empty LLM output")

    score = 1.0
    passed = score >= threshold

    return EvalResult(
        score=score,
        passed=passed,
        explanation=f"LLM ({{model}}) produced a valid response",
    )
''',
    }


def get_llm_judge_templates() -> Dict[str, str]:
    """Return LLM judge prompt templates per evaluator level.

    Templates use Python f-string syntax — expressions inside {} are evaluated
    at runtime with the appropriate trace object in scope.

    NOTE: The scoring output format (JSON with explanation + score) is
    auto-appended by the framework. Users should NOT include scoring format
    instructions, but CAN include a scoring rubric to guide consistent scoring.
    """
    return {
        "trace": (
            "You are an expert evaluator. Your sole criterion is HELPFULNESS:"
            " does the response actually help the user with what they asked"
            " for?\n"
            "\n"
            "User Query:\n"
            "{trace.input}\n"
            "\n"
            "Agent Response:\n"
            "{trace.output}\n"
            "\n"
            "Execution Summary:\n"
            "- Total spans: {len(trace.spans)}\n"
            "- Agents involved: {', '.join(a.agent_name or 'unnamed'"
            " for a in trace.get_agents()) or 'none'}\n"
            "\n"
            "Evaluation Steps:\n"
            "1. Identify what the user needs: what problem are they trying"
            " to solve or what information are they seeking?\n"
            "2. Assess whether the response provides actionable, useful"
            " content that moves the user closer to their goal.\n"
            "3. Check for empty helpfulness: does the response acknowledge"
            " the question without actually helping?\n"
            "4. Assess whether the response would leave the user better off"
            " than before they asked.\n"
            "\n"
            "Scoring Rubric:\n"
            "  0.0  = Not helpful at all; ignores the user's need or"
            " answers a completely different question\n"
            "  0.25 = Minimally helpful; touches on the topic but does not"
            " provide enough useful content\n"
            "  0.5  = Somewhat helpful; provides some useful content but"
            " the user would still need significant additional help\n"
            "  0.75 = Helpful; addresses the user's need well with only"
            " minor gaps\n"
            "  1.0  = Highly helpful; directly and fully assists the user"
            " with clear, actionable, and complete content"
        ),
        "agent": (
            "You are an expert evaluator. Your sole criterion is TOOL USAGE:"
            " does the agent choose and use the right tools effectively to"
            " accomplish its goal?\n"
            "\n"
            "Agent: {agent_trace.agent_name or 'agent'}\n"
            "Model: {agent_trace.model}\n"
            "\n"
            "Goal:\n"
            "{agent_trace.input}\n"
            "\n"
            "Final Response:\n"
            "{agent_trace.output}\n"
            "\n"
            "Tools Available: {', '.join(t.name for t in agent_trace.available_tools)}\n"
            "Tools Used: {', '.join(s.tool_name for s in"
            " agent_trace.get_tool_steps())}\n"
            "Total Steps: {len(agent_trace.steps)}\n"
            "\n"
            "Evaluation Steps:\n"
            "1. Were the right tools selected for the task? Did the agent"
            " use the most appropriate tools from what was available?\n"
            "2. Were tool inputs well-formed and effective? Did the agent"
            " pass correct arguments to get useful results?\n"
            "3. Were there unnecessary tool calls, redundant lookups, or"
            " tools that should have been used but weren't?\n"
            "4. Did the agent use tool results effectively in its final"
            " response?\n"
            "\n"
            "Scoring Rubric:\n"
            "  0.0  = Tools used incorrectly or not at all despite being"
            " needed\n"
            "  0.25 = Some tools used but with major errors in selection"
            " or usage\n"
            "  0.5  = Tools used adequately but with unnecessary calls or"
            " missed opportunities\n"
            "  0.75 = Good tool usage with only minor inefficiencies\n"
            "  1.0  = Optimal tool usage; right tools, right inputs,"
            " no waste"
        ),
        "llm": (
            "You are an expert evaluator. Your sole criterion is COHERENCE:"
            " is this LLM response well-structured, logical, and easy to"
            " follow?\n"
            "\n"
            "Model: {llm_span.model}\n"
            "Vendor: {llm_span.vendor}\n"
            "Messages in conversation: {len(llm_span.input)}\n"
            "\n"
            "LLM Response:\n"
            "{llm_span.output}\n"
            "\n"
            "Evaluation Steps:\n"
            "1. Does the response have a clear structure with logical"
            " flow from one point to the next?\n"
            "2. Are ideas connected coherently, or are there abrupt"
            " jumps or contradictions?\n"
            "3. Is the level of detail appropriate and consistent"
            " throughout?\n"
            "4. Would a reader understand the response on first reading"
            " without confusion?\n"
            "\n"
            "Scoring Rubric:\n"
            "  0.0  = Incoherent; disorganized, contradictory, or"
            " impossible to follow\n"
            "  0.25 = Poorly structured; significant logical gaps or"
            " confusing organization\n"
            "  0.5  = Understandable but with structural issues or"
            " unclear passages\n"
            "  0.75 = Well-structured and clear with only minor areas"
            " that could be tighter\n"
            "  1.0  = Exceptionally coherent; perfectly organized,"
            " logical, and easy to follow"
        ),
    }


def get_llm_judge_variables() -> Dict[str, Any]:
    """Return per-level variable documentation for LLM judge prompt templates.

    Each level exposes a root variable (trace, agent_trace, llm_span) with
    its fields, methods, and properties — the same data model available to
    code evaluators.
    """
    result: Dict[str, Any] = {}
    for level, cls in _TOP_LEVEL_TYPES.items():
        var_name = _LEVEL_VAR_NAMES.get(level, level)
        hints = get_type_hints(cls)
        fields: List[Dict[str, Any]] = []
        for f in dataclasses.fields(cls):
            if _is_internal(f):
                continue
            type_str = _format_type(hints.get(f.name, f.type))
            fields.append(
                {
                    "name": f"{var_name}.{f.name}",
                    "type": type_str,
                    "description": _get_field_description(f),
                }
            )
        # Add methods
        for m in _discover_methods(cls):
            fields.append(
                {
                    "name": f"{var_name}.{m['name']}",
                    "type": m["type"],
                    "description": m["description"],
                    "isMethod": True,
                }
            )
        # Add properties
        for p in _discover_properties(cls):
            fields.append(
                {
                    "name": f"{var_name}.{p['name']}",
                    "type": p["type"],
                    "description": p["description"],
                }
            )
        result[level] = {
            "varName": var_name,
            "className": cls.__name__,
            "members": fields,
        }
    return result


# ============================================================================
# DRIFT DETECTION
# ============================================================================


def _check_drift() -> List[str]:
    """Check for SDK drift — public API members that lack docstrings.

    Returns a list of warning messages. When run by the generator script,
    these are printed to stderr. If --strict is passed, they become errors.
    """
    issues: List[str] = []

    for cls in _INTROSPECTED_CLASSES:
        # Check that the class has a docstring
        if not cls.__doc__:
            issues.append(f"{cls.__name__}: class has no docstring")

        # Check dataclass fields have descriptions
        if dataclasses.is_dataclass(cls):
            for f in dataclasses.fields(cls):
                if _is_internal(f):
                    continue
                desc = _get_field_description(f)
                if not desc:
                    issues.append(
                        f"{cls.__name__}.{f.name}: dataclass field has no "
                        f"metadata['description']. Add it to the field definition."
                    )

        # Check public methods have docstrings
        for name, method in inspect.getmembers(cls, predicate=inspect.isfunction):
            if name.startswith("_"):
                continue
            hints = get_type_hints(method)
            if "return" not in hints:
                continue  # Skip unannotated helpers
            if not method.__doc__:
                issues.append(
                    f"{cls.__name__}.{name}(): public method has no docstring. "
                    f"The codegen will produce an empty description."
                )

        # Check @property members have docstrings
        for name in dir(cls):
            if name.startswith("_"):
                continue
            attr = getattr(cls, name, None)
            if not isinstance(attr, property) or attr.fget is None:
                continue
            hints = get_type_hints(attr.fget)
            if "return" not in hints:
                continue
            if not attr.fget.__doc__:
                issues.append(
                    f"{cls.__name__}.{name}: @property has no docstring. The codegen will produce an empty description."
                )

    # Check EvalResult __init__ args are documented
    arg_docs = _parse_docstring_args(EvalResult.__init__)
    sig = inspect.signature(EvalResult.__init__)
    for name in sig.parameters:
        if name == "self":
            continue
        if name not in arg_docs or not arg_docs[name]:
            issues.append(f"EvalResult.__init__ parameter '{name}': no description in Args: docstring section.")

    # Check _ParamDescriptor __init__ args are documented
    arg_docs = _parse_docstring_args(_ParamDescriptor.__init__)
    sig = inspect.signature(_ParamDescriptor.__init__)
    for name in sig.parameters:
        if name == "self":
            continue
        if name not in arg_docs or not arg_docs[name]:
            issues.append(f"_ParamDescriptor.__init__ parameter '{name}': no description in Args: docstring section.")

    return issues


# ============================================================================
# MASTER EXPORT
# ============================================================================


def get_evaluator_editor_schema() -> Dict[str, Any]:
    """
    Return the complete schema for the evaluator editor UI.

    This is the single entry point called by the generator script.
    Runs drift detection and prints warnings to stderr for any SDK
    members that lack documentation (which would produce empty descriptions).
    """
    # Run drift detection
    issues = _check_drift()
    if issues:
        warnings.warn(
            f"\n\nCodegen drift detected — {len(issues)} issue(s) found:\n"
            + "\n".join(f"  - {issue}" for issue in issues)
            + "\n\nFix these in the SDK source so the generated editor schema "
            "has complete descriptions.\n",
            stacklevel=2,
        )
        # Also print to stderr for CI visibility
        print(
            f"\n⚠️  Codegen drift: {len(issues)} SDK member(s) lack documentation:",
            file=sys.stderr,
        )
        for issue in issues:
            print(f"  - {issue}", file=sys.stderr)

    return {
        "model_trees": get_model_tree_schema(),
        "completions": get_completion_schema(),
        "hover_docs": get_hover_docs(),
        "code_templates": get_code_templates(),
        "llm_judge_templates": get_llm_judge_templates(),
        "llm_judge_variables": get_llm_judge_variables(),
    }
