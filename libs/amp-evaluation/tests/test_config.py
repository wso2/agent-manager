"""
Tests for configuration management using Pydantic Settings.
"""

import os
import pytest
from amp_evaluation.config import (
    AgentConfig,
    PlatformConfig,
    TraceLoaderConfig,
    ResultsConfig,
    Config,
    get_config,
    reload_config,
)


class TestAgentConfig:
    """Test AgentConfig loading and validation."""

    def test_default_values(self, monkeypatch):
        """Test that AgentConfig has sensible defaults."""
        # Clear any existing env vars
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
        # These should NOT be loaded (no AMP_ prefix)
        monkeypatch.setenv("AGENT_UID", "wrong-agent")
        monkeypatch.setenv("ENVIRONMENT_UID", "wrong-env")
        monkeypatch.delenv("AMP_AGENT_UID", raising=False)
        monkeypatch.delenv("AMP_ENVIRONMENT_UID", raising=False)

        config = AgentConfig()
        assert config.agent_uid == ""  # Should be empty, not "wrong-agent"
        assert config.environment_uid == ""


class TestPlatformConfig:
    """Test PlatformConfig loading and validation."""

    def test_default_values(self, monkeypatch):
        """Test default values for PlatformConfig."""
        monkeypatch.delenv("AMP_API_URL", raising=False)
        monkeypatch.delenv("AMP_API_KEY", raising=False)

        config = PlatformConfig()
        assert config.api_url == ""
        assert config.api_key == ""

    def test_loads_from_env_vars(self, monkeypatch):
        """Test loading from environment variables."""
        monkeypatch.setenv("AMP_API_URL", "http://localhost:8080")
        monkeypatch.setenv("AMP_API_KEY", "secret-key-123")

        config = PlatformConfig()
        assert config.api_url == "http://localhost:8080"
        assert config.api_key == "secret-key-123"


class TestTraceLoaderConfig:
    """Test TraceLoaderConfig loading and validation."""

    def test_default_values(self, monkeypatch):
        """Test default values."""
        monkeypatch.delenv("AMP_TRACE_LOADER_MODE", raising=False)
        monkeypatch.delenv("AMP_TRACE_LOADER_TRACE_FILE_PATH", raising=False)
        monkeypatch.delenv("AMP_TRACE_LOADER_BATCH_SIZE", raising=False)

        config = TraceLoaderConfig()
        assert config.mode == "platform"
        assert config.trace_file_path is None
        assert config.batch_size == 100

    def test_loads_from_env_vars(self, monkeypatch):
        """Test loading from environment variables."""
        monkeypatch.setenv("AMP_TRACE_LOADER_MODE", "file")
        monkeypatch.setenv("AMP_TRACE_LOADER_TRACE_FILE_PATH", "/path/to/traces.json")
        monkeypatch.setenv("AMP_TRACE_LOADER_BATCH_SIZE", "50")

        config = TraceLoaderConfig()
        assert config.mode == "file"
        assert config.trace_file_path == "/path/to/traces.json"
        assert config.batch_size == 50

    def test_validates_mode(self, monkeypatch):
        """Test that mode validation works."""
        monkeypatch.setenv("AMP_TRACE_LOADER_MODE", "invalid-mode")

        with pytest.raises(ValueError, match="Invalid mode 'invalid-mode'"):
            TraceLoaderConfig()

    def test_valid_modes(self, monkeypatch):
        """Test that both 'platform' and 'file' modes are valid."""
        monkeypatch.setenv("AMP_TRACE_LOADER_MODE", "platform")
        config1 = TraceLoaderConfig()
        assert config1.mode == "platform"

        monkeypatch.setenv("AMP_TRACE_LOADER_MODE", "file")
        config2 = TraceLoaderConfig()
        assert config2.mode == "file"

    def test_batch_size_type_conversion(self, monkeypatch):
        """Test that batch_size is converted from string to int."""
        monkeypatch.setenv("AMP_TRACE_LOADER_BATCH_SIZE", "200")

        config = TraceLoaderConfig()
        assert config.batch_size == 200
        assert isinstance(config.batch_size, int)


