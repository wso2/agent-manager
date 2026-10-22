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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
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

// sentinelTx hands fn the shared transaction sentinel; publisherMocks.tx adds a counter and an injected failure on top.
func sentinelTx(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(txSentinel) }

// a2aReconcilerHarness holds the reconciler and the mocks behind it, so each
// test can assert on what the publication attempt did rather than only on what
// it returned.
type a2aReconcilerHarness struct {
	svc            *a2aPublicationReconcilerService
	gatewayRepo    *repomocks.GatewayRepositoryMock
	hub            *recordingEventHub
	pubRepo        *repomocks.A2APublicationRepositoryMock
	cardRepo       *repomocks.A2AAgentCardRepositoryMock
	proxyRepo      *repomocks.A2AProxyRepositoryMock
	deploymentRepo *repomocks.DeploymentRepositoryMock
	ocClient       *clientmocks.OpenChoreoClientMock
	fetcher        *fakeA2ACardFetcher
	upstreamSecret string
}

// newA2AReconcilerHarness wires a reconciler whose every dependency succeeds and
// whose binding reports serviceURL. Tests narrow it to the condition they are
// about.
func newA2AReconcilerHarness(serviceURL string) *a2aReconcilerHarness {
	hub := &recordingEventHub{}
	pubRepo := &repomocks.A2APublicationRepositoryMock{
		MarkPublishedFunc: func(ctx context.Context, read models.A2APublication, upstreamURL string, transports json.RawMessage) error {
			return nil
		},
		MarkAttemptFailedFunc: func(ctx context.Context, read models.A2APublication, lastErr string, nextAttemptAt time.Time) error {
			return nil
		},
		MarkWaitingFunc: func(ctx context.Context, read models.A2APublication, lastErr string, nextAttemptAt time.Time) error {
			return nil
		},
		MarkFailedFunc: func(ctx context.Context, read models.A2APublication, lastErr string) error { return nil },
		GetFunc: func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
			return &models.A2APublication{}, nil
		},
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

	proxyRepo := &repomocks.A2AProxyRepositoryMock{
		LockAgentEnvFunc: func(context.Context, string, string, string, uuid.UUID) error { return nil },
	}
	// The reads happen inside the transaction; the tx-bound repositories are the mocks themselves.
	proxyRepo.WithTxFunc = func(tx *gorm.DB) repositories.A2AProxyRepository { return proxyRepo }
	cardRepo.WithTxFunc = func(tx *gorm.DB) repositories.A2AAgentCardRepository { return cardRepo }
	fetcher := &fakeA2ACardFetcher{}

	return &a2aReconcilerHarness{
		hub:            hub,
		gatewayRepo:    gatewayRepo,
		pubRepo:        pubRepo,
		cardRepo:       cardRepo,
		deploymentRepo: deploymentRepo,
		ocClient:       ocClient,
		proxyRepo:      proxyRepo,
		fetcher:        fetcher,
		svc: NewA2APublicationReconcilerService(sentinelTx, pubRepo, cardRepo, proxyRepo, deploymentRepo, gatewayRepo,
			agentConfigRepo, ocClient, fetcher, proxyTestKey, NewGatewayEventsService(hub), testLogger()).(*a2aPublicationReconcilerService),
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

// The reconciler runs off the request path, so no middleware resolves an org
// for it. The OpenChoreo client stamps X-Impersonate-Org only from the context's
// resolved org, and a platform that requires the header rejects the lookup — so
// every attempt fails until the budget runs out.
func TestReconcilerLooksUpTheBindingAsThePublicationsOrg(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")

	h.svc.publishOne(context.Background(), pendingPublication())

	lookups := h.ocClient.GetReleaseBindingServiceURLCalls()
	require.Len(t, lookups, 1)
	org, ok := orgctx.GetResolvedOrg(lookups[0].Ctx)
	require.True(t, ok, "the lookup context carries a resolved org")
	assert.Equal(t, "org-1", org.OUID)
}

func TestDriftCheckLooksUpEachBindingAsItsOwnOrg(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	first := publishedPublication("http://trip-planner.dp-default:9099")
	second := publishedPublication("http://trip-planner.dp-default:9099")
	second.OUID = "org-2"
	h.withPublished(first, second)

	h.svc.checkUpstreamDrift(context.Background())

	lookups := h.ocClient.GetReleaseBindingRolloutCalls()
	require.Len(t, lookups, 2)
	for i, want := range []string{"org-1", "org-2"} {
		org, ok := orgctx.GetResolvedOrg(lookups[i].Ctx)
		require.True(t, ok, "lookup %d carries a resolved org", i)
		assert.Equal(t, want, org.OUID)
	}
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
	h.pubRepo.MarkPublishedFunc = func(context.Context, models.A2APublication, string, json.RawMessage) error {
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

func TestReconcilerDropsTheCardRowWhenTheAgentWasReplacedMeanwhile(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.ocClient.GetComponentFunc = func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
		return &models.AgentResponse{UUID: uuid.NewString(), Name: name}, nil
	}
	h.cardRepo.DeleteForAgentEnvFunc = func(context.Context, string, string, string, string) error { return nil }
	pub := pendingPublication()
	queuedFor := uuid.New()
	pub.AgentUUID = &queuedFor

	h.svc.publishOne(context.Background(), pub)

	require.Len(t, h.cardRepo.DeleteForAgentEnvCalls(), 1)
}

func TestReconcilerKeepsTheCardRowOfTheSameAgent(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	queuedFor := uuid.New()
	h.ocClient.GetComponentFunc = func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
		return &models.AgentResponse{UUID: queuedFor.String(), Name: name}, nil
	}
	pub := pendingPublication()
	pub.AgentUUID = &queuedFor

	h.svc.publishOne(context.Background(), pub)

	assert.Empty(t, h.cardRepo.DeleteForAgentEnvCalls())
}

func TestReconcilerKeepsTheCardRowWhenTheAgentUUIDIsMissing(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.ocClient.GetComponentFunc = func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
		return &models.AgentResponse{UUID: "", Name: name}, nil
	}
	pub := pendingPublication()
	queuedFor := uuid.New()
	pub.AgentUUID = &queuedFor

	h.svc.publishOne(context.Background(), pub)

	assert.Empty(t, h.cardRepo.DeleteForAgentEnvCalls())
}

// Rows queued before the UUID was recorded keep the name-only check.
func TestReconcilerKeepsTheCardRowOfAnUnrecordedAgentThatStillExists(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.ocClient.GetComponentFunc = func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
		return &models.AgentResponse{UUID: uuid.NewString(), Name: name}, nil
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
		"that already failed on the serving release": func(h *a2aReconcilerHarness) {
			h.cardRepo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
				return &models.A2AAgentCard{
					Source: models.A2AAgentCardSourcePlatform, Status: models.A2AAgentCardStatusFailed,
					ReleaseName: "trip-planner-r1", AttemptedReleaseName: "trip-planner-r2",
				}, nil
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

// A failed card gets one new chance per release, so a fixing redeploy still heals it.
func TestDriftCheckRetriesAFailedCardOnANewRelease(t *testing.T) {
	const upstream = "http://trip-planner.dp-default:9099"
	h := newA2AReconcilerHarness(upstream)
	h.cardRepo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return &models.A2AAgentCard{
			Source: models.A2AAgentCardSourcePlatform, Status: models.A2AAgentCardStatusFailed,
			AttemptedReleaseName: "trip-planner-r1",
		}, nil
	}
	h.withPublished(publishedPublication(upstream))

	h.svc.checkUpstreamDrift(context.Background())

	require.Len(t, h.cardRepo.EnqueueCalls(), 1)
}

const originTestCardURL = "https://agent.example/.well-known/agent-card.json"

const originTestCard = `{"name":"Weather","skills":[],"supportedInterfaces":[` +
	`{"url":"https://agent.example/rpc","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`

// withExternalOrigin makes pub an external row whose environment is proxied with a stored secret.
func (h *a2aReconcilerHarness) withExternalOrigin(pub *models.A2APublication) {
	pub.Source = models.A2APublicationSourceExternal
	secret, err := utils.EncryptSecretRef("s3cret", proxyTestKey)
	if err != nil {
		panic(err)
	}
	h.upstreamSecret = secret
	h.ocClient.GetReleaseBindingServiceURLFunc = nil // an external agent has no release binding
	h.proxyRepo.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return &models.A2AProxy{UpstreamAuthHeader: "X-Upstream-Key", UpstreamAuthSecret: secret}, nil
	}
	h.cardRepo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return &models.A2AAgentCard{Source: models.A2AAgentCardSourceExternal, SourceURL: originTestCardURL}, nil
	}
	h.cardRepo.EnqueueFunc = nil // an upsert would rewrite the registered source
	h.cardRepo.RequeueFunc = func(context.Context, string, string, string, string) error { return nil }
	h.cardRepo.FailParkedFunc = func(context.Context, string, string, string, uuid.UUID, string) error { return nil }
	h.fetcher.FetchFunc = func(context.Context, string, bool) (json.RawMessage, error) {
		return json.RawMessage(originTestCard), nil
	}
}

