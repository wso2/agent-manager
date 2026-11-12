"""Mocked-HTTP tests for the heavy-tier AMP client + observer.

These exercise control flow and response parsing without a live AMP stack:
token caching, external-agent registration, best-effort teardown, and the
list→summaries→detail span assembly. They do NOT prove
the implementation works against a real cluster — only that the plumbing
and shape-coercion are internally consistent.
"""
import pytest

responses = pytest.importorskip("responses")

import re  # noqa: E402

from heavy.amp_client import AmpClient, AmpError, IdpCredentials, _safe_name  # noqa: E402
from heavy.observer import poll_traces  # noqa: E402

# AMP resource-name rule (agent-manager-service utils.ValidateResourceName):
# <=25 chars, [a-z0-9-], starts with a letter, ends alphanumeric.
_AMP_NAME_RE = re.compile(r"^[a-z][a-z0-9-]{0,23}[a-z0-9]$")

BASE = "http://amp.test"
OBS = "http://obs.test"
TOKEN_URL = "http://idp.test/oauth2/token"


def _client():
    return AmpClient(
        BASE,
        IdpCredentials(token_url=TOKEN_URL, client_id="cid", client_secret="sec"),
    )


def _mock_token():
    responses.add(
        responses.POST, TOKEN_URL,
        json={"access_token": "tok-123", "expires_in": 3600, "token_type": "Bearer"},
        status=200,
    )


@responses.activate
def test_access_token_fetches_and_caches():
    _mock_token()
    c = _client()
    assert c.access_token() == "tok-123"
    # Second call must not hit the token endpoint again (cached).
    assert c.access_token() == "tok-123"
    token_calls = [r for r in responses.calls if r.request.url.startswith(TOKEN_URL)]
    assert len(token_calls) == 1


@responses.activate
def test_access_token_requests_rbac_scopes():
    # Thunder only puts a scope in a client_credentials token when it's requested,
    # and the service authorizes routes against it. The
    # token call must therefore send the provisioning scopes, or every API call 403s.
    import urllib.parse as _u

    _mock_token()
    _client().access_token()
    body = _u.parse_qs([r for r in responses.calls if r.request.url.startswith(TOKEN_URL)][0].request.body)
    assert body["grant_type"] == ["client_credentials"]
    requested = body["scope"][0].split()
    # Thunder v0.45 namespaces AMP scopes with the `amp:` resource-server handle;
    # the service enforces the prefixed names (rbac.Permission.Scope).
    for required in ("amp:project:create", "amp:agent:create", "amp:agent:token-manage", "amp:project:delete"):
        assert required in requested


@responses.activate
def test_access_token_raises_on_failure():
    responses.add(responses.POST, TOKEN_URL, json={"error": "nope"}, status=401)
    with pytest.raises(AmpError, match="token fetch failed"):
        _client().access_token()


@responses.activate
def test_register_external_agent_creates_project_agent_and_token():
    _mock_token()
    name = "traceloop-0-60-0-langchain"
    p = f"/api/v1/orgs/default/projects/{name}"
    responses.add(responses.POST, f"{BASE}/api/v1/orgs/default/projects", json={"name": name}, status=202)
    responses.add(responses.POST, f"{BASE}{p}/agents", json={"name": name}, status=202)
    responses.add(responses.POST, f"{BASE}{p}/agents/{name}/token", json={"token": "agent-tok"}, status=200)

    assert _client().register_external_agent("default", name) == "agent-tok"

    import json as _json
    agent_body = _json.loads([c for c in responses.calls if c.request.url == f"{BASE}{p}/agents"][0].request.body)
    assert agent_body["provisioning"] == {"type": "external"}


@responses.activate
def test_register_external_agent_surfaces_unexpected_status():
    _mock_token()
    responses.add(responses.POST, f"{BASE}/api/v1/orgs/default/projects",
                  json={"code": "VALIDATION_ERROR"}, status=400)
    with pytest.raises(AmpError, match="400"):
        _client().register_external_agent("default", "x")


@responses.activate
def test_teardown_is_best_effort():
    _mock_token()
    from heavy.amp_client import DeployedAgent

    # DELETE returns 500 — teardown must swallow it, not raise.
    responses.add(responses.DELETE, f"{BASE}/api/v1/orgs/default/projects/p/agents/a", status=500)
    responses.add(responses.DELETE, f"{BASE}/api/v1/orgs/default/projects/p", status=500)
    d = DeployedAgent(org="default", project_name="p", agent_name="a", environment="default")
    _client().teardown_agent(d)  # must not raise


@responses.activate
def test_poll_traces_assembles_validator_spans():
    _mock_token()
    from heavy.amp_client import DeployedAgent

    d = DeployedAgent(org="default", project_name="p", agent_name="a", environment="default")
    responses.add(responses.GET, f"{OBS}/api/v1/traces", json={"traces": [{"traceId": "t1"}]}, status=200)
    responses.add(responses.GET, f"{OBS}/api/v1/traces/t1/spans", json={"spans": [{"spanId": "s1"}]}, status=200)
    responses.add(
        responses.GET, f"{OBS}/api/v1/traces/t1/spans/s1",
        json={
            "traceId": "t1", "spanId": "s1", "name": "openai.chat", "kind": "CLIENT",
            "attributes": {"gen_ai.request.model": "gpt-4o-mini", "traceloop.span.kind": "llm"},
            "resource": {"service.name": "agent"},
        },
        status=200,
    )
    spans = poll_traces(_client(), d, OBS, timeout_s=1)
    assert len(spans) == 1
    s = spans[0]
    assert s["name"] == "openai.chat"
    assert s["kind"] == "CLIENT"
    assert s["attributes"]["traceloop.span.kind"] == "llm"
    assert s["traceId"] == "t1" and s["spanId"] == "s1"


@responses.activate
def test_poll_traces_empty_when_no_traces():
    _mock_token()
    from heavy.amp_client import DeployedAgent

    d = DeployedAgent(org="default", project_name="p", agent_name="a", environment="default")
    responses.add(responses.GET, f"{OBS}/api/v1/traces", json={"traces": []}, status=200)
    assert poll_traces(_client(), d, OBS, timeout_s=1) == []


# A representative heavy-subset cell id — its dotted versions make the naive
# slug 47 chars, well over AMP's 25-char cap.
_LONG_CELL = "traceloop-0.60.0-anthropic-direct-0.45.0-py3.11"


def test_safe_name_obeys_amp_resource_rule():
    name = _safe_name(_LONG_CELL)
    assert len(name) <= 25
    assert _AMP_NAME_RE.match(name), name


def test_safe_name_short_id_passes_through():
    assert _safe_name("traceloop-langchain") == "traceloop-langchain"


def test_safe_name_is_stable_and_unique():
    # deterministic: same input -> same name (teardown reuses it)
    assert _safe_name(_LONG_CELL) == _safe_name(_LONG_CELL)
    # distinct long cells that share a 25-char prefix must not collide
    a = _safe_name("traceloop-0.60.0-anthropic-direct-0.45.0-py3.11")
    b = _safe_name("traceloop-0.60.0-anthropic-direct-0.45.0-py3.12")
    assert a != b
