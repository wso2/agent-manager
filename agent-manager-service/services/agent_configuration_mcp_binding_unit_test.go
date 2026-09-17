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
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// mcpVarRows builds the pair of env var rows (url + apikey) that configuring an MCP
// connection persists for an environment, whether or not that environment turned out
// to be deployable.
func mcpVarRows(configUUID uuid.UUID, envUUIDs ...uuid.UUID) []models.AgentEnvConfigVariable {
	rows := make([]models.AgentEnvConfigVariable, 0, len(envUUIDs)*2)
	for _, envUUID := range envUUIDs {
		rows = append(
			rows,
			models.AgentEnvConfigVariable{ConfigUUID: configUUID, EnvironmentUUID: envUUID, VariableKey: "url", VariableName: "BOOKING_URL"},
			models.AgentEnvConfigVariable{ConfigUUID: configUUID, EnvironmentUUID: envUUID, VariableKey: "apikey", VariableName: "BOOKING_API_KEY"},
		)
	}
	return rows
}

// mcpProxyServing builds an org-level MCP proxy with one endpoint bound to each of the
// given environments — the shape the reconcile reads to decide where a binding could go.
func mcpProxyServing(proxyUUID uuid.UUID, envUUIDs ...uuid.UUID) *models.MCPProxy {
	endpoints := make([]models.MCPProxyEndpoint, 0, len(envUUIDs))
	for _, envUUID := range envUUIDs {
		endpoints = append(endpoints, models.MCPProxyEndpoint{
			Environments: []models.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: envUUID, ArtifactUUID: uuid.New()},
			},
		})
	}
	return &models.MCPProxy{UUID: proxyUUID, Endpoints: endpoints}
}

// An environment the proxy serves but the connection has no mapping row for must be
// reported: that row is what the URL resolves through, so without it the connection's
// variables are injected empty and every tool call through it fails.
func TestMCPEnvsNeedingActivation_ReportsProxyEnvWithNoMapping(t *testing.T) {
	configUUID, proxyUUID := uuid.New(), uuid.New()
	devEnv, prodEnv := uuid.New(), uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
	}

	got := mcpEnvsNeedingActivation(mappings, mcpVarRows(configUUID, devEnv, prodEnv),
		mcpProxyServing(proxyUUID, devEnv, prodEnv))

	require.Equal(t, []uuid.UUID{prodEnv}, got,
		"prod is served by the proxy but has no mapping — it must be reported for backfill")
}

// The state the original bug left behind: the connection was configured for this
// environment (blank var rows written by provisionUnconfiguredMCPEnv, because the proxy
// had no endpoint there yet) but never bound. Once the proxy serves it, the backfill must
// pick it up — this is what promotion used to refuse for the lifetime of the agent.
func TestMCPEnvsNeedingActivation_ReportsConfiguredButUnboundEnvironment(t *testing.T) {
	configUUID, proxyUUID := uuid.New(), uuid.New()
	devEnv, envAddedLater := uuid.New(), uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
	}

	got := mcpEnvsNeedingActivation(mappings, mcpVarRows(configUUID, devEnv, envAddedLater),
		mcpProxyServing(proxyUUID, devEnv, envAddedLater))

	require.Equal(t, []uuid.UUID{envAddedLater}, got,
		"an environment in scope with no mapping must be reported")
}

// Each unmapped environment is reported once. A repeated environment would make the caller
// activate it twice and violate uq_env_mcp_mapping on the second pass. uq_proxy_env_single
// keeps a proxy to one endpoint per environment, but the scan must not depend on it.
func TestMCPEnvsNeedingActivation_ReportsEachUnmappedEnvOnce(t *testing.T) {
	configUUID, proxyUUID := uuid.New(), uuid.New()
	devEnv, prodEnv := uuid.New(), uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
	}
	proxy := mcpProxyServing(proxyUUID, devEnv, prodEnv, prodEnv)

	got := mcpEnvsNeedingActivation(mappings, mcpVarRows(configUUID, devEnv, prodEnv), proxy)

	require.Equal(t, []uuid.UUID{prodEnv}, got)
}

// An environment that already has a mapping is fully bound; re-activating it would mint a
// duplicate API key and violate uq_env_mcp_mapping.
func TestMCPEnvsNeedingActivation_SkipsAlreadyMappedEnv(t *testing.T) {
	configUUID, proxyUUID := uuid.New(), uuid.New()
	devEnv := uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
	}

	got := mcpEnvsNeedingActivation(mappings, mcpVarRows(configUUID, devEnv), mcpProxyServing(proxyUUID, devEnv))

	require.Empty(t, got)
}

