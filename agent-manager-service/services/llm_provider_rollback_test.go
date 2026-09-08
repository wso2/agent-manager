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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// serviceForRollback wires the three repository calls a rollback Delete makes for a
// provider with no deployed gateways: resolve the provider, list its deployed
// gateways, delete the row. deleteErr is what the final delete reports.
func serviceForRollback(
	created *models.LLMProvider, deleteErr error,
) (*LLMProviderService, *LLMProviderDeploymentService) {
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:            func(_, _ string) (*models.LLMProvider, error) { return created, nil },
		DeleteFunc:               func(_, _ string) error { return deleteErr },
		HasAssociatedProxiesFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		MarkDeletingFunc:         func(_ uuid.UUID) (bool, error) { return true, nil },
		ClearDeletingFunc:        func(_ uuid.UUID) error { return nil },
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			return []string{}, nil
		},
		GetTrackedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			return []string{}, nil
		},
	}
	return &LLMProviderService{providerRepo: providerRepo},
		&LLMProviderDeploymentService{deploymentRepo: deploymentRepo}
}

// A provider can be addressed by handle at the API while its gateway artifact is
// keyed by the canonical control-plane UUID. An already-undeployed provider must
// still produce a hard-delete event for its historically tracked gateway.
func TestDelete_AlreadyUndeployedProviderBroadcastsCanonicalUUID(t *testing.T) {
	created := createdProvider()
	trackedGatewayID := uuid.New().String()
	hub := &stubEventHub{}

	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByHandleFunc: func(handle, ouID string) (*models.LLMProvider, error) {
			require.Equal(t, created.Configuration.Handle, handle)
			require.Equal(t, "ou-acme", ouID)
			return created, nil
		},
		MarkDeletingFunc:         func(_ uuid.UUID) (bool, error) { return true, nil },
		HasAssociatedProxiesFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		DeleteFunc: func(providerID, ouID string) error {
			assert.Equal(t, created.UUID.String(), providerID)
			assert.Equal(t, "ou-acme", ouID)
			return nil
		},
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			return nil, nil
		},
		GetTrackedGatewaysByProviderFunc: func(providerUUID uuid.UUID, ouID string) ([]string, error) {
			assert.Equal(t, created.UUID, providerUUID)
			assert.Equal(t, "ou-acme", ouID)
			return []string{trackedGatewayID}, nil
		},
	}
	deploymentSvc := &LLMProviderDeploymentService{
		deploymentRepo:       deploymentRepo,
		gatewayEventsService: NewGatewayEventsService(hub),
	}
	svc := &LLMProviderService{providerRepo: providerRepo}

	err := svc.Delete(context.Background(), created.Configuration.Handle, "ou-acme", deploymentSvc)

	require.NoError(t, err)
	require.Len(t, hub.published, 1)
	published := hub.published[0]
	assert.Equal(t, trackedGatewayID, published.GatewayID)
	assert.Equal(t, "llmprovider.deleted", string(published.EventType))
	assert.Equal(t, "DELETE", published.Action)
	assert.Equal(t, created.UUID.String(), published.EntityID)

	var envelope struct {
		Type    string                          `json:"type"`
		Payload models.LLMProviderDeletionEvent `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(published.EventData), &envelope))
	assert.Equal(t, "llmprovider.deleted", envelope.Type)
	assert.Equal(t, created.UUID.String(), envelope.Payload.ProviderID)
}

func createdProvider() *models.LLMProvider {
	return &models.LLMProvider{
		UUID:          uuid.New(),
		Configuration: models.LLMProviderConfig{Handle: "acme-openai"},
	}
}

// A rollback that fails leaves behind the provider the rollback exists to remove.
// Swallowing that into a log meant the caller saw only "all deployments failed",
// retried the same handle, and was rejected by a provider it did not know existed.
func TestRollbackCreatedProvider_ReturnsTheRollbackFailure(t *testing.T) {
	boom := errors.New("connection refused")
	created := createdProvider()
	svc, deploymentSvc := serviceForRollback(created, boom)

	err := svc.rollbackCreatedProvider(context.Background(), created, "ou-acme", deploymentSvc)

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), created.UUID.String())
}

func TestRollbackCreatedProvider_ReportsNoErrorWhenTheProviderIsRemoved(t *testing.T) {
	created := createdProvider()
	svc, deploymentSvc := serviceForRollback(created, nil)

	err := svc.rollbackCreatedProvider(context.Background(), created, "ou-acme", deploymentSvc)

	assert.NoError(t, err)
}

// A delete rejected because the provider still has associated proxies must be a pure
// no-op: it must never touch the gateway, and it must release its deleting claim so
// the provider is usable again. Undeploying before this check is what left providers
// permanently UNDEPLOYED in the DB after a rejected delete (issue #1739).
func TestDelete_RejectsWithoutUndeployingWhenProviderHasAssociatedProxies(t *testing.T) {
	created := createdProvider()
	undeployCalled := false
	clearCalled := false

	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:            func(_, _ string) (*models.LLMProvider, error) { return created, nil },
		MarkDeletingFunc:         func(_ uuid.UUID) (bool, error) { return true, nil },
		HasAssociatedProxiesFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		ClearDeletingFunc:        func(_ uuid.UUID) error { clearCalled = true; return nil },
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			// Should never be reached: the proxies check must short-circuit first.
			undeployCalled = true
			return []string{"gw-1"}, nil
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo}
	deploymentSvc := &LLMProviderDeploymentService{deploymentRepo: deploymentRepo}

	err := svc.Delete(context.Background(), created.UUID.String(), "ou-acme", deploymentSvc)

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrLLMProviderHasProxies)
	assert.False(t, undeployCalled, "Delete must check for associated proxies before touching any gateway")
	assert.True(t, clearCalled, "Delete must release its deleting claim when rejected for associated proxies")
}

// A concurrent Delete for the same provider must be rejected once MarkDeleting has
// already claimed it, rather than undeploying a second time.
func TestDelete_RejectsWhenAlreadyMarkedDeleting(t *testing.T) {
	created := createdProvider()
	undeployCalled := false

	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:    func(_, _ string) (*models.LLMProvider, error) { return created, nil },
		MarkDeletingFunc: func(_ uuid.UUID) (bool, error) { return false, nil },
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			undeployCalled = true
			return []string{"gw-1"}, nil
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo}
	deploymentSvc := &LLMProviderDeploymentService{deploymentRepo: deploymentRepo}

	err := svc.Delete(context.Background(), created.UUID.String(), "ou-acme", deploymentSvc)

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrLLMProviderDeleteInProgress)
	assert.False(t, undeployCalled, "Delete must not undeploy again once another delete already claimed the provider")
}

// If undeployment hard-fails, the deleting flag must be cleared so the provider
// becomes available again instead of being stuck permanently unusable.
func TestDelete_ClearsDeletingFlagWhenUndeployFails(t *testing.T) {
	created := createdProvider()
	clearCalled := false

	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:            func(_, _ string) (*models.LLMProvider, error) { return created, nil },
		HasAssociatedProxiesFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		MarkDeletingFunc:         func(_ uuid.UUID) (bool, error) { return true, nil },
		ClearDeletingFunc:        func(_ uuid.UUID) error { clearCalled = true; return nil },
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			return []string{"gw-1"}, nil
		},
		GetDeploymentsWithStateFunc: func(_ string, _ string, _ *string, _ *string, _ int) ([]*models.Deployment, error) {
			// Simulates a gateway fetch failure: the only gateway fails, so Delete's
			// all-undeployments-failed branch fires and it must bail out and clear
			// the deleting flag rather than leave it set.
			return nil, errors.New("gateway unreachable")
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo}
	deploymentSvc := &LLMProviderDeploymentService{providerRepo: providerRepo, deploymentRepo: deploymentRepo}

	err := svc.Delete(context.Background(), created.UUID.String(), "ou-acme", deploymentSvc)

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrLLMProviderUndeployFailed)
	assert.True(t, clearCalled, "Delete must clear the deleting flag when it bails out without deleting the provider")
}
