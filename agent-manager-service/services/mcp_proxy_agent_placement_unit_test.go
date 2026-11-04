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
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// agentPlacementFixture is a proxy with one environment (testMCPEnvUUID) that has a managed and a
// self-hosted egress gateway, and the agents using it the test sets. The agent-type lookup answers
// from agentIsExternal; an agent missing from it is unknown to OpenChoreo.
type agentPlacementFixture struct {
	proxy           *models.MCPProxy
	managed         *models.Gateway
	selfHosted      *models.Gateway
	mappings        []models.EnvAgentMCPMapping
	configs         []models.AgentConfiguration
	agentIsExternal map[string]bool
	lookups         []string
	svc             *MCPProxyService
}

func newAgentPlacementFixture(t *testing.T) *agentPlacementFixture {
	t.Helper()
	f := &agentPlacementFixture{
		proxy:           &models.MCPProxy{UUID: uuid.New(), Handle: "weather"},
		managed:         newGateway(t, models.GatewayRoleBoth, true),
		selfHosted:      selfHostedGateway(t),
		agentIsExternal: map[string]bool{},
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
		isExternalAgent: func(_ context.Context, ouID, agentName string) (bool, error) {
			assert.Equal(t, "org1", ouID)
			f.lookups = append(f.lookups, agentName)
			external, known := f.agentIsExternal[agentName]
			if !known {
				return false, fmt.Errorf("component %s: %w", agentName, utils.ErrNotFound)
			}
			return external, nil
		},
		logger: discardLogger(),
	}
	return f
}

// agentUses records an MCP configuration of agentName that references the proxy.
func (f *agentPlacementFixture) agentUses(agentName string) models.AgentConfiguration {
	config := models.AgentConfiguration{UUID: uuid.New(), OUID: "org1", ProjectName: "default", AgentID: agentName}
	f.configs = append(f.configs, config)
	return config
}

func (f *agentPlacementFixture) internalAgentUses(agentName string) models.AgentConfiguration {
	f.agentIsExternal[agentName] = false
	return f.agentUses(agentName)
}

func (f *agentPlacementFixture) externalAgentUses(agentName string) models.AgentConfiguration {
	f.agentIsExternal[agentName] = true
	return f.agentUses(agentName)
}

func (f *agentPlacementFixture) bind(config models.AgentConfiguration, envUUID string) {
	f.mappings = append(f.mappings, models.EnvAgentMCPMapping{
		ConfigUUID: config.UUID, EnvironmentUUID: uuid.MustParse(envUUID), MCPProxyUUID: f.proxy.UUID, ArtifactUUID: uuid.New(),
	})
}

func (f *agentPlacementFixture) agentBoundIn(envUUID string) {
	f.bind(f.internalAgentUses("bound-agent"), envUUID)
}

func (f *agentPlacementFixture) agentReferences() {
	f.internalAgentUses("referencing-agent")
}

func (f *agentPlacementFixture) place(gatewayID string, existing map[string]uuid.UUID) error {
	_, err := f.svc.validateMCPEndpointPlacement(context.Background(), "org1", f.proxy.UUID,
		[]models.MCPProxyEndpointDTO{endpointBoundToGateway(gatewayID)}, existing)
	return err
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
	place := func(f *agentPlacementFixture, gatewayID string, existing map[string]uuid.UUID) error {
		return f.place(gatewayID, existing)
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
		_, err := f.svc.validateMCPEndpointPlacement(context.Background(), "org1", uuid.Nil,
			[]models.MCPProxyEndpointDTO{endpointBoundToGateway(f.selfHosted.UUID.String())}, nil)
		require.NoError(t, err)
	})
}

// External agents don't run on the platform, so they may reach the proxy through a self-hosted
// gateway. Two configurations of one agent cost one lookup.
func TestMCPProxySelfHostedPlacement_OnlyExternalAgentsIsAllowed(t *testing.T) {
	f := newAgentPlacementFixture(t)
	f.bind(f.externalAgentUses("support-bot"), testMCPEnvUUID)
	f.externalAgentUses("support-bot")
	f.externalAgentUses("triage-bot")

	require.NoError(t, f.place(f.selfHosted.UUID.String(), nil))
	assert.ElementsMatch(t, []string{"support-bot", "triage-bot"}, f.lookups)
}

