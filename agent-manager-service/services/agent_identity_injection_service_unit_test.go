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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	// testIdentityOrg is deliberately NOT "default" (the config-pinned Thunder
	// namespace — see ThunderOrgNamespace) so tests can't pass by accident if
	// the token endpoint URL is ever built from ouID again instead of the
	// resolved namespace — a real regression this exact aliasing masked before.
	testIdentityOrg     = "019f4ab9-test-ou-id"
	testIdentityProject = "proj-a"
	testIdentityAgent   = "my-agent"
	testIdentityEnv     = "staging"
)

// testIdentitySecretRefName represents the provider-managed SecretReference
// name returned by CreateSecret and persisted in SecretRefPath.
func testIdentitySecretRefName() string {
	return "cred-agent-identity-a1b2c3d4"
}

func completedInternalBinding() *models.AgentThunderClient {
	return &models.AgentThunderClient{
		OUID:             testIdentityOrg,
		ProjectName:      testIdentityProject,
		AgentName:        testIdentityAgent,
		EnvironmentName:  testIdentityEnv,
		ProvisioningType: models.AgentProvisioningTypeInternal,
		Status:           models.AgentThunderStatusCompleted,
		ThunderAgentID:   "thunder-agent-1",
		ThunderClientID:  "client-abc",
		SecretRefPath:    testIdentitySecretRefName(),
	}
}

func identityRepoReturning(binding *models.AgentThunderClient, err error) *repomocks.AgentThunderClientRepositoryMock {
	return &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return binding, err
		},
	}
}

// noMCPConfigRepo returns an AgentConfigurationRepository mock reporting no
// MCP configurations at all — the default for tests that aren't exercising
// scope resolution, so they don't also need to stub
// OpenChoreoClient.GetEnvironmentFunc (resolveAgentIdentityScopes short-
// circuits before ever calling it when there's no agent configuration).
func noMCPConfigRepo() *repomocks.AgentConfigurationRepositoryMock {
	return &repomocks.AgentConfigurationRepositoryMock{
		ListMCPConfigsByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentConfiguration, error) {
			return []models.AgentConfiguration{}, nil
		},
	}
}

// noMCPProxyScopeRepo returns an MCPProxyScopeRepository mock with every
// method left unset — any unexpected call panics (moq's default), which is
// exactly right for tests whose scope resolution short-circuits before ever
// needing it (e.g. no agent configuration).
func noMCPProxyScopeRepo() *repomocks.MCPProxyScopeRepositoryMock {
	return &repomocks.MCPProxyScopeRepositoryMock{}
}

func newTestIdentityInjectionService(
	repo *repomocks.AgentThunderClientRepositoryMock,
	oc *clientmocks.OpenChoreoClientMock,
) AgentIdentityInjectionService {
	return NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyScopeRepo(), oc, "1h", discardLogger())
}

// injectableOCClient returns a base OpenChoreo client mock. SecretReference
// creation is deliberately not stubbed: injection must use the stored
// provider-managed reference directly and must not create another one.
func injectableOCClient() *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		CreateSecretReferenceFunc: func(context.Context, string, client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			panic("injection must not create a second SecretReference")
		},
	}
}

