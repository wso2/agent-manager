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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// recordingMCPProvider records what a deployment's provider is asked to do.
type recordingMCPProvider struct {
	mu       sync.Mutex
	ensured  []MCPResourceServerRequest
	removed  []MCPResourceServerRequest
	removedA []string
	deleted  []MCPResourceServerRequest
	err      error
}

func (p *recordingMCPProvider) EnsureMCPResourceServer(_ context.Context, r MCPResourceServerRequest) (MCPResourceServerResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensured = append(p.ensured, r)
	return MCPResourceServerResult{ResourceServerID: "rs-1"}, p.err
}

func (p *recordingMCPProvider) RemoveMCPResourceServerAction(_ context.Context, r MCPResourceServerRequest, action string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removed = append(p.removed, r)
	p.removedA = append(p.removedA, action)
	return p.err
}

func (p *recordingMCPProvider) DeleteMCPResourceServer(_ context.Context, r MCPResourceServerRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deleted = append(p.deleted, r)
	return p.err
}

// useMCPMode installs the process-wide setting for one test and restores the default after it.
func useMCPMode(t *testing.T, mode thundersvc.IdentityManagementMode, provider MCPResourceServerProvider) {
	t.Helper()
	SetMCPResourceServerProvider(mode, provider)
	t.Cleanup(func() { SetMCPResourceServerProvider(thundersvc.IdentityManagementLegacyDirect, nil) })
}

// deployedIdentityProxy is a proxy deployed to envA (identity enabled) and envB (identity enabled),
// with the gateway lookups that derive its public identifier.
func deployedIdentityProxy(envA, envB uuid.UUID) (*models.MCPProxy, *MCPProxyService) {
	gwUUID := uuid.New()
	ctxPath := "/deepwiki"
	proxy := &models.MCPProxy{
		UUID:          uuid.New(),
		Artifact:      &models.Artifact{Handle: "deepwiki", Name: "Deep Wiki"},
		Configuration: models.MCPProxyConfig{Context: &ctxPath},
		Endpoints: []models.MCPProxyEndpoint{{
			Configuration: models.MCPEndpointConfig{Security: mcpIdentitySecurity()},
			Environments: []models.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: envA, ArtifactUUID: uuid.New()},
				{EnvironmentUUID: envB, ArtifactUUID: uuid.New()},
			},
		}},
	}
	svc := &MCPProxyService{
		infraManager: stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "dev", UUID: envA.String()}, {Name: "prod", UUID: envB.String()}}, nil
		}},
		deploymentRepo: &repomocks.DeploymentRepositoryMock{
			PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
				return []string{gwUUID.String()}, nil
			},
		},
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			EnvironmentMappingExistsFunc: func(_, _ string) (bool, error) { return true, nil },
			GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
				return &models.Gateway{UUID: gwUUID, Vhost: "https://gw.example.com"}, nil
			},
		},
		// resolver deliberately nil: the legacy per-environment Thunder must not be reached.
		logger: discardLogger(),
	}
	return proxy, svc
}

func TestMCPResourceServerEnsureGoesThroughTheProviderInCPManagedMode(t *testing.T) {
	provider := &recordingMCPProvider{}
	useMCPMode(t, thundersvc.IdentityManagementCPManaged, provider)
	envA, envB := uuid.New(), uuid.New()
	proxy, svc := deployedIdentityProxy(envA, envB)

	svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read", "write"})

	require.Len(t, provider.ensured, 2, "once for each deployed, identity-secured environment")
	for i, env := range []string{"dev", "prod"} {
		got := provider.ensured[i]
		require.Equal(t, "ou-1", got.OrganizationID, "the organization unit ID is always passed")
		require.Equal(t, "deepwiki", got.ProxyHandle)
		require.Equal(t, "Deep Wiki", got.DisplayName)
		require.Equal(t, env, got.Environment)
		require.Equal(t, "https://gw.example.com/deepwiki/mcp", got.Identifier)
		require.Equal(t, []string{"read", "write"}, got.Actions)
	}
}