// An environment the proxy does not serve cannot back a binding, so it is not a candidate
// however the connection is configured — activating there would deploy an agent pointing
// at a URL that resolves to nothing.
func TestMCPEnvsNeedingActivation_IgnoresEnvironmentProxyDoesNotServe(t *testing.T) {
	configUUID, proxyUUID := uuid.New(), uuid.New()
	devEnv, unservedEnv := uuid.New(), uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
	}

	got := mcpEnvsNeedingActivation(mappings, mcpVarRows(configUUID, devEnv, unservedEnv),
		mcpProxyServing(proxyUUID, devEnv))

	require.Empty(t, got)
	require.NotContains(t, got, unservedEnv)
}

// The configuration's own proxy column is what authorises binding an environment it has no
// mapping row for — and the only thing that can, for a connection with no mapping anywhere.
// That state is reachable: a proxy deployable in no environment when the connection was
// configured leaves exactly it, and it was previously unreachable from the proxy side.
func TestMCPConfigTargetsProxy_MatchesRecordedProxyWithNoMappings(t *testing.T) {
	proxyUUID := uuid.New()
	config := &models.AgentConfiguration{UUID: uuid.New(), MCPProxyUUID: &proxyUUID}

	require.True(t, mcpConfigTargetsProxy(config, proxyUUID))
}

func TestMCPConfigTargetsProxy_RejectsDifferentRecordedProxyWithNoMappings(t *testing.T) {
	proxyUUID, otherProxyUUID := uuid.New(), uuid.New()
	config := &models.AgentConfiguration{UUID: uuid.New(), MCPProxyUUID: &otherProxyUUID}

	require.False(t, mcpConfigTargetsProxy(config, proxyUUID),
		"with no mapping rows the column is the only record of intent, and it names another proxy")
}

// Unanimous mapping rows outrank the column, because they are the actual resource state
// while the column only caches an intent. This is the shape updateMCPConfig leaves when it
// re-points every environment and then fails to persist the reference: letting the stale
// column win made the configuration unreachable from both proxies' reconciles at once.
func TestMCPConfigTargetsProxy_UnanimousMappingsOutrankStaleColumn(t *testing.T) {
	newProxyUUID, staleColumnUUID := uuid.New(), uuid.New()
	config := &models.AgentConfiguration{
		UUID:         uuid.New(),
		MCPProxyUUID: &staleColumnUUID,
		EnvMCPMappings: []models.EnvAgentMCPMapping{
			{EnvironmentUUID: uuid.New(), MCPProxyUUID: newProxyUUID},
			{EnvironmentUUID: uuid.New(), MCPProxyUUID: newProxyUUID},
		},
	}

	require.True(t, mcpConfigTargetsProxy(config, newProxyUUID),
		"the proxy its environments actually point at must own the configuration")
	require.False(t, mcpConfigTargetsProxy(config, staleColumnUUID),
		"the proxy only the stale column names must not also claim it")
}

// A row migration044 left NULL — its environments named different proxies — falls back to
// the mapping rows, and is claimed only when they unanimously name this proxy.
func TestMCPConfigTargetsProxy_FallsBackToUnanimousMappings(t *testing.T) {
	proxyUUID := uuid.New()
	config := &models.AgentConfiguration{
		UUID: uuid.New(),
		EnvMCPMappings: []models.EnvAgentMCPMapping{
			{EnvironmentUUID: uuid.New(), MCPProxyUUID: proxyUUID},
			{EnvironmentUUID: uuid.New(), MCPProxyUUID: proxyUUID},
		},
	}

	require.True(t, mcpConfigTargetsProxy(config, proxyUUID))
}

// Genuinely divergent intent must not be guessed: binding the unmapped environment to
// either proxy would be as likely wrong as right.
func TestMCPConfigTargetsProxy_RejectsDivergentMappings(t *testing.T) {
	proxyUUID, otherProxyUUID := uuid.New(), uuid.New()
	config := &models.AgentConfiguration{
		UUID: uuid.New(),
		EnvMCPMappings: []models.EnvAgentMCPMapping{
			{EnvironmentUUID: uuid.New(), MCPProxyUUID: proxyUUID},
			{EnvironmentUUID: uuid.New(), MCPProxyUUID: otherProxyUUID},
		},
	}

	require.False(t, mcpConfigTargetsProxy(config, proxyUUID))
}

