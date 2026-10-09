"""AgentID client-credentials authentication for the insurance MCP proxy."""

from contextlib import asynccontextmanager

import math
import time
from urllib.parse import quote_plus

import anyio
import httpx
from strands.tools.mcp import MCPClient
from mcp.client.streamable_http import streamable_http_client

from config import Config


class AgentIDAuth(httpx.Auth):
    """Cache a resource-bound token and renew it before expiry. Never retry denied tools."""

    def __init__(self, cfg: Config):
        self.cfg = cfg
        self._token = ""
        self._expires_at = 0.0
        self._lock = anyio.Lock()

    async def async_auth_flow(self, request):
        # Do not attach bearer credentials to redirects or a different resource.
        if str(request.url) != self.cfg.insurance_mcp_url:
            raise RuntimeError("Unexpected MCP request URL")
        async with self._lock:
            if time.monotonic() >= self._expires_at:
                token_request = httpx.Request(
                    "POST",
                    self.cfg.agentid_token_endpoint,
                    data={
                        "grant_type": "client_credentials",
                        "scope": self.cfg.agentid_scopes,
                        "resource": self.cfg.insurance_mcp_url,
                    },
                )
                # RFC 6749 client_secret_basic: encode each component before Basic.
                basic = httpx.BasicAuth(
                    quote_plus(self.cfg.agentid_client_id, safe=""),
                    quote_plus(self.cfg.agentid_client_secret, safe=""),
                )
                token_request = next(basic.auth_flow(token_request))
                response = yield token_request
                await response.aread()
                if response.status_code != 200:
                    raise RuntimeError("AgentID token request failed")
                try:
                    payload = response.json()
                    token = payload["access_token"]
                    lifetime = float(payload["expires_in"])
                    if (
                        not isinstance(token, str)
                        or not token
                        or not isinstance(payload.get("token_type"), str)
                        or payload["token_type"].lower() != "bearer"
                        or not math.isfinite(lifetime)
                        or lifetime <= 0
                    ):
                        raise ValueError("Invalid token response")
                except (KeyError, ValueError, TypeError):
                    raise RuntimeError("Invalid AgentID token response") from None
                self._token = token
                self._expires_at = time.monotonic() + lifetime - min(30, lifetime / 10)
            request.headers["Authorization"] = f"Bearer {self._token}"
        response = yield request
        if response.status_code == 401:
            # A later request may acquire a new token; never replay the tool call.
            self._expires_at = 0.0


def create_mcp_client(cfg: Config) -> MCPClient:
    @asynccontextmanager
    async def transport():
        # Never follow redirects while carrying client or bearer credentials.
        async with httpx.AsyncClient(
            auth=AgentIDAuth(cfg),
            follow_redirects=False,
            timeout=httpx.Timeout(30, connect=10),
        ) as http:
            async with streamable_http_client(
                cfg.insurance_mcp_url, http_client=http
            ) as streams:
                yield streams

    return MCPClient(
        transport, tool_filters={"allowed": ["list_policies", "lookup_policy"]}
    )