// TestAgentIdentityInjection_EnvVarsForEnvironment_UsesStoredSecretReference
// guards the core flow: the pod SecretKeyRef uses the exact provider-managed
// reference name persisted in binding.SecretRefPath.
func TestAgentIdentityInjection_EnvVarsForEnvironment_UsesStoredSecretReference(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	oc := injectableOCClient()
	svc := newTestIdentityInjectionService(repo, oc)

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err)
	require.Len(t, envVars, 4)

	expectedRefName := testIdentitySecretRefName()

	byKey := map[string]client.EnvVar{}
	for _, ev := range envVars {
		byKey[ev.Key] = ev
	}
	assert.Equal(t, "client-abc", byKey[client.EnvVarAgentIDClientID].Value)

	secretVar := byKey[client.EnvVarAgentIDClientSecret]
	require.NotNil(t, secretVar.ValueFrom, "client secret must be a SecretKeyRef, never a literal")
	require.NotNil(t, secretVar.ValueFrom.SecretKeyRef)
	assert.Equal(t, expectedRefName, secretVar.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, thundersvc.AgentSecretKeyClientSecret, secretVar.ValueFrom.SecretKeyRef.Key)
	assert.Empty(t, secretVar.Value)

	assert.Equal(t, thundersvc.ThunderTokenURL(ThunderOrgNamespace(), testIdentityEnv), byKey[client.EnvVarAgentIDTokenEndpoint].Value,
		"token endpoint must be built from the org's Thunder namespace, NOT the raw ouID")
	assert.Empty(t, byKey[client.EnvVarAgentIDScopes].Value, "no agent configuration means no MCP bindings, so no scopes to request")
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_UsesExistingReferenceForLegacyPath(t *testing.T) {
	binding := completedInternalBinding()
	binding.SecretRefPath = "wc-secrets/org/generic/credential"
	svc := newTestIdentityInjectionService(identityRepoReturning(binding, nil), injectableOCClient())

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err)
	for _, envVar := range envVars {
		if envVar.Key == client.EnvVarAgentIDClientSecret {
			require.NotNil(t, envVar.ValueFrom)
			require.NotNil(t, envVar.ValueFrom.SecretKeyRef)
			assert.Equal(t,
				agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv).SecretRefName(),
				envVar.ValueFrom.SecretKeyRef.Name,
			)
			return
		}
	}
	t.Fatal("Agent ID client secret env var was not injected")
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_UsesDeploymentTokenEndpoint(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	const publicTokenURL = "https://customer-test.example.com/oauth2/token"
	resolverCalled := false
	svc := NewAgentIdentityInjectionServiceWithTokenEndpointResolver(
		repo, noMCPConfigRepo(), noMCPProxyScopeRepo(), injectableOCClient(), "1h", discardLogger(),
		func(_ context.Context, ouID, orgNamespace, envName string) (string, error) {
			resolverCalled = true
			assert.Equal(t, testIdentityOrg, ouID)
			assert.Equal(t, ThunderOrgNamespace(), orgNamespace)
			assert.Equal(t, testIdentityEnv, envName)
			return publicTokenURL, nil
		},
	)

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err)
	require.True(t, resolverCalled)
	for _, envVar := range envVars {
		if envVar.Key == client.EnvVarAgentIDTokenEndpoint {
			assert.Equal(t, publicTokenURL, envVar.Value)
			return
		}
	}
	t.Fatal("Agent ID token endpoint env var was not injected")
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_PropagatesTokenEndpointError(t *testing.T) {
	expectedErr := errors.New("token endpoint unavailable")
	svc := NewAgentIdentityInjectionServiceWithTokenEndpointResolver(
		identityRepoReturning(completedInternalBinding(), nil), noMCPConfigRepo(), noMCPProxyScopeRepo(), injectableOCClient(), "1h", discardLogger(),
		func(context.Context, string, string, string) (string, error) { return "", expectedErr },
	)

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.ErrorIs(t, err, expectedErr)
	require.Nil(t, envVars)
}

// mcpProxyBinding is one EnvAgentMCPMapping's worth of fixture data: a proxy
// bound to an environment, carrying the given scope actions — the real chain
// resolveAgentIdentityScopes now walks (proxy -> its own MCPProxyScope rows),
// not a per-environment tool binding.
type mcpProxyBinding struct {
	envUUID      string
	proxyHandle  string
	scopeActions []string
}

