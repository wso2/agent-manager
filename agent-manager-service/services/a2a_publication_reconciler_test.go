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
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
		UpdatedAt:       time.Date(2026, 9, 24, 10, 0, 0, 123456000, time.UTC),
	}
}

// a2aReconcilerHarness holds the reconciler and the mocks behind it, so each
// test can assert on what the publication attempt did rather than only on what
// it returned.
type a2aReconcilerHarness struct {
	svc            *a2aPublicationReconcilerService
	gatewayRepo    *repomocks.GatewayRepositoryMock
	hub            *recordingEventHub
	pubRepo        *repomocks.A2APublicationRepositoryMock
	cardRepo       *repomocks.A2AAgentCardRepositoryMock
	deploymentRepo *repomocks.DeploymentRepositoryMock
	ocClient       *clientmocks.OpenChoreoClientMock
}

// newA2AReconcilerHarness wires a reconciler whose every dependency succeeds and
// whose binding reports serviceURL. Tests narrow it to the condition they are
// about.
func newA2AReconcilerHarness(serviceURL string) *a2aReconcilerHarness {
	hub := &recordingEventHub{}
	pubRepo := &repomocks.A2APublicationRepositoryMock{
		MarkPublishedFunc: func(ctx context.Context, read models.A2APublication, upstreamURL string) error { return nil },
		MarkAttemptFailedFunc: func(ctx context.Context, read models.A2APublication, lastErr string, nextAttemptAt time.Time) error {
			return nil
		},
		MarkWaitingFunc: func(ctx context.Context, read models.A2APublication, lastErr string, nextAttemptAt time.Time) error {
			return nil
		},
		MarkFailedFunc: func(ctx context.Context, read models.A2APublication, lastErr string) error { return nil },
	}
	cardRepo := &repomocks.A2AAgentCardRepositoryMock{
		EnqueueFunc: func(context.Context, *models.A2AAgentCard) error { return nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
			return nil, repositories.ErrA2AAgentCardNotFound
		},
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		CreateWithLimitEnforcementFunc: func(deployment *models.Deployment, maxDeployments int) error { return nil },
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetReleaseBindingServiceURLFunc: func(ctx context.Context, ouID, componentName, environment string) (string, error) {
			return serviceURL, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Name: name}, nil
		},
		GetReleaseBindingRolloutFunc: func(context.Context, string, string, string) (client.ReleaseBindingRollout, error) {
			return client.ReleaseBindingRollout{ServiceURL: serviceURL, ReleaseName: "trip-planner-r2", Serving: true}, nil
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
		gatewayRepo:    gatewayRepo,
		pubRepo:        pubRepo,
		cardRepo:       cardRepo,
		deploymentRepo: deploymentRepo,
		ocClient:       ocClient,
		svc: &a2aPublicationReconcilerService{
			pubRepo:         pubRepo,
			cardRepo:        cardRepo,
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
// binding reconciles, so an early attempt must wait rather than emit an Agent
// that routes nowhere.
func TestReconcilerWaitsWhenServiceURLIsNotReadyYet(t *testing.T) {
	h := newA2AReconcilerHarness("")

	h.svc.publishOne(context.Background(), pendingPublication())

	assert.Empty(t, h.deploymentRepo.CreateWithLimitEnforcementCalls(),
		"nothing is published until the binding reports an upstream")
	assert.Empty(t, h.hub.published, "and no gateway is told about it")

	waits := h.pubRepo.MarkWaitingCalls()
	require.Len(t, waits, 1)
	assert.True(t, waits[0].NextAttemptAt.After(time.Now()), "the retry is scheduled forward")
	assert.Empty(t, h.pubRepo.MarkAttemptFailedCalls(), "waiting is not charged an attempt")
	assert.Empty(t, h.pubRepo.MarkFailedCalls())
}

// A first source build can take far longer than the attempt budget; waiting on
// it must never fail the row.
func TestReconcilerNeverGivesUpWaitingForTheFirstBinding(t *testing.T) {
	h := newA2AReconcilerHarness("")

	pub := pendingPublication()
	pub.AttemptCount = a2aPublicationAttemptBudget - 1
	h.svc.publishOne(context.Background(), pub)

	assert.Len(t, h.pubRepo.MarkWaitingCalls(), 1)
	assert.Empty(t, h.pubRepo.MarkFailedCalls())
}

// A retry must be due by the next tick, not land just after it and skip one.
func TestReconcilerRetryLandsOnTheNextTick(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.svc.gatewayRepo = &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(opts repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{}, nil
		},
	}

	before := time.Now()
	h.svc.publishOne(context.Background(), pendingPublication())

	retries := h.pubRepo.MarkAttemptFailedCalls()
	require.Len(t, retries, 1)
	assert.True(t, retries[0].NextAttemptAt.Before(before.Add(a2aReconcilerTickInterval)),
		"due before the next tick fires")
}

// Past the budget a real failure (here: no ingress gateway) is called failed
// rather than retried forever.
func TestReconcilerGivesUpPastTheAttemptBudget(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.svc.gatewayRepo = &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(opts repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{}, nil
		},
	}

	exhausted := pendingPublication()
	exhausted.AttemptCount = a2aPublicationAttemptBudget - 1
	h.svc.publishOne(context.Background(), exhausted)

	require.Len(t, h.pubRepo.MarkFailedCalls(), 1)
	assert.Empty(t, h.pubRepo.MarkAttemptFailedCalls(), "the retry cycle has ended")
	assert.Empty(t, h.deploymentRepo.CreateWithLimitEnforcementCalls())
}

// The ingress-role filter belongs in the query, scoped to the org.
func TestReconcilerQueriesOnlyIngressGatewaysOfTheOrg(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")

	pub := pendingPublication()
	h.svc.publishOne(context.Background(), pub)

	calls := h.gatewayRepo.ListWithFiltersCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, pub.OUID, calls[0].Filters.OrganizationID)
	assert.Equal(t, models.IngressGatewayRoles, calls[0].Filters.FunctionalityTypeIn)
	require.NotNil(t, calls[0].Filters.EnvironmentID)
	assert.Equal(t, pub.EnvironmentUUID.String(), *calls[0].Filters.EnvironmentID)
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

	marked := h.pubRepo.MarkPublishedCalls()
	require.Len(t, marked, 1)
	assert.Equal(t, pub, marked[0].Read,
		"the row is marked as it was read, so a re-enqueue meanwhile is not swallowed")
	assert.Equal(t, upstream, marked[0].UpstreamURL, "the published upstream is recorded for drift checks")
	assert.Empty(t, h.pubRepo.MarkAttemptFailedCalls())
}

