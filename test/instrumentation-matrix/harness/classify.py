"""Span-kind classifier — mirrors the logic the observer uses in process.go.

The matrix harness and the observer must classify the same span the same way;
this module is the shared canonical implementation on the Python side. If the
observer's classifier is updated, regenerate-contract (Phase 3) will surface
the divergence in CI.
"""
from __future__ import annotations

from typing import Any

_KINDS = {
    "llm",
    "embedding",
    "tool",
    "retriever",
    "rerank",
    "agent",
    "chain",
    "crewaitask",
}


def classify_span(span: dict[str, Any]) -> str:
    attrs = span.get("attributes", {}) or {}

    # Explicit traceloop.span.kind wins when present.
    tlk = attrs.get("traceloop.span.kind")
    if tlk in _KINDS:
        return tlk

    # CrewAI task spans carry crewai.task.*
    if any(k.startswith("crewai.task.") for k in attrs):
        return "crewaitask"

    # Retriever — vector DB attrs.
    if attrs.get("db.system") and "db.vector.query.top_k" in attrs:
        return "retriever"

    # Embedding — gen_ai.* with an embedding model.
    model = (attrs.get("gen_ai.request.model") or "").lower()
    if attrs.get("gen_ai.system") and "embedding" in model:
        return "embedding"

    # LLM — gen_ai.system + prompt/completion attrs.
    if attrs.get("gen_ai.system") and (
        any(k.startswith("gen_ai.prompt.") for k in attrs)
        or any(k.startswith("gen_ai.completion.") for k in attrs)
        or "gen_ai.usage.input_tokens" in attrs
    ):
        return "llm"

    return "unknown"