// With neither a recorded proxy nor a mapping row, nothing links the configuration to any
// proxy — this reconcile has no business claiming it.
func TestMCPConfigTargetsProxy_RejectsUnlinkedConfig(t *testing.T) {
	config := &models.AgentConfiguration{UUID: uuid.New()}

	require.False(t, mcpConfigTargetsProxy(config, uuid.New()))
}

// The console maps every pipeline environment to the same proxy, which is the intent the
// column records.
func TestSoleMCPProxyUUID_RecordsUnanimousProxy(t *testing.T) {
	proxyUUID := uuid.New()
	proxy := &models.MCPProxy{UUID: proxyUUID}

	got := soleMCPProxyUUID(map[string]*models.MCPProxy{"dev": proxy, "prod": proxy})

	require.NotNil(t, got)
	require.Equal(t, proxyUUID, *got)
}

// Divergent environments express no single environment-agnostic proxy, so the column stays
// NULL rather than recording one of them.
func TestSoleMCPProxyUUID_NilWhenEnvironmentsDisagree(t *testing.T) {
	got := soleMCPProxyUUID(map[string]*models.MCPProxy{
		"dev":  {UUID: uuid.New()},
		"prod": {UUID: uuid.New()},
	})

	require.Nil(t, got)
}

func TestSoleMCPProxyUUID_NilWhenNoEnvironmentsRequested(t *testing.T) {
	require.Nil(t, soleMCPProxyUUID(nil))
}

// unresolvedBindingsFixture builds the service ListUnresolvedMCPBindings needs: an
// environment lookup, the agent's configurations, and their per-environment variable rows.
// No configuration has an MCP mapping in the environment — the dead state promotion must
// refuse — so the URL each one resolves to is empty.
func unresolvedBindingsFixture(
	envUUID uuid.UUID,
	configs []models.AgentConfiguration,
	varsByConfig map[uuid.UUID][]models.AgentEnvConfigVariable,
) *agentConfigurationService {
	return &agentConfigurationService{
		ocClient: &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{Name: envName, UUID: envUUID.String()}, nil
			},
		},
		agentConfigRepo: &repomocks.AgentConfigurationRepositoryMock{
			ListByAgentFunc: func(_ context.Context, _, _, _ string, _, _ int) ([]models.AgentConfiguration, error) {
				return configs, nil
			},
		},
		envVariableRepo: &repomocks.AgentEnvConfigVariableRepositoryMock{
			ListByConfigAndEnvFunc: func(_ context.Context, configUUID, _ uuid.UUID) ([]models.AgentEnvConfigVariable, error) {
				return varsByConfig[configUUID], nil
			},
		},
		envMCPMappingRepo: &repomocks.EnvAgentMCPMappingRepositoryMock{
			ListByConfigFunc: func(_ context.Context, _ uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				return []models.EnvAgentMCPMapping{}, nil
			},
		},
	}
}

// A connection configured for the environment — its variable rows are there, so its URL and
// API key are injected into the workload — but with no mapping backing them resolves to an
// empty URL. That is the connection promotion must refuse to carry over.
func TestListUnresolvedMCPBindings_ReportsConfiguredConnectionWithNoResolvableURL(t *testing.T) {
	envUUID := uuid.New()
	bookingUUID := uuid.New()
	configs := []models.AgentConfiguration{
		{UUID: bookingUUID, Name: "booking", TypeID: models.AgentConfigTypeIDMCP},
	}
	varsByConfig := map[uuid.UUID][]models.AgentEnvConfigVariable{
		bookingUUID: mcpVarRows(bookingUUID, envUUID),
	}

	svc := unresolvedBindingsFixture(envUUID, configs, varsByConfig)

	got, err := svc.ListUnresolvedMCPBindings(context.Background(), "my-agent", "acme", "proj1", "staging")

	require.NoError(t, err)
	require.Equal(t, map[string]struct{}{"booking": {}}, got)
}

// No variable rows in the environment means the connection was never offered there, so
// nothing is injected and nothing is broken. Reporting it would block promotions that are
// perfectly safe.
func TestListUnresolvedMCPBindings_SkipsConnectionNotConfiguredForEnvironment(t *testing.T) {
	envUUID := uuid.New()
	bookingUUID := uuid.New()
	configs := []models.AgentConfiguration{
		{UUID: bookingUUID, Name: "booking", TypeID: models.AgentConfigTypeIDMCP},
	}

	svc := unresolvedBindingsFixture(envUUID, configs, nil)

	got, err := svc.ListUnresolvedMCPBindings(context.Background(), "my-agent", "acme", "proj1", "staging")

	require.NoError(t, err)
	require.Empty(t, got)
}

