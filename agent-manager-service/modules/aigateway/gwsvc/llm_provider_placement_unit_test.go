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
			StatusRecordsFunc: func(_ context.Context, _ string, artifact uuid.UUID) ([]gwmodels.DeploymentStatusRecord, error) {
				require.Equal(t, provider, artifact)
				return []gwmodels.DeploymentStatusRecord{
					{GatewayUUID: deployed.UUID, Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed, UpdatedAt: updated},
					{GatewayUUID: deploying.UUID, Status: gwmodels.DeploymentStatusPending, StatusDesired: gwmodels.DeploymentStatusDeployed},
					{GatewayUUID: undeployed.UUID, Status: gwmodels.DeploymentStatusUndeployed, StatusDesired: gwmodels.DeploymentStatusUndeployed},
					{GatewayUUID: gone, Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed},
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
			StatusRecordsFunc: func(context.Context, string, uuid.UUID) ([]gwmodels.DeploymentStatusRecord, error) { return nil, nil },
		},
	}

	got, err := svc.ProviderPlacement(context.Background(), "ou-acme", []uuid.UUID{provider})

	require.NoError(t, err)
	assert.NotNil(t, got[provider])
	assert.Empty(t, got[provider])
}
