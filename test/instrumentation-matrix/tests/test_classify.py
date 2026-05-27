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


def test_retriever_via_legacy_db_system():
    s = _span("vector_search", {"db.system": "chroma", "db.vector.query.top_k": 5})
    assert classify_span(s) == "retriever"


def test_retriever_via_db_system_name():
    # The published manual-instrumentation sample uses the current OTel key
    # `db.system.name` (with db.collection.name + db.vector.query.top_k) and
    # does NOT set traceloop.span.kind=retriever.
    s = _span(
        "vector_search",
        {"db.system.name": "chroma", "db.collection.name": "kb", "db.vector.query.top_k": 5},
    )
    assert classify_span(s) == "retriever"


def test_agent_via_invoke_agent_operation():
    s = _span("invoke_agent", {"gen_ai.operation.name": "invoke_agent"})
    assert classify_span(s) == "agent"


def test_tool_via_execute_tool_operation():
    s = _span(
        "execute_tool",
        {"gen_ai.operation.name": "execute_tool", "gen_ai.tool.name": "word_count"},
    )
    assert classify_span(s) == "tool"


def test_workflow_maps_to_chain():
    s = _span(
        "RetrieverQueryEngine.workflow",
        {"traceloop.span.kind": "workflow"},
    )
    assert classify_span(s) == "chain"


def test_task_maps_to_chain():
    s = _span(
        "SentenceSplitter.task",
        {"traceloop.span.kind": "task"},
    )
    assert classify_span(s) == "chain"


def test_embedding_inside_task_wrapper_still_classified_as_embedding():
    # LlamaIndex emits a Traceloop task wrapper that still carries the
    # gen_ai.operation.name=embeddings attribute. The classifier must prefer
    # the gen_ai.operation.name signal over the generic 'task' span kind.
    s = _span(
        "OpenAIEmbedding.task",
        {
            "traceloop.span.kind": "task",
            "gen_ai.operation.name": "embeddings",
            "gen_ai.request.model": "text-embedding-3-small",
        },
    )
    assert classify_span(s) == "embedding"


def test_unknown_when_no_signals():
    s = _span("anonymous", {})
    assert classify_span(s) == "unknown"