// An LLM configuration also owns injected system-managed variables, but its URL comes from
// the provider rather than an MCP mapping. Scanning it here would report every LLM binding
// as a broken MCP connection.
func TestListUnresolvedMCPBindings_IgnoresNonMCPConfigurations(t *testing.T) {
	envUUID := uuid.New()
	llmUUID := uuid.New()
	configs := []models.AgentConfiguration{
		{UUID: llmUUID, Name: "openai", TypeID: models.AgentConfigTypeIDLLM},
	}
	varsByConfig := map[uuid.UUID][]models.AgentEnvConfigVariable{
		llmUUID: mcpVarRows(llmUUID, envUUID),
	}

	svc := unresolvedBindingsFixture(envUUID, configs, varsByConfig)

	got, err := svc.ListUnresolvedMCPBindings(context.Background(), "my-agent", "acme", "proj1", "staging")

	require.NoError(t, err)
	require.Empty(t, got)
}

// The environment-side trigger fans out over every proxy serving the environment whose
// gateway just arrived, and only those. A proxy with no endpoint there has nothing that
// assignment could have made bindable, and reconciling it would cost a config scan per
// proxy in the organization on every gateway assignment.
func TestMCPProxiesBoundToEnvironment_SelectsOnlyProxiesServingTheEnvironment(t *testing.T) {
	targetEnv, otherEnv := uuid.New(), uuid.New()
	serving := mcpProxyServing(uuid.New(), otherEnv, targetEnv)
	notServing := mcpProxyServing(uuid.New(), otherEnv)

	svc := &agentConfigurationService{
		mcpProxyRepo: &repomocks.MCPProxyRepositoryMock{
			ListFunc: func(_ context.Context, _ string, _, offset int) ([]*models.MCPProxy, error) {
				if offset > 0 {
					return nil, nil
				}
				return []*models.MCPProxy{serving, notServing}, nil
			},
		},
	}

	got, err := svc.mcpProxiesBoundToEnvironment(context.Background(), "acme", targetEnv)

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, serving.UUID, got[0].UUID)
}

// The scan pages until a short page ends it. Stopping after the first page would silently
// skip every proxy past the page size, which reads as "no agents to reconcile" and is
// exactly the kind of silent partial pass this fix exists to remove.
func TestMCPProxiesBoundToEnvironment_PagesUntilExhausted(t *testing.T) {
	targetEnv := uuid.New()
	firstPage := make([]*models.MCPProxy, 0, mcpProxyScanPageSize)
	for i := 0; i < mcpProxyScanPageSize; i++ {
		firstPage = append(firstPage, mcpProxyServing(uuid.New(), targetEnv))
	}
	onSecondPage := mcpProxyServing(uuid.New(), targetEnv)

	pagesRequested := 0
	svc := &agentConfigurationService{
		mcpProxyRepo: &repomocks.MCPProxyRepositoryMock{
			ListFunc: func(_ context.Context, _ string, _, offset int) ([]*models.MCPProxy, error) {
				pagesRequested++
				if offset == 0 {
					return firstPage, nil
				}
				return []*models.MCPProxy{onSecondPage}, nil
			},
		},
	}

	got, err := svc.mcpProxiesBoundToEnvironment(context.Background(), "acme", targetEnv)

	require.NoError(t, err)
	require.Equal(t, 2, pagesRequested, "a full page must be followed by another request")
	require.Len(t, got, mcpProxyScanPageSize+1, "the proxy on the second page must not be skipped")
}

// mcpConfigUUIDsForProxy is the discovery step Change 3b turns on: it must reach a
// connection that references the proxy by column even when no mapping row does, because
// that is the state a proxy deployable in no environment leaves behind — and the state the
// old mapping-row-only query could never see.
func TestMCPConfigUUIDsForProxy_FindsConfigReferencedOnlyByColumn(t *testing.T) {
	proxyUUID := uuid.New()
	configUUID := uuid.New()

	svc := &agentConfigurationService{
		agentConfigRepo: &repomocks.AgentConfigurationRepositoryMock{
			ListMCPConfigsByProxyFunc: func(_ context.Context, _ string, gotProxy uuid.UUID) ([]models.AgentConfiguration, error) {
				require.Equal(t, proxyUUID, gotProxy)
				return []models.AgentConfiguration{{UUID: configUUID, MCPProxyUUID: &proxyUUID}}, nil
			},
		},
		envMCPMappingRepo: &repomocks.EnvAgentMCPMappingRepositoryMock{
			ListByMCPProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				return nil, nil // no mapping anywhere — the whole point of this case
			},
		},
	}

	got, err := svc.mcpConfigUUIDsForProxy(context.Background(), "acme", &models.MCPProxy{UUID: proxyUUID})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{configUUID}, got)
}