// mcpBoundAgentConfigRepo returns an AgentConfigurationRepository mock whose
// ListMCPConfigsByAgent returns ONE AgentConfiguration row PER given binding
// — matching real production shape exactly: each MCP proxy an agent is
// configured with is stored as its OWN AgentConfiguration row (see
// createMCPConfig), never bundled as multiple EnvAgentMCPMapping rows under
// a single config. Getting this fixture shape right is what makes
// TestResolveAgentIdentityScopes_MultipleProxies_ReturnsSortedUnion an actual
// regression test for the "only one MCP's scopes survive" bug — a single
// config with multiple mappings inside it (the old fixture shape) could
// never occur in the real database (see uq_env_mcp_mapping) and would have
// passed even with the bug that only ever loaded one config row.
func mcpBoundAgentConfigRepo(bindings ...mcpProxyBinding) (*repomocks.AgentConfigurationRepositoryMock, *repomocks.MCPProxyScopeRepositoryMock) {
	configs := make([]models.AgentConfiguration, 0, len(bindings))
	scopesByProxy := map[uuid.UUID][]models.MCPProxyScope{}

	for _, b := range bindings {
		proxyUUID := uuid.New()
		envUUID := uuid.MustParse(b.envUUID)
		proxy := &models.MCPProxy{UUID: proxyUUID, Artifact: &models.Artifact{Handle: b.proxyHandle}}
		configs = append(configs, models.AgentConfiguration{
			TypeID: models.AgentConfigTypeIDMCP,
			EnvMCPMappings: []models.EnvAgentMCPMapping{
				{EnvironmentUUID: envUUID, MCPProxyUUID: proxyUUID, MCPProxy: proxy},
			},
		})
		scopes := make([]models.MCPProxyScope, 0, len(b.scopeActions))
		for _, action := range b.scopeActions {
			scopes = append(scopes, models.MCPProxyScope{MCPProxyUUID: proxyUUID, Action: action})
		}
		scopesByProxy[proxyUUID] = scopes
	}

	configRepo := &repomocks.AgentConfigurationRepositoryMock{
		ListMCPConfigsByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentConfiguration, error) {
			return configs, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyUUIDsFunc: func(_ context.Context, proxyUUIDs []uuid.UUID) ([]models.MCPProxyScope, error) {
			out := make([]models.MCPProxyScope, 0, len(proxyUUIDs))
			for _, id := range proxyUUIDs {
				out = append(out, scopesByProxy[id]...)
			}
			return out, nil
		},
	}
	return configRepo, scopeRepo
}

func TestResolveAgentIdentityScopes_NoAgentConfiguration_ReturnsEmpty(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	svc := NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyScopeRepo(), &clientmocks.OpenChoreoClientMock{}, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.NoError(t, err)
	assert.Empty(t, scopes)
}

func TestResolveAgentIdentityScopes_SingleProxySingleTool_ReturnsItsScopes(t *testing.T) {
	envUUID := "11111111-1111-1111-1111-111111111111"
	configRepo, scopeRepo := mcpBoundAgentConfigRepo(mcpProxyBinding{
		envUUID:      envUUID,
		proxyHandle:  "tickets",
		scopeActions: []string{"read"},
	})
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: envUUID}, nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, scopeRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.NoError(t, err)
	assert.Equal(t, []string{"tickets:read"}, scopes)
}

func TestResolveAgentIdentityScopes_MultipleProxies_ReturnsSortedUnion(t *testing.T) {
	envUUID := "22222222-2222-2222-2222-222222222222"
	configRepo, scopeRepo := mcpBoundAgentConfigRepo(
		mcpProxyBinding{
			envUUID:      envUUID,
			proxyHandle:  "tickets",
			scopeActions: []string{"read", "write"},
		},
		mcpProxyBinding{
			envUUID:      envUUID,
			proxyHandle:  "incidents",
			scopeActions: []string{"write"},
		},
	)
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: envUUID}, nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, scopeRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.NoError(t, err)
	assert.Equal(t, []string{"incidents:write", "tickets:read", "tickets:write"}, scopes,
		"must be the sorted union of every bound proxy's own scopes")
}

