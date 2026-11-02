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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// agentPlacementFixture is a proxy with one environment (testMCPEnvUUID) that has a managed and a
// self-hosted egress gateway, and the agent references to it the test sets.
type agentPlacementFixture struct {
	proxy      *models.MCPProxy
	managed    *models.Gateway
	selfHosted *models.Gateway
	mappings   []models.EnvAgentMCPMapping
	configs    []models.AgentConfiguration
	svc        *MCPProxyService
}

func newAgentPlacementFixture(t *testing.T) *agentPlacementFixture {
	t.Helper()
	f := &agentPlacementFixture{
		proxy:      &models.MCPProxy{UUID: uuid.New(), Handle: "weather"},
		managed:    newGateway(t, models.GatewayRoleBoth, true),
		selfHosted: selfHostedGateway(t),
	}
	f.svc = &MCPProxyService{
		gatewayRepo: gatewayFixtureRepo(t, testMCPEnvUUID, []*models.Gateway{f.managed, f.selfHosted}),
		repo: &repomocks.MCPProxyRepositoryMock{
			GetByHandleFunc: func(context.Context, string, string) (*models.MCPProxy, error) { return f.proxy, nil },
		},
		envMCPMappingRepo: &repomocks.EnvAgentMCPMappingRepositoryMock{
			ListByMCPProxyFunc: func(_ context.Context, proxyUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				assert.Equal(t, f.proxy.UUID, proxyUUID)
				return f.mappings, nil
			},
		},
		agentConfigRepo: &repomocks.AgentConfigurationRepositoryMock{
			ListMCPConfigsByProxyFunc: func(_ context.Context, _ string, proxyUUID uuid.UUID) ([]models.AgentConfiguration, error) {
				assert.Equal(t, f.proxy.UUID, proxyUUID)
				return f.configs, nil
			},
		},
		logger: discardLogger(),
	}
	return f
}

func (f *agentPlacementFixture) agentBoundIn(envUUID string) {
	f.mappings = append(f.mappings, models.EnvAgentMCPMapping{
		ConfigUUID: uuid.New(), EnvironmentUUID: uuid.MustParse(envUUID), MCPProxyUUID: f.proxy.UUID, ArtifactUUID: uuid.New(),
	})
}

func (f *agentPlacementFixture) agentReferences() {
	f.configs = append(f.configs, models.AgentConfiguration{UUID: uuid.New()})
}

func (f *agentPlacementFixture) update(gatewayID string) error {
	_, err := f.svc.Update(context.Background(), "org1", f.proxy.Handle, &models.MCPProxyDTO{
		Name: "Weather", Version: "v1.0", Endpoints: []models.MCPProxyEndpointDTO{endpointBoundToGateway(gatewayID)},
	})
	return err
}

// The proxy owner removed the environment (tearing down its artifact) while an agent stayed bound
// there, and now re-adds it on a self-hosted gateway. The new artifact has no anchor, so nothing
// else stops it: the agent's next deploy would point at the customer gateway. The service has no
// database, so reaching the write would panic: the refusal comes before any mutation.
func TestMCPProxyUpdate_ReAddingAnAgentBoundEnvironmentOnSelfHostedIsRefused(t *testing.T) {
	f := newAgentPlacementFixture(t)
	f.agentBoundIn(testMCPEnvUUID)
	f.agentReferences()

	err := f.update(f.selfHosted.UUID.String())

	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.ErrorContains(t, err, testMCPEnvUUID)
	assert.ErrorContains(t, err, "managed gateway")
}

// An agent connection that records the proxy but has no mapping row yet would be bound by the binding
// reconcile or a promotion later, so it blocks a self-hosted placement in any environment.
func TestMCPProxyUpdate_AgentReferenceWithoutMappingsRefusesSelfHosted(t *testing.T) {
	f := newAgentPlacementFixture(t)
	f.agentReferences()

	err := f.update(f.selfHosted.UUID.String())

	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.ErrorContains(t, err, testMCPEnvUUID)
}

func TestValidateMCPEndpointPlacement_AgentReferences(t *testing.T) {
	ctx := context.Background()
	place := func(f *agentPlacementFixture, gatewayID string, existing map[string]uuid.UUID) error {
		return f.svc.validateMCPEndpointPlacement(ctx, "org1", f.proxy.UUID,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(gatewayID)}, existing)
	}

	t.Run("an agent bound in another environment refuses through its proxy reference", func(t *testing.T) {
		f := newAgentPlacementFixture(t)
		f.agentBoundIn("7b1f2f0e-0d0c-4e9a-9a53-6a0c5f1d2b11")
		f.agentReferences()
		require.ErrorIs(t, place(f, f.selfHosted.UUID.String(), nil), utils.ErrInvalidInput)
	})

	t.Run("no agent references allow a self-hosted placement", func(t *testing.T) {
		f := newAgentPlacementFixture(t)
		require.NoError(t, place(f, f.selfHosted.UUID.String(), nil))
	})

	t.Run("agent references allow a managed placement", func(t *testing.T) {
		f := newAgentPlacementFixture(t)
		f.agentBoundIn(testMCPEnvUUID)
		f.agentReferences()
		require.NoError(t, place(f, f.managed.UUID.String(), nil))
	})

	t.Run("an environment already on the self-hosted gateway keeps its placement", func(t *testing.T) {
		// Not a re-homing: the artifact is already there, and agents are refused that environment
		// on their side. Editing the proxy must stay possible.
		f := newAgentPlacementFixture(t)
		f.agentReferences()
		f.svc.deploymentRepo = deployedTo(f.selfHosted.UUID.String())
		require.NoError(t, place(f, f.selfHosted.UUID.String(), map[string]uuid.UUID{testMCPEnvUUID: uuid.New()}))
	})

	t.Run("create has no agent references to check", func(t *testing.T) {
		f := newAgentPlacementFixture(t)
		f.svc.envMCPMappingRepo = nil
		f.svc.agentConfigRepo = nil
		require.NoError(t, f.svc.validateMCPEndpointPlacement(ctx, "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(f.selfHosted.UUID.String())}, nil))
	})
}