func TestReconcilerPublishesAnExternalAgentFromItsOriginCard(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)

	h.svc.publishOne(context.Background(), pub)

	require.Len(t, h.fetcher.calls, 1)
	assert.Equal(t, fakeFetchCall{
		URL: originTestCardURL, Guarded: true,
		Header: &A2ACardFetchHeader{Name: "X-Upstream-Key", Value: "s3cret"},
	}, h.fetcher.calls[0], "the origin is SSRF-checked and sent the stored credential")
	created := h.deploymentRepo.CreateWithLimitEnforcementCalls()
	require.Len(t, created, 1)
	var doc A2AAgentDeploymentYAML
	require.NoError(t, yaml.Unmarshal(created[0].Deployment.Content, &doc))
	assert.Equal(t, "https://agent.example", doc.Spec.Upstream.URL)
	assert.Equal(t, []A2ATransport{{ProtocolBinding: "JSONRPC", PathPrefix: "/rpc"}}, doc.Spec.A2A.OperationConfigs.Transports)
	require.NotNil(t, doc.Spec.Upstream.Auth)
	assert.Equal(t, "X-Upstream-Key", *doc.Spec.Upstream.Auth.Header)
	assert.Equal(t, h.upstreamSecret, *doc.Spec.Upstream.Auth.SecretRef)

	published := h.pubRepo.MarkPublishedCalls()
	require.Len(t, published, 1)
	assert.Equal(t, "https://agent.example", published[0].UpstreamURL)
	requeued := h.cardRepo.RequeueCalls()
	require.Len(t, requeued, 1, "the parked card row is made due in place")
	assert.Equal(t, pub.EnvironmentName, requeued[0].EnvironmentName)
}

