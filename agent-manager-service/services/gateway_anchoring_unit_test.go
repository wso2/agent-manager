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

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// This file covers the six newly-anchored gateway resolution sites end to end, each
// with a two-egress-capable environment (`both` + `egress`, from gateway_roles_unit_test.go)
// so a resolver that fell back to environment-only selection instead of anchoring on the
// artifact's existing deployment would be caught by a flip to ambiguity or to the wrong
// gateway. Site 2 (validateMCPEndpointSecurity) already has 3 tests in
// mcp_proxy_service_unit_test.go from the 11-16 fix round and is not duplicated here.
//
// Per-site seam driven:
//   - site 1 (deployMCPProxyEndpoints, mcp_proxy_deployment.go): the public method
//     directly. RedeployMCPProxy is a one-line wrapper around it, so the three scope
//     mutation call sites (create/update/delete) funnel through this exact same resolve
//     and are not re-tested separately.
//   - site 3 (resolveProxyURL, monitor_executor.go): the unexported method on a bare
//     &monitorExecutor{} literal, per the brief's shape.
//   - site 5 (resolveMonitorGateway, monitor_manager.go): the unexported method on a bare
//     &monitorManagerService{} literal. The same helper backs both the monitor-create and
//     monitor-update call sites, so one test covers both.
//   - sites 4 and 6 (resolveGatewayForMCPArtifact, agent_configuration_service.go): the
//     unexported wrapper method, driven once per call site (update at :2288, create at
//     :1342) since both are the identical resolver reached from different callers.

func TestAnchoring_MCPProxyUpdate(t *testing.T) {
	env := uuid.New()
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	artifactUUID := uuid.New()

	newProxy := func() *models.MCPProxy {
		return &models.MCPProxy{
			UUID:   uuid.New(),
			Handle: "proxy-handle",
			Endpoints: []models.MCPProxyEndpoint{{
				UUID:   uuid.New(),
				Handle: "endpoint-handle",
				Configuration: models.MCPEndpointConfig{
					Upstream: &models.UpstreamEndpoint{URL: "https://upstream.example.com"},
				},
				Environments: []models.MCPProxyEndpointEnvironment{{
					EnvironmentUUID: env,
					ArtifactUUID:    artifactUUID,
				}},
			}},
		}
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(context.Context, uuid.UUID) ([]models.MCPProxyScope, error) {
			return []models.MCPProxyScope{}, nil
		},
	}

	t.Run("anchors to the gateway the endpoint is already deployed to", func(t *testing.T) {
		hub := &stubEventHub{}
		svc := &MCPProxyService{
			gatewayRepo:       gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			mcpProxyScopeRepo: scopeRepo,
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
					return []string{both.UUID.String()}, nil
				},
				CreateWithLimitEnforcementFunc: func(*models.Deployment, int) error { return nil },
			},
			gatewayEventsService: &GatewayEventsService{hub: hub},
			logger:               discardLogger(),
		}

		err := svc.deployMCPProxyEndpoints(context.Background(), newProxy(), "org")
		require.NoError(t, err)
		// The deployment event's GatewayID is the anchoring signal: a second egress
		// gateway must never steal an already-deployed (endpoint, environment) binding.
		require.Len(t, hub.published, 1)
		require.Equal(t, both.UUID.String(), hub.published[0].GatewayID)
	})

	t.Run("ambiguity fires only with no deployment", func(t *testing.T) {
		// No gatewayEventsService/CreateWithLimitEnforcementFunc stubbed: if resolution
		// wrongly proceeded to deploy, the unstubbed moq call would panic.
		svc := &MCPProxyService{
			gatewayRepo:       gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			mcpProxyScopeRepo: scopeRepo,
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
					return nil, nil
				},
			},
			logger: discardLogger(),
		}

		err := svc.deployMCPProxyEndpoints(context.Background(), newProxy(), "org")
		require.ErrorIs(t, err, errAmbiguousEgressGateway)
	})

	t.Run("caller-specified gatewayId for a new binding selects that gateway", func(t *testing.T) {
		// No prior deployment (the binding is new): resolution falls through to
		// resolveEgressGatewayForEnvironment, which must honor the caller's explicit
		// choice among the two egress-capable candidates instead of erroring ambiguous.
		hub := &stubEventHub{}
		svc := &MCPProxyService{
			gatewayRepo:       gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			mcpProxyScopeRepo: scopeRepo,
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
					return nil, nil
				},
				CreateWithLimitEnforcementFunc: func(*models.Deployment, int) error { return nil },
			},
			gatewayEventsService: &GatewayEventsService{hub: hub},
			logger:               discardLogger(),
		}

		requested := egress.UUID.String()
		p := newProxy()
		p.Endpoints[0].Environments[0].RequestedGatewayUUID = &requested

		err := svc.deployMCPProxyEndpoints(context.Background(), p, "org")
		require.NoError(t, err)
		require.Len(t, hub.published, 1)
		require.Equal(t, egress.UUID.String(), hub.published[0].GatewayID)
	})

	t.Run("caller-specified gatewayId differing from an existing deployment is rejected", func(t *testing.T) {
		// The artifact is already deployed to "both"; requesting "egress" instead must
		// fail as placement-fixed rather than silently re-homing the binding.
		svc := &MCPProxyService{
			gatewayRepo:       gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			mcpProxyScopeRepo: scopeRepo,
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
					return []string{both.UUID.String()}, nil
				},
			},
			logger: discardLogger(),
		}

		requested := egress.UUID.String()
		p := newProxy()
		p.Endpoints[0].Environments[0].RequestedGatewayUUID = &requested

		err := svc.deployMCPProxyEndpoints(context.Background(), p, "org")
		require.ErrorIs(t, err, errPlacementFixed)
	})
}