// The reconciler is the only place an A2A agent's gateway resource is built, so
// it is where A2A-Version is guaranteed and the card's CORS is attached.
func TestReconcilerPublishesA2AVersionAndInheritedCardCORS(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.svc.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.AgentConfig, error) {
			return &models.AgentConfig{
				CORSEnabled:      true,
				CORSAllowOrigins: []string{"https://client.example"},
				CORSAllowMethods: []string{"GET", "POST", "OPTIONS"},
				CORSAllowHeaders: []string{"Content-Type"},
			}, nil
		},
	}

	h.svc.publishOne(context.Background(), pendingPublication())

	created := h.deploymentRepo.CreateWithLimitEnforcementCalls()
	require.Len(t, created, 1)
	var published A2AAgentDeploymentYAML
	require.NoError(t, yaml.Unmarshal(created[0].Deployment.Content, &published))

	opCORS := published.Spec.A2A.OperationConfigs.Policies[0]
	assert.Equal(t, "cors", opCORS["name"])
	assert.Contains(t, opCORS["params"].(map[string]interface{})["allowedHeaders"], "A2A-Version")

	require.NotNil(t, published.Spec.A2A.AgentCard, "an inherited card follows enabled agent CORS")
	cardCORS := published.Spec.A2A.AgentCard.Public.Policies[0]["params"].(map[string]interface{})
	assert.Equal(t, []interface{}{"https://client.example"}, cardCORS["allowedOrigins"])
	assert.Equal(t, []interface{}{"GET", "OPTIONS"}, cardCORS["allowedMethods"])
}

func publishedPublication(upstreamURL string) models.A2APublication {
	pub := pendingPublication()
	pub.Status = models.A2APublicationStatusPublished
	pub.PublishedUpstreamURL = upstreamURL
	return pub
}

func (h *a2aReconcilerHarness) withPublished(rows ...models.A2APublication) {
	h.pubRepo.FindPublishedFunc = func(ctx context.Context, afterID uuid.UUID, limit int) ([]models.A2APublication, error) {
		return rows, nil
	}
	h.pubRepo.RequeueFunc = func(ctx context.Context, read models.A2APublication) error { return nil }
}

// withCard makes the drift check find a platform card row in the given state.
func (h *a2aReconcilerHarness) withCard(status models.A2AAgentCardStatus, releaseName string) {
	h.cardRepo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return &models.A2AAgentCard{Source: models.A2AAgentCardSourcePlatform, Status: status, ReleaseName: releaseName}, nil
	}
}

