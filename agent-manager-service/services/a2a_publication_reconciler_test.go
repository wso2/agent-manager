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
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func pendingPublication() models.A2APublication {
	return models.A2APublication{
		ID:              uuid.New(),
		OUID:            "org-1",
		ProjectName:     "checkout",
		AgentName:       "trip-planner",
		EnvironmentName: "dev",
		EnvironmentUUID: uuid.New(),
		ArtifactUUID:    uuid.New(),
		Status:          models.A2APublicationStatusPending,
	}
}

// a2aReconcilerHarness holds the reconciler and the mocks behind it, so each
// test can assert on what the publication attempt did rather than only on what
// it returned.
type a2aReconcilerHarness struct {
	svc            *a2aPublicationReconcilerService
	hub            *recordingEventHub
	pubRepo        *repomocks.A2APublicationRepositoryMock
	deploymentRepo *repomocks.DeploymentRepositoryMock
	ocClient       *clientmocks.OpenChoreoClientMock
}

// newA2AReconcilerHarness wires a reconciler whose every dependency succeeds and
// whose binding reports serviceURL. Tests narrow it to the condition they are
// about.
func newA2AReconcilerHarness(serviceURL string) *a2aReconcilerHarness {
	hub := &recordingEventHub{}
	pubRepo := &repomocks.A2APublicationRepositoryMock{
		MarkPublishedFunc:     func(ctx context.Context, id uuid.UUID) error { return nil },
		MarkAttemptFailedFunc: func(ctx context.Context, id uuid.UUID, lastErr string, nextAttemptAt time.Time) error { return nil },
		MarkFailedFunc:        func(ctx context.Context, id uuid.UUID, lastErr string) error { return nil },
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		CreateWithLimitEnforcementFunc: func(deployment *models.Deployment, maxDeployments int) error { return nil },
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetReleaseBindingServiceURLFunc: func(ctx context.Context, ouID, componentName, environment string) (string, error) {
			return serviceURL, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(opts repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{{
				UUID:                     uuid.New(),
				Name:                     "ingress-dev",
				Vhost:                    "agents.example.com",
				GatewayFunctionalityType: models.GatewayRoleIngress,
			}}, nil
		},
	}
	agentConfigRepo := &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.AgentConfig, error) {
			return &models.AgentConfig{EnableApiKeySecurity: true, CORSEnabled: true}, nil
		},
	}

	return &a2aReconcilerHarness{
		hub:            hub,
		pubRepo:        pubRepo,
		deploymentRepo: deploymentRepo,
		ocClient:       ocClient,
		svc: &a2aPublicationReconcilerService{
			pubRepo:         pubRepo,
			deploymentRepo:  deploymentRepo,
			gatewayRepo:     gatewayRepo,
			agentConfigRepo: agentConfigRepo,
			ocClient:        ocClient,
			events:          NewGatewayEventsService(hub),
			logger:          testLogger(),
		},
	}
}

// The whole reason this reconciler exists: status is populated only after the
// binding reconciles, so an early attempt must retry rather than emit an Agent
// that routes nowhere.
func TestReconcilerRetriesWhenServiceURLIsNotReadyYet(t *testing.T) {
	h := newA2AReconcilerHarness("")

	h.svc.publishOne(context.Background(), pendingPublication())

	assert.Empty(t, h.deploymentRepo.CreateWithLimitEnforcementCalls(),
		"nothing is published until the binding reports an upstream")
	assert.Empty(t, h.hub.published, "and no gateway is told about it")

	retries := h.pubRepo.MarkAttemptFailedCalls()
	require.Len(t, retries, 1)
	assert.True(t, retries[0].NextAttemptAt.After(time.Now()), "the retry is scheduled forward")
	assert.Empty(t, h.pubRepo.MarkFailedCalls(), "still well inside the budget")
}

// Past the startup budget an agent that never became ready is called failed
// rather than retried forever.
func TestReconcilerGivesUpPastTheStartupBudget(t *testing.T) {
	h := newA2AReconcilerHarness("")

	exhausted := pendingPublication()
	exhausted.AttemptCount = a2aPublicationAttemptBudget - 1
	h.svc.publishOne(context.Background(), exhausted)

	require.Len(t, h.pubRepo.MarkFailedCalls(), 1)
	assert.Empty(t, h.pubRepo.MarkAttemptFailedCalls(), "the retry cycle has ended")
	assert.Empty(t, h.deploymentRepo.CreateWithLimitEnforcementCalls())
}

// The happy path: a ready binding produces a deployments row carrying the Agent
// YAML and an agent.deployed broadcast keyed on the artifact UUID.
func TestReconcilerPublishesOnceServiceURLIsAvailable(t *testing.T) {
	const upstream = "http://trip-planner.dp-default:9099"
	h := newA2AReconcilerHarness(upstream)

	pub := pendingPublication()
	h.svc.publishOne(context.Background(), pub)

	created := h.deploymentRepo.CreateWithLimitEnforcementCalls()
	require.Len(t, created, 1)
	assert.Equal(t, pub.ArtifactUUID, created[0].Deployment.ArtifactUUID)

	var published A2AAgentDeploymentYAML
	require.NoError(t, yaml.Unmarshal(created[0].Deployment.Content, &published))
	assert.Equal(t, kindA2AAgent, published.Kind)
	assert.Equal(t, upstream, published.Spec.Upstream.URL)
	require.NotNil(t, published.Spec.Vhost)
	assert.Equal(t, "agents.example.com", *published.Spec.Vhost)

	require.Len(t, h.hub.published, 1)
	evt := h.hub.published[0]
	assert.Equal(t, "agent.deployed", string(evt.EventType))
	assert.Equal(t, pub.ArtifactUUID.String(), evt.EntityID,
		"the gateway fetches /agents/{agentId} by the artifact UUID")

	var envelope struct {
		Payload struct {
			DeploymentID string `json:"deploymentId"`
		} `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
	assert.Equal(t, created[0].Deployment.DeploymentID.String(), envelope.Payload.DeploymentID,
		"the event points at the row the gateway will fetch")

	require.Len(t, h.pubRepo.MarkPublishedCalls(), 1)
	assert.Empty(t, h.pubRepo.MarkAttemptFailedCalls())
}
