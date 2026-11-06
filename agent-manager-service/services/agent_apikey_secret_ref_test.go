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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestUseEnvVarAgentAPIKey_SetsEmptyRef(t *testing.T) {
	traitEnvConfigs := map[string]interface{}{}

	useEnvVarAgentAPIKey(traitEnvConfigs, "myagent")

	entry, ok := traitEnvConfigs["myagent-"+string(client.TraitEnvInjection)].(map[string]interface{})
	require.True(t, ok, "env-injection entry must be created")
	ref, present := entry["agentApiKeySecretRef"]
	assert.True(t, present, "an empty ref must be set, not omitted, so the trait ignores its base path")
	assert.Equal(t, "", ref)
}

func TestUseEnvVarAgentAPIKey_PreservesExistingConfig(t *testing.T) {
	envInjKey := "myagent-" + string(client.TraitEnvInjection)
	traitEnvConfigs := map[string]interface{}{
		envInjKey: map[string]interface{}{"envInjectionEnabled": true},
	}

	useEnvVarAgentAPIKey(traitEnvConfigs, "myagent")

	entry := traitEnvConfigs[envInjKey].(map[string]interface{})
	assert.Equal(t, true, entry["envInjectionEnabled"], "pre-existing config must survive")
	assert.Equal(t, "", entry["agentApiKeySecretRef"])
}

func TestAgentAPIKeyEnvVar_ReferencesSecretReferenceByName(t *testing.T) {
	envVar := agentAPIKeyEnvVar("agent-api-key-ref")

	assert.Equal(t, client.EnvVarAgentAPIKey, envVar.Key)
	assert.Empty(t, envVar.Value, "the key must never be a literal value")
	require.NotNil(t, envVar.ValueFrom)
	require.NotNil(t, envVar.ValueFrom.SecretKeyRef)
	assert.Equal(t, "agent-api-key-ref", envVar.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, secretmanagersvc.SecretKeyAPIKey, envVar.ValueFrom.SecretKeyRef.Key)
}

func TestMergeAgentAPIKeyEnvVarKey(t *testing.T) {
	assert.Equal(t, map[string]bool{client.EnvVarAgentAPIKey: true}, mergeAgentAPIKeyEnvVarKey(nil))

	keys := mergeAgentAPIKeyEnvVarKey(map[string]bool{"LLM_API_KEY": true})
	assert.True(t, keys["LLM_API_KEY"])
	assert.True(t, keys[client.EnvVarAgentAPIKey])
}

func TestStoreAgentAPIKey_ReturnsSecretReferenceName(t *testing.T) {
	// No GetSecretReferenceFunc: reading the SecretReference back would panic. The name
	// CreateSecret returns is all the caller needs.
	ocClient := &clientmocks.OpenChoreoClientMock{}
	secretMgmtClient := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			assert.Equal(t, map[string]string{secretmanagersvc.SecretKeyAPIKey: "the-api-key"}, data)
			return location.SecretRefName(), nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, secretMgmtClient: secretMgmtClient, logger: discardLogger()}

	name, err := s.storeAgentAPIKey(context.Background(), "org", "proj", "agent", "production", "the-api-key")

	require.NoError(t, err)
	assert.Equal(t, agentAPIKeySecretLocation("org", "proj", "agent", "production").SecretRefName(), name)
	assert.Empty(t, ocClient.GetSecretReferenceCalls())
}

func TestPromoteAgent_DeliversTargetAPIKeyByName(t *testing.T) {
	s, _, _, promotedTraits := promoteRestAgentHarness(t, models.KindRestAPI, client.TraitOTELInstrumentation)
	s.tokenManagerService = &stubTokenManager{generateFunc: func(_ context.Context, req GenerateTokenRequest) (*spec.TokenResponse, error) {
		assert.Equal(t, "staging", req.Environment, "the key must be minted for the target environment")
		return &spec.TokenResponse{Token: "staging-jwt"}, nil
	}}
	s.secretMgmtClient = &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return location.SecretRefName(), nil
		},
	}
	oc := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	var promotedEnv []client.EnvVar
	promote := oc.PromoteComponentFunc
	oc.PromoteComponentFunc = func(ctx context.Context, ouID, projectName, componentName, sourceEnv, targetEnv string,
		envOverrides []client.EnvVar, fileOverrides []client.FileVar, traits, ctConfigs map[string]interface{},
	) error {
		promotedEnv = envOverrides
		return promote(ctx, ouID, projectName, componentName, sourceEnv, targetEnv, envOverrides, fileOverrides, traits, ctConfigs)
	}

	// Minting the key needs the caller's organization, which promoteToStaging's context lacks.
	ctx := tierCtx(t, rbac.AgentEnvNonProduction, rbac.AgentEnvProduction)
	require.NoError(t, s.PromoteAgent(ctx, "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev", TargetEnvironment: "staging",
	}))

	wantName := agentAPIKeySecretLocation("acme", "proj1", "my-agent", "staging").SecretRefName()
	assert.Contains(t, promotedEnv, agentAPIKeyEnvVar(wantName))
	entry, ok := (*promotedTraits)["my-agent-"+string(client.TraitEnvInjection)].(map[string]interface{})
	require.True(t, ok, "the env-injection trait must be told to leave the key to the env var")
	assert.Equal(t, "", entry["agentApiKeySecretRef"])
	assert.Empty(t, oc.GetSecretReferenceCalls(), "promote must not read the SecretReference back")
}

// llmSecretRefsStub answers the two lookups getSystemManagedEnvVars makes; any other
// AgentConfigurationService call panics on the nil embed.
type llmSecretRefsStub struct {
	AgentConfigurationService
	llmSecretRefs map[string]struct{}
}

func (s *llmSecretRefsStub) ListAgentLLMConfigSecretReferences(context.Context, string, string, string) (map[string]struct{}, error) {
	return s.llmSecretRefs, nil
}

func (s *llmSecretRefsStub) ListSystemManagedEnvVarKeys(context.Context, string, string, string, string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func TestGetSystemManagedEnvVars_KeepsAgentAPIKeyEnvVar(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return []models.EnvVars{
				{Key: client.EnvVarAgentAPIKey, IsSensitive: true, SecretRef: "agent-api-key-ref", SecretKey: secretmanagersvc.SecretKeyAPIKey},
				{Key: "USER_SECRET", IsSensitive: true, SecretRef: "user-secrets", SecretKey: "USER_SECRET"},
			}, nil
		},
	}
	s := &agentManagerService{
		ocClient:                  ocClient,
		agentConfigurationService: &llmSecretRefsStub{llmSecretRefs: map[string]struct{}{}},
		logger:                    discardLogger(),
	}

	envVars, keys, err := s.getSystemManagedEnvVars(context.Background(), "acme", "proj1", "dev", "my-agent")

	require.NoError(t, err)
	assert.Equal(t, []client.EnvVar{agentAPIKeyEnvVar("agent-api-key-ref")}, envVars,
		"a deploy or configuration update must carry the agent API key forward, and only it")
	assert.Equal(t, map[string]bool{client.EnvVarAgentAPIKey: true}, keys)
}
