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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
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

// startDeployStub records nothing and marks the deployment DEPLOYING, as StartDeploy does.
func startDeployStub(_ context.Context, _ *gorm.DB, _ models.Placement, deployment *models.Deployment) error {
	deploying := models.DeploymentStatusDeploying
	performedAt := models.NewPerformedAt(time.Now())
	deployment.Status, deployment.UpdatedAt = &deploying, &performedAt
	return nil
}

// mappedEverywhere is LockEnvironmentMapping for gateways mapped to every environment.
func mappedEverywhere(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

// withMCPCore gives svc a reconcile core over its deployment repository and event hub, with an MCP kind that
// renders a minimal artifact for whatever binding it is asked for. A mock repository locks status rows as a no-op.
func withMCPCore(svc *MCPProxyService) {
	if deployments, ok := svc.deploymentRepo.(*repomocks.DeploymentRepositoryMock); ok {
		if deployments.WithTxFunc == nil {
			deployments.WithTxFunc = func(*gorm.DB) repositories.DeploymentRepository { return deployments }
		}
		if deployments.LockStatusRowsFunc == nil {
			deployments.LockStatusRowsFunc = func(context.Context, *gorm.DB, string, []uuid.UUID) error { return nil }
		}
	}
	svc.kind = &MCPProxyKind{load: func(_ context.Context, ref models.ArtifactRef) (*mcpArtifact, error) {
		return &mcpArtifact{proxyHandle: "proxy-handle", deployable: &models.MCPProxy{
			UUID: ref.ArtifactUUID, Handle: ref.Handle,
			Configuration: models.MCPProxyConfig{Upstream: models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{URL: "https://upstream.example.com"},
			}},
		}}, nil
	}}
	var hub eventhub.EventHub
	if svc.gatewayEventsService != nil {
		hub = svc.gatewayEventsService.hub
	}
	svc.core = reconcile.NewCore(nil, svc.deploymentRepo, hub, reconcile.Kinds{svc.kind}, reconcile.Config{},
		discardLogger())
}

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
				PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
					return []string{both.UUID.String()}, nil
				},
				LockEnvironmentMappingFunc: mappedEverywhere,
				StartDeployFunc:            startDeployStub,
			},
			tx:                   sentinelTx,
			gatewayEventsService: &GatewayEventsService{hub: hub},
			logger:               discardLogger(),
		}

		withMCPCore(svc)
		err := svc.deployMCPProxyEndpoints(context.Background(), txSentinel, newProxy(), "org")
		require.NoError(t, err)
		// The deployment event's GatewayID is the anchoring signal: a second egress
		// gateway must never steal an already-deployed (endpoint, environment) binding.
		require.Len(t, hub.published, 1)
		require.Equal(t, both.UUID.String(), hub.published[0].GatewayID)
	})

	t.Run("ambiguity fires only with no deployment", func(t *testing.T) {
		// No gatewayEventsService/StartDeployFunc stubbed: if resolution
		// wrongly proceeded to deploy, the unstubbed moq call would panic.
		svc := &MCPProxyService{
			gatewayRepo:       gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress}),
			mcpProxyScopeRepo: scopeRepo,
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
					return nil, nil
				},
			},
			logger: discardLogger(),
		}

		withMCPCore(svc)
		err := svc.deployMCPProxyEndpoints(context.Background(), txSentinel, newProxy(), "org")
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
				PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
					return nil, nil
				},
				LockEnvironmentMappingFunc: mappedEverywhere,
				StartDeployFunc:            startDeployStub,
			},
			tx:                   sentinelTx,
			gatewayEventsService: &GatewayEventsService{hub: hub},
			logger:               discardLogger(),
		}

		requested := egress.UUID.String()
		p := newProxy()
		p.Endpoints[0].Environments[0].RequestedGatewayUUID = &requested

		withMCPCore(svc)
		err := svc.deployMCPProxyEndpoints(context.Background(), txSentinel, p, "org")
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
				PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
					return []string{both.UUID.String()}, nil
				},
			},
			logger: discardLogger(),
		}

		requested := egress.UUID.String()
		p := newProxy()
		p.Endpoints[0].Environments[0].RequestedGatewayUUID = &requested

		withMCPCore(svc)
		err := svc.deployMCPProxyEndpoints(context.Background(), txSentinel, p, "org")
		require.ErrorIs(t, err, errPlacementFixed)
	})
}

// orderedEventHub records each publish in a log shared with the test's other fakes.
type orderedEventHub struct {
	stubEventHub
	log *[]string
}