func TestResolveAgentIdentityScopes_MappingForDifferentEnvironment_Ignored(t *testing.T) {
	boundEnvUUID := "33333333-3333-3333-3333-333333333333"
	otherEnvUUID := "44444444-4444-4444-4444-444444444444"
	// The mapping is bound to otherEnvUUID, but the binding's own environment
	// is boundEnvUUID — simulating a proxy configured for a different
	// environment than the one this agent is actually deployed to. The
	// environment-UUID filter must skip this mapping entirely, so its scopes
	// are never even looked up.
	proxyUUID := uuid.New()
	configRepo := &repomocks.AgentConfigurationRepositoryMock{
		ListMCPConfigsByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentConfiguration, error) {
			return []models.AgentConfiguration{{EnvMCPMappings: []models.EnvAgentMCPMapping{
				{
					EnvironmentUUID: uuid.MustParse(otherEnvUUID),
					MCPProxyUUID:    proxyUUID,
					MCPProxy:        &models.MCPProxy{UUID: proxyUUID, Artifact: &models.Artifact{Handle: "tickets"}},
				},
			}}}, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyUUIDsFunc: func(context.Context, []uuid.UUID) ([]models.MCPProxyScope, error) {
			t.Fatal("must not look up scopes for a mapping bound to a different environment")
			return nil, nil
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: boundEnvUUID}, nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, scopeRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.NoError(t, err)
	assert.Empty(t, scopes)
}

// TestResolveAgentIdentityScopes_AgentConfigLoadError_PropagatesError guards
// against silently falling back to an empty scope list on a transient DB
// blip: every caller of this service already aborts on an error (deploy/
// promote/config-update all log "...to prevent credential loss" and stop),
// and ReconcileForEnvironment's no-needless-rollout guarantee depends on a
// trustworthy desired scope list — an empty list on a blip would look like a
// real scope change and cause a spurious rollout, then a second one once the
// blip cleared.
func TestResolveAgentIdentityScopes_AgentConfigLoadError_PropagatesError(t *testing.T) {
	failingRepo := &repomocks.AgentConfigurationRepositoryMock{
		ListMCPConfigsByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentConfiguration, error) {
			return nil, errors.New("db unavailable")
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		failingRepo, noMCPProxyScopeRepo(), &clientmocks.OpenChoreoClientMock{}, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.Error(t, err, "a DB lookup failure must propagate, not silently fail closed to an empty scope list")
	assert.Empty(t, scopes)
}

func TestResolveAgentIdentityScopes_EnvironmentResolveError_PropagatesError(t *testing.T) {
	envUUID := "55555555-5555-5555-5555-555555555555"
	configRepo, scopeRepo := mcpBoundAgentConfigRepo(mcpProxyBinding{
		envUUID:      envUUID,
		proxyHandle:  "tickets",
		scopeActions: []string{"read"},
	})
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return nil, errors.New("environment resolution failed")
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, scopeRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.Error(t, err, "an OpenChoreo environment lookup failure must propagate, not silently fail closed to an empty scope list")
	assert.Empty(t, scopes)
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_SkipStates(t *testing.T) {
	pending := completedInternalBinding()
	pending.Status = models.AgentThunderStatusPending

	failed := completedInternalBinding()
	failed.Status = models.AgentThunderStatusFailed

	external := completedInternalBinding()
	external.ProvisioningType = models.AgentProvisioningTypeExternal

	revoked := completedInternalBinding()
	revoked.SecretRefPath = ""

	noClientID := completedInternalBinding()
	noClientID.ThunderClientID = ""

	cases := []struct {
		name    string
		binding *models.AgentThunderClient
		repoErr error
	}{
		{name: "no binding", binding: nil, repoErr: repositories.ErrAgentThunderClientNotFound},
		{name: "pending binding", binding: pending},
		{name: "failed binding", binding: failed},
		{name: "external agent", binding: external},
		{name: "revoked credential", binding: revoked},
		{name: "missing client id", binding: noClientID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := identityRepoReturning(tc.binding, tc.repoErr)
			// OpenChoreoClientMock has every func nil: any OpenChoreo call would
			// panic — proving skip states never touch OpenChoreo at all.
			svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

			envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
			require.NoError(t, err)
			assert.Nil(t, envVars)
		})
	}
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_RepoErrorPropagates(t *testing.T) {
	repoErr := errors.New("db down")
	repo := identityRepoReturning(nil, repoErr)
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, repoErr, "a real repo error must surface, never be masked as 'nothing to inject'")
	assert.Nil(t, envVars)
}

func TestAgentIdentityInjection_InjectForEnvironment_PushesVarsIntoReleaseBinding(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	var injectedEnv string
	var injectedVars []client.EnvVar
	oc := injectableOCClient()
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, envName string, envVars []client.EnvVar) error {
		injectedEnv = envName
		injectedVars = envVars
		return nil
	}
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.Equal(t, testIdentityEnv, injectedEnv)
	assert.Len(t, injectedVars, 4)
}

func TestAgentIdentityInjection_InjectForEnvironment_NothingToInject_NoWorkloadCalls(t *testing.T) {
	repo := identityRepoReturning(nil, repositories.ErrAgentThunderClientNotFound)
	// UpdateReleaseBindingEnvVarsFunc nil — a call would panic.
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	assert.NoError(t, svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
}

func TestAgentIdentityInjection_InjectForEnvironment_WorkloadUpdateErrorPropagates(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	updateErr := errors.New("binding update failed")
	oc := injectableOCClient()
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
		return updateErr
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	assert.ErrorIs(t, err, updateErr)
}

// inSyncIdentityEnvVars returns the live env vars a fully-injected workload
// would report back for the no-MCP completedInternalBinding(): all four AgentID
// keys present, with an empty scope list (no MCP bindings). Used as the
// "already in sync" baseline for the reconcile tests.
func inSyncIdentityEnvVars() []models.EnvVars {
	refName := testIdentitySecretRefName()
	return []models.EnvVars{
		{Key: "AMP_OTEL_ENDPOINT", Value: "http://otel"}, // unrelated base var, must be ignored
		{Key: client.EnvVarAgentIDClientID, Value: "client-abc"},
		{Key: client.EnvVarAgentIDClientSecret, IsSensitive: true, SecretRef: refName, SecretKey: thundersvc.AgentSecretKeyClientSecret},
		{Key: client.EnvVarAgentIDTokenEndpoint, Value: thundersvc.ThunderTokenURL(ThunderOrgNamespace(), testIdentityEnv)},
		{Key: client.EnvVarAgentIDScopes, Value: ""},
	}
}

func TestAgentIdentityInjection_ReconcileForEnvironment_InSync_DoesNotWrite(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	oc := injectableOCClient()
	oc.GetComponentConfigurationsFunc = func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
		return inSyncIdentityEnvVars(), nil
	}
	// UpdateReleaseBindingEnvVarsFunc left nil — a call would panic, proving
	// an already-in-sync workload is never re-written (no needless pod roll).
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
}

func TestAgentIdentityInjection_ReconcileForEnvironment_MissingVars_Injects(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	injectedVars := 0
	oc := injectableOCClient()
	oc.GetComponentConfigurationsFunc = func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
		// Workload just came up from a first build; only base vars present, no identity vars.
		return []models.EnvVars{{Key: "AMP_OTEL_ENDPOINT", Value: "http://otel"}}, nil
	}
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
		injectedVars = len(envVars)
		return nil
	}
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.Equal(t, 4, injectedVars, "a workload missing the identity vars must be injected with the full set")
}

