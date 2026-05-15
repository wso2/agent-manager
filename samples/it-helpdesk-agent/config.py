"""Instance-level configuration, read from env at startup.

Set ``USE_LLM_PROVIDER=true`` to route through the AM LLM provider
(with guardrails) using ``LLM_PROVIDER_URL`` and ``LLM_PROVIDER_KEY``.
By default (``false``), calls OpenAI directly using ``OPENAI_API_KEY``.
"""

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
    use_llm_provider: bool
    llm_provider_url: str
    llm_provider_key: str

    @classmethod
    def from_env(cls) -> "Config":
        return cls(
            company_name=_env("COMPANY_NAME", "AcmeCorp"),
            tone=_env("TONE", "professional and helpful"),
            max_tickets_per_query=int(_env("MAX_TICKETS_PER_QUERY", "20")),
            additional_guidance=_env("ADDITIONAL_GUIDANCE", ""),
            use_llm_provider=_env("USE_LLM_PROVIDER", "false").lower() == "true",
            llm_provider_url=_env("LLM_PROVIDER_URL", ""),
            llm_provider_key=_env("LLM_PROVIDER_KEY", ""),
        )
