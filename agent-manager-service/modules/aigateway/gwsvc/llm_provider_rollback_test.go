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
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/reconcile"
)

// providerCore reconciles LLM providers against repo and hub.
func providerCore(repo gwrepo.DeploymentRepository, hub eventhub.EventHub) *reconcile.Core {
	return reconcile.NewCore(nil, repo, hub, reconcile.Kinds{NewLLMProviderKind(nil)}, reconcile.Config{},
		slog.New(slog.DiscardHandler))
}

// A provider can be addressed by handle at the API while its gateway artifact is
// keyed by the canonical control-plane UUID. An already-undeployed provider must
// still produce a hard-delete event for its historically tracked gateway.
func TestDelete_AlreadyUndeployedProviderBroadcastsCanonicalUUID(t *testing.T) {
	created := createdProvider()
	trackedGatewayID := uuid.New().String()
	hub := &stubEventHub{}

	providerRepo := &gwrepomocks.LLMProviderRepositoryMock{
		GetByHandleFunc: func(handle, ouID string) (*gwmodels.LLMProvider, error) {
			require.Equal(t, created.Configuration.Handle, handle)
			require.Equal(t, "ou-acme", ouID)
			return created, nil
		},
		MarkDeletingFunc:         func(_ uuid.UUID) (bool, error) { return true, nil },
		HasAssociatedProxiesFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		BudgetsUsingFunc:         func(context.Context, string, string) ([]string, error) { return nil, nil },
		DeleteTxFunc: func(_ context.Context, tx *gorm.DB, providerID, ouID string) error {
			assert.Same(t, txSentinel, tx, "the provider is deleted in the transaction that sends its delete events")
			assert.Equal(t, created.UUID.String(), providerID)
			assert.Equal(t, "ou-acme", ouID)
			return nil
		},
	}
	deploymentRepo := &gwrepomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
			return nil, nil
		},
		DeleteEventGatewaysFunc: func(_ context.Context, ouID string, providerUUID uuid.UUID) ([]string, error) {
			assert.Equal(t, created.UUID, providerUUID)
			assert.Equal(t, "ou-acme", ouID)
			return []string{trackedGatewayID}, nil
		},
	}
	deploymentRepo.WithTxFunc = func(*gorm.DB) gwrepo.DeploymentRepository { return deploymentRepo }
	deploymentSvc := &LLMProviderDeploymentService{
		tx:             sentinelTx,
		deploymentRepo: deploymentRepo,
		core:           providerCore(deploymentRepo, hub),
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
		Type    string                            `json:"type"`
		Payload gwmodels.LLMProviderDeletionEvent `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(published.EventData), &envelope))
	assert.Equal(t, "llmprovider.deleted", envelope.Type)
	assert.Equal(t, created.UUID.String(), envelope.Payload.ProviderID)
}

func createdProvider() *gwmodels.LLMProvider {
	return &gwmodels.LLMProvider{
		UUID:          uuid.New(),
		Configuration: gwmodels.LLMProviderConfig{Handle: "acme-openai"},
	}
}

// A delete rejected because the provider still has associated proxies must be a pure
// no-op: it must never touch the gateway, and it must release its deleting claim so
// the provider is usable again. Undeploying before this check is what left providers
// permanently UNDEPLOYED in the DB after a rejected delete (issue #1739).
func TestDelete_RejectsWithoutUndeployingWhenProviderHasAssociatedProxies(t *testing.T) {
	created := createdProvider()
	undeployCalled := false
	clearCalled := false

	providerRepo := &gwrepomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:            func(_, _ string) (*gwmodels.LLMProvider, error) { return created, nil },
		MarkDeletingFunc:         func(_ uuid.UUID) (bool, error) { return true, nil },
		HasAssociatedProxiesFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		ClearDeletingFunc:        func(_ uuid.UUID) error { clearCalled = true; return nil },
	}
	deploymentRepo := &gwrepomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
			// Should never be reached: the proxies check must short-circuit first.
			undeployCalled = true
			return []string{"gw-1"}, nil
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo}
	deploymentSvc := &LLMProviderDeploymentService{deploymentRepo: deploymentRepo}

	err := svc.Delete(context.Background(), created.UUID.String(), "ou-acme", deploymentSvc)

	require.Error(t, err)
	assert.ErrorIs(t, err, gwerr.ErrLLMProviderHasProxies)
	assert.False(t, undeployCalled, "Delete must check for associated proxies before touching any gateway")
	assert.True(t, clearCalled, "Delete must release its deleting claim when rejected for associated proxies")
}

// A concurrent Delete for the same provider must be rejected once MarkDeleting has
// already claimed it, rather than undeploying a second time.
func TestDelete_RejectsWhenAlreadyMarkedDeleting(t *testing.T) {
	created := createdProvider()
	undeployCalled := false

	providerRepo := &gwrepomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:    func(_, _ string) (*gwmodels.LLMProvider, error) { return created, nil },
		MarkDeletingFunc: func(_ uuid.UUID) (bool, error) { return false, nil },
	}
	deploymentRepo := &gwrepomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
			undeployCalled = true
			return []string{"gw-1"}, nil
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo}
	deploymentSvc := &LLMProviderDeploymentService{deploymentRepo: deploymentRepo}

	err := svc.Delete(context.Background(), created.UUID.String(), "ou-acme", deploymentSvc)

	require.Error(t, err)
	assert.ErrorIs(t, err, gwerr.ErrLLMProviderDeleteInProgress)
	assert.False(t, undeployCalled, "Delete must not undeploy again once another delete already claimed the provider")
}

// If undeployment hard-fails, the deleting flag must be cleared so the provider
// becomes available again instead of being stuck permanently unusable.
func TestDelete_ClearsDeletingFlagWhenUndeployFails(t *testing.T) {
	created := createdProvider()
	clearCalled := false

	providerRepo := &gwrepomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:            func(_, _ string) (*gwmodels.LLMProvider, error) { return created, nil },
		HasAssociatedProxiesFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		BudgetsUsingFunc:         func(context.Context, string, string) ([]string, error) { return nil, nil },
		MarkDeletingFunc:         func(_ uuid.UUID) (bool, error) { return true, nil },
		ClearDeletingFunc:        func(_ uuid.UUID) error { clearCalled = true; return nil },
	}
	deploymentRepo := &gwrepomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
			return []string{"gw-1"}, nil
		},
		GetDeploymentsWithStateFunc: func(_ string, _ string, _ *string, _ *string, _ int) ([]*gwmodels.Deployment, error) {
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
	assert.ErrorIs(t, err, gwerr.ErrLLMProviderUndeployFailed)
	assert.True(t, clearCalled, "Delete must clear the deleting flag when it bails out without deleting the provider")
}

func TestDelete_CompletesAfterCancellationFollowingSuccessfulUndeploy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	deployed := gwmodels.DeploymentStatusDeployed
	created := createdProvider()
	gatewayID := uuid.New()
	deploymentID := uuid.New()
	deleteCalled := false

	providerRepo := &gwrepomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:            func(_, _ string) (*gwmodels.LLMProvider, error) { return created, nil },
		MarkDeletingFunc:         func(_ uuid.UUID) (bool, error) { return true, nil },
		HasAssociatedProxiesFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		BudgetsUsingFunc:         func(context.Context, string, string) ([]string, error) { return nil, nil },
		ClearDeletingFunc:        func(_ uuid.UUID) error { return nil },
		DeleteTxFunc: func(got context.Context, _ *gorm.DB, _, _ string) error {
			deleteCalled = true
			require.NoError(t, got.Err())
			_, hasDeadline := got.Deadline()
			assert.True(t, hasDeadline)
			return nil
		},
	}
	deployment := &gwmodels.Deployment{
		DeploymentID: deploymentID,
		ArtifactUUID: created.UUID,
		GatewayUUID:  gatewayID,
		Status:       &deployed,
	}
	deploymentRepo := &gwrepomocks.DeploymentRepositoryMock{
		DeleteEventGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return []string{gatewayID.String()}, nil
		},
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return []string{gatewayID.String()}, nil
		},
		GetDeploymentsWithStateFunc: func(string, string, *string, *string, int) ([]*gwmodels.Deployment, error) {
			return []*gwmodels.Deployment{deployment}, nil
		},
		GetWithStateFunc: func(string, string, string) (*gwmodels.Deployment, error) {
			return deployment, nil
		},
		BeginUndeployFunc: func(context.Context, *gorm.DB, gwmodels.Placement) (bool, error) {
			cancel()
			return true, nil
		},
		GetStatusRecordFunc: func(context.Context, gwmodels.Placement) (*gwmodels.DeploymentStatusRecord, error) {
			return &gwmodels.DeploymentStatusRecord{
				Status: gwmodels.DeploymentStatusUndeploying, PerformedAt: gwmodels.NewPerformedAt(time.Now()),
			}, nil
		},
	}
	deploymentRepo.WithTxFunc = func(*gorm.DB) gwrepo.DeploymentRepository { return deploymentRepo }
	deploymentSvc := &LLMProviderDeploymentService{
		tx:             sentinelTx,
		providerRepo:   providerRepo,
		deploymentRepo: deploymentRepo,
		core:           providerCore(deploymentRepo, &stubEventHub{}),
	}
	svc := &LLMProviderService{providerRepo: providerRepo}

	err := svc.Delete(ctx, created.UUID.String(), "ou-acme", deploymentSvc)

	require.NoError(t, err)
	assert.True(t, deleteCalled)
	assert.Empty(t, providerRepo.ClearDeletingCalls())
}

// A provider a budget limits spend on is not deleted, nor undeployed, until the budget
// stops naming it.
func TestDelete_RejectsWithoutUndeployingWhenBudgetsUseTheProvider(t *testing.T) {
	created := createdProvider()
	clearCalled := false
	var askedHandle string
	providerRepo := &gwrepomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc:            func(_, _ string) (*gwmodels.LLMProvider, error) { return created, nil },
		MarkDeletingFunc:         func(_ uuid.UUID) (bool, error) { return true, nil },
		HasAssociatedProxiesFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		BudgetsUsingFunc: func(_ context.Context, _, handle string) ([]string, error) {
			askedHandle = handle
			return []string{"openai-monthly", "support-openai"}, nil
		},
		ClearDeletingFunc: func(_ uuid.UUID) error {
			clearCalled = true
			return nil
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo}

	err := svc.Delete(context.Background(), created.UUID.String(), "ou-acme", &LLMProviderDeploymentService{})

	require.ErrorIs(t, err, gwerr.ErrLLMProviderUsedByBudgets)
	assert.Contains(t, err.Error(), "openai-monthly, support-openai")
	assert.Equal(t, created.Configuration.Handle, askedHandle, "budgets name the provider by handle, even when it is deleted by UUID")
	assert.True(t, clearCalled, "the provider is usable again")
}
