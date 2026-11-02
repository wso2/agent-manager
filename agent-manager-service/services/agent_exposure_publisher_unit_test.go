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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// exposurePublisher is the shared publisher inside m.publisher(), on a core over m's deployments and hub.
func (m *publisherMocks) exposurePublisher() *AgentExposurePublisher {
	return m.publisher().exposure
}

func TestAgentExposurePublisherExposeJoinsTheCallersTransaction(t *testing.T) {
	m := newPublisherMocks(t)
	m.txExposures.UpsertFunc = func(context.Context, *models.PlatformAgentExposure) error { return nil }
	artifactUUID := uuid.New()

	require.NoError(t, m.exposurePublisher().Expose(context.Background(), txSentinel, testA2AKey(), artifactUUID, models.AgentExposureProtocolA2A))

	exposed := m.txExposures.UpsertCalls()
	require.Len(t, exposed, 1)
	assert.Equal(t, models.AgentExposureProtocolA2A, exposed[0].Exposure.Protocol)
	assert.Equal(t, artifactUUID, exposed[0].Exposure.ArtifactUUID)
	assert.Equal(t, testA2AKey().EnvironmentUUID, exposed[0].Exposure.EnvironmentUUID)
	assert.Equal(t, "dev", exposed[0].Exposure.EnvironmentName)
}

// A switch to direct undeploys the artifact from each gateway it is placed on, in the caller's transaction, and
// keeps the artifact and its keys.
func TestAgentExposurePublisherTeardownEnvUndeploysThePlacedRows(t *testing.T) {
	m := newPublisherMocks(t)
	gateway := uuid.New()
	m.txDeployments.PlacedGatewaysFunc = func(context.Context, string, uuid.UUID) ([]string, error) {
		return []string{gateway.String()}, nil
	}
	m.txDeployments.WithTxFunc = func(*gorm.DB) repositories.DeploymentRepository { return m.txDeployments }
	m.txDeployments.GetStatusRecordFunc = func(context.Context, models.Placement) (*models.DeploymentStatusRecord, error) {
		return &models.DeploymentStatusRecord{PerformedAt: models.NewPerformedAt(time.Now())}, nil
	}
	m.deployments.BeginUndeployFunc = func(_ context.Context, tx *gorm.DB, p models.Placement) (bool, error) {
		assert.Same(t, txSentinel, tx)
		assert.Equal(t, gateway, p.GatewayUUID)
		return true, nil
	}
	m.txExposures.DeleteForAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	artifact := models.ArtifactRef{OUID: "org-1", ArtifactUUID: uuid.New(), Kind: models.KindAgent}

	require.NoError(t, m.exposurePublisher().TeardownEnv(context.Background(), txSentinel, testA2AKey(), artifact))

	require.Len(t, m.hub.published, 1)
	assert.Equal(t, "agent.undeployed", string(m.hub.published[0].EventType))
	assert.Same(t, txSentinel, m.hub.txs[0])
	assert.Empty(t, m.artifacts.DeleteCalls())
}

// MarkStale and DropArtifact address the core by the artifact's own kind, so a REST agent's artifact is never
// handled as an A2A Agent.
func TestAgentExposurePublisherUsesArtifactKind(t *testing.T) {
	m := newPublisherMocks(t)
	artifact := models.ArtifactRef{OUID: "org-1", ArtifactUUID: uuid.New(), Kind: "UnregisteredKind"}

	staleErr := m.exposurePublisher().MarkStale(context.Background(), artifact, "an input changed")
	dropErr := m.exposurePublisher().DropArtifact(context.Background(), txSentinel, artifact)

	assert.ErrorContains(t, staleErr, `"UnregisteredKind"`)
	assert.ErrorContains(t, dropErr, `"UnregisteredKind"`)
}

func TestAgentURLFor(t *testing.T) {
	cases := []struct {
		name string
		url  AgentURL
		want string
	}{
		{"placed on the gateway", AgentURL{Gateway: "https://gw/agent", Direct: false}, "https://gw/agent"},
		{"gateway without a placed row", AgentURL{Direct: true}, "https://oc/agent"},
		{"no gateway", AgentURL{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.url.For("https://oc/agent"))
		})
	}
}