func TestAgentIdentityInjection_ReconcileForEnvironment_ScopeDrift_Reinjects(t *testing.T) {
	envUUID := "44444444-4444-4444-4444-444444444444"
	configRepo, scopeRepo := mcpBoundAgentConfigRepo(mcpProxyBinding{
		envUUID:      envUUID,
		proxyHandle:  "tickets",
		scopeActions: []string{"read"},
	})
	var injectedScopes string
	oc := injectableOCClient()
	oc.GetEnvironmentFunc = func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{UUID: envUUID}, nil
	}
	oc.GetComponentConfigurationsFunc = func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
		// All four keys present, but the live scopes are stale (empty) vs the
		// now-desired "tickets:read".
		refName := testIdentitySecretRefName()
		return []models.EnvVars{
			{Key: client.EnvVarAgentIDClientID, Value: "client-abc"},
			{Key: client.EnvVarAgentIDClientSecret, IsSensitive: true, SecretRef: refName, SecretKey: thundersvc.AgentSecretKeyClientSecret},
			{Key: client.EnvVarAgentIDTokenEndpoint, Value: thundersvc.ThunderTokenURL(ThunderOrgNamespace(), testIdentityEnv)},
			{Key: client.EnvVarAgentIDScopes, Value: ""},
		}, nil
	}
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
		for _, ev := range envVars {
			if ev.Key == client.EnvVarAgentIDScopes {
				injectedScopes = ev.Value
			}
		}
		return nil
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, scopeRepo, oc, "1h", discardLogger())

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.Equal(t, "tickets:read", injectedScopes, "a drifted scope list must be re-injected with the current scopes")
}

