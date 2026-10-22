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
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// txSentinel stands in for an open transaction; tx-bound mocks check they were handed it.
var txSentinel = &gorm.DB{}

// publisherMocks keeps the plain and the transaction-bound mocks apart, so a write that
// escapes the transaction reaches a nil func and panics.
type publisherMocks struct {
	pubs, txPubs               *repomocks.A2APublicationRepositoryMock
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
		txPubs:        &repomocks.A2APublicationRepositoryMock{},
		proxies:       &repomocks.A2AProxyRepositoryMock{},
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
	m.pubs = &repomocks.A2APublicationRepositoryMock{
		WithTxFunc: func(tx *gorm.DB) repositories.A2APublicationRepository {
			assert.Same(t, txSentinel, tx)
			return m.txPubs
		},
	}
	m.deployments = &repomocks.DeploymentRepositoryMock{
		WithTxFunc: func(tx *gorm.DB) repositories.DeploymentRepository {
			assert.Same(t, txSentinel, tx)
			return m.txDeployments
		},
		GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) { return []string{"gw-1"}, nil },
	}
	return m
}

func (m *publisherMocks) publisher() *A2APublisher {
	return NewA2APublisher(m.tx(), m.pubs, m.proxies, m.envCards, m.artifacts, m.deployments, m.gateways, NewGatewayEventsService(m.hub), testLogger())
}

func testA2AKey() a2aPublicationKey {
	return a2aPublicationKey{
		OUID: "org-1", ProjectName: "checkout", AgentName: "weather", EnvironmentName: "dev",
		EnvironmentUUID: uuid.MustParse("55555555-5555-5555-5555-555555555555"),
	}
}

func externalA2AAgent() *models.AgentResponse {
	return &models.AgentResponse{
		UUID: "agent-uuid", Name: "weather",
		Provisioning: models.Provisioning{Type: "external"},
		Type:         models.AgentType{Type: "external-agent-api", SubType: "a2a-agent"},
	}
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
			name: "an external A2A agent in a direct environment", agent: externalA2AAgent(),
			lookup: func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
				return nil, repositories.ErrA2AProxyNotFound
			},
		},
		{
			name: "an external A2A agent in a proxied environment", agent: externalA2AAgent(), want: true,
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
	_, err := m.publisher().ShouldPublish(context.Background(), nil, externalA2AAgent(), "org-1", "checkout", uuid.New())
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
	m.txPubs.EnqueueFunc = func(context.Context, *models.A2APublication) error { return nil }
	var savedIn *gorm.DB

	err := m.publisher().SaveAndRepublish(context.Background(), internal, testA2AKey(), uuid.New(), func(tx *gorm.DB) error {
		savedIn = tx
		return nil
	})

	require.NoError(t, err)
	assert.Same(t, txSentinel, savedIn)
	assert.Empty(t, m.proxies.LockAgentEnvCalls())
	assert.Empty(t, m.proxies.GetByEnvUUIDCalls())
	require.Len(t, m.txPubs.EnqueueCalls(), 1)
	assert.Equal(t, models.A2APublicationSourcePlatform, m.txPubs.EnqueueCalls()[0].Pub.Source)
}

func TestA2APublisherSaveAndRepublishDoesNotSaveWhenTheLockFails(t *testing.T) {
	m := newPublisherMocks(t)
	m.proxies.WithTxFunc = func(*gorm.DB) repositories.A2AProxyRepository { return m.proxies }
	m.proxies.LockAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return assert.AnError }
	saved := false

	err := m.publisher().SaveAndRepublish(context.Background(), externalA2AAgent(), testA2AKey(), uuid.New(), func(*gorm.DB) error {
		saved = true
		return nil
	})

	assert.ErrorIs(t, err, assert.AnError)
	assert.False(t, saved)
}