func TestAgentExposurePublisherResolveURL(t *testing.T) {
	gateway := &models.Gateway{UUID: uuid.New(), Vhost: "gw.example.com"}
	soon := time.Now()
	cases := []struct {
		name        string
		gateways    []*models.Gateway
		rows        []models.DeploymentStatusRecord
		wantGateway bool
		wantDirect  bool
		wantStatus  models.DeploymentStatus
	}{
		{"no ingress gateway", nil, nil, false, false, models.DeploymentStatusUndeployed},
		{"gateway, nothing placed", []*models.Gateway{gateway}, nil, false, false, models.DeploymentStatusDeploying},
		{"deployed row on the gateway", []*models.Gateway{gateway}, []models.DeploymentStatusRecord{
			{GatewayUUID: gateway.UUID, Status: models.DeploymentStatusDeployed, StatusDesired: models.DeploymentStatusDeployed},
		}, true, false, models.DeploymentStatusDeployed},
		{"deploy owed on the gateway", []*models.Gateway{gateway}, []models.DeploymentStatusRecord{
			{GatewayUUID: gateway.UUID, Status: models.DeploymentStatusPending, StatusDesired: models.DeploymentStatusUndeployed, NextAttemptAt: &soon},
		}, true, false, models.DeploymentStatusDeploying},
		{"undeployed row, nothing owed", []*models.Gateway{gateway}, []models.DeploymentStatusRecord{
			{GatewayUUID: gateway.UUID, Status: models.DeploymentStatusUndeployed, StatusDesired: models.DeploymentStatusUndeployed},
		}, false, false, models.DeploymentStatusUndeployed},
		{"deployed row on another gateway", []*models.Gateway{gateway}, []models.DeploymentStatusRecord{
			{GatewayUUID: uuid.New(), Status: models.DeploymentStatusDeployed, StatusDesired: models.DeploymentStatusDeployed},
		}, false, false, models.DeploymentStatusDeployed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newPublisherMocks(t)
			m.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) { return c.gateways, nil }
			m.artifacts.GetByHandleFunc = func(string, string) (*models.Artifact, error) {
				return &models.Artifact{UUID: uuid.New(), Kind: models.KindAgent}, nil
			}
			m.deployments.StatusRecordsFunc = func(context.Context, string, uuid.UUID) ([]models.DeploymentStatusRecord, error) {
				return c.rows, nil
			}
			m.exposures.GetFunc = exposedAgent

			url, err := m.exposurePublisher().ResolveURL(context.Background(), testA2AKey())

			require.NoError(t, err)
			assert.Equal(t, c.wantGateway, url.Gateway != "")
			assert.Equal(t, c.wantDirect, url.Direct)
			require.NotNil(t, url.Status)
			assert.Equal(t, c.wantStatus, url.Status.Status)
		})
	}
}

func TestAgentExposurePublisherResolveURLWithoutAnArtifactIsOwedItsDeploy(t *testing.T) {
	m := newPublisherMocks(t)
	m.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return []*models.Gateway{{UUID: uuid.New()}}, nil
	}
	m.artifacts.GetByHandleFunc = func(string, string) (*models.Artifact, error) { return nil, utils.ErrArtifactNotFound }
	m.exposures.GetFunc = exposedAgent

	url, err := m.exposurePublisher().ResolveURL(context.Background(), testA2AKey())

	require.NoError(t, err)
	assert.False(t, url.Direct)
	require.NotNil(t, url.Status)
	assert.Equal(t, notPlacedReason, url.Status.Reason)
}

// An agent with no exposure in the environment, such as a REST agent still on its trait, is reached directly and
// has no gateway status: the gateway is not involved.
func TestAgentExposurePublisherResolveURLWithoutAnExposureHasNoStatus(t *testing.T) {
	m := newPublisherMocks(t)
	m.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return []*models.Gateway{{UUID: uuid.New()}}, nil
	}
	m.artifacts.GetByHandleFunc = func(string, string) (*models.Artifact, error) {
		return &models.Artifact{UUID: uuid.New(), Kind: models.KindAgent}, nil
	}
	m.deployments.StatusRecordsFunc = func(context.Context, string, uuid.UUID) ([]models.DeploymentStatusRecord, error) {
		return nil, nil
	}
	m.exposures.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*models.PlatformAgentExposure, error) {
		return nil, repositories.ErrPlatformAgentExposureNotFound
	}

	url, err := m.exposurePublisher().ResolveURL(context.Background(), testA2AKey())

	require.NoError(t, err)
	assert.True(t, url.Direct)
	assert.Nil(t, url.Status)
}

func exposedAgent(context.Context, string, string, string, uuid.UUID) (*models.PlatformAgentExposure, error) {
	return &models.PlatformAgentExposure{}, nil
}
