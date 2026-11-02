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
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestProxyDelete_RejectsWrongProjectBeforeCleanup(t *testing.T) {
	proxy := &models.LLMProxy{UUID: uuid.New(), ProjectUUID: uuid.New()}
	repo := &repomocks.LLMProxyRepositoryMock{GetByIDCtxFunc: func(context.Context, string, string) (*models.LLMProxy, error) { return proxy, nil }}
	svc := NewLLMProxyService(repo, nil, nil)
	err := svc.DeleteInProject(context.Background(), "proxy", "org", uuid.NewString(), &LLMProxyDeploymentService{})
	require.ErrorIs(t, err, utils.ErrLLMProxyNotFound)
	assert.Empty(t, repo.DeleteInProjectCalls())
}

func TestProxyDelete_PropagatesLookupErrors(t *testing.T) {
	for _, cause := range []error{errors.New("database unavailable"), context.Canceled, gorm.ErrRecordNotFound} {
		t.Run(cause.Error(), func(t *testing.T) {
			repo := &repomocks.LLMProxyRepositoryMock{GetByIDCtxFunc: func(context.Context, string, string) (*models.LLMProxy, error) { return nil, cause }}
			svc := NewLLMProxyService(repo, nil, nil)
			err := svc.Delete(context.Background(), "proxy", "org", nil)
			if errors.Is(cause, gorm.ErrRecordNotFound) {
				require.ErrorIs(t, err, utils.ErrLLMProxyNotFound)
			} else {
				require.ErrorIs(t, err, cause)
				assert.NotErrorIs(t, err, utils.ErrLLMProxyNotFound)
			}
			assert.Empty(t, repo.DeleteInProjectCalls())
		})
	}
}

// A proxy's delete events are built before its status rows cascade away and written in the deleting transaction,
// which deletes only within the proxy's project.
func TestProxyDeleteSendsItsDeleteEventsInTheDeletingTransaction(t *testing.T) {
	proxy := &models.LLMProxy{UUID: uuid.New(), ProjectUUID: uuid.New()}
	gateway := uuid.NewString()
	repo := &repomocks.LLMProxyRepositoryMock{
		GetByIDCtxFunc: func(context.Context, string, string) (*models.LLMProxy, error) { return proxy, nil },
		DeleteInProjectTxFunc: func(_ context.Context, tx *gorm.DB, id, org, project string) error {
			assert.Same(t, txSentinel, tx)
			assert.Equal(t, "proxy", id)
			assert.Equal(t, "org", org)
			assert.Equal(t, proxy.ProjectUUID.String(), project)
			return nil
		},
	}
	deployments := &repomocks.DeploymentRepositoryMock{
		DeleteEventGatewaysFunc: func(_ context.Context, org string, id uuid.UUID) ([]string, error) {
			assert.Equal(t, proxy.UUID, id)
			return []string{gateway}, nil
		},
	}
	deployments.WithTxFunc = func(*gorm.DB) repositories.DeploymentRepository { return deployments }
	hub := &recordingEventHub{}
	core := reconcile.NewCore(nil, deployments, hub, reconcile.Kinds{proxyKindOf(proxy)}, reconcile.Config{},
		slog.New(slog.DiscardHandler))

	err := NewLLMProxyService(repo, nil, nil).DeleteInProject(context.Background(), "proxy", "org",
		proxy.ProjectUUID.String(), &LLMProxyDeploymentService{tx: sentinelTx, core: core})

	require.NoError(t, err)
	require.Len(t, repo.DeleteInProjectTxCalls(), 1)
	require.Len(t, hub.published, 1)
	assert.Same(t, txSentinel, hub.txs[0])
	assert.Equal(t, gateway, hub.published[0].GatewayID)
	assert.Equal(t, "proxy", hub.published[0].EntityID, "gateways store proxies under their handle")
	assert.Equal(t, eventhub.EventType("llmproxy.deleted"), hub.published[0].EventType)
}