// The recorded transports are exactly what the gateway route was given, so the platform defaults are stored too.
func TestReconcilerRecordsThePlatformTransports(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")

	h.svc.publishOne(context.Background(), pendingPublication())

	marked := h.pubRepo.MarkPublishedCalls()
	require.Len(t, marked, 1)
	assert.JSONEq(t,
		`[{"protocolBinding":"JSONRPC","pathPrefix":"/rpc"},{"protocolBinding":"HTTP+JSON","pathPrefix":"/rest"}]`,
		string(marked[0].Transports))
}

func TestReconcilerRecordsTheOriginsTransports(t *testing.T) {
	cases := map[string]struct {
		iface string
		want  string
	}{
		"only HTTP+JSON below the origin": {
			iface: `{"url":"https://agent.example/rest","protocolBinding":"HTTP+JSON","protocolVersion":"1.0"}`,
			want:  `[{"protocolBinding":"HTTP+JSON","pathPrefix":"/rest"}]`,
		},
		"JSONRPC at the origin root": {
			iface: `{"url":"https://agent.example","protocolBinding":"JSONRPC","protocolVersion":"1.0"}`,
			want:  `[{"protocolBinding":"JSONRPC"}]`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newA2AReconcilerHarness("")
			pub := pendingPublication()
			h.withExternalOrigin(&pub)
			card := `{"name":"Weather","skills":[],"supportedInterfaces":[` + tc.iface + `]}`
			h.fetcher.FetchFunc = func(context.Context, string, bool) (json.RawMessage, error) {
				return json.RawMessage(card), nil
			}

			h.svc.publishOne(context.Background(), pub)

			marked := h.pubRepo.MarkPublishedCalls()
			require.Len(t, marked, 1)
			assert.JSONEq(t, tc.want, string(marked[0].Transports))
		})
	}
}

