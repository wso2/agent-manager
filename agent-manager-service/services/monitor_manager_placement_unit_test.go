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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// monitorPlacementService is a monitor manager over the LLM placement fixture. Repositories the monitor
// would write through have no funcs, so any write before the refusal panics.
func monitorPlacementService(f *llmPlacementFixture) *monitorManagerService {
	return &monitorManagerService{
		logger: discardLogger(),
		llmProvisioner: &LLMProxyProvisioner{
			llmProviderRepo:           f.svc.llmProviderRepo,
			gatewayRepo:               f.gatewayRepo,
			llmProxyDeploymentService: f.svc.llmProxyDeploymentService,
		},
		ocClient: &clientmocks.OpenChoreoClientMock{
			GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
				return &models.AgentResponse{}, nil
			},
			GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
				return f.envs[name], nil
			},
		},
	}
}

func TestMonitorRefusesSelfHostedOnlyProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	f.provider("self-p", models.GatewayHostingSelfHosted)
	svc := monitorPlacementService(f)

	t.Run("create", func(t *testing.T) {
		_, err := svc.CreateMonitor(context.Background(), llmPlacementOU, &models.CreateMonitorRequest{
			Name: "m", ProjectName: "proj", AgentName: "agent", EnvironmentName: "dev", Type: models.MonitorTypeFuture,
			LLMProvider: &models.MonitorLLMProviderRef{ProviderName: "self-p"},
		})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, "managed gateway")
		assert.ErrorContains(t, err, `LLM provider "self-p" in environment dev`)
	})

	t.Run("update changing the provider", func(t *testing.T) {
		svc.monitorRepo = &repomocks.MonitorRepositoryMock{
			GetMonitorByNameFunc: func(string, string, string, string) (*models.Monitor, error) {
				return &models.Monitor{ID: uuid.New(), EnvironmentID: f.envs["dev"].UUID, EnvironmentName: "dev", ProjectName: "proj"}, nil
			},
		}
		svc.monitorLLMMappingRepo = &repomocks.MonitorLLMMappingRepositoryMock{
			ListByMonitorIDFunc: func(context.Context, uuid.UUID) ([]models.MonitorLLMMapping, error) { return nil, nil },
		}
		name := "renamed"
		_, err := svc.UpdateMonitor(context.Background(), llmPlacementOU, "proj", "agent", "m", &models.UpdateMonitorRequest{
			DisplayName: &name, LLMProvider: &models.MonitorLLMProviderRef{ProviderName: "self-p"},
		})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, "managed gateway")
		assert.ErrorContains(t, err, `LLM provider "self-p" in environment dev`)
	})
}

func TestResolveMonitorGateway_PrefersManagedProviderAnchor(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	selfHosted := &models.Gateway{UUID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Hosting: models.GatewayHostingSelfHosted}
	managed := &models.Gateway{UUID: uuid.MustParse("ffffffff-0000-0000-0000-000000000002")}
	svc := monitorPlacementService(f)
	svc.llmProvisioner.llmProxyDeploymentService = &LLMProxyDeploymentService{deploymentRepo: &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return []string{selfHosted.UUID.String(), managed.UUID.String()}, nil
		},
	}}
	f.gatewayRepo.GetByUUIDFunc = func(id string) (*models.Gateway, error) {
		if id == managed.UUID.String() {
			return managed, nil
		}
		return selfHosted, nil
	}

	got, err := svc.resolveMonitorGateway(context.Background(), llmPlacementOU, f.env("dev"), uuid.NewString())

	require.NoError(t, err)
	assert.Equal(t, managed.UUID, got.UUID)
}

func TestResolveMonitorGateway_IgnoresSelfHostedOnlyProviderAnchor(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	svc := monitorPlacementService(f)
	deployments, managed := selfHostedOnlyProviderAnchor(f)
	svc.llmProvisioner.llmProxyDeploymentService = deployments

	got, err := svc.resolveMonitorGateway(context.Background(), llmPlacementOU, f.env("dev"), uuid.NewString())

	require.NoError(t, err)
	assert.Equal(t, managed.UUID, got.UUID)
}
