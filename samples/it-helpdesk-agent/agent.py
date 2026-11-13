"""LangGraph IT helpdesk agent construction.

Builds a ReAct-style agent bound to the instance config.

When ``USE_LLM_PROVIDER=true``, requests are routed through the AM LLM
provider (which applies guardrails). Otherwise calls OpenAI directly.

When ``USE_MCP=true``, tools discovered from an AM MCP proxy are merged with
the in-process tools. When it is off, the agent is exactly the v1 agent.
``MCP_OAUTH=true`` authenticates to that proxy with the agent's AgentID
(OAuth 2.0 bearer token) instead of an API key.
"""

from __future__ import annotations

import logging
from typing import Any

from langchain_openai import ChatOpenAI
from langgraph.checkpoint.memory import InMemorySaver
from langgraph.prebuilt import create_react_agent

from agent_identity import AgentIdentityAuth
from config import Config
from tools import build_tools

log = logging.getLogger("it-helpdesk")

MODEL = "gpt-4o-mini"

SYSTEM_PROMPT_TEMPLATE = (
    "You are an IT helpdesk agent for {company_name}. "
    "You provide L1 technical support to employees.\n\n"
    "CAPABILITIES:\n"
    "- Look up employees and verify their identity\n"
    "- Check and create IT support tickets\n"
    "- Reset passwords (non-admin accounts only, after identity verification)\n"
    "- Request software access based on department eligibility\n"
    "- Check system status for outages and maintenance\n"
    "- Search IT policies\n"
    "- Escalate complex issues to L2 support\n"
    "{mcp_capabilities}"
    "\n"
    "RULES YOU MUST FOLLOW:\n"
    "1. IDENTITY FIRST: Before any write action (password reset, software access, "
    "ticket creation), verify the employee's identity using verify_identity. "
    "They must provide both their email and employee ID.\n"
    "2. CHECK BEFORE CREATE: Before creating a ticket, check system_status for "
    "known outages and get_open_tickets for duplicates.\n"
    "3. ADMIN ACCOUNTS: Never reset passwords for admin accounts (is_admin=true). "
    "Always escalate these to L2.\n"
    "4. POLICY CITATION: Search and cite the relevant IT policy before denying a "
    "request or performing a sensitive action.\n"
    "5. PRIVACY: Never disclose another employee's tickets, access, or personal info. "
    "Only show data belonging to the verified requester.\n"
    "6. ESCALATE WHEN UNSURE: If you cannot resolve an issue safely, escalate to L2 "
    "rather than guessing.\n"
    "{mcp_rules}"
    "\n"
    "Tone: {tone}. {additional_guidance}"
)

# Appended to the prompt only when MCP tools are loaded, so the base agent's
# behaviour — and the evaluators written against it — stay unchanged when the
# toggle is off.
MCP_CAPABILITIES = (
    "- Search the IT team's issue tracker for known problems that match what an "
    "employee is reporting\n"
)

# {issue_tracker_repo} is substituted at build time. Naming the repository in the
# prompt is what keeps searches scoped: without it the agent would search issues
# across all of GitHub, and a match in an unrelated project is not a known issue
# at AcmeCorp.
MCP_RULES = (
    "7. CHECK KNOWN ISSUES FIRST: When an employee reports something broken, "
    "search the IT team's issue tracker before creating a ticket, in addition to "
    "checking system_status and their open tickets (rule 2). The tracker is the "
    "repository {issue_tracker_repo} — always scope issue searches to it, for "
    "example by including 'repo:{issue_tracker_repo}' in the search query. Never "
    "report an issue from any other repository as a known issue. If a matching "
    "known issue exists, tell the employee its number and any workaround it "
    "documents instead of opening a duplicate ticket.\n"
    "8. THE ISSUE TRACKER IS READ-ONLY: You may search and read issues. Never "
    "create, comment on, edit, close, or reopen one — that is the engineering "
    "team's call, not L1's. If an issue needs changing, escalate to L2.\n"
)


async def load_mcp_tools(cfg: Config) -> list[Any]:
    """Discover tools from the AM MCP proxy.

    By default the proxy is reached with the Agent Manager-issued key in an
    ``X-API-Key`` header — the default for MCP proxies, and *not* the ``API-Key``
    the LLM provider uses. Each proxy's header name is a Security setting, so
    check yours if the gateway answers 401.

    With ``MCP_OAUTH=true`` (the proxy's Security tab set to OAuth) no API key is
    sent. Instead the agent's AgentID credentials mint a bearer token scoped to
    the proxy URL, refreshed as it nears expiry.

    Either way the agent never holds the upstream GitHub credential; the gateway
    attaches it on the way out.
    """
    if not cfg.use_mcp:
        return []

    from langchain_mcp_adapters.client import MultiServerMCPClient

    connection: dict[str, Any] = {
        "url": cfg.mcp_url,
        "transport": "streamable_http",
    }
    if cfg.mcp_oauth:
        if not cfg.agentid_ready:
            # AgentID provisioning finishes shortly after registration, so
            # start without MCP tools rather than crash until it lands.
            log.warning(
                "MCP_OAUTH is true but AgentID is not provisioned yet "
                "(AMP_AGENTID_* unset); starting without MCP tools"
            )
            return []
        connection["auth"] = AgentIdentityAuth(cfg, resource=cfg.mcp_url)
    else:
        connection["headers"] = {"X-API-Key": cfg.mcp_api_key}

    client = MultiServerMCPClient({"github": connection})
    return list(await client.get_tools())


def build_agent(cfg: Config, mcp_tools: list[Any] | None = None) -> Any:
    if cfg.use_llm_provider:
        llm = ChatOpenAI(
            model=MODEL,
            temperature=0,
            base_url=cfg.llm_provider_url,
            api_key="not-used",
            default_headers={
                "API-Key": cfg.llm_provider_key,
                "Authorization": "",
            },
        )
    else:
        llm = ChatOpenAI(model=MODEL, temperature=0)

    tools = build_tools(cfg) + list(mcp_tools or [])
    system_prompt = SYSTEM_PROMPT_TEMPLATE.format(
        company_name=cfg.company_name,
        tone=cfg.tone,
        additional_guidance=cfg.additional_guidance,
        mcp_capabilities=MCP_CAPABILITIES if mcp_tools else "",
        mcp_rules=(
            MCP_RULES.format(issue_tracker_repo=cfg.issue_tracker_repo)
            if mcp_tools
            else ""
        ),
    )

    # Conversation state keyed on thread_id (the chat session_id). Without this
    # every turn arrives as a fresh message list, so "verify me, then reset my
    # password" cannot work across two turns.
    return create_react_agent(
        model=llm,
        tools=tools,
        prompt=system_prompt,
        checkpointer=InMemorySaver(),
    )