// oauthForwardingConfig is an environment config with OAuth on and the caller's token forwarded upstream.
func oauthForwardingConfig() *models.AgentConfig {
	return &models.AgentConfig{EnableOAuthSecurity: true, OAuthIssuers: []string{"idp"}, OAuthForwardToken: true}
}

// publishedJWTAuthParams returns the params of the jwt-auth policy in the one Agent the harness published.
func publishedJWTAuthParams(t *testing.T, h *a2aReconcilerHarness) map[string]interface{} {
	t.Helper()
	created := h.deploymentRepo.CreateWithLimitEnforcementCalls()
	require.Len(t, created, 1)
	var doc A2AAgentDeploymentYAML
	require.NoError(t, yaml.Unmarshal(created[0].Deployment.Content, &doc))
	for _, p := range doc.Spec.A2A.OperationConfigs.Policies {
		if p["name"] == client.OAuthPolicyName {
			return p["params"].(map[string]interface{})
		}
	}
	require.FailNow(t, "no jwt-auth policy was published")
	return nil
}

func TestReconcilerNeverForwardsTheCallersTokenToAnExternalOrigin(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.svc.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
			return oauthForwardingConfig(), nil
		},
	}

	h.svc.publishOne(context.Background(), pub)

	assert.Equal(t, false, publishedJWTAuthParams(t, h)["forwardToken"], "a third-party origin never sees the caller's token")
}

func TestReconcilerForwardsTheCallersTokenToAPlatformAgentWhenConfigured(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.svc.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
			return oauthForwardingConfig(), nil
		},
	}

	h.svc.publishOne(context.Background(), pendingPublication())

	assert.Equal(t, true, publishedJWTAuthParams(t, h)["forwardToken"])
}

func TestReconcilerFailsAnUnproxyableExternalRowAtOnce(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.cardRepo.GetFunc = func(context.Context, string, string, string, string) (*models.A2AAgentCard, error) {
		return &models.A2AAgentCard{Source: models.A2AAgentCardSourceExternal, SourceURL: "https://agent.example/card.json"}, nil
	}

	h.svc.publishOne(context.Background(), pub)

	failed := h.pubRepo.MarkFailedCalls()
	require.Len(t, failed, 1, "no attempt budget is spent on a card that can never be fronted")
	assert.Contains(t, failed[0].LastErr, "/.well-known/agent-card.json")
	assert.Empty(t, h.pubRepo.MarkAttemptFailedCalls())
	assert.Empty(t, h.fetcher.calls, "a URL the gateway cannot fetch from is rejected before any network call")
	assert.Empty(t, h.deploymentRepo.CreateWithLimitEnforcementCalls())
	cardFailed := h.cardRepo.FailParkedCalls()
	require.Len(t, cardFailed, 1, "the parked card says why it will not arrive instead of staying pending")
	assert.Contains(t, cardFailed[0].LastErr, "/.well-known/agent-card.json")
	assert.Equal(t, pub.EnvironmentUUID, cardFailed[0].EnvironmentUUID)
	assert.Empty(t, h.cardRepo.RequeueCalls())
}

