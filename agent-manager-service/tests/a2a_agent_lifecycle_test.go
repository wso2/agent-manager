//go:build integration

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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var (
	a2aLifecycleOrgName   = fmt.Sprintf("a2a-lc-org-%s", uuid.New().String()[:5])
	a2aLifecycleProjName  = fmt.Sprintf("a2a-lc-proj-%s", uuid.New().String()[:5])
	a2aLifecycleAgentName = fmt.Sprintf("a2a-lc-agent-%s", uuid.New().String()[:5])
)

const a2aLifecycleUpstream = "http://a2a-lc-agent.dp-default:9099"

// lifecycleEventHub records what the test's reconcile core writes, so the event the gateway would act on can be
// asserted without a live hub or a connected gateway. Only publication is exercised; the rest of the EventHub
// surface is present to satisfy the interface.
type lifecycleEventHub struct {
	mu        sync.Mutex
	published []eventhub.Event
}

func (h *lifecycleEventHub) PublishEvent(ctx context.Context, gatewayID string, evt eventhub.Event) error {
	return h.PublishEventTx(ctx, nil, gatewayID, evt)
}

func (h *lifecycleEventHub) PublishEventTx(_ context.Context, _ *gorm.DB, _ string, evt eventhub.Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.published = append(h.published, evt)
	return nil
}

func (h *lifecycleEventHub) events() []eventhub.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]eventhub.Event(nil), h.published...)
}

func (h *lifecycleEventHub) Initialize() error                      { return nil }
func (h *lifecycleEventHub) RegisterGateway(gatewayID string) error { return nil }

func (h *lifecycleEventHub) Subscribe(gatewayID string) (<-chan eventhub.Event, error) {
	return nil, nil //nolint:nilnil // the recorder never delivers; only publication is exercised
}

func (h *lifecycleEventHub) Unsubscribe(gatewayID string, subscriber <-chan eventhub.Event) error {
	return nil
}
func (h *lifecycleEventHub) UnsubscribeAll(gatewayID string) error { return nil }
func (h *lifecycleEventHub) CleanUpEvents() error                  { return nil }
func (h *lifecycleEventHub) Close() error                          { return nil }

func testLifecycleLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// seedIngressGateway inserts a gateway and maps it to the environment, because
// an API agent is created, deployed and published through its environment's INGRESS gateway and the test
// database carries none by default.
func seedIngressGatewayForDefaultEnvironment(t *testing.T, ouID string) uuid.UUID {
	t.Helper()
	return seedIngressGateway(t, ouID, uuid.MustParse(apitestutils.DefaultEnvironmentUUID))
}

