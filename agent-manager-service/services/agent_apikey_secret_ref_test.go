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
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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

// agentAPIKeyPromotion is what promoteAgentWithAPIKey observed.
type agentAPIKeyPromotion struct {
	// env and envInjection are the env vars and env-injection trait config the target was
	// written with, nil when PromoteComponent was not reached.
	env          []client.EnvVar
	envInjection map[string]interface{}
	secrets      *clientmocks.SecretManagementClientMock
	tokens       *stubTokenManager
	oc           *clientmocks.OpenChoreoClientMock
	err          error
}

// promoteAgentWithAPIKey promotes a REST agent from dev to staging, reporting releaseTrait for
// the dev release and targetRef (or targetRefErr) as staging's current agentApiKeySecretRef.
// With sourceOverrides, the promote clones dev's env vars, which are sourceOverrides.
func promoteAgentWithAPIKey(t *testing.T, releaseTrait client.ReleaseAgentAPIKeyTrait, targetRef string, targetRefErr, mintErr error,
	sourceOverrides ...client.EnvVar,
) agentAPIKeyPromotion {
	t.Helper()
	s, _, _, promotedTraits := promoteRestAgentHarness(t, models.KindRestAPI, client.TraitOTELInstrumentation)
	tokens := &stubTokenManager{generateFunc: func(_ context.Context, req GenerateTokenRequest) (*spec.TokenResponse, error) {
		assert.Equal(t, "staging", req.Environment, "the key must be minted for the target environment")
		if mintErr != nil {
			return nil, mintErr
		}
		return &spec.TokenResponse{Token: "staging-jwt"}, nil
	}}
	s.tokenManagerService = tokens
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
		return targetRef, targetRefErr
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
	req := &spec.PromoteAgentRequest{SourceEnvironment: "dev", TargetEnvironment: "staging"}
	if len(sourceOverrides) > 0 {
		useSource := true
		req.UseConfigFromSourceEnv = &useSource
		oc.GetSourceEnvWorkloadOverridesFunc = func(_ context.Context, _, _, environment string) ([]client.EnvVar, []client.FileVar, error) {
			assert.Equal(t, "dev", environment)
			return sourceOverrides, nil, nil
		}
	}
	err := s.PromoteAgent(ctx, "acme", "proj1", "my-agent", req)
	assert.Empty(t, oc.GetSecretReferenceCalls(), "promote must not read the SecretReference back")
	envInjection, _ := (*promotedTraits)["my-agent-"+string(client.TraitEnvInjection)].(map[string]interface{})
	return agentAPIKeyPromotion{env: promotedEnv, envInjection: envInjection, secrets: secrets, tokens: tokens, oc: oc, err: err}
}

