# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

"""
Configuration loader for the evaluation framework.
Loads configuration from environment variables using Pydantic Settings.
"""

from typing import Optional
from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class AgentConfig(BaseSettings):
    """Agent configuration loaded from environment."""

    agent_uid: str = Field(default="", description="Unique identifier for the agent")
    environment_uid: str = Field(default="", description="Unique identifier for the environment")

    model_config = SettingsConfigDict(
        env_prefix="AMP_",
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )


class TraceConfig(BaseSettings):
    """
    Trace source configuration.

    The trace source is auto-detected from which fields are set:
      - api_url set  → TraceFetcher (live traces from platform API)
      - file_path set → TraceLoader (traces from a local JSON file)
      - neither       → error at runtime when a runner tries to fetch traces

    Environment variables:
      AMP_TRACE_API_URL   - Platform API base URL
      AMP_TRACE_API_KEY   - API key for authentication (optional)
      AMP_TRACE_FILE_PATH - Path to trace JSON file
    """

    api_url: str = Field(default="", description="Platform API base URL for fetching live traces")
    api_key: str = Field(default="", description="API key for authentication (optional)")
    file_path: Optional[str] = Field(default=None, description="Path to local trace JSON file")

    model_config = SettingsConfigDict(
        env_prefix="AMP_TRACE_",
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )


class LLMJudgeConfig(BaseSettings):
    """
    Global defaults for LLM-as-judge evaluators.

    Sets the default LLM model used by all built-in LLM judge evaluators.
    Individual evaluators can always override this via the model= kwarg:

        builtin("helpfulness", model="gpt-4o")  # overrides global default

    Override priority (highest to lowest):
        1. Per-evaluator kwarg:   builtin("safety", model="gpt-4o")
        2. Environment variable:  AMP_LLM_JUDGE_DEFAULT_MODEL=gpt-4o
        3. Framework default:     gpt-4o-mini
    """

    default_model: str = Field(
        default="gpt-4o-mini",
        description="Default LLM model for all LLM-as-judge evaluators (overridable per-evaluator)",
    )

    model_config = SettingsConfigDict(
        env_prefix="AMP_LLM_JUDGE_",
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )


class Config(BaseSettings):
    """Complete configuration for the evaluation framework."""

    agent: AgentConfig = Field(default_factory=AgentConfig)
    trace: TraceConfig = Field(default_factory=TraceConfig)
    llm_judge: LLMJudgeConfig = Field(default_factory=LLMJudgeConfig)

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )


# Global config instance (lazy loaded)
_config: Optional[Config] = None


def get_config() -> Config:
    """
    Get the global configuration instance.

    Automatically loads from environment variables and .env file.

    Usage:
        from amp_evaluation.config import get_config

        config = get_config()
        print(f"Agent: {config.agent.agent_uid}")
        print(f"Environment: {config.agent.environment_uid}")
    """
    global _config
    if _config is None:
        _config = Config()
    return _config


def reload_config() -> Config:
    """Reload configuration from environment variables."""
    global _config
    _config = Config()
    return _config