func TestA2APublisherEnqueueJoinsTheCallersTransaction(t *testing.T) {
	m := newPublisherMocks(t)
	m.txPubs.EnqueueFunc = func(context.Context, *models.A2APublication) error { return nil }
	artifactUUID := uuid.New()

	require.NoError(t, m.publisher().Enqueue(context.Background(), txSentinel, testA2AKey(),
		models.A2APublicationSourceExternal, artifactUUID))

	queued := m.txPubs.EnqueueCalls()
	require.Len(t, queued, 1)
	assert.Equal(t, models.A2APublicationSourceExternal, queued[0].Pub.Source)
	assert.Equal(t, artifactUUID, queued[0].Pub.ArtifactUUID)
	assert.Equal(t, testA2AKey().EnvironmentUUID, queued[0].Pub.EnvironmentUUID)
	assert.Equal(t, "dev", queued[0].Pub.EnvironmentName)
}

// The artifact stays so the environment's API keys survive a switch back to proxied.
func TestA2APublisherTeardownEnvKeepsTheArtifactAndWaitsToTellTheGateway(t *testing.T) {
	m := newPublisherMocks(t)
	m.txDeployments.DeleteForArtifactFunc = func(context.Context, uuid.UUID, string) error { return nil }
	m.txPubs.DeleteForAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	artifactUUID := uuid.New()
	publisher := m.publisher()

	teardown, err := publisher.TeardownEnv(context.Background(), txSentinel, testA2AKey(), artifactUUID)
	require.NoError(t, err)
	assert.Empty(t, m.artifacts.DeleteCalls())
	require.Len(t, m.txDeployments.DeleteForArtifactCalls(), 1)
	assert.Equal(t, artifactUUID, m.txDeployments.DeleteForArtifactCalls()[0].ArtifactUUID)
	require.Len(t, m.txPubs.DeleteForAgentEnvCalls(), 1)
	assert.Empty(t, m.hub.published, "nothing is sent before the transaction commits")

	publisher.SendTeardown(context.Background(), teardown)
	require.Len(t, m.hub.published, 1)
	assert.Equal(t, "gw-1", m.hub.published[0].GatewayID)
	assert.Equal(t, artifactUUID.String(), undeployedAgentID(t, m.hub.published[0]))
}

func TestA2APublisherDeleteForAgentDropsPublicationsAndProxies(t *testing.T) {
	m := newPublisherMocks(t)
	m.pubs.DeleteForAgentFunc = func(context.Context, string, string, string) error { return nil }
	m.proxies.DeleteForAgentFunc = func(context.Context, string, string, string) error { return nil }

	require.NoError(t, m.publisher().DeleteForAgent(context.Background(), "org-1", "checkout", "weather"))
	assert.Len(t, m.pubs.DeleteForAgentCalls(), 1)
	assert.Len(t, m.proxies.DeleteForAgentCalls(), 1)
}

func TestA2APublisherTeardownEnvAndDropArtifactDeletesTheArtifactInTheTransaction(t *testing.T) {
	m := newPublisherMocks(t)
	m.txDeployments.DeleteForArtifactFunc = func(context.Context, uuid.UUID, string) error { return nil }
	m.txPubs.DeleteForAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	m.artifacts.DeleteFunc = func(tx *gorm.DB, _ string) error {
		assert.Same(t, txSentinel, tx)
		return gorm.ErrRecordNotFound // already gone is not a failure
	}
	artifactUUID := uuid.New()

	teardown, err := m.publisher().TeardownEnvAndDropArtifact(context.Background(), txSentinel, testA2AKey(), artifactUUID)

	require.NoError(t, err)
	assert.Equal(t, artifactUUID, teardown.artifactUUID)
	require.Len(t, m.artifacts.DeleteCalls(), 1)
	assert.Equal(t, artifactUUID.String(), m.artifacts.DeleteCalls()[0].UUID)
	assert.Len(t, m.txPubs.DeleteForAgentEnvCalls(), 1)
}

func TestA2APublisherTeardownEnvAndDropArtifactSkipsAMissingArtifact(t *testing.T) {
	m := newPublisherMocks(t)
	m.txPubs.DeleteForAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	// artifacts.DeleteFunc is nil: deleting would panic.

	_, err := m.publisher().TeardownEnvAndDropArtifact(context.Background(), txSentinel, testA2AKey(), uuid.Nil)

	require.NoError(t, err)
	assert.Empty(t, m.txDeployments.DeleteForArtifactCalls())
	assert.Len(t, m.txPubs.DeleteForAgentEnvCalls(), 1)
}