func TestAgentIdentityInjection_ReconcileForEnvironment_NothingToInject_NoReadOrWrite(t *testing.T) {
	repo := identityRepoReturning(nil, repositories.ErrAgentThunderClientNotFound)
	// GetComponentConfigurationsFunc / UpdateReleaseBindingEnvVarsFunc left nil —
	// a call would panic, proving an uninjectable binding short-circuits before
	// touching the workload at all.
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
}

func TestAgentIdentityInjection_ReconcileForEnvironment_ConfigReadError_Propagates(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	oc := injectableOCClient()
	oc.GetComponentConfigurationsFunc = func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
		return nil, errors.New("openchoreo unavailable")
	}
	// UpdateReleaseBindingEnvVarsFunc left nil — must not write when it can't
	// determine the current state.
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	assert.Error(t, err, "an unreadable current state must not silently proceed to a blind write")
}

// TestAgentIdentityInjection_RefreshAfterRotation_WaitsAndRollsPod guards
// rotation's contract: the provider updates the existing SecretReference, then
// the pod rolls only after waiting out the refresh cadence.
func TestAgentIdentityInjection_RefreshAfterRotation_WaitsAndRollsPod(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	rolled := make(chan struct{})
	oc := injectableOCClient()
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
		assert.Len(t, envVars, 4)
		close(rolled)
		return nil
	}

	svc := newTestIdentityInjectionService(repo, oc)
	impl, ok := svc.(*agentIdentityInjectionService)
	require.True(t, ok)
	var slept time.Duration
	impl.after = func(d time.Duration) <-chan time.Time {
		slept = d
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}

	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))

	select {
	case <-rolled:
	case <-time.After(2 * time.Second):
		t.Fatal("rotation must roll the pod (after the wait) so it starts with the refreshed secret value")
	}
	assert.Equal(t, secretSyncWaitDuration("1h"), slept, "the roll must wait out the configured refresh cadence before rolling")
}

// TestAgentIdentityInjection_RefreshAfterRotation_CoalescesRapidRotations
// guards against a second regenerate for the same binding, fired before the
// first one's deferred roll runs, causing two pod rollouts instead of one:
// only the latest rotation's roll must actually call UpdateReleaseBindingEnvVars.
func TestAgentIdentityInjection_RefreshAfterRotation_CoalescesRapidRotations(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	oc := injectableOCClient()

	var rollCount int32
	rolled := make(chan struct{}, 2)
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
		atomic.AddInt32(&rollCount, 1)
		rolled <- struct{}{}
		return nil
	}

	svc := newTestIdentityInjectionService(repo, oc)
	impl, ok := svc.(*agentIdentityInjectionService)
	require.True(t, ok)

	firstSleepStarted := make(chan struct{})
	secondRotationDone := make(chan struct{})
	var sleepMu sync.Mutex
	sleepCalls := 0
	impl.after = func(time.Duration) <-chan time.Time {
		sleepMu.Lock()
		sleepCalls++
		isFirstCall := sleepCalls == 1
		sleepMu.Unlock()
		ch := make(chan time.Time, 1)
		if isFirstCall {
			close(firstSleepStarted)
			go func() {
				<-secondRotationDone // hold until the second rotation has superseded this one
				ch <- time.Now()
			}()
		} else {
			ch <- time.Now()
		}
		return ch
	}

	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	<-firstSleepStarted

	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	close(secondRotationDone)

	select {
	case <-rolled:
	case <-time.After(2 * time.Second):
		t.Fatal("the latest rotation must still roll the pod")
	}

	// The superseded first rotation's goroutine has by now also run past its
	// (already unblocked) sleep and taken its token check — give it a moment
	// rather than asserting immediately after the one roll we expect.
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&rollCount),
		"a rotation superseded by a later one for the same binding must not also roll the pod")
}

