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

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
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

// lifecycleEventHub records what the reconciler broadcasts, so the event the
// gateway would act on can be asserted without a live hub or a connected
// gateway. Only publication is exercised; the rest of the EventHub surface is
// present to satisfy the interface.
type lifecycleEventHub struct {
	mu        sync.Mutex
	published []eventhub.Event
}

func (h *lifecycleEventHub) PublishEvent(gatewayID string, evt eventhub.Event) error {
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
	return nil, nil //nolint:nilnil // the recorder never delivers; only PublishEvent is exercised
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
// an A2A agent is published to its environment's INGRESS gateway and the test
// database carries none by default.
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

// TestA2AAgentLifecycle drives create -> deploy -> publish -> delete for an
// a2a-agent. Every task before this one covered a single seam; this is the test
// that they join up.
//
// The reconciler cycle cannot be driven through the HTTP surface — nothing
// triggers it but its own ticker — so it is constructed here over the same
// repositories the app was built on and advanced with RunOnce.
func TestA2AAgentLifecycle(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	environmentUUID := uuid.New()
	serviceURL := ""

	openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
	openChoreoClient.ComponentExistsFunc = func(ctx context.Context, orgName, projName, agentName string) (bool, error) {
		return true, nil
	}
	// Deploy reads the agent's subtype off the component OpenChoreo reports, and
	// the default mock reports none — so a deploy would take the REST path and
	// never queue a publication.
	openChoreoClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID:         uuid.New().String(),
			Name:         componentName,
			ProjectName:  projectName,
			Provisioning: models.Provisioning{Type: "internal"},
			Type:         models.AgentType{Type: "agent-api", SubType: "a2a-agent"},
			CreatedAt:    time.Now(),
		}, nil
	}
	// The default mock returns a non-UUID environment id; publication keys its
	// queue row on the environment UUID, so this test needs a real one.
	openChoreoClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{UUID: environmentUUID.String(), Name: environmentName}, nil
	}
	openChoreoClient.GetReleaseBindingServiceURLFunc = func(ctx context.Context, ouID, componentName, environment string) (string, error) {
		return serviceURL, nil
	}

	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{
		OpenChoreoClient: openChoreoClient,
		SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
	}, authMiddleware)

	gdb := db.GetDB()
	pubRepo := repositories.NewA2APublicationRepository(gdb)
	deploymentRepo := repositories.NewDeploymentRepo(gdb)
	gatewayRepo := repositories.NewGatewayRepo(gdb)
	artifactRepo := repositories.NewArtifactRepo(gdb)
	ouID := "mock-org-id"

	gatewayID := seedIngressGateway(t, ouID, environmentUUID)

	hub := &lifecycleEventHub{}
	reconciler := services.NewA2APublicationReconcilerService(
		pubRepo, deploymentRepo, gatewayRepo,
		repositories.NewAgentConfigRepo(gdb),
		openChoreoClient,
		services.NewGatewayEventsService(hub),
		testLifecycleLogger(),
	)

	t.Cleanup(func() {
		_ = pubRepo.DeleteForAgent(context.Background(), ouID, a2aLifecycleProjName, a2aLifecycleAgentName)
	})

	// duePublication returns the queue row for this agent, or nil.
	duePublication := func(t *testing.T) *models.A2APublication {
		t.Helper()
		var row models.A2APublication
		err := gdb.Where("ou_id = ? AND project_name = ? AND agent_name = ?",
			ouID, a2aLifecycleProjName, a2aLifecycleAgentName).First(&row).Error
		if err != nil {
			return nil
		}
		return &row
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

	t.Run("deploy queues a publication rather than publishing inline", func(t *testing.T) {
		deployAgent(t)

		row := duePublication(t)
		require.NotNil(t, row, "the deploy recorded its intent to publish")
		assert.Equal(t, models.A2APublicationStatusPending, row.Status)
		assert.Equal(t, environmentUUID, row.EnvironmentUUID)
		assert.NotEqual(t, uuid.Nil, row.ArtifactUUID, "keyed on the artifact the gateway is told about")

		// The binding has published no ServiceURL, so an Agent with an empty
		// upstream must never be emitted.
		reconciler.RunOnce(context.Background())
		current, err := deploymentRepo.GetCurrentByGateway(row.ArtifactUUID.String(), gatewayID.String(), ouID)
		require.NoError(t, err)
		assert.Nil(t, current, "nothing is published until the binding reports an upstream")

		retried := duePublication(t)
		require.NotNil(t, retried)
		assert.Equal(t, models.A2APublicationStatusPending, retried.Status, "the row is retried, not failed")
		assert.Equal(t, 1, retried.AttemptCount, "the attempt was counted")
		require.NotNil(t, retried.NextAttemptAt)
		assert.True(t, retried.NextAttemptAt.After(time.Now()), "and the next try is scheduled forward")
	})

	t.Run("the reconciler publishes once the binding reports a service URL", func(t *testing.T) {
		serviceURL = a2aLifecycleUpstream

		// The failed attempt above scheduled the next try 30s out. Winding the
		// clock back is what a real tick past that point would see; sleeping
		// through the backoff would only make the test slow.
		require.NoError(t, gdb.Model(&models.A2APublication{}).
			Where("ou_id = ? AND project_name = ? AND agent_name = ?",
				ouID, a2aLifecycleProjName, a2aLifecycleAgentName).
			Update("next_attempt_at", time.Now().Add(-time.Second)).Error)

		row := duePublication(t)
		require.NotNil(t, row)

		reconciler.RunOnce(context.Background())

		current, err := deploymentRepo.GetCurrentByGateway(row.ArtifactUUID.String(), gatewayID.String(), ouID)
		require.NoError(t, err)
		require.NotNil(t, current, "the gateway fetches the Agent back out of this row")

		var published services.A2AAgentDeploymentYAML
		require.NoError(t, yaml.Unmarshal(current.Content, &published))
		assert.Equal(t, "Agent", published.Kind)
		assert.Equal(t, a2aLifecycleUpstream, published.Spec.Upstream.URL)
		require.Len(t, published.Spec.A2A.OperationConfigs.Transports, 2)
		assert.Equal(t, "JSONRPC", published.Spec.A2A.OperationConfigs.Transports[0].ProtocolBinding)
		assert.Equal(t, "HTTP+JSON", published.Spec.A2A.OperationConfigs.Transports[1].ProtocolBinding)
		assert.NotContains(t, string(current.Content), "agentCard")
		assert.NotContains(t, string(current.Content), "resilience")

		// The artifact the gateway is told about is the KindAgent row, which is
		// also what the agent's API keys are bound to.
		artifact, err := artifactRepo.GetByUUID(row.ArtifactUUID.String(), ouID)
		require.NoError(t, err)
		assert.Equal(t, models.KindAgent, artifact.Kind)

		assert.Equal(t, models.A2APublicationStatusPublished, duePublication(t).Status,
			"a published row is no longer due")

		events := hub.events()
		require.Len(t, events, 1)
		assert.Equal(t, eventhub.EventType("agent.deployed"), events[0].EventType)
		assert.Equal(t, "CREATE", events[0].Action)
		assert.Equal(t, row.ArtifactUUID.String(), events[0].EntityID,
			"the gateway fetches /agents/{agentId} by the artifact UUID")
	})

	t.Run("redeploy re-queues and re-publishes", func(t *testing.T) {
		before := duePublication(t)
		require.NotNil(t, before)

		deployAgent(t)

		requeued := duePublication(t)
		require.NotNil(t, requeued)
		assert.Equal(t, models.A2APublicationStatusPending, requeued.Status, "a redeploy must re-publish")
		assert.Equal(t, 0, requeued.AttemptCount, "with a fresh attempt budget")

		reconciler.RunOnce(context.Background())

		var deploymentCount int64
		require.NoError(t, gdb.Model(&models.Deployment{}).
			Where("artifact_uuid = ? AND ou_id = ?", requeued.ArtifactUUID, ouID).
			Count(&deploymentCount).Error)
		assert.EqualValues(t, 2, deploymentCount,
			"a redeploy writes a fresh row and re-broadcasts CREATE, as MCP does")
	})

	t.Run("delete clears the publication queue", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s",
			a2aLifecycleOrgName, a2aLifecycleProjName, a2aLifecycleAgentName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		require.Contains(t, []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent}, rr.Code,
			"body: %s", rr.Body.String())

		// Deletion is asynchronous in places, so give the cleanup a moment before
		// asserting the queue is empty.
		require.Eventually(t, func() bool {
			return duePublication(t) == nil
		}, 5*time.Second, 100*time.Millisecond,
			"a deleted agent leaves no publication row behind")
	})
}