func TestA2APublisherTeardownEnvAndDropArtifactReportsAFailedDelete(t *testing.T) {
	m := newPublisherMocks(t)
	m.txDeployments.DeleteForArtifactFunc = func(context.Context, uuid.UUID, string) error { return nil }
	m.txPubs.DeleteForAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	m.artifacts.DeleteFunc = func(*gorm.DB, string) error { return assert.AnError }

	_, err := m.publisher().TeardownEnvAndDropArtifact(context.Background(), txSentinel, testA2AKey(), uuid.New())

	assert.ErrorIs(t, err, assert.AnError)
}

func TestA2APublisherTeardownEnvironmentRemovesEverythingAndTellsTheGateways(t *testing.T) {
	m := newPublisherMocks(t)
	envUUID, artifactUUID := uuid.New(), uuid.New()
	pub := models.A2APublication{
		OUID: "org-1", ProjectName: "checkout", AgentName: "weather", EnvironmentName: "dev",
		EnvironmentUUID: envUUID, ArtifactUUID: artifactUUID, Source: models.A2APublicationSourceExternal,
	}
	m.pubs.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2APublication, error) {
		return []models.A2APublication{pub}, nil
	}
	m.pubs.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.proxies.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.txDeployments.DeleteForArtifactFunc = func(context.Context, uuid.UUID, string) error { return nil }
	m.txPubs.DeleteForAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	m.artifacts.DeleteFunc = func(tx *gorm.DB, _ string) error {
		assert.Same(t, txSentinel, tx)
		return nil
	}

	require.NoError(t, m.publisher().TeardownEnvironment(context.Background(), "org-1", envUUID))

	require.Len(t, m.artifacts.DeleteCalls(), 1, "a deleted environment needs no artifact")
	assert.Len(t, m.pubs.DeleteForEnvironmentCalls(), 1)
	assert.Len(t, m.proxies.DeleteForEnvironmentCalls(), 1)
	require.Len(t, m.hub.published, 1)
	assert.Equal(t, artifactUUID.String(), deletedAgentID(t, m.hub.published[0]))
}

// A switch to direct drops the publication but keeps the artifact; the card row still names the agent, and
// the handle it derives names the environment.
func TestA2APublisherTeardownEnvironmentDropsTheArtifactOfADirectEnvironment(t *testing.T) {
	m := newPublisherMocks(t)
	envUUID, gwUUID := uuid.New(), uuid.New()
	leftover := &models.Artifact{UUID: uuid.New(), Handle: "checkout/weather/" + envUUID.String(), Kind: models.KindAgent}
	m.pubs.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2APublication, error) {
		return []models.A2APublication{}, nil
	}
	m.envCards.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2AAgentCard, error) {
		return []models.A2AAgentCard{{ProjectName: "checkout", AgentName: "weather", EnvironmentUUID: envUUID}}, nil
	}
	// The switch to direct dropped the deployment rows, so only the org's active gateways name the holder.
	m.deployments.GetDeployedGatewaysByProviderFunc = func(uuid.UUID, string) ([]string, error) { return []string{}, nil }
	m.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return []*models.Gateway{{UUID: gwUUID}}, nil
	}
	m.pubs.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.proxies.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.artifacts.GetByHandleFunc = func(handle, ouID string) (*models.Artifact, error) {
		assert.Equal(t, leftover.Handle, handle)
		assert.Equal(t, "org-1", ouID)
		return leftover, nil
	}
	m.artifacts.DeleteFunc = func(tx *gorm.DB, _ string) error {
		assert.Same(t, txSentinel, tx)
		return nil
	}

	require.NoError(t, m.publisher().TeardownEnvironment(context.Background(), "org-1", envUUID))

	require.Len(t, m.artifacts.GetByHandleCalls(), 1)
	require.Len(t, m.artifacts.DeleteCalls(), 1)
	assert.Equal(t, leftover.UUID.String(), m.artifacts.DeleteCalls()[0].UUID)
	require.Len(t, m.hub.published, 1)
	assert.Equal(t, gwUUID.String(), m.hub.published[0].GatewayID)
	assert.Equal(t, eventhub.EventType("agent.deleted"), m.hub.published[0].EventType)
	assert.Equal(t, leftover.UUID.String(), deletedAgentID(t, m.hub.published[0]))
	filters := m.gateways.ListWithFiltersCalls()
	require.Len(t, filters, 1)
	assert.Equal(t, "org-1", filters[0].Filters.OrganizationID)
	require.NotNil(t, filters[0].Filters.Status)
	assert.True(t, *filters[0].Filters.Status)
}

