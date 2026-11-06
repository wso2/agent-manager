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
	"errors"
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

func TestSetTraitAgentAPIKeySecretRef_SetsEmptyRef(t *testing.T) {
	traitEnvConfigs := map[string]interface{}{}

	setTraitAgentAPIKeySecretRef(traitEnvConfigs, "myagent", "", "")

	entry, ok := traitEnvConfigs["myagent-"+string(client.TraitEnvInjection)].(map[string]interface{})
	require.True(t, ok, "env-injection entry must be created")
	ref, present := entry["agentApiKeySecretRef"]
	assert.True(t, present, "an empty ref must be set, not omitted, so the trait ignores its base path")
	assert.Equal(t, "", ref)
	_, present = entry["agentApiKeySecretProperty"]
	assert.False(t, present)
}

func TestSetTraitAgentAPIKeySecretRef_PreservesExistingConfig(t *testing.T) {
	envInjKey := "myagent-" + string(client.TraitEnvInjection)
	traitEnvConfigs := map[string]interface{}{
		envInjKey: map[string]interface{}{"envInjectionEnabled": true},
	}

	setTraitAgentAPIKeySecretRef(traitEnvConfigs, "myagent", "kv/dev/path", "api-key")

	entry := traitEnvConfigs[envInjKey].(map[string]interface{})
	assert.Equal(t, true, entry["envInjectionEnabled"], "pre-existing config must survive")
	assert.Equal(t, "kv/dev/path", entry["agentApiKeySecretRef"])
	assert.Equal(t, "api-key", entry["agentApiKeySecretProperty"])
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

// promoteWithAgentAPIKey promotes a REST agent from dev to staging, reporting releaseTrait for
// the dev release and targetRef as staging's current agentApiKeySecretRef. It returns the
// env vars and env-injection trait config the target was written with (nil when PromoteComponent
// was not reached), the secret client, and the promote error.
func promoteWithAgentAPIKey(t *testing.T, releaseTrait client.ReleaseAgentAPIKeyTrait, targetRef string, mintErr error) (
	[]client.EnvVar, map[string]interface{}, *clientmocks.SecretManagementClientMock, error,
) {
	t.Helper()
	s, _, _, promotedTraits := promoteRestAgentHarness(t, models.KindRestAPI, client.TraitOTELInstrumentation)
	s.tokenManagerService = &stubTokenManager{generateFunc: func(_ context.Context, req GenerateTokenRequest) (*spec.TokenResponse, error) {
		assert.Equal(t, "staging", req.Environment, "the key must be minted for the target environment")
		if mintErr != nil {
			return nil, mintErr
		}
		return &spec.TokenResponse{Token: "staging-jwt"}, nil
	}}
	secrets := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return location.SecretRefName(), nil
		},
	}
	s.secretMgmtClient = secrets
	oc := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	oc.GetReleaseAgentAPIKeyTraitFunc = func(_ context.Context, _, _, environment string) (client.ReleaseAgentAPIKeyTrait, error) {
		assert.Equal(t, "dev", environment, "a promote binds the source environment's release")
		return releaseTrait, nil
	}
	oc.GetReleaseBindingAgentAPIKeySecretRefFunc = func(_ context.Context, _, _, environment string) (string, error) {
		assert.Equal(t, "staging", environment)
		return targetRef, nil
	}
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
	err := s.PromoteAgent(ctx, "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev", TargetEnvironment: "staging",
	})
	assert.Empty(t, oc.GetSecretReferenceCalls(), "promote must not read the SecretReference back")
	envInjection, _ := (*promotedTraits)["my-agent-"+string(client.TraitEnvInjection)].(map[string]interface{})
	return promotedEnv, envInjection, secrets, err
}

var stagingAPIKeyEnvVar = agentAPIKeyEnvVar(agentAPIKeySecretLocation("acme", "proj1", "my-agent", "staging").SecretRefName())

func TestPromoteAgent_DeliversTargetAPIKeyByName(t *testing.T) {
	env, envInjection, _, err := promoteWithAgentAPIKey(t, client.ReleaseAgentAPIKeyTrait{Attached: true, TakesEnvVarKey: true}, "", nil)

	require.NoError(t, err)
	assert.Contains(t, env, stagingAPIKeyEnvVar)
	ref, present := envInjection["agentApiKeySecretRef"]
	assert.True(t, present)
	assert.Equal(t, "", ref, "the trait must leave the key to the env var")
}

func TestPromoteAgent_ReleaseBeforeEnvVarKey_KeepsTargetsOwnPath(t *testing.T) {
	// The target's own path holds the key just issued; overwriting it with the release's base
	// path would hand the target the first environment's key.
	env, envInjection, _, err := promoteWithAgentAPIKey(t, client.ReleaseAgentAPIKeyTrait{
		Attached: true, SecretRef: "kv/my-agent-agent-api-key-dev-secrets", SecretProperty: "api-key",
	}, "kv/my-agent-agent-api-key-staging-secrets", nil)

	require.NoError(t, err)
	assert.Contains(t, env, stagingAPIKeyEnvVar)
	_, present := envInjection["agentApiKeySecretRef"]
	assert.False(t, present, "left unset, the binding update carries the target's own path forward")
}

func TestPromoteAgent_ReleaseBeforeEnvVarKey_FallsBackToReleasePath(t *testing.T) {
	// A release cut before the trait could leave the key to the env var would fetch an empty
	// path and the target pod would never start, so an empty or missing target path is replaced.
	env, envInjection, _, err := promoteWithAgentAPIKey(t, client.ReleaseAgentAPIKeyTrait{
		Attached: true, SecretRef: "kv/my-agent-agent-api-key-dev-secrets", SecretProperty: "api-key",
	}, "", nil)

	require.NoError(t, err)
	assert.Contains(t, env, stagingAPIKeyEnvVar)
	assert.Equal(t, "kv/my-agent-agent-api-key-dev-secrets", envInjection["agentApiKeySecretRef"])
	assert.Equal(t, "api-key", envInjection["agentApiKeySecretProperty"])
}

func TestPromoteAgent_NoEnvInjectionTrait_IssuesNoKey(t *testing.T) {
	env, envInjection, secrets, err := promoteWithAgentAPIKey(t, client.ReleaseAgentAPIKeyTrait{TakesEnvVarKey: true}, "", nil)

	require.NoError(t, err)
	assert.NotContains(t, env, stagingAPIKeyEnvVar)
	assert.Nil(t, envInjection)
	assert.Empty(t, secrets.CreateSecretCalls(), "an agent created without a key gets none on promotion either")
}

func TestPromoteAgent_KeyCannotBeIssued_AbortsPromotion(t *testing.T) {
	// The promotion replaces the target's env vars; going ahead would leave it with no key.
	env, _, _, err := promoteWithAgentAPIKey(t, client.ReleaseAgentAPIKeyTrait{Attached: true, TakesEnvVarKey: true}, "", errors.New("signing key unavailable"))

	require.Error(t, err)
	assert.Nil(t, env, "nothing may be written to the target")
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

// noAgentAPIKeyTrait stubs GetReleaseAgentAPIKeyTrait for a source release without the
// env-injection trait, so a promote issues no agent API key.
func noAgentAPIKeyTrait(context.Context, string, string, string) (client.ReleaseAgentAPIKeyTrait, error) {
	return client.ReleaseAgentAPIKeyTrait{}, nil
}