class TestResultsConfig:
    """Test ResultsConfig loading and validation."""

    def test_default_value(self, monkeypatch):
        """Test default value for publish_to_platform."""
        monkeypatch.delenv("AMP_PUBLISH_TO_PLATFORM", raising=False)

        config = ResultsConfig()
        assert config.publish_to_platform is False

    def test_loads_true_from_env(self, monkeypatch):
        """Test loading boolean true value."""
        monkeypatch.setenv("AMP_PUBLISH_TO_PLATFORM", "true")

        config = ResultsConfig()
        assert config.publish_to_platform is True

    def test_loads_false_from_env(self, monkeypatch):
        """Test loading boolean false value."""
        monkeypatch.setenv("AMP_PUBLISH_TO_PLATFORM", "false")

        config = ResultsConfig()
        assert config.publish_to_platform is False

    def test_boolean_case_insensitive(self, monkeypatch):
        """Test that boolean parsing is case insensitive."""
        monkeypatch.setenv("AMP_PUBLISH_TO_PLATFORM", "True")
        config1 = ResultsConfig()
        assert config1.publish_to_platform is True

        monkeypatch.setenv("AMP_PUBLISH_TO_PLATFORM", "FALSE")
        config2 = ResultsConfig()
        assert config2.publish_to_platform is False

    def test_boolean_numeric_values(self, monkeypatch):
        """Test that boolean accepts numeric values."""
        monkeypatch.setenv("AMP_PUBLISH_TO_PLATFORM", "1")
        config1 = ResultsConfig()
        assert config1.publish_to_platform is True

        monkeypatch.setenv("AMP_PUBLISH_TO_PLATFORM", "0")
        config2 = ResultsConfig()
        assert config2.publish_to_platform is False


class TestConfig:
    """Test the main Config class."""

    def test_default_values(self, monkeypatch):
        """Test that Config creates with all defaults."""
        # Clear all env vars
        for key in list(os.environ.keys()):
            if key.startswith("AMP_"):
                monkeypatch.delenv(key, raising=False)

        config = Config()
        assert config.agent.agent_uid == ""
        assert config.platform.api_url == ""
        assert config.trace_loader.mode == "platform"
        assert config.results.publish_to_platform is False

    def test_nested_config_loading(self, monkeypatch):
        """Test that nested configs load correctly from env vars."""
        monkeypatch.setenv("AMP_AGENT_UID", "agent-123")
        monkeypatch.setenv("AMP_API_URL", "http://api.example.com")
        monkeypatch.setenv("AMP_TRACE_LOADER_MODE", "file")
        monkeypatch.setenv("AMP_PUBLISH_TO_PLATFORM", "true")

        config = Config()
        assert config.agent.agent_uid == "agent-123"
        assert config.platform.api_url == "http://api.example.com"
        assert config.trace_loader.mode == "file"
        assert config.results.publish_to_platform is True

    def test_validation_passes_with_minimal_config(self, monkeypatch):
        """Test that validation passes with minimal required config."""
        # Clear all env vars
        for key in list(os.environ.keys()):
            if key.startswith("AMP_"):
                monkeypatch.delenv(key, raising=False)

        # Should not raise - validation is lenient now
        config = Config()
        assert config is not None

    def test_can_access_nested_fields(self, monkeypatch):
        """Test that we can access nested configuration fields."""
        monkeypatch.setenv("AMP_AGENT_UID", "test-agent")
        monkeypatch.setenv("AMP_ENVIRONMENT_UID", "test-env")
        monkeypatch.setenv("AMP_TRACE_LOADER_BATCH_SIZE", "150")

        config = Config()

        # Agent config
        assert config.agent.agent_uid == "test-agent"
        assert config.agent.environment_uid == "test-env"

        # Trace loader config
        assert config.trace_loader.batch_size == 150


