// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func llmEnvVarNamed(vars []client.EnvVar, key string) *client.EnvVar {
	for i := range vars {
		if vars[i].Key == key {
			return &vars[i]
		}
	}
	return nil
}

// A new LLM config gets a third, non-secret variable carrying the provider's template handle,
// named after the config like the URL and API key variables.
func TestBuildEnvironmentVariablesIncludesProviderKey(t *testing.T) {
	svc := &agentConfigurationService{}

	templates, err := svc.buildEnvironmentVariables("my-llm", nil)

	require.NoError(t, err)
	require.Equal(t, []EnvConfigTemplate{
		{Key: "url", Name: "MY_LLM_URL"},
		{Key: "apikey", Name: "MY_LLM_API_KEY", IsSecret: true},
		{Key: "provider", Name: "MY_LLM_PROVIDER"},
	}, templates)
}

func TestBuildEnvironmentVariablesProviderNameCanBeOverridden(t *testing.T) {
	svc := &agentConfigurationService{}

	templates, err := svc.buildEnvironmentVariables("my-llm", []models.EnvironmentVariableConfig{
		{Key: "provider", Name: "LLM_VENDOR"},
	})

	require.NoError(t, err)
	require.Equal(t, "LLM_VENDOR", templates[2].Name)
	require.Equal(t, "provider", templates[2].Key)
}

func TestBuildEnvironmentVariablesRejectsProviderNameClash(t *testing.T) {
	svc := &agentConfigurationService{}

	_, err := svc.buildEnvironmentVariables("my-llm", []models.EnvironmentVariableConfig{
		{Key: "provider", Name: "MY_LLM_URL"},
	})

	require.ErrorContains(t, err, "duplicate environment variable name")
}

// MCP configs share the env var machinery but have no LLM provider: the key must stay LLM-only.
func TestBuildMCPMappingEnvironmentVariablesRejectsProviderKey(t *testing.T) {
	svc := &agentConfigurationService{}

	_, err := svc.buildMCPMappingEnvironmentVariables("my-mcp", []models.EnvironmentVariableConfig{
		{Key: "provider", Name: "MY_MCP_PROVIDER"},
	})

	require.ErrorContains(t, err, "unknown environment variable key")
}

func TestBuildLLMEnvVarsInjectsProviderHandle(t *testing.T) {
	templates := []EnvConfigTemplate{
		{Key: "url", Name: "MY_LLM_URL"},
		{Key: "apikey", Name: "MY_LLM_API_KEY", IsSecret: true},
		{Key: "provider", Name: "MY_LLM_PROVIDER"},
	}

	vars := buildLLMEnvVars(templates, "http://gw/llm", "secret-ref", "openai")

	require.Len(t, vars, 3)
	require.Equal(t, "http://gw/llm", llmEnvVarNamed(vars, "MY_LLM_URL").Value)
	require.Equal(t, "secret-ref", llmEnvVarNamed(vars, "MY_LLM_API_KEY").ValueFrom.SecretKeyRef.Name)
	provider := llmEnvVarNamed(vars, "MY_LLM_PROVIDER")
	require.Equal(t, "openai", provider.Value)
	require.Nil(t, provider.ValueFrom, "the provider handle is plain text, never a secret reference")
}

// A config created before the provider variable existed has no provider template, and an
// unresolved handle must not produce an empty variable: both leave the output as it was.
func TestBuildLLMEnvVarsOmitsProviderWhenNotApplicable(t *testing.T) {
	legacy := []EnvConfigTemplate{
		{Key: "url", Name: "MY_LLM_URL"},
		{Key: "apikey", Name: "MY_LLM_API_KEY", IsSecret: true},
	}
	require.Len(t, buildLLMEnvVars(legacy, "http://gw/llm", "secret-ref", "openai"), 2)

	withProvider := append(legacy, EnvConfigTemplate{Key: "provider", Name: "MY_LLM_PROVIDER"})
	require.Len(t, buildLLMEnvVars(withProvider, "http://gw/llm", "secret-ref", ""), 2)
}

// The URL and API key names stay as persisted when a config is renamed, so a provider variable
// added later (provider switch, new environment) must follow their prefix, not the config name.
func TestBuildEnvironmentVariablesProviderFollowsExistingNamePrefix(t *testing.T) {
	svc := &agentConfigurationService{}

	templates, err := svc.buildEnvironmentVariables("anthropic-01", []models.EnvironmentVariableConfig{
		{Key: "url", Name: "ANTHROPIC_URL"},
		{Key: "apikey", Name: "ANTHROPIC_API_KEY"},
	})

	require.NoError(t, err)
	require.Equal(t, "ANTHROPIC_PROVIDER", templates[2].Name)
}

func TestBuildEnvironmentVariablesProviderFollowsBareNames(t *testing.T) {
	svc := &agentConfigurationService{}

	templates, err := svc.buildEnvironmentVariables("my-llm", []models.EnvironmentVariableConfig{
		{Key: "url", Name: "URL"},
		{Key: "apikey", Name: "API_KEY"},
	})

	require.NoError(t, err)
	require.Equal(t, "PROVIDER", templates[2].Name)
}

