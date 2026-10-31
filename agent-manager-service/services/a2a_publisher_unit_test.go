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
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// txSentinel stands in for an open transaction; tx-bound mocks check they were handed it.
var txSentinel = &gorm.DB{}

// sentinelTx hands fn the shared transaction sentinel; publisherMocks.tx adds a counter and an injected failure on top.
func sentinelTx(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(txSentinel) }

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// publisherMocks keeps the plain and the transaction-bound mocks apart, so a write that
// escapes the transaction reaches a nil func and panics.
type publisherMocks struct {
	exposures, txExposures     *repomocks.PlatformAgentExposureRepositoryMock
	proxies                    *repomocks.A2AProxyRepositoryMock
	envCards                   *repomocks.A2AAgentCardRepositoryMock
	deployments, txDeployments *repomocks.DeploymentRepositoryMock
	gateways                   *repomocks.GatewayRepositoryMock
	artifacts                  *repomocks.ArtifactRepositoryMock
	hub                        *recordingEventHub
	// transactions counts the transactions opened; txErr, when set, makes each one fail to commit.
	transactions int
	txErr        error
}

// tx is the runner the services open transactions with; it hands fn the sentinel.
func (m *publisherMocks) tx() TxRunner {
	return func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		m.transactions++
		if m.txErr != nil {
			return m.txErr
		}
		return sentinelTx(ctx, fn)
	}
}

func newPublisherMocks(t *testing.T) *publisherMocks {
	m := &publisherMocks{
		txExposures: &repomocks.PlatformAgentExposureRepositoryMock{},
		proxies: &repomocks.A2AProxyRepositoryMock{
			ListForEnvironmentFunc: func(context.Context, string, uuid.UUID) ([]models.A2AProxy, error) {
				return []models.A2AProxy{}, nil
			},
		},
		txDeployments: &repomocks.DeploymentRepositoryMock{},
		gateways: &repomocks.GatewayRepositoryMock{
			ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
				return []*models.Gateway{}, nil
			},
		},
		envCards: &repomocks.A2AAgentCardRepositoryMock{
			ListForEnvironmentFunc: func(context.Context, string, uuid.UUID) ([]models.A2AAgentCard, error) {
				return []models.A2AAgentCard{}, nil
			},
		},
		artifacts: &repomocks.ArtifactRepositoryMock{},
		hub:       &recordingEventHub{},
	}
	m.artifacts.WithTxFunc = func(*gorm.DB) repositories.ArtifactRepository { return m.artifacts }
	m.gateways.WithTxFunc = func(*gorm.DB) repositories.GatewayRepository { return m.gateways }
	m.exposures = &repomocks.PlatformAgentExposureRepositoryMock{
		WithTxFunc: func(tx *gorm.DB) repositories.PlatformAgentExposureRepository {
			assert.Same(t, txSentinel, tx)
			return m.txExposures
		},
	}
	m.deployments = &repomocks.DeploymentRepositoryMock{
		WithTxFunc: func(tx *gorm.DB) repositories.DeploymentRepository {
			assert.Same(t, txSentinel, tx)
			return m.txDeployments
		},
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) { return []string{"gw-1"}, nil },
		QueueRenderFunc: func(context.Context, *gorm.DB, string, uuid.UUID, []uuid.UUID, string,
			repositories.QueueScope,
		) (int64, error) {
			return 0, nil
		},
	}
	return m
}

func (m *publisherMocks) publisher() *A2APublisher {
	exposure := NewAgentExposurePublisher(m.tx(), m.exposures, m.artifacts, m.deployments, m.gateways, nil, testLogger())
	return withA2ACore(NewA2APublisher(m.tx(), m.exposures, m.proxies, m.envCards, m.artifacts, m.deployments, testLogger(), exposure, nil), m.hub)
}

func testA2AKey() agentExposureKey {
	return agentExposureKey{
		OUID: "org-1", ProjectName: "checkout", AgentName: "weather", EnvironmentName: "dev",
		EnvironmentUUID: uuid.MustParse("55555555-5555-5555-5555-555555555555"),
	}
}

// externalAgent carries no subtype: an external agent serves A2A wherever it registers a card URL.
func externalAgent() *models.AgentResponse {
	return &models.AgentResponse{
		UUID: "agent-uuid", Name: "weather", ProjectName: "checkout",
		Provisioning: models.Provisioning{Type: "external"},
		Type:         models.AgentType{Type: "external-agent-api"},
	}
}

// legacyExternalA2AAgent was created when A2A was an external subtype; its label no longer matters.
func legacyExternalA2AAgent() *models.AgentResponse {
	agent := externalAgent()
	agent.Type.SubType = "a2a-agent"
	return agent
}

