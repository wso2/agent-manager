from harness.classify import classify_span


def _span(name, attrs, kind="CLIENT"):
    return {"name": name, "kind": kind, "attributes": attrs}


def test_llm_via_traceloop_kind_legacy():
    s = _span("openai.chat", {"traceloop.span.kind": "llm", "gen_ai.system": "openai"})
    assert classify_span(s) == "llm"


def test_llm_via_otel_genai_operation_name():
    s = _span(
        "ChatOpenAI.chat",
        {"gen_ai.operation.name": "chat", "gen_ai.provider.name": "openai"},
    )
    assert classify_span(s) == "llm"


def test_tool_via_traceloop_kind():
    s = _span("my-tool", {"traceloop.span.kind": "tool"})
    assert classify_span(s) == "tool"


def test_embedding_via_otel_operation_name():
    s = _span(
        "OpenAIEmbeddings.embed",
        {"gen_ai.operation.name": "embeddings", "gen_ai.provider.name": "openai"},
    )
    assert classify_span(s) == "embedding"


def test_embedding_via_legacy_attribute_heuristic():
    s = _span(
        "openai.embedding",
        {"gen_ai.system": "openai", "gen_ai.request.model": "text-embedding-3-small"},
    )
    assert classify_span(s) == "embedding"


def test_retriever_via_db_attrs():
    s = _span("vector_search", {"db.system": "chroma", "db.vector.query.top_k": 5})
    assert classify_span(s) == "retriever"


def test_unknown_when_no_signals():
    s = _span("anonymous", {})
    assert classify_span(s) == "unknown"
