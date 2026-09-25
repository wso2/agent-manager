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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// mcpIdentitySecurity builds an identity(OAuth2)-enabled SecurityConfig,
// mirroring identityEnabledEndpoint's shape in mcp_proxy_scope_service_unit_test.go.
func mcpIdentitySecurity() *models.SecurityConfig {
	on := true
	return &models.SecurityConfig{Enabled: &on, Identity: &models.IdentitySecurity{Enabled: &on}}
}

func TestMCPResourceServerIdentifier_DerivesPublicURI(t *testing.T) {
	envUUID := uuid.New()
	gwUUID := uuid.New()
	artifactUUID := uuid.New()
	ctxPath := "/github"
	proxy := &models.MCPProxy{
		UUID:          uuid.New(),
		Handle:        "gh-proxy",
		Configuration: models.MCPProxyConfig{Context: &ctxPath},
		Endpoints: []models.MCPProxyEndpoint{{
			Environments: []models.MCPProxyEndpointEnvironment{{
				EnvironmentUUID: envUUID,
				ArtifactUUID:    artifactUUID,
			}},
		}},
	}
	svc := &MCPProxyService{
		infraManager: stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "dev", UUID: envUUID.String()}}, nil
		}},
		deploymentRepo: &repomocks.DeploymentRepositoryMock{
			GetDeployedGatewaysByProviderFunc: func(providerUUID uuid.UUID, _ string) ([]string, error) {
				require.Equal(t, artifactUUID, providerUUID)
				return []string{gwUUID.String()}, nil
			},
		},
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			EnvironmentMappingExistsFunc: func(gatewayID, environmentID string) (bool, error) {
				return gatewayID == gwUUID.String() && environmentID == envUUID.String(), nil
			},
			GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
				return &models.Gateway{UUID: gwUUID, Vhost: "https://gw.example.com"}, nil
			},
		},
		logger: discardLogger(),
	}

	envID, err := svc.EnvironmentUUIDByName(context.Background(), "ou-1", "dev")
	require.NoError(t, err)
	require.Equal(t, envUUID, envID)

	id, err := svc.MCPResourceServerIdentifier(context.Background(), "ou-1", envID, proxy)

	require.NoError(t, err)
	require.Equal(t, "https://gw.example.com/github/mcp", id)
}

func TestMCPResourceServerIdentifier_NotDeployedToEnvironment(t *testing.T) {
	proxy := &models.MCPProxy{UUID: uuid.New(), Handle: "gh-proxy"}
	svc := &MCPProxyService{logger: discardLogger()}

	_, err := svc.MCPResourceServerIdentifier(context.Background(), "ou-1", uuid.New(), proxy)

	require.ErrorIs(t, err, ErrMCPProxyNotDeployedToEnvironment)
}

func TestEnvironmentUUIDByName_NotFound(t *testing.T) {
	svc := &MCPProxyService{
		infraManager: stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "dev", UUID: uuid.NewString()}}, nil
		}},
		logger: discardLogger(),
	}

	_, err := svc.EnvironmentUUIDByName(context.Background(), "ou-1", "prod")

	require.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
}

// TestEnsureResourceServersForProxy_RegistersOnlyIdentitySecuredDeployedEnvironments
// is the core regression guard for the bug this method exists to fix: a proxy's
// resource server must be (re)registered, with its given full action set, in
// every environment it is both deployed to and identity(OAuth2)-secured in —
// and nowhere else.
func TestEnsureResourceServersForProxy_RegistersOnlyIdentitySecuredDeployedEnvironments(t *testing.T) {
	envA, envB := uuid.New(), uuid.New()
	gwUUID := uuid.New()
	ctxPath := "/deepwiki"
	proxy := &models.MCPProxy{
		UUID:          uuid.New(),
		Artifact:      &models.Artifact{Handle: "deepwiki"},
		Configuration: models.MCPProxyConfig{Context: &ctxPath},
		Endpoints: []models.MCPProxyEndpoint{
			{
				Configuration: models.MCPEndpointConfig{Security: mcpIdentitySecurity()},
				Environments: []models.MCPProxyEndpointEnvironment{
					{EnvironmentUUID: envA, ArtifactUUID: uuid.New()},
				},
			},
			{
				// No security configured at all (API-key or unsecured) — must be skipped.
				Environments: []models.MCPProxyEndpointEnvironment{
					{EnvironmentUUID: envB, ArtifactUUID: uuid.New()},
				},
			},
		},
	}

	var resolvedEnvs []string
	var ensuredActions [][]string
	svc := &MCPProxyService{
		infraManager: stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{
				{Name: "env-a", UUID: envA.String()},
				{Name: "env-b", UUID: envB.String()},
			}, nil
		}},
		deploymentRepo: &repomocks.DeploymentRepositoryMock{
			GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
				return []string{gwUUID.String()}, nil
			},
		},
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			EnvironmentMappingExistsFunc: func(_, _ string) (bool, error) { return true, nil },
			GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
				return &models.Gateway{UUID: gwUUID, Vhost: "https://gw.example.com"}, nil
			},
		},
		resolver: &clientmocks.EnvThunderResolverMock{
			ResolveIdentityFunc: func(_ context.Context, _, _, envName string) (thundersvc.EnvIdentityClient, error) {
				resolvedEnvs = append(resolvedEnvs, envName)
				return &clientmocks.EnvIdentityClientMock{
					EnsureProxyResourceServerFunc: func(_ context.Context, _, _, _ string, actions []string) (string, error) {
						ensuredActions = append(ensuredActions, actions)
						return "rs-1", nil
					},
				}, nil
			},
		},
		logger: discardLogger(),
	}

	svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read", "write"})

	require.Equal(t, []string{"env-a"}, resolvedEnvs, "must resolve only the identity-secured environment")
	require.Len(t, ensuredActions, 1)
	require.ElementsMatch(t, []string{"read", "write"}, ensuredActions[0])
}

