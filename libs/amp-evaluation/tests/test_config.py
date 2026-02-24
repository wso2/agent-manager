"""
Tests for configuration management using Pydantic Settings.
"""

import os
from amp_evaluation.config import (
    AgentConfig,
    TraceConfig,
    LLMJudgeConfig,
    Config,
    get_config,
    reload_config,
)


class TestAgentConfig:
    """Test AgentConfig loading and validation."""

    def test_default_values(self, monkeypatch):
        """Test that AgentConfig has sensible defaults."""
        monkeypatch.delenv("AMP_AGENT_UID", raising=False)
        monkeypatch.delenv("AMP_ENVIRONMENT_UID", raising=False)

        config = AgentConfig()
        assert config.agent_uid == ""
        assert config.environment_uid == ""

    def test_loads_from_env_vars(self, monkeypatch):
        """Test that AgentConfig loads from environment variables."""
        monkeypatch.setenv("AMP_AGENT_UID", "test-agent-123")
        monkeypatch.setenv("AMP_ENVIRONMENT_UID", "test-env-456")

        config = AgentConfig()
        assert config.agent_uid == "test-agent-123"
        assert config.environment_uid == "test-env-456"

    def test_env_prefix_required(self, monkeypatch):
        """Test that AMP_ prefix is required for env vars."""
        monkeypatch.setenv("AGENT_UID", "wrong-agent")
        monkeypatch.setenv("ENVIRONMENT_UID", "wrong-env")
        monkeypatch.delenv("AMP_AGENT_UID", raising=False)
        monkeypatch.delenv("AMP_ENVIRONMENT_UID", raising=False)

        config = AgentConfig()
        assert config.agent_uid == ""
        assert config.environment_uid == ""


class TestTraceConfig:
    """Test TraceConfig loading and validation."""

    def test_default_values(self, monkeypatch):
        """Test default values for TraceConfig."""
        monkeypatch.delenv("AMP_TRACE_API_URL", raising=False)
        monkeypatch.delenv("AMP_TRACE_API_KEY", raising=False)
        monkeypatch.delenv("AMP_TRACE_FILE_PATH", raising=False)

        config = TraceConfig()
        assert config.api_url == ""
        assert config.api_key == ""
        assert config.file_path is None

    def test_loads_api_url_from_env(self, monkeypatch):
        """Test loading api_url from environment variable."""
        monkeypatch.setenv("AMP_TRACE_API_URL", "http://localhost:8080")

        config = TraceConfig()
        assert config.api_url == "http://localhost:8080"

    def test_loads_api_key_from_env(self, monkeypatch):
        """Test loading api_key from environment variable."""
        monkeypatch.setenv("AMP_TRACE_API_KEY", "secret-key-123")

        config = TraceConfig()
        assert config.api_key == "secret-key-123"

    def test_loads_file_path_from_env(self, monkeypatch):
        """Test loading file_path from environment variable."""
        monkeypatch.setenv("AMP_TRACE_FILE_PATH", "/path/to/traces.json")

        config = TraceConfig()
        assert config.file_path == "/path/to/traces.json"

    def test_all_fields_from_env(self, monkeypatch):
        """Test loading all fields from environment variables."""
        monkeypatch.setenv("AMP_TRACE_API_URL", "http://api.example.com")
        monkeypatch.setenv("AMP_TRACE_API_KEY", "my-key")
        monkeypatch.setenv("AMP_TRACE_FILE_PATH", "/traces/data.json")

        config = TraceConfig()
        assert config.api_url == "http://api.example.com"
        assert config.api_key == "my-key"
        assert config.file_path == "/traces/data.json"


class TestLLMJudgeConfig:
    """Test LLMJudgeConfig loading and validation."""

    def test_default_model(self, monkeypatch):
        """Test default model value."""
        monkeypatch.delenv("AMP_LLM_JUDGE_DEFAULT_MODEL", raising=False)

        config = LLMJudgeConfig()
        assert config.default_model == "gpt-4o-mini"

    def test_loads_from_env(self, monkeypatch):
        """Test loading model from environment variable."""
        monkeypatch.setenv("AMP_LLM_JUDGE_DEFAULT_MODEL", "gpt-4o")

        config = LLMJudgeConfig()
        assert config.default_model == "gpt-4o"