// A pod from before the external source claims external rows during a rolling upgrade and fails them; the
// new reconciler makes them due again before its first tick, within one attempt's timeout, and a failure to
// do so is logged rather than stopping the loop.
func TestReconcilerRequeuesFailedExternalRowsBeforeTheFirstTick(t *testing.T) {
	for name, requeueErr := range map[string]error{"requeued": nil, "requeue failed": assert.AnError} {
		t.Run(name, func(t *testing.T) {
			h := newA2AReconcilerHarness("")
			var deadlineSet bool
			h.pubRepo.RequeueFailedExternalFunc = func(ctx context.Context) (int64, error) {
				_, deadlineSet = ctx.Deadline()
				return 2, requeueErr
			}

			h.svc.requeueFailedExternal(context.Background())

			assert.Len(t, h.pubRepo.RequeueFailedExternalCalls(), 1)
			assert.True(t, deadlineSet, "the update is bounded like an attempt")
		})
	}
}

// The proxy service clears a credential and moves a card URL under the environment's lock, in separate
// transactions; reading the two rows under that same lock keeps an attempt from pairing the old secret
// with the new origin.
func TestReconcilerReadsTheProxyAndTheCardUnderTheEnvironmentLock(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	var steps []string
	h.proxyRepo.LockAgentEnvFunc = func(_ context.Context, _, _, _ string, envUUID uuid.UUID) error {
		assert.Equal(t, pub.EnvironmentUUID, envUUID)
		steps = append(steps, "lock")
		return nil
	}
	h.proxyRepo.WithTxFunc = func(tx *gorm.DB) repositories.A2AProxyRepository {
		assert.Same(t, txSentinel, tx)
		steps = append(steps, "proxies-in-tx")
		return h.proxyRepo
	}
	h.cardRepo.WithTxFunc = func(tx *gorm.DB) repositories.A2AAgentCardRepository {
		assert.Same(t, txSentinel, tx)
		steps = append(steps, "cards-in-tx")
		return h.cardRepo
	}

	h.svc.publishOne(context.Background(), pub)

	assert.Equal(t, []string{"proxies-in-tx", "lock", "cards-in-tx"}, steps)
	require.Len(t, h.fetcher.calls, 1, "the fetch happens after the transaction released the lock")
}

func TestReconcilerFailsWhenTheEnvironmentLockCannotBeTaken(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.proxyRepo.LockAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return assert.AnError }

	h.svc.publishOne(context.Background(), pub)

	assert.Empty(t, h.fetcher.calls)
	require.Len(t, h.pubRepo.MarkAttemptFailedCalls(), 1)
}

func TestReconcilerFailsAnExternalRowWhoseProxyIsGone(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.proxyRepo.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return nil, repositories.ErrA2AProxyNotFound
	}

	h.svc.publishOne(context.Background(), pub)

	require.Len(t, h.pubRepo.MarkFailedCalls(), 1)
	assert.Empty(t, h.fetcher.calls)
}

// The third-party origin is not called for a publish a local lookup already rules out.
func TestReconcilerFetchesTheOriginOnlyAfterTheLocalLookups(t *testing.T) {
	t.Run("no ingress gateway", func(t *testing.T) {
		h := newA2AReconcilerHarness("")
		pub := pendingPublication()
		h.withExternalOrigin(&pub)
		h.svc.gatewayRepo = &repomocks.GatewayRepositoryMock{
			ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
				return []*models.Gateway{}, nil
			},
		}

		h.svc.publishOne(context.Background(), pub)

		assert.Empty(t, h.fetcher.calls)
		retries := h.pubRepo.MarkAttemptFailedCalls()
		require.Len(t, retries, 1)
		assert.Contains(t, retries[0].LastErr, "no ingress gateway")
	})
	t.Run("unreadable agent config", func(t *testing.T) {
		h := newA2AReconcilerHarness("")
		pub := pendingPublication()
		h.withExternalOrigin(&pub)
		h.svc.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{
			GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
				return nil, assert.AnError
			},
		}

		h.svc.publishOne(context.Background(), pub)

		assert.Empty(t, h.fetcher.calls)
		require.Len(t, h.pubRepo.MarkAttemptFailedCalls(), 1)
	})
}