// promoteWithAgentAPIKey is promoteAgentWithAPIKey with a target lookup that succeeds, returning
// the target's env vars and env-injection trait config, the secret client, and the promote error.
func promoteWithAgentAPIKey(t *testing.T, releaseTrait client.ReleaseAgentAPIKeyTrait, targetRef string, mintErr error, sourceOverrides ...client.EnvVar) (
	[]client.EnvVar, map[string]interface{}, *clientmocks.SecretManagementClientMock, error,
) {
	t.Helper()
	p := promoteAgentWithAPIKey(t, releaseTrait, targetRef, nil, mintErr, sourceOverrides...)
	return p.env, p.envInjection, p.secrets, p.err
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

func TestPromoteAgent_ReleaseBeforeEnvVarKey_NoTargetPath_RefusesPromotion(t *testing.T) {
	// The release reads the key from the trait's path, and without a path of its own the target
	// would fall back to the first environment's key and report its traces as that environment's.
	p := promoteAgentWithAPIKey(t, client.ReleaseAgentAPIKeyTrait{
		Attached: true, SecretRef: "kv/my-agent-agent-api-key-dev-secrets", SecretProperty: "api-key",
	}, "", nil, nil)

	require.ErrorIs(t, p.err, utils.ErrPromotionNeedsRedeploy)
	assert.Contains(t, p.err.Error(), "deploy the agent again")
	assert.Empty(t, p.tokens.gotRequests, "no key may be minted for a promotion that is refused")
	assert.Empty(t, p.secrets.CreateSecretCalls(), "nothing may be stored for a promotion that is refused")
	assert.Empty(t, p.oc.PromoteComponentCalls(), "the target must not be written")
}

func TestPromoteAgent_ReleaseBeforeEnvVarKey_TargetPathLookupFails_AbortsPromotion(t *testing.T) {
	lookupErr := errors.New("openchoreo unavailable")

	p := promoteAgentWithAPIKey(t, client.ReleaseAgentAPIKeyTrait{
		Attached: true, SecretRef: "kv/my-agent-agent-api-key-dev-secrets", SecretProperty: "api-key",
	}, "", lookupErr, nil)

	require.ErrorIs(t, p.err, lookupErr)
	assert.NotErrorIs(t, p.err, utils.ErrPromotionNeedsRedeploy, "a failed lookup is not a missing path")
	assert.Empty(t, p.tokens.gotRequests)
	assert.Empty(t, p.secrets.CreateSecretCalls(), "nothing may be stored when the target's config cannot be read")
	assert.Empty(t, p.oc.PromoteComponentCalls())
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

// ListAgentLLMConfigSecretReferences returns the stub's LLM config secret references.
func (s *llmSecretRefsStub) ListAgentLLMConfigSecretReferences(context.Context, string, string, string) (map[string]struct{}, error) {
	return s.llmSecretRefs, nil
}

// ListSystemManagedEnvVarKeys reports no system-managed keys from the agent configuration.
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

func TestPromoteAgent_ClonedAgentAPIKey_ReplacedByTargetsOwn(t *testing.T) {
	devAPIKeyEnvVar := agentAPIKeyEnvVar(agentAPIKeySecretLocation("acme", "proj1", "my-agent", "dev").SecretRefName())

	env, _, _, err := promoteWithAgentAPIKey(t, client.ReleaseAgentAPIKeyTrait{Attached: true, TakesEnvVarKey: true}, "", nil,
		devAPIKeyEnvVar, client.EnvVar{Key: "USER_VAR", Value: "kept"})

	require.NoError(t, err)
	assert.Contains(t, env, stagingAPIKeyEnvVar)
	assert.NotContains(t, env, devAPIKeyEnvVar, "the source environment's key must not reach the target")
	assert.Contains(t, env, client.EnvVar{Key: "USER_VAR", Value: "kept"})
}

func TestPromoteAgent_NoEnvInjectionTrait_KeepsUsersAgentAPIKey(t *testing.T) {
	// An agent that never receives a system key may set AMP_AGENT_API_KEY itself.
	userKey := client.EnvVar{Key: client.EnvVarAgentAPIKey, Value: "user-key"}

	env, _, _, err := promoteWithAgentAPIKey(t, client.ReleaseAgentAPIKeyTrait{TakesEnvVarKey: true}, "", nil, userKey)

	require.NoError(t, err)
	assert.Contains(t, env, userKey)
}

func TestGetSystemManagedEnvVars_IgnoresUsersAgentAPIKey(t *testing.T) {
	// A user's own AMP_AGENT_API_KEY lives in the agent's secret under its own name, unlike the
	// system key ("api-key"). Treating it as the system key would freeze it and drop its value.
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return []models.EnvVars{
				{Key: client.EnvVarAgentAPIKey, IsSensitive: true, SecretRef: "my-agent-dev-secrets", SecretKey: client.EnvVarAgentAPIKey},
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
	assert.Empty(t, envVars)
	assert.Empty(t, keys)
}

// deployWithBaseEnvVars deploys an API agent into dev, which has no release binding yet, with
// baseEnvVars saved on the Component and reqEnv in the request. It returns the env vars written
// to dev's release binding and the deploy error.
func deployWithBaseEnvVars(t *testing.T, baseEnvVars []client.EnvVar, reqEnv []spec.EnvironmentVariable) ([]client.EnvVar, error) {
	t.Helper()
	s, _ := deployAPIAgentMocks(nil)
	oc := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	oc.GetComponentBaseEnvVarsFunc = func(_ context.Context, _, componentName string) ([]client.EnvVar, error) {
		assert.Equal(t, "my-agent", componentName)
		return baseEnvVars, nil
	}
	var written []client.EnvVar
	oc.EnsureReleaseAndBindingFunc = func(_ context.Context, _, _, _, environment string, envOverrides []client.EnvVar, _ []client.FileVar,
		_, _ map[string]interface{},
	) error {
		assert.Equal(t, "dev", environment)
		written = envOverrides
		return nil
	}

	_, err := s.DeployAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent",
		&spec.DeployAgentRequest{ImageId: "registry.example.com/my-agent:v1", Env: reqEnv})
	return written, err
}

func TestDeployAgent_NoBindingYet_CarriesSeededAgentAPIKey(t *testing.T) {
	// The key seeded at creation is on the Component only. A deploy that creates the first binding
	// must write it there, or the binding's env vars replace it and it is lost for good.
	seeded := agentAPIKeyEnvVar(agentAPIKeySecretLocation("acme", "proj1", "my-agent", "dev").SecretRefName())
	echoed := spec.EnvironmentVariable{Key: client.EnvVarAgentAPIKey}
	echoed.SetValue("user-echoed")

	written, err := deployWithBaseEnvVars(t, []client.EnvVar{seeded, {Key: "USER_VAR", Value: "base"}}, []spec.EnvironmentVariable{echoed})

	require.NoError(t, err)
	assert.Contains(t, written, seeded)
	assert.NotContains(t, written, client.EnvVar{Key: client.EnvVarAgentAPIKey, Value: "user-echoed"},
		"a user-echoed copy must not replace the system key")
	assert.NotContains(t, written, client.EnvVar{Key: "USER_VAR", Value: "base"}, "only the agent API key is carried from the Component")
}

func TestDeployAgent_NoSeed_KeepsUsersAgentAPIKey(t *testing.T) {
	// A user's own AMP_AGENT_API_KEY reads its value under the variable's name, not "api-key", so
	// it is not the system key and stays an ordinary variable.
	usersOwn := client.EnvVar{Key: client.EnvVarAgentAPIKey, ValueFrom: &client.EnvVarValueFrom{
		SecretKeyRef: &client.SecretKeyRef{Name: "my-agent-dev-secrets", Key: client.EnvVarAgentAPIKey},
	}}
	userKey := spec.EnvironmentVariable{Key: client.EnvVarAgentAPIKey}
	userKey.SetValue("user-key")

	written, err := deployWithBaseEnvVars(t, []client.EnvVar{usersOwn}, []spec.EnvironmentVariable{userKey})

	require.NoError(t, err)
	assert.Contains(t, written, client.EnvVar{Key: client.EnvVarAgentAPIKey, Value: "user-key"})
	assert.NotContains(t, written, usersOwn)
}

// updateConfigurationsService returns a service for UpdateAgentConfigurations on agent "my-agent"
// in "dev", whose configurations are configs (or configsErr). Saved secret values are recorded
// in saved, and the env vars the binding is rewritten with in rewritten (nil env leaves it alone).
func updateConfigurationsService(t *testing.T, configs []models.EnvVars, configsErr error, existingSecretKeys []string) (
	svc AgentManagerService, saved *map[string]string, rewritten *[]client.EnvVar, rewriteCalled *bool,
) {
	t.Helper()
	saved, rewritten, rewriteCalled = new(map[string]string), new([]client.EnvVar), new(bool)
	secrets := &clientmocks.SecretManagementClientMock{
		GetSecretFunc: func(context.Context, string) (*secretmanagersvc.SecretInfo, error) {
			return &secretmanagersvc.SecretInfo{Keys: existingSecretKeys}, nil
		},
		PatchSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, values map[string]string, _ []string) (string, error) {
			*saved = values
			return location.SecretRefName(), nil
		},
		DeleteSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, string) error {
			return nil
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return configs, configsErr
		},
		SyncEditedWorkloadSecretsFunc: func(context.Context, string, string, string, []string) error {
			return nil
		},
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, env []client.EnvVar, _ []client.FileVar) error {
			*rewriteCalled = true
			*rewritten = env
			return nil
		},
	}
	injector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(context.Context, string, string, string, string) ([]client.EnvVar, error) {
			return []client.EnvVar{}, nil
		},
	}
	svc = NewAgentManagerService(nil, oc, secrets, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		injector, nil, nil, nil, nil, nil, nil, nil, nil, nil, discardLogger())
	svc.(*agentManagerService).agentConfigurationService = &llmSecretRefsStub{llmSecretRefs: map[string]struct{}{}}
	return svc, saved, rewritten, rewriteCalled
}

