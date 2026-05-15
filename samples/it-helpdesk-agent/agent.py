"""LangGraph IT helpdesk agent construction.

Builds a ReAct-style agent bound to the instance config. When
``LLM_PROVIDER=agent-manager``, requests are routed through the AM LLM
provider (which applies guardrails). Otherwise calls OpenAI directly.
"""

from __future__ import annotations

from typing import Any

from langchain_openai import ChatOpenAI
from langgraph.prebuilt import create_react_agent

from config import Config
from tools import build_tools

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
    "- Escalate complex issues to L2 support\n\n"
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
    "rather than guessing.\n\n"
    "Tone: {tone}. {additional_guidance}"
)


def build_agent(cfg: Config) -> Any:
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
    tools = build_tools(cfg)
    system_prompt = SYSTEM_PROMPT_TEMPLATE.format(
        company_name=cfg.company_name,
        tone=cfg.tone,
        additional_guidance=cfg.additional_guidance,
    )
    return create_react_agent(model=llm, tools=tools, prompt=system_prompt)
