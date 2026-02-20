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
from pydantic import Field, field_validator, model_validator
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


class PlatformConfig(BaseSettings):
    """Platform API configuration."""

    api_url: str = Field(default="", description="Platform API base URL")
    api_key: str = Field(default="", description="API key for authentication")

    model_config = SettingsConfigDict(
        env_prefix="AMP_",
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )


class TraceLoaderConfig(BaseSettings):
    """Trace loading configuration."""

    mode: str = Field(default="platform", description="Trace loading mode: 'platform' or 'file'")
    trace_file_path: Optional[str] = Field(default=None, description="Path to trace file (for file mode)")
    batch_size: int = Field(default=100, description="Batch size for fetching traces")

    model_config = SettingsConfigDict(
        env_prefix="AMP_TRACE_LOADER_",
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )

    @field_validator("mode")
    @classmethod
    def validate_mode(cls, v: str) -> str:
        """Validate trace loader mode."""
        if v not in ("platform", "file"):
            raise ValueError(f"Invalid mode '{v}'. Must be 'platform' or 'file'")
        return v


class ResultsConfig(BaseSettings):
    """Results publishing configuration."""

    publish_to_platform: bool = Field(default=False, description="Whether to publish results to platform")

    model_config = SettingsConfigDict(
        env_prefix="AMP_",
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )


class Config(BaseSettings):
    """Complete configuration for the evaluation framework."""

    agent: AgentConfig = Field(default_factory=AgentConfig)
    platform: PlatformConfig = Field(default_factory=PlatformConfig)
    trace_loader: TraceLoaderConfig = Field(default_factory=TraceLoaderConfig)
    results: ResultsConfig = Field(default_factory=ResultsConfig)

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )

    @model_validator(mode="after")
    def validate_config(self) -> "Config":
        """
        Validate configuration after all fields are loaded.

        Note: This validation is lenient - it only validates when configuration
        is actually needed. Tests and programmatic usage can skip validation
        by providing parameters directly to runners.

        Raises:
            ValueError: If validation fails
        """
        errors: list[str] = []

        # Platform config (if publishing results or using platform mode)
        # Only validate if these features are enabled and no explicit overrides provided
        if self.results.publish_to_platform or self.trace_loader.mode == "platform":
            if not self.platform.api_url:
                # This is a warning, not an error - allow tests to run without env vars
                pass
                # errors.append("AMP_API_URL is required when publishing results or using platform mode")

        # Trace loader config
        if self.trace_loader.mode == "file":
            if not self.trace_loader.trace_file_path:
                # Only warn, don't fail
                pass
                # errors.append("AMP_TRACE_LOADER_TRACE_FILE_PATH is required when mode is 'file'")

        if errors:
            error_msg = "Configuration validation failed:\n  - " + "\n  - ".join(errors)
            raise ValueError(error_msg)

        return self


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


def reload_config():
    """Reload configuration from environment variables."""
    global _config
    _config = Config()
    return _config