// TestEnsureResourceServersForProxy_SkipsUndeployedEnvironment guards the exact
// real-world scenario a live investigation traced: scopes get saved (or a role
// created) for a proxy whose (endpoint, environment) row exists but was never
// actually deployed (ArtifactUUID is nil) — that environment must be skipped
// entirely, never reaching the resolver at all (a nil resolver here would
// panic if it were).
func TestEnsureResourceServersForProxy_SkipsUndeployedEnvironment(t *testing.T) {
	envA := uuid.New()
	proxy := &models.MCPProxy{
		UUID:     uuid.New(),
		Artifact: &models.Artifact{Handle: "deepwiki"},
		Endpoints: []models.MCPProxyEndpoint{{
			Configuration: models.MCPEndpointConfig{Security: mcpIdentitySecurity()},
			Environments: []models.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: envA, ArtifactUUID: uuid.Nil},
			},
		}},
	}
	svc := &MCPProxyService{
		infraManager: stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "env-a", UUID: envA.String()}}, nil
		}},
		logger: discardLogger(),
		// resolver deliberately left nil: reaching it would panic, proving the
		// undeployed environment never gets past the ArtifactUUID check.
	}

	require.NotPanics(t, func() {
		svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})
	})
}

// TestEnsureResourceServersForProxy_SkipsWhenNoIdentityEndpoint guards the
// cheap early exit: a proxy with no identity-secured endpoint anywhere must
// never touch the infra manager or env-Thunder for this.
func TestEnsureResourceServersForProxy_SkipsWhenNoIdentityEndpoint(t *testing.T) {
	proxy := &models.MCPProxy{
		UUID:      uuid.New(),
		Artifact:  &models.Artifact{Handle: "deepwiki"},
		Endpoints: []models.MCPProxyEndpoint{{}},
	}
	svc := &MCPProxyService{logger: discardLogger()}
	// infraManager/resolver deliberately left nil: reaching either would panic.

	require.NotPanics(t, func() {
		svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})
	})
}

// TestEnsureResourceServersForProxy_SurvivesResolverError guards the
// best-effort contract: scopes/environment bindings can be saved while
// env-Thunder is temporarily unreachable, so a resolver failure must be
// logged and skipped, never panic or propagate.
func TestEnsureResourceServersForProxy_SurvivesResolverError(t *testing.T) {
	envA := uuid.New()
	proxy := &models.MCPProxy{
		UUID:     uuid.New(),
		Artifact: &models.Artifact{Handle: "deepwiki"},
		Endpoints: []models.MCPProxyEndpoint{{
			Configuration: models.MCPEndpointConfig{Security: mcpIdentitySecurity()},
			Environments: []models.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: envA, ArtifactUUID: uuid.New()},
			},
		}},
	}
	svc := &MCPProxyService{
		infraManager: stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "env-a", UUID: envA.String()}}, nil
		}},
		resolver: &clientmocks.EnvThunderResolverMock{
			ResolveIdentityFunc: func(_ context.Context, _, _, _ string) (thundersvc.EnvIdentityClient, error) {
				return nil, errors.New("env-thunder unreachable")
			},
		},
		logger: discardLogger(),
	}

	require.NotPanics(t, func() {
		svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})
	})
}
