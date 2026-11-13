from __future__ import annotations

import os
from dataclasses import dataclass


def _env(name: str, default: str | None = None) -> str:
    val = os.environ.get(name, default)
    if val is None:
        raise RuntimeError(f"Missing required env var: {name}")
    return val


@dataclass(frozen=True)
class Config:
    company_name: str
    tone: str
    max_tickets_per_query: int
    additional_guidance: str
    agent_version: str
    use_llm_provider: bool
    llm_provider_url: str
    llm_provider_key: str
    use_mcp: bool
    mcp_url: str
    mcp_api_key: str
    mcp_oauth: bool
    agentid_client_id: str
    agentid_client_secret: str
    agentid_token_endpoint: str
    agentid_scopes: str
    issue_tracker_repo: str

    @property
    def agentid_ready(self) -> bool:
        return bool(
            self.agentid_client_id
            and self.agentid_client_secret
            and self.agentid_token_endpoint
        )

    @classmethod
    def from_env(cls) -> "Config":
        raw_max_tickets = _env("MAX_TICKETS_PER_QUERY", "20")
        try:
            max_tickets = int(raw_max_tickets)
        except ValueError:
            raise RuntimeError(
                f"MAX_TICKETS_PER_QUERY must be an integer, got: {raw_max_tickets!r}"
            ) from None

        use_llm_provider = _env("USE_LLM_PROVIDER", "false").lower() == "true"
        llm_provider_url = _env("LLM_PROVIDER_URL", "")
        llm_provider_key = _env("LLM_PROVIDER_KEY", "")

        if use_llm_provider:
            if not llm_provider_url:
                raise RuntimeError(
                    "USE_LLM_PROVIDER is true but LLM_PROVIDER_URL is not set"
                )
            if not llm_provider_key:
                raise RuntimeError(
                    "USE_LLM_PROVIDER is true but LLM_PROVIDER_KEY is not set"
                )


        use_mcp = _env("USE_MCP", "false").lower() == "true"
        mcp_url = _env("GITHUB_MCP_URL", "")
        mcp_api_key = _env("GITHUB_MCP_API_KEY", "")

        # With MCP_OAUTH=true the proxy is reached with an AgentID bearer token
        # instead of an API key — for a proxy whose Security tab is set to OAuth.
        # The AMP_AGENTID_* vars come from the agent's Agent ID page in the
        # console; they are not validated here because provisioning finishes
        # asynchronously after registration.
        mcp_oauth = _env("MCP_OAUTH", "false").lower() == "true"

        # Which repository holds the IT team's known-issue tracker. Without this the
        # agent would search issues across the whole of GitHub, which is both slow
        # and wrong — a match in someone else's project is not a known issue here.
        issue_tracker_repo = _env("ISSUE_TRACKER_REPO", "")

        if use_mcp:
            if not mcp_url:
                raise RuntimeError("USE_MCP is true but GITHUB_MCP_URL is not set")
            if not mcp_oauth and not mcp_api_key:
                raise RuntimeError("USE_MCP is true but GITHUB_MCP_API_KEY is not set")
            if not issue_tracker_repo:
                raise RuntimeError(
                    "USE_MCP is true but ISSUE_TRACKER_REPO is not set "
                    "(expected owner/repo, e.g. acme/it-tooling)"
                )
            if "/" not in issue_tracker_repo:
                raise RuntimeError(
                    f"ISSUE_TRACKER_REPO must be owner/repo, got: {issue_tracker_repo!r}"
                )

        return cls(
            company_name=_env("COMPANY_NAME", "AcmeCorp"),
            tone=_env("TONE", "professional and helpful"),
            max_tickets_per_query=max_tickets,
            additional_guidance=_env("ADDITIONAL_GUIDANCE", ""),
            # Echoed in /health and every chat response so a rollout or a
            # rollback is visible from the outside without reading the console.
            agent_version=_env("AGENT_VERSION", "dev"),
            use_llm_provider=use_llm_provider,
            llm_provider_url=llm_provider_url,
            llm_provider_key=llm_provider_key,
            use_mcp=use_mcp,
            mcp_url=mcp_url,
            mcp_api_key=mcp_api_key,
            mcp_oauth=mcp_oauth,
            agentid_client_id=_env("AMP_AGENTID_CLIENT_ID", ""),
            agentid_client_secret=_env("AMP_AGENTID_CLIENT_SECRET", ""),
            agentid_token_endpoint=_env("AMP_AGENTID_TOKEN_ENDPOINT", ""),
            agentid_scopes=_env("AMP_AGENTID_SCOPES", ""),
            issue_tracker_repo=issue_tracker_repo,
        )
