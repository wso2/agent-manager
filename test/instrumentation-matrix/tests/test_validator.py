from harness.validator import ContractValidator


# ---------- LLM ----------


def _llm_span(extra=None):
    attrs = {
        "gen_ai.provider.name": "openai",
        "gen_ai.request.model": "gpt-4o-mini",
        "gen_ai.operation.name": "chat",
        "gen_ai.usage.input_tokens": 10,
        "gen_ai.usage.output_tokens": 2,
    }
    if extra:
        attrs.update(extra)
    return {"name": "ChatOpenAI.chat", "kind": "CLIENT", "attributes": attrs}


def test_validator_passes_well_formed_llm_span():
    v = ContractValidator.load("traceloop/v1")
    assert v.validate(_llm_span(), kind="llm").ok


def test_validator_rejects_missing_required_attribute():
    # Drop both vendor keys; the anyOf clause in the schema requires at least
    # one of gen_ai.system or gen_ai.provider.name.
    span = _llm_span()
    del span["attributes"]["gen_ai.provider.name"]
    v = ContractValidator.load("traceloop/v1")
    result = v.validate(span, kind="llm")
    assert not result.ok


def test_validator_accepts_legacy_vendor_key():
    # gen_ai.system (legacy) and gen_ai.provider.name (current) both satisfy
    # the schema's vendor anyOf clause; the observer accepts either.
    span = _llm_span()
    del span["attributes"]["gen_ai.provider.name"]
    span["attributes"]["gen_ai.system"] = "openai"
    v = ContractValidator.load("traceloop/v1")
    assert v.validate(span, kind="llm").ok


def test_validator_allows_additional_attributes():
    span = _llm_span(extra={"some.future.key": "value"})
    v = ContractValidator.load("traceloop/v1")
    assert v.validate(span, kind="llm").ok


# ---------- Embedding ----------


def test_validator_passes_embedding_span():
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "OpenAIEmbeddings.embed",
        "kind": "CLIENT",
        "attributes": {
            "gen_ai.provider.name": "openai",
            "gen_ai.request.model": "text-embedding-3-small",
            "gen_ai.operation.name": "embeddings",
        },
    }
    assert v.validate(span, kind="embedding").ok


def test_validator_rejects_embedding_missing_model():
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "OpenAIEmbeddings.embed",
        "kind": "CLIENT",
        "attributes": {
            "gen_ai.provider.name": "openai",
            "gen_ai.operation.name": "embeddings",
        },
    }
    result = v.validate(span, kind="embedding")
    assert not result.ok
    assert "gen_ai.request.model" in result.message


# ---------- Tool ----------


def test_validator_passes_minimal_tool_span():
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "my_tool.execute",
        "kind": "INTERNAL",
        "attributes": {"traceloop.entity.name": "my_tool"},
    }
    assert v.validate(span, kind="tool").ok


def test_validator_rejects_tool_with_wrong_span_kind_const():
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "my_tool.execute",
        "kind": "INTERNAL",
        "attributes": {"traceloop.span.kind": "agent"},   # const says "tool"
    }
    result = v.validate(span, kind="tool")
    assert not result.ok


# ---------- Retriever ----------


def test_validator_passes_retriever_span():
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "chroma.query",
        "kind": "CLIENT",
        "attributes": {"db.system": "chroma", "db.vector.query.top_k": 5},
    }
    assert v.validate(span, kind="retriever").ok


def test_validator_rejects_retriever_missing_vector_db():
    # The schema's anyOf requires either db.system or db.system.name to be
    # present so RetrieverData.VectorDB can be populated by the observer.
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "unknown.query",
        "kind": "CLIENT",
        "attributes": {"db.vector.query.top_k": 5},
    }
    result = v.validate(span, kind="retriever")
    assert not result.ok


# ---------- Rerank ----------


def test_validator_passes_minimal_rerank_span():
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "cohere.rerank",
        "kind": "CLIENT",
        "attributes": {"traceloop.span.kind": "rerank"},
    }
    assert v.validate(span, kind="rerank").ok


# ---------- Agent ----------


def test_validator_passes_minimal_agent_span():
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "AgentExecutor.invoke",
        "kind": "INTERNAL",
        "attributes": {"gen_ai.agent.name": "research-agent"},
    }
    assert v.validate(span, kind="agent").ok


# ---------- Chain ----------


def test_validator_passes_minimal_chain_span():
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "LLMChain.invoke",
        "kind": "INTERNAL",
        "attributes": {"traceloop.span.kind": "chain"},
    }
    assert v.validate(span, kind="chain").ok


# ---------- CrewAI task ----------


def test_validator_passes_minimal_crewaitask_span():
    # Required attribute is crewai.task.description, not name — see FINDINGS.md
    # F-002 for why (CrewAI Tasks lack a `name` field).
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "Look up the capital of France..task",
        "kind": "INTERNAL",
        "attributes": {"crewai.task.description": "Look up the capital of France."},
    }
    assert v.validate(span, kind="crewaitask").ok


def test_validator_rejects_crewaitask_missing_description():
    v = ContractValidator.load("traceloop/v1")
    span = {
        "name": "anonymous.task",
        "kind": "INTERNAL",
        "attributes": {"crewai.task.tools": "[]"},
    }
    result = v.validate(span, kind="crewaitask")
    assert not result.ok
    assert "crewai.task.description" in result.message


# ---------- Unknown (permissive) ----------


def test_validator_passes_unknown_span_with_no_attributes():
    v = ContractValidator.load("traceloop/v1")
    span = {"name": "anonymous", "kind": "INTERNAL", "attributes": {}}
    assert v.validate(span, kind="unknown").ok


# ---------- Coverage ----------


def test_assert_coverage_reports_missing_kinds():
    v = ContractValidator.load("traceloop/v1")
    spans = [_llm_span()]
    cov = v.assert_coverage(spans, expected_kinds=["llm", "tool"])
    assert not cov.ok
    assert "tool" in cov.missing


def test_assert_coverage_passes_when_all_kinds_present():
    v = ContractValidator.load("traceloop/v1")
    spans = [_llm_span()]
    cov = v.assert_coverage(spans, expected_kinds=["llm"])
    assert cov.ok


# ---------- Resource ----------


def test_validator_rejects_resource_missing_service_name():
    v = ContractValidator.load("traceloop/v1")
    r = v.validate_resource({})
    assert not r.ok
    assert "service.name" in r.message


def test_validator_passes_resource_with_service_name():
    v = ContractValidator.load("traceloop/v1")
    assert v.validate_resource({"service.name": "my-agent"}).ok