func TestA2APublisherTeardownEnvironmentDoesNotDropAnArtifactTwice(t *testing.T) {
	m := newPublisherMocks(t)
	envUUID, artifactUUID := uuid.New(), uuid.New()
	m.pubs.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2APublication, error) {
		return []models.A2APublication{{
			OUID: "org-1", AgentName: "weather", EnvironmentUUID: envUUID, ArtifactUUID: artifactUUID,
		}}, nil
	}
	m.pubs.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.proxies.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.txDeployments.DeleteForArtifactFunc = func(context.Context, uuid.UUID, string) error { return nil }
	m.txPubs.DeleteForAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	m.envCards.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2AAgentCard, error) {
		return []models.A2AAgentCard{{ProjectName: "checkout", AgentName: "weather", EnvironmentUUID: envUUID}}, nil
	}
	m.artifacts.GetByHandleFunc = func(string, string) (*models.Artifact, error) {
		return &models.Artifact{UUID: artifactUUID, Kind: models.KindAgent}, nil
	}
	m.artifacts.DeleteFunc = func(*gorm.DB, string) error { return nil }

	require.NoError(t, m.publisher().TeardownEnvironment(context.Background(), "org-1", envUUID))

	require.Len(t, m.artifacts.DeleteCalls(), 1)
	assert.Equal(t, artifactUUID.String(), m.artifacts.DeleteCalls()[0].UUID)
	assert.Len(t, m.hub.published, 1)
}

func TestA2APublisherTeardownEnvironmentReportsAnArtifactItCannotListOrDelete(t *testing.T) {
	envUUID := uuid.New()
	leftover := &models.Artifact{UUID: uuid.New(), Handle: "checkout/weather/" + envUUID.String(), Kind: models.KindAgent}
	namesWeather := func(m *publisherMocks) {
		m.envCards.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2AAgentCard, error) {
			return []models.A2AAgentCard{{ProjectName: "checkout", AgentName: "weather", EnvironmentUUID: envUUID}}, nil
		}
	}
	cases := []struct {
		name  string
		setup func(m *publisherMocks)
	}{
		{"listing the cards fails", func(m *publisherMocks) {
			m.envCards.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2AAgentCard, error) {
				return nil, assert.AnError
			}
		}},
		{"reading the artifact fails", func(m *publisherMocks) {
			namesWeather(m)
			m.artifacts.GetByHandleFunc = func(string, string) (*models.Artifact, error) { return nil, assert.AnError }
		}},
		{"deleting fails", func(m *publisherMocks) {
			namesWeather(m)
			m.artifacts.GetByHandleFunc = func(string, string) (*models.Artifact, error) { return leftover, nil }
			m.artifacts.DeleteFunc = func(*gorm.DB, string) error { return assert.AnError }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newPublisherMocks(t)
			m.pubs.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2APublication, error) {
				return []models.A2APublication{}, nil
			}
			m.pubs.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
			m.proxies.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
			tc.setup(m)

			err := m.publisher().TeardownEnvironment(context.Background(), "org-1", envUUID)

			assert.ErrorIs(t, err, assert.AnError)
			assert.Len(t, m.pubs.DeleteForEnvironmentCalls(), 1, "the rest of the teardown still runs")
			assert.Len(t, m.proxies.DeleteForEnvironmentCalls(), 1)
		})
	}
}