func TestA2APublisherShouldPublish(t *testing.T) {
	internal := &models.AgentResponse{
		Name: "weather", Provisioning: models.Provisioning{Type: "internal"},
		Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"},
	}
	chat := &models.AgentResponse{Name: "weather", Type: models.AgentType{Type: "agent-api", SubType: "chat-api"}}
	cases := []struct {
		name   string
		agent  *models.AgentResponse
		lookup func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error)
		want   bool
	}{
		{name: "an internal A2A agent always, without a proxy lookup", agent: internal, want: true},
		{name: "a non-A2A agent never", agent: chat, want: false},
		{
			name: "an external agent in a direct environment", agent: externalAgent(),
			lookup: func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
				return nil, repositories.ErrA2AProxyNotFound
			},
		},
		{
			name: "an external agent in a proxied environment", agent: externalAgent(), want: true,
			lookup: func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
				return &models.A2AProxy{}, nil
			},
		},
		{
			name: "a legacy external A2A agent in a proxied environment", agent: legacyExternalA2AAgent(), want: true,
			lookup: func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
				return &models.A2AProxy{}, nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newPublisherMocks(t)
			m.proxies.GetByEnvUUIDFunc = tc.lookup
			got, err := m.publisher().ShouldPublish(context.Background(), nil, tc.agent, "org-1", "checkout", testA2AKey().EnvironmentUUID)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestA2APublisherShouldPublishDoesNotMaskALookupFailure(t *testing.T) {
	m := newPublisherMocks(t)
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return nil, assert.AnError
	}
	_, err := m.publisher().ShouldPublish(context.Background(), nil, externalAgent(), "org-1", "checkout", uuid.New())
	assert.ErrorIs(t, err, assert.AnError)
}

// An internal agent always publishes and has no proxy row to race with, so its save takes no lock.
func TestA2APublisherSaveAndRepublishSkipsTheLockForAnInternalAgent(t *testing.T) {
	m := newPublisherMocks(t)
	internal := &models.AgentResponse{
		Name: "weather", Provisioning: models.Provisioning{Type: "internal"},
		Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"},
	}
	m.proxies.WithTxFunc = func(*gorm.DB) repositories.A2AProxyRepository { return m.proxies }
	m.txExposures.UpsertFunc = func(context.Context, *models.PlatformAgentExposure) error { return nil }
	var savedIn *gorm.DB

	err := m.publisher().SaveAndRepublish(context.Background(), internal, testA2AKey(), models.ArtifactRef{ArtifactUUID: uuid.New()}, func(tx *gorm.DB) error {
		savedIn = tx
		return nil
	})

	require.NoError(t, err)
	assert.Same(t, txSentinel, savedIn)
	assert.Empty(t, m.proxies.LockAgentEnvCalls())
	assert.Empty(t, m.proxies.GetByEnvUUIDCalls())
	require.Len(t, m.txExposures.UpsertCalls(), 1)
	assert.Equal(t, models.AgentExposureProtocolA2A, m.txExposures.UpsertCalls()[0].Exposure.Protocol)
}

// An internal agent's save and exposure are one transaction: a failed exposure fails the save.
func TestA2APublisherSaveAndRepublishFailsWhenTheExposureCannotBeWritten(t *testing.T) {
	m := newPublisherMocks(t)
	internal := &models.AgentResponse{
		Name: "weather", Provisioning: models.Provisioning{Type: "internal"},
		Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"},
	}
	m.txExposures.UpsertFunc = func(context.Context, *models.PlatformAgentExposure) error { return assert.AnError }

	err := m.publisher().SaveAndRepublish(context.Background(), internal, testA2AKey(), models.ArtifactRef{ArtifactUUID: uuid.New()}, func(*gorm.DB) error {
		return nil
	})

	assert.ErrorIs(t, err, assert.AnError)
}

func TestA2APublisherSaveAndRepublishDoesNotSaveWhenTheLockFails(t *testing.T) {
	m := newPublisherMocks(t)
	m.proxies.WithTxFunc = func(*gorm.DB) repositories.A2AProxyRepository { return m.proxies }
	m.proxies.LockAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return assert.AnError }
	saved := false

	err := m.publisher().SaveAndRepublish(context.Background(), externalAgent(), testA2AKey(), models.ArtifactRef{ArtifactUUID: uuid.New()}, func(*gorm.DB) error {
		saved = true
		return nil
	})

	assert.ErrorIs(t, err, assert.AnError)
	assert.False(t, saved)
}

// A proxied external agent's record is its proxy row; its save writes no exposure.
func TestA2APublisherSaveAndRepublishExposesNoExternalAgent(t *testing.T) {
	m := newPublisherMocks(t) // txExposures.UpsertFunc is nil: an exposure would panic
	m.proxies.WithTxFunc = func(*gorm.DB) repositories.A2AProxyRepository { return m.proxies }
	m.proxies.LockAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return &models.A2AProxy{}, nil
	}
	saved := false

	err := m.publisher().SaveAndRepublish(context.Background(), externalAgent(), testA2AKey(), models.ArtifactRef{ArtifactUUID: uuid.New()}, func(*gorm.DB) error {
		saved = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, saved)
	assert.Empty(t, m.txExposures.UpsertCalls())
}