func TestAnchoring_MonitorRunAgainstDeployedProxy(t *testing.T) {
	env := uuid.New()
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	proxy := &models.LLMProxy{UUID: uuid.New()}

	t.Run("resolves to the gateway the proxy is deployed to", func(t *testing.T) {
		exec := &monitorExecutor{
			gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
					return []string{both.UUID.String()}, nil
				},
			},
		}
		url, err := exec.resolveProxyURL(context.Background(), "org", env.String(), proxy)
		require.NoError(t, err)
		// Runtime URLs are per-gateway in the fixture, so asserting this one rules out the
		// other gateway. The monitor's evaluation job runs in-cluster and gets the runtime
		// address verbatim — no scheme defaulting, and never the public vhost.
		require.Equal(t, both.RuntimeURL, url)
	})

	t.Run("fails when the anchored gateway has no runtimeUrl", func(t *testing.T) {
		unregistered := newGateway(t, models.GatewayRoleBoth, true)
		unregistered.RuntimeURL = ""
		exec := &monitorExecutor{
			gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{unregistered}),
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
					return []string{unregistered.UUID.String()}, nil
				},
			},
		}
		_, err := exec.resolveProxyURL(context.Background(), "org", env.String(), proxy)
		require.ErrorIs(t, err, errGatewayRuntimeURLUnregistered)
	})

	t.Run("uses the public vhost when evaluation jobs run outside the cluster", func(t *testing.T) {
		cfg := config.GetConfig()
		orig := cfg.EvalJobsUsePublicGatewayURL
		cfg.EvalJobsUsePublicGatewayURL = true
		t.Cleanup(func() { cfg.EvalJobsUsePublicGatewayURL = orig })

		exec := &monitorExecutor{
			gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
					return []string{both.UUID.String()}, nil
				},
			},
		}
		url, err := exec.resolveProxyURL(context.Background(), "org", env.String(), proxy)
		require.NoError(t, err)
		// Still anchored to the deployed gateway, but its public address: a job in another
		// cluster cannot route to the in-cluster runtime URL.
		require.Equal(t, buildPublicProxyURL(both, proxy.Configuration.Context), url)
		require.NotEqual(t, both.RuntimeURL, url)
	})

	t.Run("ambiguity fires only with no deployment", func(t *testing.T) {
		exec := &monitorExecutor{
			gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
					return nil, nil
				},
			},
		}
		_, err := exec.resolveProxyURL(context.Background(), "org", env.String(), proxy)
		require.ErrorIs(t, err, errAmbiguousEgressGateway)
	})
}

