"""Mocked-HTTP tests for the heavy-tier AMP client + observer.

These exercise control flow and response parsing without a live AMP stack:
token caching, the deploy sequence, build-failure handling, best-effort
teardown, and the list→summaries→detail span assembly. They do NOT prove
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
    # and the service authorizes routes against it under RBAC_ENABLED=true. The
    # token call must therefore send the provisioning scopes, or every API call 403s.
    import urllib.parse as _u

    _mock_token()
    _client().access_token()
    body = _u.parse_qs([r for r in responses.calls if r.request.url.startswith(TOKEN_URL)][0].request.body)
    assert body["grant_type"] == ["client_credentials"]
    requested = body["scope"][0].split()
    # Thunder v0.45 namespaces AMP scopes with the `amp:` resource-server handle;
    # the service enforces the prefixed names (rbac.Permission.Scope).
    for required in ("amp:project:create", "amp:agent:create", "amp:agent:api-key-manage", "amp:project:delete"):
        assert required in requested


@responses.activate
def test_access_token_raises_on_failure():
    responses.add(responses.POST, TOKEN_URL, json={"error": "nope"}, status=401)
    with pytest.raises(AmpError, match="token fetch failed"):
        _client().access_token()


def _mock_happy_deploy(org="default", name="traceloop-0-60-0-langchain-0-3-27-py3-11"):
    p = f"/api/v1/orgs/{org}/projects/{name}"
    a = f"{p}/agents/{name}"
    responses.add(responses.POST, f"{BASE}/api/v1/orgs/{org}/projects", json={"name": name}, status=202)
    responses.add(responses.POST, f"{BASE}{p}/agents", json={"name": name}, status=202)
    # build: appears, then completes with an imageId
    responses.add(responses.GET, f"{BASE}{a}/builds", json={"builds": [{"buildName": "b1"}]}, status=200)
    responses.add(responses.GET, f"{BASE}{a}/builds/b1", json={"buildName": "b1", "status": "Completed", "imageId": "img-9"}, status=200)
    # No POST /deployments: the agent auto-deploys on create; the driver only
    # polls GET /deployments for the active endpoint.
    responses.add(
        responses.GET, f"{BASE}{a}/deployments",
        # endpoint is the agent's base URL; the driver appends /chat at invoke.
        json={"default": {"status": "active", "endpoints": [{"url": "http://agent.test"}]}},
        status=200,
    )
    responses.add(responses.POST, f"{BASE}{a}/environments/default/api-keys", json={"apiKey": "key-xyz"}, status=201)
    return name


@responses.activate
def test_deploy_agent_happy_path():
    _mock_token()
    cell_id = "traceloop-0.60.0-langchain-0.3.27-py3.11"
    name = _mock_happy_deploy(name=_safe_name(cell_id))
    d = _client().deploy_agent(
        cell_id=cell_id,
        instrumentation_version="0.2.1",
        framework_name="langchain",
        framework_package="langchain",
        framework_version="0.3.27",
        python_version="3.11",
    )
    assert d.endpoint_url == "http://agent.test"
    assert d.api_key == "key-xyz"
    assert d.image_id == "img-9"
    assert d.agent_name == name


@responses.activate
def test_deploy_agent_uses_framework_specific_sample_path():
    """The agent-create payload's appPath comes from DEPLOYABLE_SAMPLES, so a
    crewai cell builds samples/crewai-agent, not the langchain default."""
    import json

    _mock_token()
    cell_id = "traceloop-0.60.0-crewai-1.1.0-py3.11"
    _mock_happy_deploy(name=_safe_name(cell_id))
    _client().deploy_agent(
        cell_id=cell_id,
        instrumentation_version="0.2.1",
        framework_name="crewai",
        framework_package="crewai",
        framework_version="1.1.0",
        python_version="3.11",
    )
    agent_post = next(
        c for c in responses.calls
        if c.request.method == "POST" and c.request.url.rstrip("/").endswith("/agents")
    )
    body = json.loads(agent_post.request.body)
    repo = body["provisioning"]["repository"]
    assert repo["appPath"] == "/samples/crewai-agent"
    assert body["build"]["buildpack"]["runCommand"] == "python main.py"
    # Source ref defaults to wso2/agent-manager@main.
    assert repo["url"] == "https://github.com/wso2/agent-manager"
    assert repo["branch"] == "main"
    # The crewai sample's writable-HOME/storage env is set on the workload
    # (non-sensitive) so the instrumentor + app survive the read-only HOME.
    env = {e["key"]: e for e in body["configurations"]["env"]}
    assert env["HOME"]["value"] == "/tmp" and env["HOME"]["isSensitive"] is False
    assert env["CREWAI_STORAGE_DIR"]["value"] == "/tmp/crewai"


@responses.activate
def test_deploy_agent_repo_ref_is_overridable():
    """repo_url/repo_branch override the agent source the buildpack clones, so
    a PR can validate a sample on its own branch before it merges to main."""
    import json

    _mock_token()
    cell_id = "traceloop-0.60.0-langchain-0.3.27-py3.11"
    _mock_happy_deploy(name=_safe_name(cell_id))
    _client().deploy_agent(
        cell_id=cell_id,
        instrumentation_version="0.2.1",
        framework_name="langchain",
        framework_package="langchain",
        framework_version="0.3.27",
        python_version="3.11",
        repo_url="https://github.com/acme/fork",
        repo_branch="fix/my-branch",
    )
    agent_post = next(
        c for c in responses.calls
        if c.request.method == "POST" and c.request.url.rstrip("/").endswith("/agents")
    )
    repo = json.loads(agent_post.request.body)["provisioning"]["repository"]
    assert repo["url"] == "https://github.com/acme/fork"
    assert repo["branch"] == "fix/my-branch"


@responses.activate
def test_deploy_agent_raises_on_build_failure():
    _mock_token()
    name = "x"
    a = f"/api/v1/orgs/default/projects/{name}/agents/{name}"
    responses.add(responses.POST, f"{BASE}/api/v1/orgs/default/projects", json={}, status=202)
    responses.add(responses.POST, f"{BASE}/api/v1/orgs/default/projects/{name}/agents", json={}, status=202)
    responses.add(responses.GET, f"{BASE}{a}/builds", json={"builds": [{"buildName": "b1"}]}, status=200)
    responses.add(responses.GET, f"{BASE}{a}/builds/b1", json={"buildName": "b1", "status": "Failed"}, status=200)
    with pytest.raises(AmpError, match="failed"):
        _client().deploy_agent(
            cell_id=name, instrumentation_version="0.2.1",
            framework_name="langchain",
            framework_package="langchain", framework_version="0.3.27", python_version="3.11",
        )


@responses.activate
def test_deploy_agent_surfaces_unexpected_status():
    _mock_token()
    responses.add(responses.POST, f"{BASE}/api/v1/orgs/default/projects", json={"error": "dup"}, status=409)
    with pytest.raises(AmpError, match="→ 409"):
        _client().deploy_agent(
            cell_id="x", instrumentation_version="0.2.1",
            framework_name="langchain",
            framework_package="langchain", framework_version="0.3.27", python_version="3.11",
        )


@responses.activate
def test_teardown_is_best_effort():
    _mock_token()
    from heavy.amp_client import DeployedAgent

    # DELETE returns 500 — teardown must swallow it, not raise.
    responses.add(responses.DELETE, f"{BASE}/api/v1/orgs/default/projects/p/agents/a", status=500)
    responses.add(responses.DELETE, f"{BASE}/api/v1/orgs/default/projects/p", status=500)
    d = DeployedAgent(
        org="default", project_name="p", agent_name="a", environment="default",
        endpoint_url="http://x", api_key="k",
    )
    _client().teardown_agent(d)  # must not raise


@responses.activate
def test_poll_traces_assembles_validator_spans():
    _mock_token()
    from heavy.amp_client import DeployedAgent

    d = DeployedAgent(
        org="default", project_name="p", agent_name="a", environment="default",
        endpoint_url="http://x", api_key="k",
    )
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

    d = DeployedAgent(
        org="default", project_name="p", agent_name="a", environment="default",
        endpoint_url="http://x", api_key="k",
    )
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