// The mapping-row side stays live for rows migration044 left NULL (divergent per-environment
// proxies). Dropping it would silently orphan every legacy connection the column cannot
// describe.
func TestMCPConfigUUIDsForProxy_FindsConfigReferencedOnlyByMappingRow(t *testing.T) {
	proxyUUID := uuid.New()
	legacyConfigUUID := uuid.New()

	svc := &agentConfigurationService{
		agentConfigRepo: &repomocks.AgentConfigurationRepositoryMock{
			ListMCPConfigsByProxyFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]models.AgentConfiguration, error) {
				return nil, nil // column is NULL, so the column query cannot see it
			},
		},
		envMCPMappingRepo: &repomocks.EnvAgentMCPMappingRepositoryMock{
			ListByMCPProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				return []models.EnvAgentMCPMapping{{ConfigUUID: legacyConfigUUID, MCPProxyUUID: proxyUUID}}, nil
			},
		},
	}

	got, err := svc.mcpConfigUUIDsForProxy(context.Background(), "acme", &models.MCPProxy{UUID: proxyUUID})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{legacyConfigUUID}, got)
}

// The normal steady state hits BOTH sides — a bound connection has a column reference and a
// mapping row per environment. It must be visited once: reconcileConfigMCPBindings activates
// every unmapped environment it finds, so a duplicate visit would try to bind each of them
// twice and the second pass would collide with uq_env_mcp_mapping.
func TestMCPConfigUUIDsForProxy_DedupesConfigFoundOnBothSides(t *testing.T) {
	proxyUUID := uuid.New()
	configUUID := uuid.New()

	svc := &agentConfigurationService{
		agentConfigRepo: &repomocks.AgentConfigurationRepositoryMock{
			ListMCPConfigsByProxyFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]models.AgentConfiguration, error) {
				return []models.AgentConfiguration{{UUID: configUUID, MCPProxyUUID: &proxyUUID}}, nil
			},
		},
		envMCPMappingRepo: &repomocks.EnvAgentMCPMappingRepositoryMock{
			ListByMCPProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				// Two environments of the same configuration, as a bound connection has.
				return []models.EnvAgentMCPMapping{
					{ConfigUUID: configUUID, MCPProxyUUID: proxyUUID, EnvironmentUUID: uuid.New()},
					{ConfigUUID: configUUID, MCPProxyUUID: proxyUUID, EnvironmentUUID: uuid.New()},
				}, nil
			},
		},
	}

	got, err := svc.mcpConfigUUIDsForProxy(context.Background(), "acme", &models.MCPProxy{UUID: proxyUUID})

	require.NoError(t, err)
	require.Len(t, got, 1, "a configuration on both sides must be visited exactly once")
	require.Equal(t, configUUID, got[0])
}

// The regression guard for `amctl agent mcp unset --env`. Removing an environment from a
// connection deletes its mapping row AND its env var rows, leaving a state identical to
// "never configured" except for the absent rows. The proxy still serves the environment,
// so a candidate scan that ignored scope would re-bind it on the next proxy update —
// re-minting an API key and re-injecting variables the user deliberately removed.
func TestMCPEnvsNeedingActivation_DoesNotResurrectRemovedEnvironment(t *testing.T) {
	configUUID, proxyUUID := uuid.New(), uuid.New()
	devEnv, unsetEnv := uuid.New(), uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
	}
	// unsetEnv is served by the proxy but has no var rows — it was unset on purpose.
	varsAfterUnset := mcpVarRows(configUUID, devEnv)

	got := mcpEnvsNeedingActivation(mappings, varsAfterUnset, mcpProxyServing(proxyUUID, devEnv, unsetEnv))

	require.Empty(t, got, "an environment removed from the connection must stay removed")
}

// A connection with no scope recorded anywhere has nothing to bind, however many
// environments the proxy serves. Guards the empty-vars edge of the scope check.
func TestMCPEnvsNeedingActivation_NoScopeMeansNoCandidates(t *testing.T) {
	proxyUUID := uuid.New()
	devEnv, prodEnv := uuid.New(), uuid.New()

	got := mcpEnvsNeedingActivation(nil, nil, mcpProxyServing(proxyUUID, devEnv, prodEnv))

	require.Empty(t, got)
}
