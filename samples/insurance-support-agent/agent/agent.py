"""Strands agent construction."""

from __future__ import annotations

from strands import Agent
from strands.models.openai import OpenAIModel
from strands.models import BedrockModel
import boto3
from botocore import UNSIGNED
from botocore.config import Config as BotoConfig

from config import Config
from system_prompt import SYSTEM_PROMPT
from tools import (
    file_claim,
    get_claim_status,
    list_claims,
    list_policies,
    lookup_policy,
)


def build_model(cfg: Config):
    if cfg.model_provider == "bedrock-gateway":
        # Only the gateway holds the upstream Bedrock credential.
        model = BedrockModel(
            boto_session=boto3.Session(
                aws_access_key_id="unused",
                aws_secret_access_key="unused",
                region_name=cfg.bedrock_region,
            ),
            boto_client_config=BotoConfig(
                signature_version=UNSIGNED,
                connect_timeout=10,
                read_timeout=60,
                retries={"total_max_attempts": 2, "mode": "standard"},
            ),
            endpoint_url=cfg.llm_gateway_url,
            model_id=cfg.bedrock_model_id,
            streaming=False,
            max_tokens=600,
            temperature=0,
        )

        def authenticate_gateway(request, **kwargs):
            # Avoid forwarding ambient AWS bearer credentials to the proxy.
            for header in ("Authorization", "X-Amz-Security-Token"):
                if header in request.headers:
                    del request.headers[header]
            request.headers["api-key"] = cfg.llm_gateway_api_key

        model.client.meta.events.register(
            "before-send.bedrock-runtime.*", authenticate_gateway
        )
        return model

    client_args: dict[str, str] = {"api_key": cfg.openai_api_key}
    if cfg.openai_base_url:
        client_args["base_url"] = cfg.openai_base_url

    return OpenAIModel(client_args=client_args, model_id=cfg.openai_model)


def build_agent(cfg: Config, mcp_tools=None) -> Agent:
    model = build_model(cfg)

    if cfg.use_mcp and not mcp_tools:
        raise RuntimeError(
            "MCP policy tools are unavailable; local fallback is disabled"
        )
    prompt = SYSTEM_PROMPT.format(company=cfg.company_name)
    if cfg.use_mcp:
        prompt = (
            f"You are a read-only policy support agent for {cfg.company_name}. "
            "Use list_policies and lookup_policy for all policy facts. "
            "You cannot access claims, file claims, or change records. "
            "If a tool is denied or unavailable, say so without guessing. "
            "Use only the current tool result for policy facts, not earlier messages. "
            "Keep responses brief and refer advice or complaints to a human."
        )

    return Agent(
        model=model,
        tools=mcp_tools
        if cfg.use_mcp
        else [
            list_policies,
            list_claims,
            lookup_policy,
            get_claim_status,
            file_claim,
        ],
        system_prompt=prompt,
        callback_handler=None,
    )
