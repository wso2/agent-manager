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
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// homeIdentityClient is a CP managed client of the home environment: the mock,
// a read capability for resource servers, and activation.
type homeIdentityClient struct {
	*clientmocks.EnvIdentityClientMock
	existing    map[string]bool
	activations int
	activateErr error
	existsErr   error
	checked     []string
}

func (c *homeIdentityClient) ProxyResourceServerExists(_ context.Context, handle, _ string) (bool, error) {
	c.checked = append(c.checked, handle)
	return c.existing[handle], c.existsErr
}

func (c *homeIdentityClient) ActivateAgentIdentity(context.Context, string) error { return nil }
func (c *homeIdentityClient) ActivateMCPResourceServer(context.Context, string) error {
	c.activations++
	return c.activateErr
}

func (c *homeIdentityClient) ActivateDeletedMCPResourceServer(context.Context, string) error {
	return nil
}

// blindHomeClient has no read capability, as an older client.
type blindHomeClient struct {
	*clientmocks.EnvIdentityClientMock
	activations int
}

func (c *blindHomeClient) ActivateAgentIdentity(context.Context, string) error { return nil }
func (c *blindHomeClient) ActivateMCPResourceServer(context.Context, string) error {
	c.activations++
	return nil
}

func (c *blindHomeClient) ActivateDeletedMCPResourceServer(context.Context, string) error { return nil }

type preflightFixture struct {
	svc      *MCPProxyService
	ensured  []string
	actions  map[string][]string
	envsUsed []string
	proxies  map[string]*models.MCPProxy
}

func newPreflightFixture(t *testing.T, client func(f *preflightFixture) thundersvc.EnvIdentityClient) *preflightFixture {
	t.Helper()
	homeUUID, stagingUUID, gwUUID := uuid.New(), uuid.New(), uuid.New()
	f := &preflightFixture{actions: map[string][]string{}, proxies: map[string]*models.MCPProxy{}}
	deployedIn := func(handle string, envs ...uuid.UUID) *models.MCPProxy {
		ctxPath := "/" + handle
		environments := []models.MCPProxyEndpointEnvironment{}
		for _, env := range envs {
			environments = append(environments, models.MCPProxyEndpointEnvironment{EnvironmentUUID: env, ArtifactUUID: uuid.New()})
		}
		return &models.MCPProxy{
			UUID: uuid.New(), Handle: handle, Artifact: &models.Artifact{Handle: handle, Name: handle + " server"},
			Configuration: models.MCPProxyConfig{Context: &ctxPath},
			Endpoints:     []models.MCPProxyEndpoint{{Environments: environments}},
		}
	}
	f.proxies["booking"] = deployedIn("booking", homeUUID, stagingUUID)
	f.proxies["weather"] = deployedIn("weather", homeUUID, stagingUUID)
	f.proxies["stagingonly"] = deployedIn("stagingonly", stagingUUID)

	f.svc = &MCPProxyService{
		repo: &repomocks.MCPProxyRepositoryMock{
			GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) {
				if proxy, ok := f.proxies[handle]; ok {
					return proxy, nil
				}
				return nil, gorm.ErrRecordNotFound
			},
		},
		mcpProxyScopeRepo: &repomocks.MCPProxyScopeRepositoryMock{
			ListByProxyFunc: func(context.Context, uuid.UUID) ([]models.MCPProxyScope, error) {
				return []models.MCPProxyScope{{Action: "read"}, {Action: "write"}}, nil
			},
		},
		infraManager: stubInfraManager{listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{
				{Name: "development", UUID: homeUUID.String()}, {Name: "staging", UUID: stagingUUID.String()},
			}, nil
		}},
		deploymentRepo: &repomocks.DeploymentRepositoryMock{
			GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) { return []string{gwUUID.String()}, nil },
		},
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			EnvironmentMappingExistsFunc: func(string, string) (bool, error) { return true, nil },
			GetByUUIDFunc: func(string) (*models.Gateway, error) {
				return &models.Gateway{UUID: gwUUID, Vhost: "https://gw.example.com"}, nil
			},
		},
		logger: discardLogger(),
	}
	identity := client(f)
	f.svc.resolver = &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, _, _, env string) (thundersvc.EnvIdentityClient, error) {
			f.envsUsed = append(f.envsUsed, env)
			if env != "development" {
				return nil, thundersvc.ErrIdentityEnvironmentUnsupported
			}
			return identity, nil
		},
	}
	return f
}

func (f *preflightFixture) ensuringMock() *clientmocks.EnvIdentityClientMock {
	return &clientmocks.EnvIdentityClientMock{
		EnsureProxyResourceServerFunc: func(_ context.Context, handle, _, _ string, actions []string) (string, error) {
			f.ensured = append(f.ensured, handle)
			f.actions[handle] = actions
			return "rs-" + handle, nil
		},
	}
}

func preflightQuery(handles ...string) MCPResourceServerPreflightQuery {
	return MCPResourceServerPreflightQuery{OUID: "org-id", HomeEnvironment: "development", ProxyHandles: handles}
}

func TestPreflightCreatesMissingResourceServersInTheHomeEnvironmentOnlyAndActivatesOnce(t *testing.T) {
	var home *homeIdentityClient
	f := newPreflightFixture(t, func(f *preflightFixture) thundersvc.EnvIdentityClient {
		home = &homeIdentityClient{EnvIdentityClientMock: f.ensuringMock(), existing: map[string]bool{"weather": true}}
		return home
	})

	result, err := f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery("booking", "weather", "booking"))

	require.NoError(t, err)
	require.Empty(t, result.Blockers)
	require.Equal(t, []string{"booking"}, f.ensured, "only the missing one is created")
	require.Equal(t, []string{"read", "write"}, f.actions["booking"], "with the full action list of the MCP server")
	require.Equal(t, []string{"booking", "weather"}, home.checked, "each handle is checked once")
	require.Equal(t, 1, home.activations)
	require.Equal(t, []string{"development"}, f.envsUsed, "no other environment is ever resolved")
}