func seedIngressGateway(t *testing.T, ouID string, environmentUUID uuid.UUID) uuid.UUID {
	t.Helper()
	gdb := db.GetDB()
	gatewayID := uuid.New()
	name := "a2a-lc-gw-" + gatewayID.String()[:8]
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, name, name, "agents.example.com", models.GatewayRoleIngress, ouID,
	).Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`,
		gatewayID, environmentUUID,
	).Error)
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	return gatewayID
}

// TestA2AAgentLifecycle drives create -> deploy -> publish -> deploy settings -> delete for an a2a-agent through
// the API and the reconcile model: the requests write the exposure and the owed row, and a worker over the same
// database renders it. The worker isn't started by the app in tests, so the test builds one with its own event
// recorder and renders the agent's artifact with RunOnceFor.
func TestA2AAgentLifecycle(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	environmentUUID := uuid.New()
	var serviceURL string
	var serviceURLMu sync.Mutex
	currentServiceURL := func() string {
		serviceURLMu.Lock()
		defer serviceURLMu.Unlock()
		return serviceURL
	}
	agentUUID := uuid.New()

	openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
	openChoreoClient.ComponentExistsFunc = func(ctx context.Context, orgName, projName, agentName string) (bool, error) {
		return true, nil
	}
	// Deploy reads the agent's subtype off the component OpenChoreo reports, and
	// the default mock reports none — so a deploy would take the REST path and
	// never record an exposure.
	openChoreoClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID:         agentUUID.String(),
			Name:         componentName,
			ProjectName:  projectName,
			Provisioning: models.Provisioning{Type: "internal"},
			Type:         models.AgentType{Type: "agent-api", SubType: "a2a-agent"},
			CreatedAt:    time.Now(),
		}, nil
	}
	// The default mock returns a non-UUID environment id; the exposure is keyed
	// on the environment UUID, so this test needs a real one.
	openChoreoClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{UUID: environmentUUID.String(), Name: environmentName}, nil
	}
	openChoreoClient.GetReleaseBindingServiceURLFunc = func(ctx context.Context, ouID, componentName, environment string) (string, error) {
		return currentServiceURL(), nil
	}
	openChoreoClient.GetReleaseBindingRolloutFunc = func(ctx context.Context, ouID, componentName, environment string) (client.ReleaseBindingRollout, error) {
		return client.ReleaseBindingRollout{Deployed: true}, nil
	}

	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{
		OpenChoreoClient: openChoreoClient,
		SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
	}, authMiddleware)

	gdb := db.GetDB()
	exposureRepo := repositories.NewPlatformAgentExposureRepository(gdb)
	cardRepo := repositories.NewA2AAgentCardRepository(gdb)
	deploymentRepo := repositories.NewDeploymentRepo(gdb)
	artifactRepo := repositories.NewArtifactRepo(gdb)
	ouID := "mock-org-id"

	gatewayID := seedIngressGateway(t, ouID, environmentUUID)

	hub := &lifecycleEventHub{}
	tx := services.NewTxRunner(gdb)
	kind := services.NewA2AAgentKind(tx,
		services.NewPlatformAgentExposures(exposureRepo, repositories.NewGatewayRepo(gdb), openChoreoClient),
		repositories.NewA2AProxyRepository(gdb), cardRepo, deploymentRepo, repositories.NewAgentConfigRepo(gdb), openChoreoClient,
		services.NewA2ACardFetcher(), repositories.NewAPIKeyRepo(gdb), services.NewGatewayEventsService(hub), bytes.Repeat([]byte{7}, 32), testLifecycleLogger())
	core := reconcile.NewCore(gdb, deploymentRepo, hub, reconcile.Kinds{kind},
		reconcile.Config{Lease: time.Minute, AttemptBudget: 3}, testLifecycleLogger())
	worker := reconcile.NewWorker(core, testLifecycleLogger())

	t.Cleanup(func() {
		_ = exposureRepo.DeleteForAgent(context.Background(), ouID, a2aLifecycleProjName, a2aLifecycleAgentName)
		_ = cardRepo.DeleteForAgent(context.Background(), ouID, a2aLifecycleProjName, a2aLifecycleAgentName)
	})

	// exposure returns the agent's exposure in the environment, or nil.
	exposure := func(t *testing.T) *models.PlatformAgentExposure {
		t.Helper()
		row, err := exposureRepo.Get(context.Background(), ouID, a2aLifecycleProjName, a2aLifecycleAgentName, environmentUUID)
		if err != nil {
			return nil
		}
		return row
	}
	placement := func(t *testing.T) models.Placement {
		t.Helper()
		row := exposure(t)
		require.NotNil(t, row)
		return models.Placement{
			ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: row.ArtifactUUID, Kind: models.KindAgent},
			GatewayUUID: gatewayID,
		}
	}
	statusRow := func(t *testing.T) *models.DeploymentStatusRecord {
		t.Helper()
		row, err := deploymentRepo.GetStatusRecord(context.Background(), placement(t))
		require.NoError(t, err)
		require.NotNil(t, row, "the exposure is placed on the environment's ingress gateway")
		return row
	}
	render := func(t *testing.T) {
		t.Helper()
		p := placement(t)
		require.NoError(t, worker.RunOnceFor(context.Background(), p.OUID, p.ArtifactUUID))
	}
	published := func(t *testing.T) services.A2AAgentDeploymentYAML {
		t.Helper()
		p := placement(t)
		current, err := deploymentRepo.GetPullable(context.Background(), ouID, p.ArtifactUUID, gatewayID.String())
		require.NoError(t, err)
		require.NotNil(t, current, "the gateway fetches the Agent back out of this deployment")
		var agent services.A2AAgentDeploymentYAML
		require.NoError(t, yaml.Unmarshal(current.Content, &agent))
		return agent
	}

	deployAgent := func(t *testing.T) {
		t.Helper()
		body := new(bytes.Buffer)
		require.NoError(t, json.NewEncoder(body).Encode(map[string]interface{}{
			"imageId": "registry.example.com/a2a:v1.0.0",
		}))
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
			a2aLifecycleOrgName, a2aLifecycleProjName, a2aLifecycleAgentName)
		req := httptest.NewRequest(http.MethodPost, url, body)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())
	}

	t.Run("create an a2a agent", func(t *testing.T) {
		body := new(bytes.Buffer)
		require.NoError(t, json.NewEncoder(body).Encode(map[string]interface{}{
			"name":        a2aLifecycleAgentName,
			"displayName": "A2A Lifecycle Agent",
			"description": "Drives the full A2A lifecycle",
			"provisioning": map[string]interface{}{
				"type": "internal",
				"repository": map[string]interface{}{
					"url":     "https://github.com/example/a2a-agent",
					"branch":  "main",
					"appPath": "/",
				},
			},
			"agentType": map[string]interface{}{"type": "agent-api", "subType": "a2a-agent"},
			"build": map[string]interface{}{
				"type":   "docker",
				"docker": map[string]interface{}{"dockerfilePath": "/Dockerfile"},
			},
			"inputInterface": map[string]interface{}{"type": "HTTP", "port": 9099},
		}))
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", a2aLifecycleOrgName, a2aLifecycleProjName)
		req := httptest.NewRequest(http.MethodPost, url, body)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())
	})

	t.Run("deploy exposes the agent and waits for its endpoint", func(t *testing.T) {
		deployAgent(t)

		row := exposure(t)
		require.NotNil(t, row, "the deploy recorded the exposure")
		assert.Equal(t, models.AgentExposureProtocolA2A, row.Protocol)
		require.NotNil(t, row.AgentUUID)
		assert.Equal(t, agentUUID, *row.AgentUUID)
		// The artifact the gateway is told about is the KindAgent row, which is
		// also what the agent's API keys are bound to.
		artifact, err := artifactRepo.GetByUUID(row.ArtifactUUID.String(), ouID)
		require.NoError(t, err)
		assert.Equal(t, models.KindAgent, artifact.Kind)

		// The binding has published no ServiceURL, so an Agent with an empty upstream must never be emitted.
		render(t)
		status := statusRow(t)
		assert.Equal(t, models.DeploymentStatusPending, status.Status)
		shown := models.UserStatusOf(*status)
		assert.Equal(t, models.DeploymentStatusDeploying, shown.Status)
		assert.Equal(t, "waiting for the agent's endpoint", shown.Reason)
		assert.Zero(t, status.AttemptCount, "a not-ready wait is not charged against the budget")
		require.NotNil(t, status.NextAttemptAt)
		assert.True(t, status.NextAttemptAt.After(time.Now()), "and the next try is scheduled forward")
		assert.Empty(t, hub.events())
	})

	t.Run("a redeploy once the endpoint appears publishes the Agent", func(t *testing.T) {
		serviceURLMu.Lock()
		serviceURL = a2aLifecycleUpstream
		serviceURLMu.Unlock()

		deployAgent(t) // marks the backed-off row due now
		render(t)

		status := statusRow(t)
		assert.Equal(t, models.DeploymentStatusDeploying, status.Status)
		agent := published(t)
		assert.Equal(t, "Agent", agent.Kind)
		assert.Equal(t, a2aLifecycleUpstream, agent.Spec.Upstream.URL)
		require.NotNil(t, agent.Spec.A2A.AgentCard, "the card inherits the default agent CORS")
		events := hub.events()
		require.Len(t, events, 1)
		assert.Equal(t, eventhub.EventType("agent.deployed"), events[0].EventType)
		assert.Equal(t, placement(t).ArtifactUUID.String(), events[0].EntityID,
			"the gateway fetches /agents/{agentId} by the artifact UUID")

		applied, err := deploymentRepo.ApplyAck(context.Background(), gatewayID.String(), models.DeploymentAck{
			DeploymentID: status.DeploymentID.String(), ArtifactID: placement(t).ArtifactUUID.String(),
			Action: "deploy", Status: "success", PerformedAt: status.PerformedAt,
		}, core.AfterLive())
		require.NoError(t, err)
		require.True(t, applied)
		assert.Equal(t, models.DeploymentStatusDeployed, statusRow(t).Status)
		card, err := cardRepo.Get(context.Background(), ouID, a2aLifecycleProjName, a2aLifecycleAgentName, exposure(t).EnvironmentName)
		require.NoError(t, err, "going live queues the card fetch")
		require.NotNil(t, card.AgentUUID)
		assert.Equal(t, agentUUID, *card.AgentUUID)
	})

	t.Run("deploy settings republish card CORS and A2A-Version", func(t *testing.T) {
		before := statusRow(t)

		body := new(bytes.Buffer)
		require.NoError(t, json.NewEncoder(body).Encode(map[string]interface{}{
			"environmentName": exposure(t).EnvironmentName,
			"corsConfig": map[string]interface{}{
				"enabled":      true,
				"allowOrigin":  []string{"https://client.example"},
				"allowHeaders": []string{"Content-Type"},
			},
			"agentCardCorsConfig": map[string]interface{}{
				"enabled":      true,
				"allowOrigin":  []string{"*"},
				"allowHeaders": []string{"Content-Type"},
			},
		}))
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deploy-settings",
			a2aLifecycleOrgName, a2aLifecycleProjName, a2aLifecycleAgentName)
		req := httptest.NewRequest(http.MethodPut, url, body)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		require.Equal(t, http.StatusNoContent, rr.Code, "body: %s", rr.Body.String())
		require.NotNil(t, statusRow(t).NextAttemptAt, "a settings change owes a re-render")

		render(t)

		after := statusRow(t)
		assert.NotEqual(t, *before.DeploymentID, *after.DeploymentID, "the settings change rendered a new deployment")
		agent := published(t)
		opCORS := agent.Spec.A2A.OperationConfigs.Policies[0]["params"].(map[string]interface{})
		assert.Equal(t, []interface{}{"https://client.example"}, opCORS["allowedOrigins"])
		assert.Contains(t, opCORS["allowedHeaders"], "A2A-Version", "stored and published even though the request omitted it")
		require.NotNil(t, agent.Spec.A2A.AgentCard)
		cardCORS := agent.Spec.A2A.AgentCard.Public.Policies[0]["params"].(map[string]interface{})
		assert.Equal(t, []interface{}{"*"}, cardCORS["allowedOrigins"])
	})

	t.Run("delete removes the exposure, the artifact and its rows", func(t *testing.T) {
		artifactUUID := placement(t).ArtifactUUID
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s",
			a2aLifecycleOrgName, a2aLifecycleProjName, a2aLifecycleAgentName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		require.Contains(t, []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent}, rr.Code,
			"body: %s", rr.Body.String())

		require.Eventually(t, func() bool {
			return exposure(t) == nil
		}, 5*time.Second, 100*time.Millisecond, "a deleted agent leaves no exposure behind")
		_, err := artifactRepo.GetByUUID(artifactUUID.String(), ouID)
		assert.Error(t, err, "the artifact is gone")
		var rows int64
		require.NoError(t, gdb.Model(&models.DeploymentStatusRecord{}).Where("artifact_uuid = ?", artifactUUID).
			Count(&rows).Error)
		assert.Zero(t, rows, "its status rows went with it")
	})
}
