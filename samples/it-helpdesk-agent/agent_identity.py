"""AgentID bearer tokens for OAuth-secured MCP proxies.

The four ``AMP_AGENTID_*`` env vars hold the agent's AgentID client credentials
(see ``config.py``). When ``MCP_OAUTH=true``, this module turns them
into an OAuth 2.0 ``client_credentials`` access token and attaches it to every
MCP request as ``Authorization: Bearer``, in place of the proxy API key.
"""

from __future__ import annotations

import asyncio
import logging
import time
from collections.abc import AsyncGenerator

import httpx

from config import Config

log = logging.getLogger("it-helpdesk")


class AgentIdentityNotProvisionedError(Exception):
    """AgentID has not finished provisioning for this agent yet."""


class AgentIdentityAuth(httpx.Auth):
    """Attaches a cached AgentID token to every request, minting a new one when
    the cached token is close to expiry.

    The token is requested with the proxy URL as its ``resource`` (RFC 8707), so
    it is valid for that one proxy only. Use one instance per proxy.
    """

    def __init__(self, cfg: Config, resource: str) -> None:
        self._cfg = cfg
        self._resource = resource
        self._token: str | None = None
        self._expiry = 0.0
        # Coalesces concurrent refreshes onto a single token-endpoint call.
        self._lock = asyncio.Lock()

    async def async_auth_flow(
        self, request: httpx.Request
    ) -> AsyncGenerator[httpx.Request, httpx.Response]:
        request.headers["Authorization"] = f"Bearer {await self._get_token()}"
        yield request

    async def _get_token(self) -> str:
        async with self._lock:
            if self._token and time.monotonic() < self._expiry:
                return self._token

            if not self._cfg.agentid_ready:
                raise AgentIdentityNotProvisionedError(
                    "AMP_AGENTID_CLIENT_ID / AMP_AGENTID_CLIENT_SECRET / "
                    "AMP_AGENTID_TOKEN_ENDPOINT are not set"
                )

            async with httpx.AsyncClient(timeout=10) as client:
                response = await client.post(
                    self._cfg.agentid_token_endpoint,
                    data={
                        "grant_type": "client_credentials",
                        "scope": self._cfg.agentid_scopes,
                        "resource": self._resource,
                    },
                    auth=(self._cfg.agentid_client_id, self._cfg.agentid_client_secret),
                )
                response.raise_for_status()
                token_data = response.json()

            self._token = token_data["access_token"]
            # Refresh at 75% of the lifetime so a request never goes out on a
            # token that expires mid-flight.
            self._expiry = time.monotonic() + token_data["expires_in"] * 0.75
            log.info("Minted AgentID access token (expires_in=%s)", token_data["expires_in"])
            return self._token
