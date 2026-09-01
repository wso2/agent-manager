"""Client for agent-manager-service REST API — used by the heavy-tier driver
to provision an agent per cell against the live AMP stack.

The flow mirrors the Go e2e suite (`test/e2e/framework/` +
`test/e2e/operations/`), which is the authoritative reference for the real
endpoints and payloads:

  1. OAuth2 client_credentials grant against Thunder IDP        (auth.go)
  2. POST create project                                        (project/create_project.go)
  3. POST create agent — Configurations.instrumentationVersion  (agent/create_agent.go,
     pins the cell's init-container image                        framework/factories.go)
  4. poll builds until Completed → imageId                      (build/build_operations.go)
  5. POST deploy with imageId                                   (deployment/deploy_agent.go)
  6. poll deployments until status == "active" → endpoint URL
  7. POST mint API key                                          (agent/agent_apikey.go)

NOTE: this is implemented against the e2e Go reference but has not yet run
against a live AMP stack. Treat the timing constants, the IDP/observer URLs,
and the namespace/component mapping in heavy/observer.py as first-run-
tunable. The heavy job is `continue-on-error: true` in CI until a real run
validates this end to end.
"""
from __future__ import annotations

import hashlib
import time
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone

import requests

from harness.deployable_samples import DEPLOYABLE_SAMPLES

# Timeout budget (see RUNBOOK.md §7).
_BUILD_TIMEOUT_S = 600
_BUILD_POLL_S = 10
_DEPLOY_TIMEOUT_S = 600
_DEPLOY_POLL_S = 10
# Consecutive "failed" polls before giving up (tolerates a transient failed
# during reconciliation while still bailing fast on a genuinely stuck deploy).
_DEPLOY_FAILED_STREAK = 3
_HTTP_TIMEOUT_S = 30
_TOKEN_REFRESH_SKEW_S = 30

# OAuth2 scopes requested in the client_credentials grant. With RBAC_ENABLED=true
# the service authorizes every route against the token's scopes, and Thunder only
# puts a scope in a client_credentials token when it is *explicitly requested*
# (and the app is registered for it + assigned a role that grants it — both done
# for amp-api-client in the Thunder bootstrap). Omitting `scope` yields a token
# with no scope claim, so every call 403s. Request exactly the permissions this
# heavy-tier flow exercises: provisioning (create project/agent, read
# builds+deployments, mint API key, teardown deletes) plus reading the emitted
# spans back from the observer (heavy/observer.py reuses this same token for
# GET /api/v1/traces[/…], which the observer wraps in RequirePermission(
# TraceRead) under RBAC_ENABLED=true — so amp:observability:trace-read is
# required or every trace poll 403s and the cell reports a pipeline error
# instead of no-spans-captured). Mirrors how the console requests its scope
# list. Unauthorised/unknown scopes are silently filtered by Thunder, so this
# stays a tight, intention-revealing set.
#
# Thunder v0.45 namespaces every AMP scope with the `amp:` resource-server
# handle (permissions are built as <resource-server>:<resource>:<action>; see
# agent-manager-service rbac.Permission.Scope). The service enforces the
# prefixed names, and Thunder filters the old bare names, so these MUST carry
# the `amp:` prefix or every call 403s.
_TOKEN_SCOPES = (
    "amp:project:create amp:project:read amp:project:delete "
    "amp:agent:create amp:agent:read amp:agent:delete amp:agent:build "
    "amp:agent:env-non-production amp:agent:api-key-manage "
    "amp:observability:trace-read"
)

# The deployed agent's source is cloned from this repo ref by the in-cluster
# buildpack. Defaults to wso2/agent-manager@main; overridable (see
# heavy/driver.py AMP_AGENT_REPO_*) so a PR can validate a new/changed sample
# on its own branch before it merges to main — the build pulls the agent code
# from GitHub, not from the working tree the platform is built from.
_DEFAULT_AGENT_REPO_URL = "https://github.com/wso2/agent-manager"
_DEFAULT_AGENT_REPO_BRANCH = "main"

# AMP caps resource names at 25 chars (utils.MaxResourceNameLength); when a
# cell id exceeds it, _safe_name keeps a prefix plus this many hash chars.
_MAX_NAME_LEN = 25
_HASH_LEN = 6


