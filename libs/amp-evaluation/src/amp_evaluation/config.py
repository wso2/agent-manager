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
Loads configuration from environment variables.
"""

import os
from typing import Optional
from dataclasses import dataclass


@dataclass
class AgentConfig:
    """Agent configuration loaded from environment."""

    agent_uid: str
    environment_uid: str

    @classmethod
    def from_env(cls) -> "AgentConfig":
        """Load agent config from environment variables."""
        return cls(agent_uid=os.getenv("AGENT_UID", ""), environment_uid=os.getenv("ENVIRONMENT_UID", ""))


@dataclass
class PlatformConfig:
    """Platform API configuration."""

    api_url: str
    api_key: str

    @classmethod
    def from_env(cls) -> "PlatformConfig":
        """Load platform config from environment variables."""
        return cls(api_url=os.getenv("AMP_API_URL", ""), api_key=os.getenv("AMP_API_KEY", ""))


@dataclass
class TraceLoaderConfig:
    """Trace loading configuration."""

    mode: str  # "platform" | "file"
    trace_file_path: Optional[str] = None

    @classmethod
    def from_env(cls) -> "TraceLoaderConfig":
        """Load trace loader config from environment variables."""
        return cls(mode=os.getenv("TRACE_LOADER_MODE", "platform"), trace_file_path=os.getenv("TRACE_FILE_PATH"))


@dataclass
class ResultsConfig:
    """Results publishing configuration."""

    publish_to_platform: bool = True

    @classmethod
    def from_env(cls) -> "ResultsConfig":
        """Load results config from environment variables."""
        return cls(publish_to_platform=os.getenv("PUBLISH_RESULTS", "true").lower() == "true")


@dataclass
class Config:
    """Complete configuration for the evaluation framework."""

    agent: AgentConfig
    platform: PlatformConfig
    trace_loader: TraceLoaderConfig
    results: ResultsConfig

    @classmethod
    def from_env(cls) -> "Config":
        """
        Load complete configuration from environment variables.

        Raises:
            ValueError: If required configuration is missing

        Usage:
            from amp_evaluation.config import Config

            config = Config.from_env()
            print(f"Agent: {config.agent.agent_uid}")
            print(f"Platform: {config.platform.api_url}")
        """
        config = cls(
            agent=AgentConfig.from_env(),
            platform=PlatformConfig.from_env(),
            trace_loader=TraceLoaderConfig.from_env(),
            results=ResultsConfig.from_env(),
        )

        # Validate configuration
        errors = config.validate()
        if errors:
            error_msg = "Configuration validation failed:\n  - " + "\n  - ".join(errors)
            raise ValueError(error_msg)

        return config

    def validate(self) -> list[str]:
        """
        Validate configuration and return list of errors.

        Returns:
            List of validation error messages (empty if valid)
        """
        errors = []

        # Required fields
        if not self.agent.agent_uid:
            errors.append("AGENT_UID is required")
        if not self.agent.environment_uid:
            errors.append("ENVIRONMENT_UID is required")

        # Platform config (if publishing results or using platform mode)
        if self.results.publish_to_platform or self.trace_loader.mode == "platform":
            if not self.platform.api_url:
                errors.append("AMP_API_URL is required when PUBLISH_RESULTS=true or TRACE_LOADER_MODE=platform")
            if not self.platform.api_key:
                errors.append("AMP_API_KEY is required when PUBLISH_RESULTS=true or TRACE_LOADER_MODE=platform")

        # Trace loader config
        if self.trace_loader.mode == "file":
            if not self.trace_loader.trace_file_path:
                errors.append("TRACE_FILE_PATH is required when TRACE_LOADER_MODE=file")

        return errors


# Global config instance (lazy loaded)
_config: Optional[Config] = None


def get_config() -> Config:
    """
    Get the global configuration instance.

    Usage:
        from amp_evaluation.config import get_config

        config = get_config()
        print(f"Agent: {config.agent.agent_uid}")
        print(f"Environment: {config.agent.environment_uid}")
    """
    global _config
    if _config is None:
        _config = Config.from_env()
    return _config


def reload_config():
    """Reload configuration from environment variables."""
    global _config
    _config = Config.from_env()
    return _config
