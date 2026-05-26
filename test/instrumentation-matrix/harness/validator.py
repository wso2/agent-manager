"""ContractValidator — loads a versioned JSON-schema bundle for a provider and
validates captured spans against it (shape + coverage).
"""
from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator

from harness.classify import classify_span

_HERE = Path(__file__).resolve().parent.parent
_CONTRACTS = _HERE / "contracts"


@dataclass
class ValidationResult:
    ok: bool
    span_name: str
    kind: str
    message: str = ""
    path: str = ""


@dataclass
class CoverageResult:
    ok: bool
    expected: list[str]
    actual: set[str]
    missing: set[str]


class ContractValidator:
    def __init__(self, schema_id: str, kind_schemas: dict[str, Draft202012Validator]):
        self.schema_id = schema_id
        self._validators = kind_schemas

    @classmethod
    def load(cls, schema_id: str) -> "ContractValidator":
        bundle_dir = _CONTRACTS / schema_id / "kinds"
        validators: dict[str, Draft202012Validator] = {}
        for schema_file in bundle_dir.glob("*.schema.json"):
            kind = schema_file.stem.removesuffix(".schema")
            schema = json.loads(schema_file.read_text())
            validators[kind] = Draft202012Validator(schema)
        if not validators:
            raise FileNotFoundError(f"no schemas under {bundle_dir}")
        return cls(schema_id, validators)

    def validate(self, span: dict[str, Any], kind: str) -> ValidationResult:
        validator = self._validators.get(kind)
        if validator is None:
            return ValidationResult(
                ok=False,
                span_name=span.get("name", "?"),
                kind=kind,
                message=f"no schema for kind '{kind}' in bundle {self.schema_id}",
            )
        errors = sorted(validator.iter_errors(span), key=lambda e: list(e.path))
        if not errors:
            return ValidationResult(ok=True, span_name=span.get("name", "?"), kind=kind)
        e = errors[0]
        return ValidationResult(
            ok=False,
            span_name=span.get("name", "?"),
            kind=kind,
            message=e.message,
            path="/" + "/".join(str(p) for p in e.absolute_path),
        )

    def validate_all(self, spans: list[dict[str, Any]]) -> list[ValidationResult]:
        return [self.validate(s, classify_span(s)) for s in spans]

    def validate_resource(self, resource: dict[str, Any]) -> ValidationResult:
        path = _CONTRACTS / self.schema_id / "resource.schema.json"
        v = Draft202012Validator(json.loads(path.read_text()))
        errors = sorted(v.iter_errors(resource), key=lambda e: list(e.path))
        if not errors:
            return ValidationResult(ok=True, span_name="<resource>", kind="resource")
        e = errors[0]
        return ValidationResult(
            ok=False,
            span_name="<resource>",
            kind="resource",
            message=e.message,
            path="/" + "/".join(str(p) for p in e.absolute_path),
        )

    def assert_coverage(
        self, spans: list[dict[str, Any]], expected_kinds: list[str]
    ) -> CoverageResult:
        actual = {classify_span(s) for s in spans}
        missing = set(expected_kinds) - actual
        return CoverageResult(
            ok=not missing, expected=expected_kinds, actual=actual, missing=missing
        )