func TestMCPResourceServerProviderFailureIsLoggedNotPropagated(t *testing.T) {
	provider := &recordingMCPProvider{err: ErrMCPResourceServerEnvironmentNotReady}
	useMCPMode(t, thundersvc.IdentityManagementCPManaged, provider)
	envA, envB := uuid.New(), uuid.New()
	proxy, svc := deployedIdentityProxy(envA, envB)

	require.NotPanics(t, func() {
		svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})
	})
	require.Len(t, provider.ensured, 2, "one environment failing does not stop the next")
}

func TestMCPResourceServerCPManagedWithoutAProviderFailsClosed(t *testing.T) {
	useMCPMode(t, thundersvc.IdentityManagementCPManaged, nil)
	envA, envB := uuid.New(), uuid.New()
	proxy, svc := deployedIdentityProxy(envA, envB)

	// A nil resolver would panic if the per-environment Thunder were reached.
	require.NotPanics(t, func() {
		svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})
	})

	provider := mcpResourceServerProviderFor(nil, discardLogger())
	req := MCPResourceServerRequest{OrganizationID: "ou-1", ProxyHandle: "deepwiki", Environment: "dev", Identifier: "https://x/y"}
	_, err := provider.EnsureMCPResourceServer(context.Background(), req)
	require.ErrorIs(t, err, ErrMCPResourceServerUnavailable)
	require.ErrorIs(t, provider.RemoveMCPResourceServerAction(context.Background(), req, "read"), ErrMCPResourceServerUnavailable)
	require.ErrorIs(t, provider.DeleteMCPResourceServer(context.Background(), req), ErrMCPResourceServerUnavailable)
}

func TestMCPResourceServerLegacyModeIgnoresAProvider(t *testing.T) {
	provider := &recordingMCPProvider{}
	useMCPMode(t, thundersvc.IdentityManagementLegacyDirect, provider)
	envA, envB := uuid.New(), uuid.New()
	proxy, svc := deployedIdentityProxy(envA, envB)

	var resolved []string
	svc.resolver = &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, ouID, _, envName string) (thundersvc.EnvIdentityClient, error) {
			require.Equal(t, "ou-1", ouID)
			resolved = append(resolved, envName)
			return &clientmocks.EnvIdentityClientMock{
				EnsureProxyResourceServerFunc: func(_ context.Context, handle, name, identifier string, actions []string) (string, error) {
					require.Equal(t, "deepwiki", handle)
					require.Equal(t, "Deep Wiki", name)
					require.Equal(t, "https://gw.example.com/deepwiki/mcp", identifier)
					require.Equal(t, []string{"read"}, actions)
					return "rs-1", nil
				},
			}, nil
		},
	}

	svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})

	require.Equal(t, []string{"dev", "prod"}, resolved, "each environment's own Thunder, as before")
	require.Empty(t, provider.ensured, "the provider is not used outside CP_MANAGED mode")
}

func TestMCPResourceServerScopeDeleteGoesThroughTheProvider(t *testing.T) {
	provider := &recordingMCPProvider{}
	useMCPMode(t, thundersvc.IdentityManagementCPManaged, provider)
	envA, envB := uuid.New(), uuid.New()
	proxy, _ := deployedIdentityProxy(envA, envB)
	scopeSvc := &mcpProxyScopeService{
		infraManager: stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "dev", UUID: envA.String()}, {Name: "prod", UUID: envB.String()}}, nil
		}},
		logger: discardLogger(),
	}

	scopeSvc.cleanupDeletedScope(context.Background(), "ou-1", proxy, "read")

	require.Equal(t, []string{"read", "read"}, provider.removedA)
	require.Len(t, provider.removed, 2)
	require.Equal(t, "ou-1", provider.removed[0].OrganizationID)
	require.Equal(t, "dev", provider.removed[0].Environment)
	require.Equal(t, "prod", provider.removed[1].Environment)
	require.Equal(t, "deepwiki", provider.removed[0].ProxyHandle)
}

func TestMCPResourceServerProxyDeleteGoesThroughTheProvider(t *testing.T) {
	provider := &recordingMCPProvider{}
	useMCPMode(t, thundersvc.IdentityManagementCPManaged, provider)
	envA, envB := uuid.New(), uuid.New()
	proxy, svc := deployedIdentityProxy(envA, envB)

	svc.cleanupProxyResourceServers(context.Background(), "ou-1", proxy, []models.MCPProxyScope{{Action: "read"}, {Action: "write"}})

	require.Len(t, provider.deleted, 2)
	for i, env := range []string{"dev", "prod"} {
		require.Equal(t, "ou-1", provider.deleted[i].OrganizationID)
		require.Equal(t, env, provider.deleted[i].Environment)
		require.Equal(t, "deepwiki", provider.deleted[i].ProxyHandle)
		require.Equal(t, []string{"read", "write"}, provider.deleted[i].Actions, "the actions the proxy had")
	}
}