func (h *orderedEventHub) PublishEventTx(ctx context.Context, tx *gorm.DB, gatewayID string, e eventhub.Event) error {
	*h.log = append(*h.log, "publish")
	return h.stubEventHub.PublishEventTx(ctx, tx, gatewayID, e)
}

// An MCP save locks every binding's status row before its first event locks a gateway, the order a render of one
// binding takes, so a concurrent render can't deadlock with it.
func TestMCPSaveLocksItsStatusRowsBeforeTheFirstEvent(t *testing.T) {
	env := uuid.New()
	gateway := newGateway(t, models.GatewayRoleEgress, true)
	binding := func(handle string) models.MCPProxyEndpoint {
		return models.MCPProxyEndpoint{
			UUID: uuid.New(), Handle: handle,
			Configuration: models.MCPEndpointConfig{Upstream: &models.UpstreamEndpoint{URL: "https://upstream.example.com"}},
			Environments:  []models.MCPProxyEndpointEnvironment{{EnvironmentUUID: env, ArtifactUUID: uuid.New()}},
		}
	}
	proxy := &models.MCPProxy{
		UUID: uuid.New(), Handle: "proxy-handle",
		Endpoints: []models.MCPProxyEndpoint{binding("a"), binding("b")},
	}

	var log []string
	var lockedArtifacts []uuid.UUID
	svc := &MCPProxyService{
		gatewayRepo: gatewayFixtureRepo(t, env.String(), []*models.Gateway{gateway}),
		deploymentRepo: &repomocks.DeploymentRepositoryMock{
			PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
				return []string{gateway.UUID.String()}, nil
			},
			LockEnvironmentMappingFunc: mappedEverywhere,
			LockStatusRowsFunc: func(_ context.Context, _ *gorm.DB, _ string, artifactUUIDs []uuid.UUID) error {
				log = append(log, "lock")
				lockedArtifacts = artifactUUIDs
				return nil
			},
			StartDeployFunc: func(ctx context.Context, tx *gorm.DB, p models.Placement, d *models.Deployment) error {
				log = append(log, "start")
				return startDeployStub(ctx, tx, p, d)
			},
		},
		tx:                   sentinelTx,
		gatewayEventsService: &GatewayEventsService{hub: &orderedEventHub{log: &log}},
		logger:               discardLogger(),
	}
	withMCPCore(svc)

	require.NoError(t, svc.deployMCPProxyEndpoints(context.Background(), txSentinel, proxy, "org"))

	assert.Equal(t, []string{"lock", "start", "publish", "start", "publish"}, log)
	assert.ElementsMatch(t, []uuid.UUID{
		proxy.Endpoints[0].Environments[0].ArtifactUUID, proxy.Endpoints[1].Environments[0].ArtifactUUID,
	}, lockedArtifacts)
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
				PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
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
				PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
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
				PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
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
				PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
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
						PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
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
						PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
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
					PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
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
					PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
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
					PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
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
					PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
						return nil, nil
					},
				},
			},
		}
		_, err := svc.resolveGatewayForMCPArtifact(context.Background(), sharedArtifactUUID, "org", env)
		require.ErrorIs(t, err, errAmbiguousEgressGateway)
	})
}

// A binding's anchor is read through the save's transaction, so the save sees its own rows; nothing is read on
// the pool (the pool mocks answer no reads).
func TestMCPEnvGatewayReadsThroughTheSavesTransaction(t *testing.T) {
	env := uuid.New()
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	txGateways := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress})
	txDeployments := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return []string{both.UUID.String()}, nil
		},
	}
	svc := &MCPProxyService{
		gatewayRepo: &repomocks.GatewayRepositoryMock{WithTxFunc: func(tx *gorm.DB) repositories.GatewayRepository {
			assert.Same(t, txSentinel, tx)
			return txGateways
		}},
		deploymentRepo: &repomocks.DeploymentRepositoryMock{WithTxFunc: func(tx *gorm.DB) repositories.DeploymentRepository {
			assert.Same(t, txSentinel, tx)
			return txDeployments
		}},
		logger: discardLogger(),
	}

	gateway, err := svc.mcpEnvGateway(context.Background(), txSentinel, &models.MCPProxy{},
		&models.MCPProxyEndpointEnvironment{EnvironmentUUID: env, ArtifactUUID: uuid.New()}, "org")

	require.NoError(t, err)
	assert.Equal(t, both.UUID, *gateway, "anchored on the placed gateway")
}