// Only the environment's card rows name agents to sweep, so a REST or chat agent's artifact for the
// environment is never read, deleted or broadcast about; a card whose artifact is already gone is skipped.
func TestA2APublisherTeardownEnvironmentSweepsOnlyA2AAgents(t *testing.T) {
	m := newPublisherMocks(t)
	envUUID := uuid.New()
	m.pubs.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2APublication, error) {
		return []models.A2APublication{}, nil
	}
	m.envCards.ListForEnvironmentFunc = func(_ context.Context, ouID string, env uuid.UUID) ([]models.A2AAgentCard, error) {
		assert.Equal(t, "org-1", ouID)
		assert.Equal(t, envUUID, env)
		return []models.A2AAgentCard{{ProjectName: "checkout", AgentName: "direct-now", EnvironmentUUID: envUUID}}, nil
	}
	m.artifacts.GetByHandleFunc = func(handle, _ string) (*models.Artifact, error) {
		assert.Equal(t, agentEnvAPIArtifactHandle("checkout", "direct-now", envUUID.String()), handle)
		return nil, utils.ErrArtifactNotFound
	}
	m.pubs.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.proxies.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }

	require.NoError(t, m.publisher().TeardownEnvironment(context.Background(), "org-1", envUUID))

	assert.Len(t, m.artifacts.GetByHandleCalls(), 1, "one lookup per A2A agent the cards name, nothing else")
	assert.Empty(t, m.artifacts.DeleteCalls())
	assert.Empty(t, m.hub.published)
}

// One agent's failure does not keep the environment's other rows alive, and its gateways are
// still told: with its rows gone nothing could find the Agent to stop it later.
func TestA2APublisherTeardownEnvironmentStillDropsTheRowsAfterAFailure(t *testing.T) {
	m := newPublisherMocks(t)
	artifactUUID := uuid.New()
	m.pubs.ListForEnvironmentFunc = func(context.Context, string, uuid.UUID) ([]models.A2APublication, error) {
		return []models.A2APublication{{AgentName: "weather", ArtifactUUID: artifactUUID}}, nil
	}
	m.txErr = assert.AnError // every per-row transaction fails to commit
	m.pubs.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }
	m.proxies.DeleteForEnvironmentFunc = func(context.Context, string, uuid.UUID) error { return nil }

	err := m.publisher().TeardownEnvironment(context.Background(), "org-1", uuid.New())

	assert.ErrorIs(t, err, assert.AnError)
	assert.Len(t, m.pubs.DeleteForEnvironmentCalls(), 1)
	assert.Len(t, m.proxies.DeleteForEnvironmentCalls(), 1)
	require.Len(t, m.hub.published, 1)
	assert.Equal(t, "gw-1", m.hub.published[0].GatewayID)
	assert.Equal(t, artifactUUID.String(), deletedAgentID(t, m.hub.published[0]))
}

func TestA2APublisherEnvironmentUUIDsListsProxiesWhenPublicationsFail(t *testing.T) {
	id := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	m := newPublisherMocks(t)
	m.pubs.ListEnvironmentUUIDsFunc = func(context.Context, string, string) ([]uuid.UUID, error) {
		return nil, errors.New("publication listing down")
	}
	m.proxies.ListEnvironmentUUIDsFunc = func(context.Context, string, string) ([]uuid.UUID, error) {
		return []uuid.UUID{id}, nil
	}

	ids, err := m.publisher().EnvironmentUUIDs(context.Background(), "acme", "dev")

	require.Error(t, err)
	assert.Contains(t, ids, id)
}

// gatewayStatusMocks maps an ingress gateway to the environment and answers the publication read with get.
func gatewayStatusMocks(t *testing.T, get func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error)) *publisherMocks {
	m := newPublisherMocks(t)
	m.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return []*models.Gateway{{Vhost: "agents.example.com", GatewayFunctionalityType: models.GatewayRoleIngress}}, nil
	}
	m.pubs.GetFunc = get
	return m
}

func readGatewayStatus(m *publisherMocks, source models.A2APublicationSource) (A2AGatewayStatus, error) {
	key := testA2AKey()
	return m.publisher().GatewayStatus(context.Background(), key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID, source)
}