func TestPreflightActivatesOncePerPreflightNotPerServer(t *testing.T) {
	var home *homeIdentityClient
	f := newPreflightFixture(t, func(f *preflightFixture) thundersvc.EnvIdentityClient {
		home = &homeIdentityClient{EnvIdentityClientMock: f.ensuringMock(), existing: map[string]bool{}}
		return home
	})
	_, err := f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery("booking", "weather"))
	require.NoError(t, err)
	require.Equal(t, []string{"booking", "weather"}, f.ensured)
	require.Equal(t, 1, home.activations)
}

func TestPreflightWithNothingMissingWritesAndActivatesNothing(t *testing.T) {
	var home *homeIdentityClient
	f := newPreflightFixture(t, func(f *preflightFixture) thundersvc.EnvIdentityClient {
		home = &homeIdentityClient{EnvIdentityClientMock: f.ensuringMock(), existing: map[string]bool{"booking": true, "weather": true}}
		return home
	})
	_, err := f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery("booking", "weather"))
	require.NoError(t, err)
	require.Empty(t, f.ensured)
	require.Zero(t, home.activations)
}

func TestPreflightBlocksAnMCPServerNotDeployedInTheHomeEnvironmentAndCreatesNothing(t *testing.T) {
	var home *homeIdentityClient
	f := newPreflightFixture(t, func(f *preflightFixture) thundersvc.EnvIdentityClient {
		home = &homeIdentityClient{EnvIdentityClientMock: f.ensuringMock(), existing: map[string]bool{}}
		return home
	})

	result, err := f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery("booking", "stagingonly"))

	require.NoError(t, err)
	require.Len(t, result.Blockers, 1)
	require.Contains(t, result.Blockers[0].Message, "where identity is managed")
	require.Equal(t, "stagingonly server", result.Blockers[0].ResourceName, "named by display name")
	require.Contains(t, result.Blockers[0].Message, "stagingonly server")
	require.NotContains(t, result.Blockers[0].Message, "https://")
	require.Empty(t, f.ensured, "a blocked preflight creates nothing, not even the others")
	require.Zero(t, home.activations)

	result, err = f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery("ghost"))
	require.NoError(t, err)
	require.Contains(t, result.Blockers[0].Message, "not an MCP server that Agent Manager manages")
}

func TestPreflightWithAClientThatCannotReadEnsuresEverythingOnceIdempotently(t *testing.T) {
	var blind *blindHomeClient
	f := newPreflightFixture(t, func(f *preflightFixture) thundersvc.EnvIdentityClient {
		blind = &blindHomeClient{EnvIdentityClientMock: f.ensuringMock()}
		return blind
	})
	_, err := f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery("booking", "weather"))
	require.NoError(t, err)
	require.Equal(t, []string{"booking", "weather"}, f.ensured)
	require.Equal(t, 1, blind.activations)
}

func TestPreflightFailuresAreTypedAndSayNothingInternal(t *testing.T) {
	var home *homeIdentityClient
	f := newPreflightFixture(t, func(f *preflightFixture) thundersvc.EnvIdentityClient {
		home = &homeIdentityClient{EnvIdentityClientMock: f.ensuringMock(), existing: map[string]bool{}}
		return home
	})

	home.activateErr = errors.New("apply failed at https://cp.internal.svc/gateways/gw-1")
	_, err := f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery("booking"))
	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionUnavailable, typed.Kind)
	require.NotContains(t, typed.Message, "cp.internal")

	home.activateErr = nil
	home.EnsureProxyResourceServerFunc = func(context.Context, string, string, string, []string) (string, error) {
		return "", errors.New("thunder down")
	}
	_, err = f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery("booking"))
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionUnavailable, typed.Kind)
	require.Contains(t, typed.Message, "booking server")

	home.EnsureProxyResourceServerFunc = f.ensuringMock().EnsureProxyResourceServerFunc
	home.existsErr = errors.New("thunder down")
	_, err = f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery("booking"))
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionUnavailable, typed.Kind)
}

func TestPreflightIsAConflictWhenTheHomeEnvironmentCannotBeWritten(t *testing.T) {
	f := newPreflightFixture(t, func(f *preflightFixture) thundersvc.EnvIdentityClient {
		return &homeIdentityClient{EnvIdentityClientMock: f.ensuringMock()}
	})
	query := preflightQuery("booking")
	query.HomeEnvironment = "staging"

	_, err := f.svc.EnsureResourceServersForAgent(context.Background(), query)

	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionConflict, typed.Kind)
	require.Empty(t, f.ensured)
}

func TestPreflightWithNoMCPServersDoesNothing(t *testing.T) {
	f := newPreflightFixture(t, func(f *preflightFixture) thundersvc.EnvIdentityClient {
		return &homeIdentityClient{EnvIdentityClientMock: f.ensuringMock()}
	})
	result, err := f.svc.EnsureResourceServersForAgent(context.Background(), preflightQuery())
	require.NoError(t, err)
	require.Empty(t, result.Blockers)
	require.Empty(t, f.envsUsed, "not even the identity client is resolved")
}