func TestReconcilerRetriesWhileTheOriginIsUnreachable(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.fetcher.FetchFunc = func(context.Context, string, bool) (json.RawMessage, error) {
		return nil, newA2APublicError("could not reach the agent card URL", nil)
	}

	h.svc.publishOne(context.Background(), pub)

	require.Len(t, h.pubRepo.MarkAttemptFailedCalls(), 1)
	assert.Empty(t, h.pubRepo.MarkFailedCalls())
}

// A switch to direct committed while this attempt was publishing.
func TestReconcilerWithdrawsAnAgentTornDownMidPublish(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.pubRepo.MarkPublishedFunc = func(context.Context, models.A2APublication, string, json.RawMessage) error {
		return repositories.ErrA2APublicationSuperseded
	}
	h.pubRepo.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
		return nil, repositories.ErrA2APublicationNotFound
	}
	h.deploymentRepo.DeleteForArtifactFunc = func(context.Context, uuid.UUID, string) error { return nil }

	h.svc.publishOne(context.Background(), pub)

	deleted := h.deploymentRepo.DeleteForArtifactCalls()
	require.Len(t, deleted, 1)
	assert.Equal(t, pub.ArtifactUUID, deleted[0].ArtifactUUID)
	require.Len(t, h.hub.published, 2, "the deploy event, then its withdrawal")
	assert.Equal(t, pub.ArtifactUUID.String(), undeployedAgentID(t, h.hub.published[1]), "the artifact and its keys may still be wanted")
	assert.Empty(t, h.cardRepo.RequeueCalls())
}

// A plain re-enqueue also reports superseded; that row is published again next tick, untouched.
func TestReconcilerLeavesAReEnqueuedRowForTheNextTick(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.pubRepo.MarkPublishedFunc = func(context.Context, models.A2APublication, string, json.RawMessage) error {
		return repositories.ErrA2APublicationSuperseded
	}
	h.pubRepo.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
		return &pub, nil
	}
	// DeleteForArtifactFunc is nil: withdrawing would panic.

	h.svc.publishOne(context.Background(), pub)

	assert.Len(t, h.hub.published, 1)
}

func TestReconcilerLogsWhenItCannotTellIfASupersededRowWasTornDown(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	var logs bytes.Buffer
	h.svc.logger = slog.New(slog.NewTextHandler(&logs, nil))
	h.pubRepo.MarkPublishedFunc = func(context.Context, models.A2APublication, string, json.RawMessage) error {
		return repositories.ErrA2APublicationSuperseded
	}
	h.pubRepo.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
		return nil, errors.New("connection reset")
	}
	// DeleteForArtifactFunc is nil: withdrawing would panic.

	h.svc.publishOne(context.Background(), pub)

	assert.Contains(t, logs.String(), "level=ERROR")
	assert.Contains(t, logs.String(), pub.ArtifactUUID.String())
	assert.Contains(t, logs.String(), "connection reset")
	assert.Len(t, h.hub.published, 1, "nothing is withdrawn")
}

func TestReconcilerSkipsTheCardTriggerWhenTheExternalRowIsGone(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.cardRepo.RequeueFunc = func(context.Context, string, string, string, string) error {
		return repositories.ErrA2AAgentCardNotFound
	}

	h.svc.publishOne(context.Background(), pub)

	require.Len(t, h.pubRepo.MarkPublishedCalls(), 1)
	assert.Empty(t, h.ocClient.GetComponentCalls(), "nothing was written, so nothing to drop")
}

func TestReconcilerStoresAValidUTF8LastError(t *testing.T) {
	h := newA2AReconcilerHarness("")
	h.ocClient.GetReleaseBindingServiceURLFunc = func(context.Context, string, string, string) (string, error) {
		return "", errors.New("lookup \xff failed")
	}

	h.svc.publishOne(context.Background(), pendingPublication())

	failed := h.pubRepo.MarkAttemptFailedCalls()
	require.Len(t, failed, 1)
	assert.True(t, utf8.ValidString(failed[0].LastErr))
}