// A new release with a changed port moves the Service; the gateway must follow.
func TestDriftCheckRequeuesWhenTheUpstreamMoved(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:8080")
	stale := publishedPublication("http://trip-planner.dp-default:9099")
	h.withPublished(stale)

	h.svc.checkUpstreamDrift(context.Background())

	requeued := h.pubRepo.RequeueCalls()
	require.Len(t, requeued, 1)
	assert.Equal(t, stale, requeued[0].Read)
}

func TestDriftCheckLeavesAMatchingUpstreamAlone(t *testing.T) {
	const upstream = "http://trip-planner.dp-default:9099"
	h := newA2AReconcilerHarness(upstream)
	h.withPublished(publishedPublication(upstream))

	h.svc.checkUpstreamDrift(context.Background())

	assert.Empty(t, h.pubRepo.RequeueCalls())
}

// An unreadable or empty binding URL is not evidence of drift; requeueing
// would only park a working agent in the waiting state.
func TestDriftCheckIgnoresAnUnknownCurrentUpstream(t *testing.T) {
	h := newA2AReconcilerHarness("")
	h.withPublished(publishedPublication("http://trip-planner.dp-default:9099"))
	h.svc.checkUpstreamDrift(context.Background())
	assert.Empty(t, h.pubRepo.RequeueCalls())

	h.ocClient.GetReleaseBindingRolloutFunc = func(context.Context, string, string, string) (client.ReleaseBindingRollout, error) {
		return client.ReleaseBindingRollout{}, assert.AnError
	}
	h.svc.checkUpstreamDrift(context.Background())
	assert.Empty(t, h.pubRepo.RequeueCalls())
}

// The scan pages through published rows across ticks and wraps at the end, so
// every row is checked without any tick doing unbounded work.
func TestDriftCheckPagesThroughPublishedRowsAcrossTicks(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	page := make([]models.A2APublication, a2aDriftCheckBatch)
	for i := range page {
		page[i] = publishedPublication("http://trip-planner.dp-default:9099")
	}
	var cursors []uuid.UUID
	h.pubRepo.FindPublishedFunc = func(ctx context.Context, afterID uuid.UUID, limit int) ([]models.A2APublication, error) {
		cursors = append(cursors, afterID)
		assert.Equal(t, a2aDriftCheckBatch, limit)
		if afterID == uuid.Nil {
			return page, nil
		}
		return []models.A2APublication{}, nil
	}

	h.svc.checkUpstreamDrift(context.Background())
	h.svc.checkUpstreamDrift(context.Background())
	h.svc.checkUpstreamDrift(context.Background())

	require.Len(t, cursors, 3)
	assert.Equal(t, uuid.Nil, cursors[0])
	assert.Equal(t, page[len(page)-1].ID, cursors[1], "the next tick resumes after the last row checked")
	assert.Equal(t, uuid.Nil, cursors[2], "a short page wraps the scan back to the start")
}

// The gateway serves the card only once it has the Agent, so publishing is the card's trigger.
func TestReconcilerQueuesAPlatformCardFetchOncePublished(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	pub := pendingPublication()

	h.svc.publishOne(context.Background(), pub)

	queued := h.cardRepo.EnqueueCalls()
	require.Len(t, queued, 1)
	got := queued[0].Card
	assert.Equal(t, pub.OUID, got.OUID)
	assert.Equal(t, pub.ProjectName, got.ProjectName)
	assert.Equal(t, pub.AgentName, got.AgentName)
	assert.Equal(t, pub.EnvironmentName, got.EnvironmentName)
	assert.Equal(t, pub.EnvironmentUUID, got.EnvironmentUUID)
	assert.Equal(t, models.A2AAgentCardSourcePlatform, got.Source)
}

// Best effort: the agent is published; a missed card row is recovered by the next deploy or a refresh.
func TestReconcilerPublishesEvenWhenTheCardQueueFails(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.cardRepo.EnqueueFunc = func(context.Context, *models.A2AAgentCard) error { return assert.AnError }

	h.svc.publishOne(context.Background(), pendingPublication())

	assert.Len(t, h.pubRepo.MarkPublishedCalls(), 1)
	assert.Empty(t, h.pubRepo.MarkAttemptFailedCalls())
}

// A publish whose row was superseded is redone next tick; that publish queues the card.
func TestReconcilerQueuesNoCardForASupersededPublish(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.pubRepo.MarkPublishedFunc = func(context.Context, models.A2APublication, string) error {
		return repositories.ErrA2APublicationSuperseded
	}

	h.svc.publishOne(context.Background(), pendingPublication())

	assert.Empty(t, h.cardRepo.EnqueueCalls())
}