class TestGlobalConfig:
    """Test global config singleton functions."""

    def test_get_config_returns_singleton(self, monkeypatch):
        """Test that get_config returns the same instance."""
        # Clear the global config first
        from amp_evaluation import config as config_module

        config_module._config = None

        config1 = get_config()
        config2 = get_config()

        assert config1 is config2

    def test_reload_config_creates_new_instance(self, monkeypatch):
        """Test that reload_config creates a fresh instance."""
        from amp_evaluation import config as config_module

        config_module._config = None

        # Get initial config
        config1 = get_config()

        # Change environment
        monkeypatch.setenv("AMP_AGENT_UID", "new-agent-123")

        # Reload should create new instance
        config2 = reload_config()

        assert config1 is not config2
        assert config2.agent.agent_uid == "new-agent-123"

    def test_get_config_after_reload(self, monkeypatch):
        """Test that get_config returns the reloaded instance."""
        from amp_evaluation import config as config_module

        config_module._config = None

        monkeypatch.setenv("AMP_AGENT_UID", "initial-agent")
        get_config()  # Initial load

        monkeypatch.setenv("AMP_AGENT_UID", "reloaded-agent")
        reload_config()
        config2 = get_config()

        assert config2.agent.agent_uid == "reloaded-agent"


class TestEnvFileLoading:
    """Test .env file loading functionality."""

    def test_loads_from_env_file(self, tmp_path, monkeypatch):
        """Test that config loads from .env file."""
        # Create a temporary .env file
        env_file = tmp_path / ".env"
        env_file.write_text(
            "AMP_AGENT_UID=from-file-agent\nAMP_API_URL=http://from-file.com\nAMP_TRACE_LOADER_BATCH_SIZE=250\n"
        )

        # Change to the temp directory so .env is found
        monkeypatch.chdir(tmp_path)

        config = Config()
        assert config.agent.agent_uid == "from-file-agent"
        assert config.platform.api_url == "http://from-file.com"
        assert config.trace_loader.batch_size == 250

    def test_env_vars_override_env_file(self, tmp_path, monkeypatch):
        """Test that environment variables override .env file values."""
        # Create .env file
        env_file = tmp_path / ".env"
        env_file.write_text("AMP_AGENT_UID=from-file\n")

        monkeypatch.chdir(tmp_path)

        # Set env var that should override
        monkeypatch.setenv("AMP_AGENT_UID", "from-env-var")

        config = Config()
        assert config.agent.agent_uid == "from-env-var"


class TestConfigEdgeCases:
    """Test edge cases and error conditions."""

    def test_empty_string_values(self, monkeypatch):
        """Test that empty string values work correctly."""
        monkeypatch.setenv("AMP_AGENT_UID", "")
        monkeypatch.setenv("AMP_API_URL", "")

        config = Config()
        assert config.agent.agent_uid == ""
        assert config.platform.api_url == ""

    def test_whitespace_not_trimmed_by_default(self, monkeypatch):
        """Test that Pydantic does not trim whitespace by default."""
        monkeypatch.setenv("AMP_AGENT_UID", "  agent-with-spaces  ")

        config = Config()
        # Pydantic does NOT trim whitespace unless configured to do so
        assert config.agent.agent_uid == "  agent-with-spaces  "

    def test_extra_fields_ignored(self, monkeypatch):
        """Test that extra environment variables are ignored."""
        monkeypatch.setenv("AMP_UNKNOWN_FIELD", "some-value")
        monkeypatch.setenv("AMP_RANDOM_SETTING", "random")

        # Should not raise an error - extra fields ignored
        config = Config()
        assert not hasattr(config, "unknown_field")
        assert not hasattr(config, "random_setting")

    def test_invalid_batch_size_raises_error(self, monkeypatch):
        """Test that invalid batch_size type raises validation error."""
        monkeypatch.setenv("AMP_TRACE_LOADER_BATCH_SIZE", "not-a-number")

        with pytest.raises(ValueError):
            TraceLoaderConfig()