func serviceURLBlocksUntilCancelled(h *a2aReconcilerHarness) {
	h.ocClient.GetReleaseBindingServiceURLFunc = func(ctx context.Context, _, _, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
}

func TestReconcilerBoundsEachPublishAttempt(t *testing.T) {
	h := newA2AReconcilerHarness("")
	serviceURLBlocksUntilCancelled(h)
	h.svc.attemptTimeout = 20 * time.Millisecond

	runWithin(t, 2*time.Second, func() { h.svc.publishOne(context.Background(), pendingPublication()) })

	require.Len(t, h.pubRepo.MarkAttemptFailedCalls(), 1, "a timed-out attempt is still recorded")
}

// Rows still running when the lease ends could be re-claimed and published twice.
func TestReconcilerStopsTheBatchBeforeTheLeaseEnds(t *testing.T) {
	h := newA2AReconcilerHarness("")
	serviceURLBlocksUntilCancelled(h)
	h.svc.batchBudget = 20 * time.Millisecond
	h.pubRepo.ClaimDueFunc = func(context.Context, time.Time, int) ([]models.A2APublication, error) {
		rows := make([]models.A2APublication, a2aWorkerConcurrency+2)
		for i := range rows {
			rows[i] = pendingPublication()
		}
		return rows, nil
	}
	h.pubRepo.FindPublishedFunc = func(context.Context, uuid.UUID, int) ([]models.A2APublication, error) {
		return []models.A2APublication{}, nil
	}

	runWithin(t, 2*time.Second, func() { h.svc.RunOnce(context.Background()) })

	assert.Len(t, h.ocClient.GetReleaseBindingServiceURLCalls(), a2aWorkerConcurrency)
	assert.Len(t, h.pubRepo.MarkAttemptFailedCalls(), a2aWorkerConcurrency, "only the cut-off rows are charged")
}

func TestPublicationBatchFitsInsideTheClaimLease(t *testing.T) {
	assert.Less(t, a2aPublicationBatchBudget, repositories.A2APublicationClaimLease)
}

func TestReconcilerHidesInternalErrorsFromLastError(t *testing.T) {
	h := newA2AReconcilerHarness("http://svc.dev:8080")
	h.deploymentRepo.CreateWithLimitEnforcementFunc = func(*models.Deployment, int) error {
		return errors.New("failed to connect to `user=ams database=amdb host=10.0.0.5`: SQLSTATE 28P01")
	}

	h.svc.publishOne(context.Background(), pendingPublication())

	retries := h.pubRepo.MarkAttemptFailedCalls()
	require.Len(t, retries, 1)
	assert.Equal(t, a2aPublishInternalError, retries[0].LastErr)
}

func TestReconcilerNamesTheOriginInAFetchFailure(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.fetcher.FetchFunc = func(context.Context, string, bool) (json.RawMessage, error) {
		return nil, newA2APublicError("agent card request returned HTTP 503", nil)
	}

	h.svc.publishOne(context.Background(), pub)

	retries := h.pubRepo.MarkAttemptFailedCalls()
	require.Len(t, retries, 1)
	assert.Equal(t, "could not fetch the origin agent card: agent card request returned HTTP 503", retries[0].LastErr)
}

func TestReconcilerFetchesAnUnauthenticatedOriginWithoutAHeader(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.proxyRepo.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return &models.A2AProxy{}, nil
	}

	h.svc.publishOne(context.Background(), pub)

	require.Len(t, h.fetcher.calls, 1)
	assert.Nil(t, h.fetcher.calls[0].Header)
}

func TestReconcilerRetriesWhenTheUpstreamSecretCannotBeDecrypted(t *testing.T) {
	h := newA2AReconcilerHarness("")
	pub := pendingPublication()
	h.withExternalOrigin(&pub)
	h.proxyRepo.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return &models.A2AProxy{UpstreamAuthHeader: "X-Upstream-Key", UpstreamAuthSecret: "bm90LWNpcGhlcnRleHQ="}, nil
	}

	h.svc.publishOne(context.Background(), pub)

	assert.Empty(t, h.fetcher.calls, "no fetch without the credential the origin needs")
	retries := h.pubRepo.MarkAttemptFailedCalls()
	require.Len(t, retries, 1)
	assert.Equal(t, a2aPublishInternalError, retries[0].LastErr)
}