func publicationRow(pub models.A2APublication) func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
	return func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
		return &pub, nil
	}
}

func publicationReadFails(err error) func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
	return func(context.Context, string, string, string, uuid.UUID) (*models.A2APublication, error) {
		return nil, err
	}
}

func TestGatewayStatusReportsAMissingPublicationPerSource(t *testing.T) {
	for _, tc := range []struct {
		source models.A2APublicationSource
		want   string
	}{
		{models.A2APublicationSourceExternal, "no gateway publication is queued for this environment; refresh to publish it again"},
		{models.A2APublicationSourcePlatform, "no gateway publication is queued for this environment; redeploy the agent or save its deploy settings to publish it again"},
	} {
		t.Run(string(tc.source), func(t *testing.T) {
			m := gatewayStatusMocks(t, publicationReadFails(repositories.ErrA2APublicationNotFound))

			got, err := readGatewayStatus(m, tc.source)

			require.NoError(t, err)
			assert.Contains(t, got.GatewayURL, "agents.example.com")
			assert.Equal(t, models.A2APublicationStatusFailed, got.Status)
			assert.Equal(t, tc.want, got.LastError)
			assert.Nil(t, got.Transports)
		})
	}
}

func TestGatewayStatusCopiesTheRow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		transports json.RawMessage
		want       []A2ATransport
	}{
		{"recorded transports", json.RawMessage(`[{"protocolBinding":"JSONRPC","pathPrefix":""}]`), []A2ATransport{{ProtocolBinding: "JSONRPC", PathPrefix: ""}}},
		{"unrecorded transports", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := gatewayStatusMocks(t, publicationRow(models.A2APublication{
				Status: models.A2APublicationStatusPublished, LastError: "", PublishedTransports: tc.transports,
			}))

			got, err := readGatewayStatus(m, models.A2APublicationSourcePlatform)

			require.NoError(t, err)
			assert.Contains(t, got.GatewayURL, "agents.example.com")
			assert.Equal(t, models.A2APublicationStatusPublished, got.Status)
			assert.Empty(t, got.LastError)
			assert.Equal(t, tc.want, got.Transports)
		})
	}
}

func TestGatewayStatusCopiesTheRowsLastError(t *testing.T) {
	m := gatewayStatusMocks(t, publicationRow(models.A2APublication{
		Status: models.A2APublicationStatusFailed, LastError: "gateway rejected the agent",
	}))

	got, err := readGatewayStatus(m, models.A2APublicationSourceExternal)

	require.NoError(t, err)
	assert.Equal(t, models.A2APublicationStatusFailed, got.Status)
	assert.Equal(t, "gateway rejected the agent", got.LastError)
}

func TestGatewayStatusFailsOnUnparseableTransports(t *testing.T) {
	m := gatewayStatusMocks(t, publicationRow(models.A2APublication{
		Status: models.A2APublicationStatusPublished, PublishedTransports: json.RawMessage(`{`),
	}))

	_, err := readGatewayStatus(m, models.A2APublicationSourcePlatform)

	require.Error(t, err)
	assert.NotErrorIs(t, err, repositories.ErrA2APublicationNotFound)
}

func TestGatewayStatusFailsWhenTheGatewayLookupFails(t *testing.T) {
	m := gatewayStatusMocks(t, publicationRow(models.A2APublication{Status: models.A2APublicationStatusPublished}))
	m.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return nil, assert.AnError
	}

	_, err := readGatewayStatus(m, models.A2APublicationSourcePlatform)

	require.ErrorIs(t, err, assert.AnError)
}

func TestGatewayStatusWrapsPublicationReadErrors(t *testing.T) {
	m := gatewayStatusMocks(t, publicationReadFails(assert.AnError))

	_, err := readGatewayStatus(m, models.A2APublicationSourceExternal)

	require.ErrorIs(t, err, assert.AnError)
	assert.True(t, strings.HasPrefix(err.Error(), "failed to read the A2A publication"), err.Error())
}