func TestMCPResourceServerRequestsAreCheckedBeforeTheProvider(t *testing.T) {
	provider := &recordingMCPProvider{}
	useMCPMode(t, thundersvc.IdentityManagementCPManaged, provider)
	checked := mcpResourceServerProviderFor(nil, discardLogger())
	ctx := context.Background()
	valid := MCPResourceServerRequest{OrganizationID: "ou-1", ProxyHandle: "deepwiki", Environment: "dev", Identifier: "https://x/y"}

	for name, mutate := range map[string]func(*MCPResourceServerRequest){
		"no organization unit ID": func(r *MCPResourceServerRequest) { r.OrganizationID = " " },
		"no proxy handle":         func(r *MCPResourceServerRequest) { r.ProxyHandle = "" },
		"no environment":          func(r *MCPResourceServerRequest) { r.Environment = "" },
		"no identifier":           func(r *MCPResourceServerRequest) { r.Identifier = "" },
	} {
		t.Run(name, func(t *testing.T) {
			request := valid
			mutate(&request)
			_, err := checked.EnsureMCPResourceServer(ctx, request)
			require.ErrorIs(t, err, utils.ErrInvalidInput)
		})
	}
	require.Empty(t, provider.ensured, "an invalid request never reaches the provider")

	require.ErrorIs(t, checked.RemoveMCPResourceServerAction(ctx, valid, " "), utils.ErrInvalidInput)
	require.ErrorIs(t, checked.DeleteMCPResourceServer(ctx, MCPResourceServerRequest{ProxyHandle: "deepwiki", Environment: "dev"}), utils.ErrInvalidInput)
	require.Empty(t, provider.removed)
	require.Empty(t, provider.deleted)

	// A valid delete needs no identifier.
	require.NoError(t, checked.DeleteMCPResourceServer(ctx, MCPResourceServerRequest{OrganizationID: "ou-1", ProxyHandle: "deepwiki", Environment: "dev"}))
	require.Len(t, provider.deleted, 1)
}

func TestLegacyMCPResourceServerProvider(t *testing.T) {
	ctx := context.Background()
	request := MCPResourceServerRequest{OrganizationID: "ou-1", ProxyHandle: "deepwiki", Environment: "dev"}

	t.Run("a resolver failure is returned, not swallowed", func(t *testing.T) {
		provider := &legacyMCPResourceServerProvider{resolver: &clientmocks.EnvThunderResolverMock{
			ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
				return nil, errors.New("unreachable")
			},
		}}
		require.ErrorContains(t, provider.RemoveMCPResourceServerAction(ctx, request, "read"), "env-Thunder unavailable")
	})

	t.Run("no roles are swept when the resource server was never there", func(t *testing.T) {
		// ListRoles is not stubbed: the mock panics if it is called.
		provider := &legacyMCPResourceServerProvider{logger: discardLogger(), resolver: &clientmocks.EnvThunderResolverMock{
			ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
				return &clientmocks.EnvIdentityClientMock{
					DeleteProxyResourceServerActionFunc: func(context.Context, string, string) (string, error) { return "", nil },
				}, nil
			},
		}}
		require.NoError(t, provider.RemoveMCPResourceServerAction(ctx, request, "read"))
	})

	t.Run("a failed server delete skips the role sweep", func(t *testing.T) {
		provider := &legacyMCPResourceServerProvider{logger: discardLogger(), resolver: &clientmocks.EnvThunderResolverMock{
			ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
				return &clientmocks.EnvIdentityClientMock{
					DeleteProxyResourceServerFunc: func(context.Context, string) error { return errors.New("boom") },
				}, nil
			},
		}}
		request := request
		request.Actions = []string{"read"}
		require.ErrorContains(t, provider.DeleteMCPResourceServer(ctx, request), "boom")
	})
}