def _safe_name(cell_id: str) -> str:
    """Coerce a cell id into an AMP project/agent name.

    AMP enforces RFC 1035 DNS labels (agent-manager-service
    utils.ValidateResourceName): at most 25 chars, only `[a-z0-9-]`, starts
    with a letter, ends alphanumeric. Cell ids carry dotted versions and run
    ~47 chars, so fold non-alnum to `-` and, when too long, keep a readable
    prefix plus a short stable hash — deterministic so teardown reuses the
    same name, and collision-resistant when prefixes coincide."""
    slug = "".join(ch if (ch.isalnum() or ch == "-") else "-" for ch in cell_id.lower()).strip("-")
    if len(slug) <= _MAX_NAME_LEN:
        return slug
    digest = hashlib.sha1(cell_id.encode()).hexdigest()[:_HASH_LEN]
    prefix = slug[: _MAX_NAME_LEN - _HASH_LEN - 1].strip("-")
    return f"{prefix}-{digest}"


def _utc_rfc3339(*, hours_from_now: int) -> str:
    return (datetime.now(timezone.utc) + timedelta(hours=hours_from_now)).strftime(
        "%Y-%m-%dT%H:%M:%SZ"
    )


@dataclass
class IdpCredentials:
    """Thunder OAuth2 client_credentials grant inputs."""

    token_url: str  # e.g. http://thunder.amp.localhost:8080/oauth2/token
    client_id: str  # e.g. amp-api-client
    client_secret: str


@dataclass
class DeployedAgent:
    org: str
    project_name: str
    agent_name: str
    environment: str
    endpoint_url: str
    api_key: str
    image_id: str = ""


class AmpError(RuntimeError):
    """Raised when an AMP API call returns an unexpected status."""