func TestAnchoring_MonitorCreateAgainstDeployedProvider(t *testing.T) {
	// resolveMonitorGateway backs both the monitor-create call site (monitor_manager.go:219)
	// and the monitor-update call site (:540) identically, so this one test covers both
	// TestAnchoring_MonitorCreateAgainstDeployedProvider and
	// TestAnchoring_MonitorUpdateAgainstDeployedProvider from the brief.
	env := uuid.New()
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	providerUUID := uuid.New()

	t.Run("resolves to the gateway the provider is deployed to", func(t *testing.T) {
		mgr := &monitorManagerService{
			llmProvisioner: &LLMProxyProvisioner{
				gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
				llmProxyDeploymentService: &LLMProxyDeploymentService{
					deploymentRepo: &repomocks.DeploymentRepositoryMock{
						GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
							return []string{egress.UUID.String()}, nil
						},
					},
				},
			},
		}
		gw, err := mgr.resolveMonitorGateway(context.Background(), "org", env, providerUUID.String())
		require.NoError(t, err)
		require.Equal(t, egress.UUID, gw.UUID)
	})

	t.Run("ambiguity fires only with no deployment", func(t *testing.T) {
		mgr := &monitorManagerService{
			llmProvisioner: &LLMProxyProvisioner{
				gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
				llmProxyDeploymentService: &LLMProxyDeploymentService{
					deploymentRepo: &repomocks.DeploymentRepositoryMock{
						GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
							return nil, nil
						},
					},
				},
			},
		}
		_, err := mgr.resolveMonitorGateway(context.Background(), "org", env, providerUUID.String())
		require.ErrorIs(t, err, errAmbiguousEgressGateway)
	})
}

func TestAnchoring_AgentMCPConfigCreateWithExistingMapping(t *testing.T) {
	// Drives resolveGatewayForMCPArtifact as reached from the agent MCP config create
	// path (agent_configuration_service.go:1342), which resolves the environment's gateway
	// for the MCP proxy's already-deployed shared artifact without placing anything itself.
	env := uuid.New()
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	sharedArtifactUUID := uuid.New()

	t.Run("resolves to the gateway the shared artifact is deployed to", func(t *testing.T) {
		svc := &agentConfigurationService{
			gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			mcpProxyService: &MCPProxyService{
				deploymentRepo: &repomocks.DeploymentRepositoryMock{
					GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
						return []string{both.UUID.String()}, nil
					},
				},
			},
		}
		gw, err := svc.resolveGatewayForMCPArtifact(context.Background(), sharedArtifactUUID, "org", env)
		require.NoError(t, err)
		require.Equal(t, both.UUID, gw.UUID)
	})

	t.Run("ambiguity fires only with no deployment", func(t *testing.T) {
		svc := &agentConfigurationService{
			gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			mcpProxyService: &MCPProxyService{
				deploymentRepo: &repomocks.DeploymentRepositoryMock{
					GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
						return nil, nil
					},
				},
			},
		}
		_, err := svc.resolveGatewayForMCPArtifact(context.Background(), sharedArtifactUUID, "org", env)
		require.ErrorIs(t, err, errAmbiguousEgressGateway)
	})
}

func TestAnchoring_AgentMCPConfigUpdateWithExistingMapping(t *testing.T) {
	// Same resolveGatewayForMCPArtifact seam as the create test above, but exercised as
	// reached from updateMCPConfig's deployability probe (agent_configuration_service.go:2288).
	// It's the identical resolver at a second call site, not a distinct code path — kept as
	// its own test to mirror the brief's naming for both anchored sites (4 and 6).
	env := uuid.New()
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	sharedArtifactUUID := uuid.New()

	t.Run("resolves to the gateway the shared artifact is deployed to", func(t *testing.T) {
		svc := &agentConfigurationService{
			gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			mcpProxyService: &MCPProxyService{
				deploymentRepo: &repomocks.DeploymentRepositoryMock{
					GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
						return []string{egress.UUID.String()}, nil
					},
				},
			},
		}
		gw, err := svc.resolveGatewayForMCPArtifact(context.Background(), sharedArtifactUUID, "org", env)
		require.NoError(t, err)
		require.Equal(t, egress.UUID, gw.UUID)
	})

	t.Run("ambiguity fires only with no deployment", func(t *testing.T) {
		svc := &agentConfigurationService{
			gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			mcpProxyService: &MCPProxyService{
				deploymentRepo: &repomocks.DeploymentRepositoryMock{
					GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
						return nil, nil
					},
				},
			},
		}
		_, err := svc.resolveGatewayForMCPArtifact(context.Background(), sharedArtifactUUID, "org", env)
		require.ErrorIs(t, err, errAmbiguousEgressGateway)
	})
}