class TestConfig:
    """Test the main Config class."""

    def test_default_values(self, monkeypatch):
        """Test that Config creates with all defaults."""
        for key in list(os.environ.keys()):
            if key.startswith("AMP_"):
                monkeypatch.delenv(key, raising=False)

        config = Config()
        assert config.agent.agent_uid == ""
        assert config.trace.api_url == ""
        assert config.trace.file_path is None
        assert config.llm_judge.default_model == "gpt-4o-mini"

    def test_nested_config_loading(self, monkeypatch):
        """Test that nested configs load correctly from env vars."""
        monkeypatch.setenv("AMP_AGENT_UID", "agent-123")
        monkeypatch.setenv("AMP_TRACE_API_URL", "http://api.example.com")
        monkeypatch.setenv("AMP_TRACE_FILE_PATH", "/path/to/traces.json")

        config = Config()
        assert config.agent.agent_uid == "agent-123"
        assert config.trace.api_url == "http://api.example.com"
        assert config.trace.file_path == "/path/to/traces.json"

    def test_instantiates_without_error(self, monkeypatch):
        """Test that Config instantiates cleanly with no env vars set."""
        for key in list(os.environ.keys()):
            if key.startswith("AMP_"):
                monkeypatch.delenv(key, raising=False)

        config = Config()
        assert config is not None

    def test_trace_api_key_optional(self, monkeypatch):
        """Test that api_key is optional (defaults to empty string)."""
        monkeypatch.setenv("AMP_TRACE_API_URL", "http://api.example.com")
        monkeypatch.delenv("AMP_TRACE_API_KEY", raising=False)

        config = Config()
        assert config.trace.api_url == "http://api.example.com"
        assert config.trace.api_key == ""


class TestGlobalConfig:
    """Test global config singleton functions."""

    def test_get_config_returns_singleton(self, monkeypatch):
        """Test that get_config returns the same instance."""
        from amp_evaluation import config as config_module

        config_module._config = None

        config1 = get_config()
        config2 = get_config()

        assert config1 is config2

    def test_reload_config_creates_new_instance(self, monkeypatch):
        """Test that reload_config creates a fresh instance."""
        from amp_evaluation import config as config_module

        config_module._config = None

        config1 = get_config()

        monkeypatch.setenv("AMP_AGENT_UID", "new-agent-123")

        config2 = reload_config()

        assert config1 is not config2
        assert config2.agent.agent_uid == "new-agent-123"

    def test_get_config_after_reload(self, monkeypatch):
        """Test that get_config returns the reloaded instance."""
        from amp_evaluation import config as config_module

        config_module._config = None

        monkeypatch.setenv("AMP_AGENT_UID", "initial-agent")
        get_config()

        monkeypatch.setenv("AMP_AGENT_UID", "reloaded-agent")
        reload_config()
        config2 = get_config()

        assert config2.agent.agent_uid == "reloaded-agent"


class TestEnvFileLoading:
    """Test .env file loading functionality."""

    def test_loads_from_env_file(self, tmp_path, monkeypatch):
        """Test that config loads from .env file."""
        env_file = tmp_path / ".env"
        env_file.write_text("AMP_AGENT_UID=from-file-agent\nAMP_TRACE_API_URL=http://from-file.com\n")

        monkeypatch.chdir(tmp_path)

        config = Config()
        assert config.agent.agent_uid == "from-file-agent"
        assert config.trace.api_url == "http://from-file.com"

    def test_env_vars_override_env_file(self, tmp_path, monkeypatch):
        """Test that environment variables override .env file values."""
        env_file = tmp_path / ".env"
        env_file.write_text("AMP_AGENT_UID=from-file\n")

        monkeypatch.chdir(tmp_path)

        monkeypatch.setenv("AMP_AGENT_UID", "from-env-var")

        config = Config()
        assert config.agent.agent_uid == "from-env-var"


class TestConfigEdgeCases:
    """Test edge cases and error conditions."""

    def test_empty_string_values(self, monkeypatch):
        """Test that empty string values work correctly."""
        monkeypatch.setenv("AMP_AGENT_UID", "")
        monkeypatch.setenv("AMP_TRACE_API_URL", "")

        config = Config()
        assert config.agent.agent_uid == ""
        assert config.trace.api_url == ""

    def test_extra_fields_ignored(self, monkeypatch):
        """Test that extra environment variables are ignored."""
        monkeypatch.setenv("AMP_UNKNOWN_FIELD", "some-value")
        monkeypatch.setenv("AMP_RANDOM_SETTING", "random")

        config = Config()
        assert not hasattr(config, "unknown_field")
        assert not hasattr(config, "random_setting")
