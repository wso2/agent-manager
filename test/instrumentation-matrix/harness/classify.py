"""Span-kind classifier — mirrors the logic the observer uses in process.go.

The matrix harness and the observer must classify the same span the same way;
this module is the shared canonical implementation on the Python side. If the
observer's classifier is updated, regenerate-contract (Phase 3) will surface
the divergence in CI.

Recognises both the legacy attribute namespace (gen_ai.system,
traceloop.span.kind) and the current OTel GenAI semconv (gen_ai.provider.name,
gen_ai.operation.name). Traceloop 0.60+ emits the latter.
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

# Traceloop emits `workflow` and `task` as generic wrapper-span kinds; AMP's
# observer collapses both into the `chain` kind (see INSTRUMENTATION.md §6.4
# and types.go: SpanTypeChain = "Generic tasks/workflows").
_TRACELOOP_CHAIN_KINDS = {"workflow", "task"}


def classify_span(span: dict[str, Any]) -> str:
    attrs = span.get("attributes", {}) or {}

    # Explicit AMP-kind hint via traceloop.span.kind wins when present.
    tlk = attrs.get("traceloop.span.kind")
    if tlk in _KINDS:
        return tlk

    # CrewAI task spans carry crewai.task.*
    if any(k.startswith("crewai.task.") for k in attrs):
        return "crewaitask"

    # Retriever — vector DB attrs. Accept the current OTel db.system.name
    # in addition to the legacy db.system; the published manual-instrumentation
    # sample uses db.system.name with db.collection.name + db.vector.query.top_k
    # but does NOT set traceloop.span.kind=retriever.
    if attrs.get("db.system") or attrs.get("db.system.name"):
        return "retriever"

    # OTel GenAI semconv (current): gen_ai.operation.name discriminates.
    # Checked *before* the workflow/task fallback so a real embedding span
    # wrapped in a Traceloop task span still classifies as embedding.
    op = (attrs.get("gen_ai.operation.name") or "").lower()
    if op in {"chat", "text_completion", "generate_content"}:
        return "llm"
    if op in {"embeddings", "embedding"}:
        return "embedding"
    if op == "invoke_agent":
        return "agent"
    if op == "execute_tool":
        return "tool"

    # Legacy heuristics for older Traceloop versions.
    model = (attrs.get("gen_ai.request.model") or "").lower()
    if attrs.get("gen_ai.system") and "embedding" in model:
        return "embedding"
    if attrs.get("gen_ai.system") and (
        any(k.startswith("gen_ai.prompt.") for k in attrs)
        or any(k.startswith("gen_ai.completion.") for k in attrs)
        or "gen_ai.usage.input_tokens" in attrs
    ):
        return "llm"

    # Generic Traceloop wrappers map to chain.
    if tlk in _TRACELOOP_CHAIN_KINDS:
        return "chain"

    return "unknown"