func TestA2APublisherDeleteForAgentDropsPublicationsAndProxies(t *testing.T) {
	m := newPublisherMocks(t)
	m.exposures.DeleteForAgentFunc = func(context.Context, string, string, string) error { return nil }
	m.proxies.DeleteForAgentFunc = func(context.Context, string, string, string) error { return nil }

	require.NoError(t, m.publisher().DeleteForAgent(context.Background(), "org-1", "checkout", "weather"))
	assert.Len(t, m.exposures.DeleteForAgentCalls(), 1)
	assert.Len(t, m.proxies.DeleteForAgentCalls(), 1)
}

func TestA2APublisherEnvironmentUUIDsListsProxiesWhenPublicationsFail(t *testing.T) {
	id := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	m := newPublisherMocks(t)
	m.exposures.ListEnvironmentUUIDsFunc = func(context.Context, string, string) ([]uuid.UUID, error) {
		return nil, errors.New("publication listing down")
	}
	m.proxies.ListEnvironmentUUIDsFunc = func(context.Context, string, string) ([]uuid.UUID, error) {
		return []uuid.UUID{id}, nil
	}

	ids, err := m.publisher().EnvironmentUUIDs(context.Background(), "acme", "dev")

	require.Error(t, err)
	assert.Contains(t, ids, id)
}

// withA2ACore gives p a reconcile core over its deployment repository, hub and gateways, and rebuilds its shared
// exposure publisher on that core.
func withA2ACore(p *A2APublisher, hub eventhub.EventHub) *A2APublisher {
	shared := p.exposure
	p.kind = &A2AAgentKind{
		exposures: NewPlatformAgentExposures(p.exposureRepo, shared.gatewayRepo, nil),
		proxyRepo: p.proxyRepo, cardRepo: p.cardRepo, deploymentRepo: p.deploymentRepo, logger: testLogger(),
	}
	core := reconcile.NewCore(nil, p.deploymentRepo, hub, reconcile.Kinds{p.kind}, reconcile.Config{}, testLogger())
	p.exposure = NewAgentExposurePublisher(p.tx, p.exposureRepo, p.artifactRepo, p.deploymentRepo, shared.gatewayRepo, core, testLogger())
	return p
}

// A deleted environment's exposed agents lose their artifacts, each with its delete events in the same transaction.
func TestA2APublisherTeardownEnvironmentDropsEachArtifactWithItsDeleteEvents(t *testing.T) {
	m := newPublisherMocks(t)
	envUUID, artifactUUID, gateway := uuid.New(), uuid.New(), uuid.New()
	m.exposures.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.PlatformAgentExposure, error) {
		return []models.PlatformAgentExposure{{AgentName: "trip-planner", ArtifactUUID: artifactUUID, Protocol: models.AgentExposureProtocolA2A}}, nil
	}
	m.exposures.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.proxies.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.txDeployments.DeleteEventGatewaysFunc = func(context.Context, string, uuid.UUID) ([]string, error) {
		return []string{gateway.String()}, nil
	}
	m.artifacts.DeleteFunc = func(tx *gorm.DB, id string) error {
		assert.Same(t, txSentinel, tx)
		assert.Equal(t, artifactUUID.String(), id)
		return nil
	}

	require.NoError(t, m.publisher().TeardownEnvironment(context.Background(), "org-1", envUUID))

	require.Len(t, m.hub.published, 1)
	assert.Equal(t, "agent.deleted", string(m.hub.published[0].EventType))
	assert.Equal(t, gateway.String(), m.hub.published[0].GatewayID)
	assert.Same(t, txSentinel, m.hub.txs[0])
	require.Len(t, m.artifacts.DeleteCalls(), 1)
}

// A deleted environment's proxied external agents are found by their proxy rows, card or no card, and lose their
// artifacts with their delete events.
func TestA2APublisherTeardownEnvironmentDropsAProxiedAgentsArtifact(t *testing.T) {
	m := newPublisherMocks(t)
	envUUID, artifactUUID, gateway := uuid.New(), uuid.New(), uuid.New()
	m.exposures.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.PlatformAgentExposure, error) {
		return nil, nil
	}
	m.proxies.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2AProxy, error) {
		return []models.A2AProxy{{ProjectName: "checkout", AgentName: "weather"}}, nil
	}
	m.exposures.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.proxies.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.artifacts.GetByHandleFunc = func(handle, _ string) (*models.Artifact, error) {
		assert.Equal(t, agentEnvAPIArtifactHandle("checkout", "weather", envUUID.String()), handle)
		return &models.Artifact{UUID: artifactUUID, Handle: handle, Kind: models.KindAgent}, nil
	}
	m.txDeployments.DeleteEventGatewaysFunc = func(context.Context, string, uuid.UUID) ([]string, error) {
		return []string{gateway.String()}, nil
	}
	m.artifacts.DeleteFunc = func(*gorm.DB, string) error { return nil }

	require.NoError(t, m.publisher().TeardownEnvironment(context.Background(), "org-1", envUUID))

	require.Len(t, m.artifacts.DeleteCalls(), 1)
	assert.Equal(t, artifactUUID.String(), m.artifacts.DeleteCalls()[0].UUID)
	require.Len(t, m.hub.published, 1)
	assert.Equal(t, "agent.deleted", string(m.hub.published[0].EventType))
}