// TestAgentIdentityInjection_RefreshAfterRotation_AbortsOnShutdown guards the
// wait itself: once the app's shutdown context is done, a deferred rollout
// must not roll the pod, even though its own wait timer never separately fires.
func TestAgentIdentityInjection_RefreshAfterRotation_AbortsOnShutdown(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	oc := injectableOCClient()

	rolled := make(chan struct{})
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
		close(rolled)
		return nil
	}

	svc := newTestIdentityInjectionService(repo, oc)
	impl, ok := svc.(*agentIdentityInjectionService)
	require.True(t, ok)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	impl.SetShutdownContext(shutdownCtx)
	shutdownCancel() // app is already shutting down before the rotation starts

	afterCalled := make(chan struct{})
	impl.after = func(time.Duration) <-chan time.Time {
		close(afterCalled)
		return make(chan time.Time) // never fires; only shutdownCtx.Done() can win the select
	}

	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))

	select {
	case <-afterCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("the deferred goroutine must still start its wait")
	}

	select {
	case <-rolled:
		t.Fatal("a rotation must not roll out the pod once the app has started shutting down")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestAgentIdentityInjection_RefreshAfterRotation_CancelsInFlightRollOnShutdown
// guards the roll itself, not just the wait before it: shutdown must cancel
// an already-in-flight UpdateReleaseBindingEnvVars call rather than letting
// it run to completion.
func TestAgentIdentityInjection_RefreshAfterRotation_CancelsInFlightRollOnShutdown(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	oc := injectableOCClient()

	callStarted := make(chan struct{})
	cancelled := make(chan struct{})
	oc.UpdateReleaseBindingEnvVarsFunc = func(ctx context.Context, _, _, _, _ string, _ []client.EnvVar) error {
		close(callStarted)
		<-ctx.Done() // blocks until the shutdown bridge cancels this call's context
		close(cancelled)
		return ctx.Err()
	}

	svc := newTestIdentityInjectionService(repo, oc)
	impl, ok := svc.(*agentIdentityInjectionService)
	require.True(t, ok)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	impl.SetShutdownContext(shutdownCtx)
	impl.after = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now() // wait completes immediately, moving straight into the roll
		return ch
	}

	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))

	select {
	case <-callStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("the roll must start calling UpdateReleaseBindingEnvVars")
	}

	shutdownCancel() // app starts shutting down while the roll call is in flight

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown must cancel an in-flight roll's context, not let it run to completion")
	}
}

func TestAgentIdentityInjection_RefreshAfterRotation_NoBinding_NoOp(t *testing.T) {
	repo := identityRepoReturning(nil, repositories.ErrAgentThunderClientNotFound)
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	assert.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
}

func TestAgentIdentityInjection_RemoveForEnvironment_RemovesVars(t *testing.T) {
	// Post-revoke state: still internal + completed, but no stored secret.
	binding := completedInternalBinding()
	binding.SecretRefPath = ""
	repo := identityRepoReturning(binding, nil)

	var removedKeys []string
	oc := &clientmocks.OpenChoreoClientMock{
		RemoveReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, envName string, keys []string) error {
			assert.Equal(t, testIdentityEnv, envName)
			removedKeys = keys
			return nil
		},
		// RemoveWorkloadEnvVarsFunc nil — includeWorkloadLevel=false must not touch the workload.
		DeleteSecretReferenceFunc: func(_ context.Context, _, _ string) error { return nil },
	}
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.RemoveForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv, false))

	expectedKeys := make([]string, 0, 4)
	for k := range AgentIdentityEnvVarKeys() {
		expectedKeys = append(expectedKeys, k)
	}
	assert.ElementsMatch(t, expectedKeys, removedKeys)
}

func TestAgentIdentityInjection_RemoveForEnvironment_IncludeWorkloadLevel(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	workloadRemoved := false
	oc := &clientmocks.OpenChoreoClientMock{
		RemoveReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, _ []string) error { return nil },
		RemoveWorkloadEnvVarsFunc: func(_ context.Context, _, _ string, keys []string) error {
			workloadRemoved = true
			assert.Len(t, keys, 4)
			return nil
		},
		DeleteSecretReferenceFunc: func(_ context.Context, _, _ string) error { return nil },
	}
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.RemoveForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv, true))
	assert.True(t, workloadRemoved)
}