func TestUpdateAgentConfigurations_NoSystemKey_UpdatesUsersAgentAPIKey(t *testing.T) {
	// The agent has no system key (no env-injection trait); its AMP_AGENT_API_KEY is the user's.
	svc, saved, rewritten, _ := updateConfigurationsService(t, []models.EnvVars{
		{Key: client.EnvVarAgentAPIKey, IsSensitive: true, SecretRef: "my-agent-dev-secrets", SecretKey: client.EnvVarAgentAPIKey},
	}, nil, []string{client.EnvVarAgentAPIKey})
	env := spec.EnvironmentVariable{Key: client.EnvVarAgentAPIKey}
	env.SetIsSensitive(true)
	env.SetValue("new-user-key")

	err := svc.UpdateAgentConfigurations(tierGrantedCtx(t), "acme", "proj1", "my-agent",
		&spec.UpdateAgentConfigurationsRequest{EnvironmentName: "dev", Env: []spec.EnvironmentVariable{env}})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{client.EnvVarAgentAPIKey: "new-user-key"}, *saved, "the user's new value must be saved")
	require.Len(t, *rewritten, 1)
	assert.Equal(t, client.EnvVarAgentAPIKey, (*rewritten)[0].Key)
	require.NotNil(t, (*rewritten)[0].ValueFrom)
	assert.Equal(t, client.EnvVarAgentAPIKey, (*rewritten)[0].ValueFrom.SecretKeyRef.Key)
}

