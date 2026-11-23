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

package gwsvc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

// Placement lists the gateways a provider is heading for, with each one's status; undeployed and unknown gateways are left out.
func TestProviderPlacement_ReportsPlacedGateways(t *testing.T) {
	provider := uuid.New()
	deployed := &gwmodels.Gateway{UUID: uuid.New(), Name: "gw-deployed", Vhost: "a.example"}
	deploying := &gwmodels.Gateway{UUID: uuid.New(), Name: "gw-deploying"}
	undeployed := &gwmodels.Gateway{UUID: uuid.New(), Name: "gw-undeployed"}
	gone := uuid.New()
	updated := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	svc := &LLMProviderService{
		gatewayRepo: &gwrepomocks.GatewayRepositoryMock{
			GetByOrganizationIDFunc: func(context.Context, string) ([]*gwmodels.Gateway, error) {
				return []*gwmodels.Gateway{deployed, deploying, undeployed}, nil
			},
		},
		deploymentRepo: &gwrepomocks.DeploymentRepositoryMock{
			StatusRecordsForFunc: func(_ context.Context, _ string, artifacts []uuid.UUID) ([]gwmodels.DeploymentStatusRecord, error) {
				require.Equal(t, []uuid.UUID{provider}, artifacts)
				return []gwmodels.DeploymentStatusRecord{
					{ArtifactUUID: provider, GatewayUUID: deployed.UUID, Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed, UpdatedAt: updated},
					{ArtifactUUID: provider, GatewayUUID: deploying.UUID, Status: gwmodels.DeploymentStatusPending, StatusDesired: gwmodels.DeploymentStatusDeployed},
					{ArtifactUUID: provider, GatewayUUID: undeployed.UUID, Status: gwmodels.DeploymentStatusUndeployed, StatusDesired: gwmodels.DeploymentStatusUndeployed},
					{ArtifactUUID: provider, GatewayUUID: gone, Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed},
				}, nil
			},
		},
	}

	got, err := svc.ProviderPlacement(context.Background(), "ou-acme", []uuid.UUID{provider})

	require.NoError(t, err)
	require.Len(t, got[provider], 2)
	assert.Equal(t, gwmodels.DeploymentSummary{
		GatewayID: deployed.UUID, GatewayName: "gw-deployed", Status: gwmodels.DeploymentStatusDeployed, DeployedAt: &updated, VHost: "a.example",
	}, got[provider][0])
	assert.Equal(t, deploying.UUID, got[provider][1].GatewayID)
	assert.Equal(t, gwmodels.DeploymentStatusDeploying, got[provider][1].Status, "a pending deploy reads as DEPLOYING")
}

// A provider placed nowhere reports an empty list, not nil, so its document says "placed nowhere".
func TestProviderPlacement_EmptyWhenPlacedNowhere(t *testing.T) {
	provider := uuid.New()
	svc := &LLMProviderService{
		gatewayRepo: &gwrepomocks.GatewayRepositoryMock{
			GetByOrganizationIDFunc: func(context.Context, string) ([]*gwmodels.Gateway, error) { return nil, nil },
		},
		deploymentRepo: &gwrepomocks.DeploymentRepositoryMock{
			StatusRecordsForFunc: func(context.Context, string, []uuid.UUID) ([]gwmodels.DeploymentStatusRecord, error) { return nil, nil },
		},
	}

	got, err := svc.ProviderPlacement(context.Background(), "ou-acme", []uuid.UUID{provider})

	require.NoError(t, err)
	assert.NotNil(t, got[provider])
	assert.Empty(t, got[provider])
}

// A page of providers costs one status query, and each row lands on its own provider.
func TestProviderPlacement_ReadsThePageInOneQuery(t *testing.T) {
	first, second, idle := uuid.New(), uuid.New(), uuid.New()
	gw := &gwmodels.Gateway{UUID: uuid.New(), Name: "gw"}
	repo := &gwrepomocks.DeploymentRepositoryMock{
		StatusRecordsForFunc: func(context.Context, string, []uuid.UUID) ([]gwmodels.DeploymentStatusRecord, error) {
			return []gwmodels.DeploymentStatusRecord{
				{ArtifactUUID: first, GatewayUUID: gw.UUID, Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed},
				{ArtifactUUID: second, GatewayUUID: gw.UUID, Status: gwmodels.DeploymentStatusPending, StatusDesired: gwmodels.DeploymentStatusDeployed},
			}, nil
		},
	}
	svc := &LLMProviderService{
		gatewayRepo: &gwrepomocks.GatewayRepositoryMock{
			GetByOrganizationIDFunc: func(context.Context, string) ([]*gwmodels.Gateway, error) { return []*gwmodels.Gateway{gw}, nil },
		},
		deploymentRepo: repo,
	}

	got, err := svc.ProviderPlacement(context.Background(), "ou-acme", []uuid.UUID{first, second, idle})

	require.NoError(t, err)
	require.Len(t, repo.StatusRecordsForCalls(), 1)
	require.Len(t, got[first], 1)
	assert.Equal(t, gwmodels.DeploymentStatusDeployed, got[first][0].Status)
	require.Len(t, got[second], 1)
	assert.Equal(t, gwmodels.DeploymentStatusDeploying, got[second][0].Status)
	assert.NotNil(t, got[idle])
	assert.Empty(t, got[idle])
}
