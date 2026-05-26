from harness.validator import ContractValidator


def _llm_span(extra=None):
    attrs = {
        "gen_ai.system": "openai",
        "gen_ai.request.model": "gpt-4o-mini",
        "gen_ai.usage.input_tokens": 10,
        "gen_ai.usage.output_tokens": 2,
        "traceloop.span.kind": "llm",
    }
    if extra:
        attrs.update(extra)
    return {"name": "openai.chat", "kind": "CLIENT", "attributes": attrs}


def test_validator_passes_well_formed_llm_span():
    v = ContractValidator.load("traceloop/v1")
    result = v.validate(_llm_span(), kind="llm")
    assert result.ok, result


def test_validator_rejects_missing_required_attribute():
    span = _llm_span()
    del span["attributes"]["gen_ai.system"]
    v = ContractValidator.load("traceloop/v1")
    result = v.validate(span, kind="llm")
    assert not result.ok
    assert "gen_ai.system" in result.message


def test_validator_allows_additional_attributes():
    span = _llm_span(extra={"some.future.key": "value"})
    v = ContractValidator.load("traceloop/v1")
    result = v.validate(span, kind="llm")
    assert result.ok


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