func TestReconcilerQueuesNoCardWhenThePublishFails(t *testing.T) {
	h := newA2AReconcilerHarness("")

	h.svc.publishOne(context.Background(), pendingPublication())

	assert.Empty(t, h.cardRepo.EnqueueCalls())
}

// A delete that clears the card rows between MarkPublished and the enqueue must not leave an orphan.
func TestReconcilerDropsTheCardRowWhenTheAgentWasDeletedMeanwhile(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.ocClient.GetComponentFunc = func(context.Context, string, string, string) (*models.AgentResponse, error) {
		return nil, utils.ErrNotFound
	}
	h.cardRepo.DeleteForAgentEnvFunc = func(context.Context, string, string, string, string) error { return nil }
	pub := pendingPublication()

	h.svc.publishOne(context.Background(), pub)

	require.Len(t, h.cardRepo.EnqueueCalls(), 1)
	deleted := h.cardRepo.DeleteForAgentEnvCalls()
	require.Len(t, deleted, 1)
	assert.Equal(t, pub.AgentName, deleted[0].AgentName)
	assert.Equal(t, pub.EnvironmentName, deleted[0].EnvironmentName)
}

func TestReconcilerKeepsTheCardRowWhenTheAgentLookupFails(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.ocClient.GetComponentFunc = func(context.Context, string, string, string) (*models.AgentResponse, error) {
		return nil, assert.AnError
	}

	h.svc.publishOne(context.Background(), pendingPublication())

	assert.Empty(t, h.cardRepo.DeleteForAgentEnvCalls())
}

// A rebuild or redeploy moves the binding to a new release without telling AMS; the card must follow.
func TestDriftCheckRefetchesACardFromAnOlderRelease(t *testing.T) {
	const upstream = "http://trip-planner.dp-default:9099"
	for _, status := range []models.A2AAgentCardStatus{models.A2AAgentCardStatusFetched, models.A2AAgentCardStatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			h := newA2AReconcilerHarness(upstream)
			h.withCard(status, "trip-planner-r1")
			pub := publishedPublication(upstream)
			h.withPublished(pub)

			h.svc.checkUpstreamDrift(context.Background())

			queued := h.cardRepo.EnqueueCalls()
			require.Len(t, queued, 1)
			assert.Equal(t, pub.AgentName, queued[0].Card.AgentName)
			assert.Equal(t, pub.EnvironmentName, queued[0].Card.EnvironmentName)
			assert.Equal(t, pub.EnvironmentUUID, queued[0].Card.EnvironmentUUID)
			assert.Equal(t, models.A2AAgentCardSourcePlatform, queued[0].Card.Source)
			assert.Empty(t, h.pubRepo.RequeueCalls())
		})
	}
}

func TestDriftCheckLeavesACardAlone(t *testing.T) {
	const upstream = "http://trip-planner.dp-default:9099"
	cases := map[string]func(h *a2aReconcilerHarness){
		"from the serving release": func(h *a2aReconcilerHarness) { h.withCard(models.A2AAgentCardStatusFetched, "trip-planner-r2") },
		"already being fetched":    func(h *a2aReconcilerHarness) { h.withCard(models.A2AAgentCardStatusPending, "trip-planner-r1") },
		"with no row": func(h *a2aReconcilerHarness) {
			h.cardRepo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
				return nil, repositories.ErrA2AAgentCardNotFound
			}
		},
		"while the new release is still rolling out": func(h *a2aReconcilerHarness) {
			h.withCard(models.A2AAgentCardStatusFetched, "trip-planner-r1")
			h.ocClient.GetReleaseBindingRolloutFunc = func(context.Context, string, string, string) (client.ReleaseBindingRollout, error) {
				return client.ReleaseBindingRollout{ServiceURL: upstream, ReleaseName: "trip-planner-r2"}, nil
			}
		},
		"of an external agent": func(h *a2aReconcilerHarness) {
			h.cardRepo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
				return &models.A2AAgentCard{Source: models.A2AAgentCardSourceExternal, Status: models.A2AAgentCardStatusFetched}, nil
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			h := newA2AReconcilerHarness(upstream)
			h.withPublished(publishedPublication(upstream))
			setup(h)

			h.svc.checkUpstreamDrift(context.Background())

			assert.Empty(t, h.cardRepo.EnqueueCalls())
		})
	}
}