func TestMCPProxySelfHostedPlacement_MixedInternalAndExternalIsRefused(t *testing.T) {
	f := newAgentPlacementFixture(t)
	f.bind(f.externalAgentUses("support-bot"), testMCPEnvUUID)
	f.internalAgentUses("billing-agent")

	err := f.place(f.selfHosted.UUID.String(), nil)

	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.ErrorContains(t, err, "use a separate MCP proxy for self-hosted gateways")
}

// A configuration whose agent OpenChoreo no longer knows is left over from a deleted agent: it
// runs nowhere, so it does not hold the proxy on managed gateways.
func TestMCPProxySelfHostedPlacement_OrphanConfigIsSkipped(t *testing.T) {
	f := newAgentPlacementFixture(t)
	f.bind(f.agentUses("deleted-agent"), testMCPEnvUUID)
	f.externalAgentUses("support-bot")

	require.NoError(t, f.place(f.selfHosted.UUID.String(), nil))
	assert.Contains(t, f.lookups, "deleted-agent")
}

func TestMCPProxySelfHostedPlacement_LookupErrorFailsClosed(t *testing.T) {
	f := newAgentPlacementFixture(t)
	f.externalAgentUses("support-bot")
	unavailable := errors.New("openchoreo unavailable")
	f.svc.isExternalAgent = func(context.Context, string, string) (bool, error) { return false, unavailable }

	err := f.place(f.selfHosted.UUID.String(), nil)

	require.ErrorIs(t, err, unavailable)
}

// A mapping row whose configuration no longer references the proxy names no agent to ask about,
// so it counts as an internal agent bound in its environment.
func TestMCPProxySelfHostedPlacement_UnmatchedMappingRowCountsAsInternal(t *testing.T) {
	const otherEnvUUID = "7b1f2f0e-0d0c-4e9a-9a53-6a0c5f1d2b11"

	t.Run("in its environment", func(t *testing.T) {
		f := newAgentPlacementFixture(t)
		f.externalAgentUses("support-bot")
		f.bind(models.AgentConfiguration{UUID: uuid.New()}, testMCPEnvUUID)

		err := f.place(f.selfHosted.UUID.String(), nil)

		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, testMCPEnvUUID)
	})

	t.Run("not in another environment", func(t *testing.T) {
		f := newAgentPlacementFixture(t)
		f.externalAgentUses("support-bot")
		f.bind(models.AgentConfiguration{UUID: uuid.New()}, otherEnvUUID)

		require.NoError(t, f.place(f.selfHosted.UUID.String(), nil))
	})
}

// The lookup calls OpenChoreo, so it runs only when an environment newly lands on a self-hosted gateway.
func TestMCPProxySelfHostedPlacement_NoLookupWithoutSelfHostedPlacement(t *testing.T) {
	newFixture := func(t *testing.T) *agentPlacementFixture {
		f := newAgentPlacementFixture(t)
		f.bind(f.internalAgentUses("billing-agent"), testMCPEnvUUID)
		f.externalAgentUses("support-bot")
		f.svc.isExternalAgent = func(context.Context, string, string) (bool, error) {
			panic("agent type looked up without a new self-hosted placement")
		}
		return f
	}

	t.Run("managed placement", func(t *testing.T) {
		f := newFixture(t)
		require.NoError(t, f.place(f.managed.UUID.String(), nil))
	})

	t.Run("existing self-hosted placement", func(t *testing.T) {
		f := newFixture(t)
		f.svc.deploymentRepo = deployedTo(f.selfHosted.UUID.String())
		require.NoError(t, f.place(f.selfHosted.UUID.String(), map[string]uuid.UUID{testMCPEnvUUID: uuid.New()}))
	})
}