class AmpClient:
    """REST client for agent-manager-service.

    Authenticates against Thunder IDP via OAuth2 `client_credentials` and
    attaches `Authorization: Bearer <token>` to every call, refreshing the
    token proactively before expiry.
    """

    def __init__(self, base_url: str, idp: IdpCredentials, *, org: str = "default",
                 environment: str = "default", deployment_pipeline: str = "default"):
        self.base_url = base_url.rstrip("/")
        self.idp = idp
        self.org = org
        self.environment = environment
        self.deployment_pipeline = deployment_pipeline
        self._token: str | None = None
        self._token_expires_at: float = 0.0
        self._session = requests.Session()

    # ── auth ────────────────────────────────────────────────────────────

    def access_token(self) -> str:
        """Return a current access token, refreshing within 30s of expiry.

        Follows test/e2e/framework/auth.go: POST the token endpoint with
        `grant_type=client_credentials` and HTTP Basic client credentials, plus
        an explicit `scope` (see _TOKEN_SCOPES) so the token carries the
        permissions RBAC checks against.
        """
        if self._token and time.monotonic() < self._token_expires_at - _TOKEN_REFRESH_SKEW_S:
            return self._token

        resp = self._session.post(
            self.idp.token_url,
            data={"grant_type": "client_credentials", "scope": _TOKEN_SCOPES},
            auth=(self.idp.client_id, self.idp.client_secret),
            headers={"Content-Type": "application/x-www-form-urlencoded"},
            timeout=_HTTP_TIMEOUT_S,
        )
        if resp.status_code != 200:
            raise AmpError(
                f"token fetch failed: {resp.status_code} {resp.text[:200]}"
            )
        body = resp.json()
        self._token = body["access_token"]
        # expires_in is seconds-from-now; track an absolute monotonic deadline.
        self._token_expires_at = time.monotonic() + int(body.get("expires_in", 3600))
        return self._token

    def _headers(self, *, json_body: bool = False) -> dict[str, str]:
        h = {"Authorization": f"Bearer {self.access_token()}"}
        if json_body:
            h["Content-Type"] = "application/json"
        return h

    def _request(self, method: str, path: str, *, json=None, expect: tuple[int, ...]):
        url = f"{self.base_url}{path}"
        resp = self._session.request(
            method, url, json=json,
            headers=self._headers(json_body=json is not None),
            timeout=_HTTP_TIMEOUT_S,
        )
        if resp.status_code not in expect:
            raise AmpError(
                f"{method} {path} → {resp.status_code} (expected {expect}): "
                f"{resp.text[:300]}"
            )
        return resp

    # ── provisioning ────────────────────────────────────────────────────

    def deploy_agent(
        self,
        *,
        cell_id: str,
        instrumentation_version: str,
        framework_name: str,
        framework_package: str,
        framework_version: str,
        python_version: str,
        agent_env: dict[str, str] | None = None,
        repo_url: str = _DEFAULT_AGENT_REPO_URL,
        repo_branch: str = _DEFAULT_AGENT_REPO_BRANCH,
    ) -> DeployedAgent:
        """Create project + agent + build + deploy + API key; return the
        endpoint and key. Raises AmpError / TimeoutError on failure.

        `cell_id` is sanitised into the project/agent names. The cell's
        `instrumentation_version` pins the init-container image; the
        `framework_name` selects which deployable sample to build (its
        `appPath`/`runCommand` come from harness.deployable_samples).
        `agent_env` (LLM keys) is injected as sensitive env on the agent so
        the deployed pod can make real provider calls. `repo_url`/`repo_branch`
        are the GitHub source the buildpack clones the sample from (default
        wso2/agent-manager@main); override to validate a sample on its own
        branch before it merges.
        """
        name = _safe_name(cell_id)
        org = self.org

        try:
            sample = DEPLOYABLE_SAMPLES[framework_name]
        except KeyError:
            raise AmpError(
                f"no deployable sample for framework {framework_name!r}; "
                f"deployable: {sorted(DEPLOYABLE_SAMPLES)}"
            ) from None

        # LLM keys (sensitive) + the framework pin (informational).
        env = [
            {"key": k, "value": v, "isSensitive": True}
            for k, v in (agent_env or {}).items()
        ]
        env.append({
            "key": "AMP_MATRIX_FRAMEWORK",
            "value": f"{framework_package}=={framework_version}",
            "isSensitive": False,
        })
        # Sample-declared env (non-sensitive). Set on the workload so it exists
        # before the interpreter starts — the instrumentation provider's
        # sitecustomize imports the framework at startup, so env the framework
        # needs at import (e.g. CrewAI's writable HOME/CREWAI_STORAGE_DIR) must
        # be present before the app's own code runs.
        for k, v in sample.env:
            env.append({"key": k, "value": v, "isSensitive": False})

        # (2) project
        self._request(
            "POST", f"/api/v1/orgs/{org}/projects",
            json={
                "name": name,
                "displayName": name,
                "deploymentPipeline": self.deployment_pipeline,
            },
            expect=(202,),
        )

        # (3) agent — pin instrumentation_version + auto-instrumentation on.
        self._request(
            "POST", f"/api/v1/orgs/{org}/projects/{name}/agents",
            json={
                "name": name,
                "displayName": name,
                "provisioning": {
                    "type": "internal",
                    "repository": {
                        "url": repo_url,
                        "branch": repo_branch,
                        "appPath": sample.app_path,
                    },
                },
                "agentType": {"type": "agent-api", "subType": "chat-api"},
                "build": {
                    "type": "buildpack",
                    "buildpack": {
                        "language": "python",
                        "languageVersion": python_version,
                        "runCommand": sample.run_command,
                    },
                },
                "configurations": {
                    "enableAutoInstrumentation": True,
                    "instrumentationVersion": instrumentation_version,
                    "env": env,
                },
                "inputInterface": {"type": "HTTP"},
            },
            expect=(202,),
        )

        image_id = self._wait_for_build(org, name)
        # No explicit deploy: creating an internal agent auto-triggers build and
        # deploy (mirroring test/e2e's shared-agent setup, which only waits).
        # A redundant POST /deployments re-renders the workload without the
        # sensitive env, dropping the SecretReference and failing the binding.
        endpoint = self._wait_for_active_endpoint(org, name)
        api_key = self._mint_api_key(org, name)

        return DeployedAgent(
            org=org,
            project_name=name,
            agent_name=name,
            environment=self.environment,
            endpoint_url=endpoint,
            api_key=api_key,
            image_id=image_id,
        )

    def _wait_for_build(self, org: str, name: str) -> str:
        base = f"/api/v1/orgs/{org}/projects/{name}/agents/{name}/builds"
        deadline = time.monotonic() + _BUILD_TIMEOUT_S
        build_name = None
        # Step 1: wait for a build to appear.
        while time.monotonic() < deadline:
            builds = self._request("GET", base, expect=(200,)).json().get("builds", [])
            if builds:
                build_name = builds[-1]["buildName"]
                break
            time.sleep(_BUILD_POLL_S)
        if not build_name:
            raise TimeoutError(f"no build appeared for {name} within {_BUILD_TIMEOUT_S}s")
        # Step 2: poll the build until Completed.
        while time.monotonic() < deadline:
            detail = self._request("GET", f"{base}/{build_name}", expect=(200,)).json()
            status = detail.get("status")
            if status == "Completed":
                image_id = detail.get("imageId")
                if not image_id:
                    raise AmpError(f"build {build_name} completed without an imageId")
                return image_id
            if status == "Failed":
                raise AmpError(f"build {build_name} failed")
            time.sleep(_BUILD_POLL_S)
        raise TimeoutError(f"build {build_name} did not complete within {_BUILD_TIMEOUT_S}s")

    def _wait_for_active_endpoint(self, org: str, name: str) -> str:
        # FIRST-RUN-TUNABLE: reads the endpoint URL from the deployments
        # response (`endpoints[].url`). The e2e suite instead reads it from
        # GET .../endpoints?environment=<env> — if the deployments payload
        # turns out not to carry the URL, switch to that endpoint here.
        path = f"/api/v1/orgs/{org}/projects/{name}/agents/{name}/deployments"
        deadline = time.monotonic() + _DEPLOY_TIMEOUT_S
        last_seen = "<no deployments response>"
        failed_streak = 0
        while time.monotonic() < deadline:
            deployments = self._request("GET", path, expect=(200,)).json()
            dep = deployments.get(self.environment)
            if dep is None:
                last_seen = f"env {self.environment!r} absent; present={list(deployments)}"
                failed_streak = 0
            else:
                status = dep.get("status")
                last_seen = f"status={status!r}"
                if status == "active":
                    endpoints = dep.get("endpoints") or []
                    if endpoints:
                        return endpoints[0]["url"]
                    raise AmpError(f"{name} active but exposes no endpoint")
                # Tolerate a transient failed during reconciliation (the e2e
                # suite polls through non-active states); bail only once it
                # persists, so a genuinely stuck deploy fails fast.
                if status == "failed":
                    failed_streak += 1
                    if failed_streak >= _DEPLOY_FAILED_STREAK:
                        raise AmpError(f"{name} deployment failed in {self.environment}")
                else:
                    failed_streak = 0
            time.sleep(_DEPLOY_POLL_S)
        raise TimeoutError(
            f"{name} not active in {self.environment} within {_DEPLOY_TIMEOUT_S}s "
            f"(last: {last_seen})"
        )

    def _mint_api_key(self, org: str, name: str) -> str:
        # expiresAt must be a concrete RFC3339 timestamp (the e2e suite sends
        # now+24h); an empty string can be rejected by validation.
        expires_at = _utc_rfc3339(hours_from_now=24)
        resp = self._request(
            "POST",
            f"/api/v1/orgs/{org}/projects/{name}/agents/{name}"
            f"/environments/{self.environment}/api-keys",
            json={"displayName": f"matrix-{name}", "expiresAt": expires_at},
            expect=(201,),
        )
        return resp.json()["apiKey"]

    def teardown_agent(self, deployed: DeployedAgent) -> None:
        """Best-effort delete of the agent + project. Never raises — teardown
        runs in a finally block and a failure here shouldn't mask the cell's
        actual result."""
        for path in (
            f"/api/v1/orgs/{deployed.org}/projects/{deployed.project_name}"
            f"/agents/{deployed.agent_name}",
            f"/api/v1/orgs/{deployed.org}/projects/{deployed.project_name}",
        ):
            try:
                self._request("DELETE", path, expect=(200, 202, 204, 404))
            except Exception:  # noqa: BLE001 - teardown is best-effort
                pass