func TestUpdateAgentConfigurations_SystemKey_DropsUserEchoedCopy(t *testing.T) {
	systemKey := agentAPIKeyEnvVar("my-agent-agent-api-key-dev-secrets")
	svc, saved, rewritten, _ := updateConfigurationsService(t, []models.EnvVars{
		{Key: client.EnvVarAgentAPIKey, IsSensitive: true, SecretRef: systemKey.ValueFrom.SecretKeyRef.Name, SecretKey: secretmanagersvc.SecretKeyAPIKey},
	}, nil, nil)
	env := spec.EnvironmentVariable{Key: client.EnvVarAgentAPIKey}
	env.SetIsSensitive(true)
	env.SetValue("echoed")

	err := svc.UpdateAgentConfigurations(tierGrantedCtx(t), "acme", "proj1", "my-agent",
		&spec.UpdateAgentConfigurationsRequest{EnvironmentName: "dev", Env: []spec.EnvironmentVariable{env}})

	require.NoError(t, err)
	assert.NotContains(t, *saved, client.EnvVarAgentAPIKey, "an echoed copy must not be saved as a user secret")
	assert.Equal(t, []client.EnvVar{systemKey}, *rewritten, "the system key is carried forward unchanged")
}

func TestUpdateAgentConfigurations_FilesOnly_SkipsSystemManagedLookup(t *testing.T) {
	// A files-only update leaves the env vars alone, so a failing configurations lookup must not block it.
	svc, _, rewritten, rewriteCalled := updateConfigurationsService(t, nil, errors.New("openchoreo unavailable"), []string{"credentials.json"})
	file := spec.FileMount{Key: "credentials.json", MountPath: "/etc/credentials.json"}
	file.SetIsSensitive(true)
	file.SetValue("dummy")

	err := svc.UpdateAgentConfigurations(tierGrantedCtx(t), "acme", "proj1", "my-agent",
		&spec.UpdateAgentConfigurationsRequest{EnvironmentName: "dev", Files: []spec.FileMount{file}})

	require.NoError(t, err)
	assert.True(t, *rewriteCalled)
	assert.Nil(t, *rewritten, "a files-only update leaves the env vars alone")
}