func TestBuildEnvironmentVariablesProviderFallsBackToConfigNameForCustomNames(t *testing.T) {
	svc := &agentConfigurationService{}

	templates, err := svc.buildEnvironmentVariables("my-llm", []models.EnvironmentVariableConfig{
		{Key: "url", Name: "LLM_BASE"},
		{Key: "apikey", Name: "LLM_TOKEN"},
	})

	require.NoError(t, err)
	require.Equal(t, "MY_LLM_PROVIDER", templates[2].Name)
}

func TestBuildEnvironmentVariablesExplicitProviderNameBeatsSiblingPrefix(t *testing.T) {
	svc := &agentConfigurationService{}

	templates, err := svc.buildEnvironmentVariables("anthropic-01", []models.EnvironmentVariableConfig{
		{Key: "url", Name: "ANTHROPIC_URL"},
		{Key: "provider", Name: "LLM_VENDOR"},
	})

	require.NoError(t, err)
	require.Equal(t, "LLM_VENDOR", templates[2].Name)
}

// A legacy config (no provider variable) whose URL is named like the provider's default name must
// still be able to rename another variable: only the variables it has are validated.
func TestBuildEnvironmentVariablesForKeysIgnoresAbsentProviderOnRename(t *testing.T) {
	svc := &agentConfigurationService{}
	overrides := []models.EnvironmentVariableConfig{
		{Key: "url", Name: "MY_LLM_PROVIDER"},
		{Key: "apikey", Name: "NEW_TOKEN"},
	}

	_, err := svc.buildEnvironmentVariables("my-llm", overrides)
	require.ErrorContains(t, err, "duplicate environment variable name", "all three keys still clash")

	templates, err := svc.buildEnvironmentVariablesForKeys("my-llm", overrides, persistedLLMEnvVarKeys(map[string]string{
		"url": "MY_LLM_PROVIDER", "apikey": "TOKEN",
	}))
	require.NoError(t, err)
	require.Len(t, templates, 2)
	require.Equal(t, "NEW_TOKEN", templates[1].Name)
}

func TestPersistedLLMEnvVarKeys(t *testing.T) {
	require.Equal(t, []string{"url", "apikey"}, persistedLLMEnvVarKeys(map[string]string{"apikey": "A", "url": "U"}))
	require.Equal(t, []string{"url", "apikey", "provider"}, persistedLLMEnvVarKeys(map[string]string{"provider": "P", "apikey": "A", "url": "U"}))
	require.Equal(t, []string{"url", "apikey", "provider"}, persistedLLMEnvVarKeys(nil), "no rows falls back to every key")
}

// One config can have the provider row in a newer environment and not in an older one; the older
// one must not be handed a variable its table does not track.
func TestTemplatesTrackedInFollowsTheEnvironmentsOwnRows(t *testing.T) {
	templates := []EnvConfigTemplate{
		{Key: "url", Name: "X_URL"},
		{Key: "apikey", Name: "X_API_KEY"},
		{Key: "provider", Name: "X_PROVIDER"},
	}
	legacyEnv := []models.AgentEnvConfigVariable{{VariableKey: "url"}, {VariableKey: "apikey"}}
	newerEnv := append(legacyEnv, models.AgentEnvConfigVariable{VariableKey: "provider"})

	require.Len(t, templatesTrackedIn(templates, legacyEnv), 2)
	require.Equal(t, templates, templatesTrackedIn(templates, newerEnv))
}

// A custom URL name that merely ends in _URL is the user's own choice: it must not set the prefix.
func TestBuildEnvironmentVariablesProviderIgnoresCustomURLName(t *testing.T) {
	svc := &agentConfigurationService{}

	templates, err := svc.buildEnvironmentVariables("my-llm", []models.EnvironmentVariableConfig{
		{Key: "url", Name: "OPENAI_BASE_URL"},
		{Key: "apikey", Name: "OPENAI_API_KEY"},
	})

	require.NoError(t, err)
	require.Equal(t, "MY_LLM_PROVIDER", templates[2].Name)
}

func TestBuildEnvironmentVariablesProviderNeedsBothNamesOnTheDefaultPair(t *testing.T) {
	svc := &agentConfigurationService{}

	for name, overrides := range map[string][]models.EnvironmentVariableConfig{
		"only url":          {{Key: "url", Name: "ANTHROPIC_URL"}},
		"only apikey":       {{Key: "apikey", Name: "ANTHROPIC_API_KEY"}},
		"prefixes differ":   {{Key: "url", Name: "OPENAI_URL"}, {Key: "apikey", Name: "ANTHROPIC_API_KEY"}},
		"custom apikey":     {{Key: "url", Name: "ANTHROPIC_URL"}, {Key: "apikey", Name: "ANTHROPIC_TOKEN"}},
		"bare and prefixed": {{Key: "url", Name: "URL"}, {Key: "apikey", Name: "ANTHROPIC_API_KEY"}},
	} {
		templates, err := svc.buildEnvironmentVariables("anthropic-01", overrides)
		require.NoError(t, err, name)
		require.Equal(t, "ANTHROPIC_01_PROVIDER", templates[2].Name, name)
	}
}