func TestAgentIdentityInjection_RemoveForEnvironment_ExternalAgent_NoOp(t *testing.T) {
	binding := completedInternalBinding()
	binding.ProvisioningType = models.AgentProvisioningTypeExternal
	repo := identityRepoReturning(binding, nil)
	// All OpenChoreo funcs nil — any call would panic.
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	assert.NoError(t, svc.RemoveForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv, true))
}

// TestAgentIdentitySecretLocation_EntityNameIsAgentScoped guards the specific
// property agentIdentitySecretLocation must hold: EntityName includes the
// agent name, not just a fixed "agent-identity" marker — secretmanagersvc's
// SecretRefName only derives the SecretReference name from EntityName (+
// EnvironmentName), so two different agents in the same environment would
// collide onto the identical name (one agent's credential silently
// overwriting another's) if EntityName weren't agent-scoped. Collision
// avoidance for very long names beyond that is secretmanagersvc's own
// concern (SecretLocation.SecretRefName), not re-tested here.
func TestAgentIdentitySecretLocation_EntityNameIsAgentScoped(t *testing.T) {
	locA := agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, "agent-a", testIdentityEnv)
	locB := agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, "agent-b", testIdentityEnv)

	assert.NotEqual(t, locA.SecretRefName(), locB.SecretRefName(),
		"two different agents in the same environment must never derive the same SecretReference name")
	assert.Contains(t, locA.EntityName, "agent-a")
}

// TestAgentIdentitySecretLocation_IsDeterministic guards stable secret naming
// for create, rotate, and delete operations.
func TestAgentIdentitySecretLocation_IsDeterministic(t *testing.T) {
	loc1 := agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	loc2 := agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)

	assert.Equal(t, loc1.SecretRefName(), loc2.SecretRefName())
}

func TestAgentIdentityInjection_InjectForEnvironment_RetriesOnTransientConflictThenSucceeds(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	attempts := 0
	oc := injectableOCClient()
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
		attempts++
		if attempts < 2 {
			return utils.ErrConflict
		}
		return nil
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err, "a transient conflict on the first attempt must be retried, not surfaced as a failure")
	assert.Equal(t, 2, attempts)
}

// TestAgentIdentityInjection_InjectForEnvironment_RetriesOnInternalServerErrorConflict
// guards the realistic error path: OpenChoreo's UpdateReleaseBindingResp has no
// JSON409 field at all (see openchoreosvc/client.retryReleaseBindingUpdate's
// sibling comment), so a stale-resourceVersion conflict on this call can only
// ever surface as utils.ErrInternalServerError, never utils.ErrConflict. If the
// retry gate only matched ErrConflict, this exact scenario would never retry.
func TestAgentIdentityInjection_InjectForEnvironment_RetriesOnInternalServerErrorConflict(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	attempts := 0
	oc := injectableOCClient()
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
		attempts++
		if attempts < 2 {
			return utils.ErrInternalServerError
		}
		return nil
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err, "a stale-resourceVersion conflict surfaced as a 500 (OpenChoreo's actual behavior for this call) must be retried")
	assert.Equal(t, 2, attempts)
}

func TestAgentIdentityInjection_InjectForEnvironment_GivesUpAfterRetriesExhausted(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	attempts := 0
	oc := injectableOCClient()
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
		attempts++
		return utils.ErrConflict
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrConflict)
	assert.Equal(t, releaseBindingUpdateRetries, attempts, "must give up after the bounded retry budget, not retry forever")
}

func TestAgentIdentityInjection_InjectForEnvironment_DoesNotRetryPermanentError(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	attempts := 0
	permanentErr := errors.New("release binding validation failed")
	oc := injectableOCClient()
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
		attempts++
		return permanentErr
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, permanentErr)
	assert.Equal(t, 1, attempts, "a non-conflict error is permanent and must not be retried")
}

func TestAgentIdentityInjection_InjectForEnvironment_StopsRetryingOnContextCancel(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	oc := injectableOCClient()
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
		attempts++
		cancel() // simulate the caller's context being cancelled mid-retry
		return utils.ErrConflict
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(ctx, testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrConflict)
	assert.Equal(t, 1, attempts, "must stop retrying once the context is cancelled, not sleep out the full retry budget")
}
