"""Instance-level configuration, read from env at startup.

All fields have sensible defaults — only ``OPENAI_API_KEY`` is truly required
(and that is read by ``ChatOpenAI`` directly from the environment).
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

    @classmethod
    def from_env(cls) -> "Config":
        return cls(
            company_name=_env("COMPANY_NAME", "AcmeCorp"),
            tone=_env("TONE", "professional and helpful"),
            max_tickets_per_query=int(_env("MAX_TICKETS_PER_QUERY", "20")),
            additional_guidance=_env("ADDITIONAL_GUIDANCE", ""),
        )
