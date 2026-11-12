"""Client for agent-manager-service REST API — used by the heavy-tier driver
and the sample smoke tests to register external agents against a running
Agent Manager and read their traces back.

The flow mirrors the Go e2e suite (`test/e2e/framework/` +
`test/e2e/operations/`), which is the authoritative reference for the real
endpoints and payloads:
  1. OAuth2 client_credentials grant against Thunder IDP        (auth.go)
  2. POST create project                                        (project/create_project.go)
  3. POST create external agent                                 (agent/create_agent.go)
  4. POST mint the agent's trace token                          (agent/generate_agent_token.go)
"""
from __future__ import annotations

import hashlib
import time
from dataclasses import dataclass

import requests

_HTTP_TIMEOUT_S = 30
_TOKEN_REFRESH_SKEW_S = 30

# OAuth2 scopes requested in the client_credentials grant. The service authorizes
# every route against the token's scopes, and Thunder only
# puts a scope in a client_credentials token when it is *explicitly requested*
# (and the app is registered for it + assigned a role that grants it — both done
# for amp-api-client in the Thunder bootstrap). Omitting `scope` yields a token
# with no scope claim, so every call 403s. Request exactly the permissions this
# heavy-tier flow exercises: provisioning (create project/agent, mint the
# agent's trace token, teardown deletes) plus reading the emitted
# spans back from the observer (heavy/observer.py reuses this same token for
# GET /api/v1/traces[/…], which the observer wraps in RequirePermission(
# TraceRead) — so amp:observability:trace-read is
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
    "amp:agent:create amp:agent:read amp:agent:delete amp:agent:token-manage "
    "amp:observability:trace-read"
)

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


@dataclass
class IdpCredentials:
    """Thunder OAuth2 client_credentials grant inputs."""

    token_url: str  # e.g. http://thunder.amp.localhost:8080/oauth2/token
    client_id: str  # e.g. amp-api-client
    client_secret: str


@dataclass
class DeployedAgent:
    """An agent registered for a run; the observer is queried by these names."""
    org: str
    project_name: str
    agent_name: str
    environment: str
    endpoint_url: str = ""
    api_key: str = ""


class AmpError(RuntimeError):
    """Raised when an AMP API call returns an unexpected status."""


class AmpClient:
    """REST client for agent-manager-service.

    Authenticates against Thunder IDP via OAuth2 `client_credentials` and
    attaches `Authorization: Bearer <token>` to every call, refreshing the
    token proactively before expiry.
    """

    def __init__(self, base_url: str, idp: IdpCredentials, *, org: str = "default",
                 environment: str = "default"):
        self.base_url = base_url.rstrip("/")
        self.idp = idp
        self.org = org
        self.environment = environment
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

    def register_external_agent(self, org: str, name: str) -> str:
        """Create project `name` with an external agent of the same name and
        return the agent's trace token (sent as AMP_AGENT_API_KEY).

        An external agent runs wherever its owner runs it; Agent Manager only
        issues the token its spans are accepted with at the AI Gateway."""
        self._request(
            "POST", f"/api/v1/orgs/{org}/projects",
            json={"name": name, "displayName": name}, expect=(202,),
        )
        self._request(
            "POST", f"/api/v1/orgs/{org}/projects/{name}/agents",
            json={
                "name": name, "displayName": name,
                "provisioning": {"type": "external"},
                "agentType": {"type": "external-agent-api", "subType": "custom-api"},
            },
            expect=(202,),
        )
        resp = self._request(
            "POST", f"/api/v1/orgs/{org}/projects/{name}/agents/{name}/token",
            json={"expires_in": "24h"}, expect=(200,),
        )
        return resp.json()["token"]

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
