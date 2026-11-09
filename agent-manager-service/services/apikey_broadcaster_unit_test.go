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
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// txHub records, for each event, the transaction it was written in.
type txHub struct {
	stubEventHub
	txs []*gorm.DB
}

func (h *txHub) PublishEventTx(ctx context.Context, tx *gorm.DB, gatewayID string, e eventhub.Event) error {
	h.txs = append(h.txs, tx)
	return h.stubEventHub.PublishEventTx(ctx, tx, gatewayID, e)
}

// A key write and its gateway events go in one transaction, write first, gateways in ID order; a failed write sends
// nothing.
func TestBroadcaster_WriteAndEventsShareATransaction(t *testing.T) {
	tx := &gorm.DB{}
	var steps []string
	inTx := false
	repo := &repomocks.APIKeyRepositoryMock{}
	repo.TransactionFunc = func(_ context.Context, fn func(repositories.APIKeyRepository, *gorm.DB) error) error {
		inTx = true
		defer func() { inTx = false }()
		return fn(repo, tx)
	}
	repo.UpsertFunc = func(*models.StoredAPIKey) error {
		require.True(t, inTx, "the write is in the transaction")
		steps = append(steps, "write")
		return nil
	}
	repo.DeleteFunc = func(string, string) error {
		require.True(t, inTx)
		steps = append(steps, "write")
		return nil
	}
	hub := &txHub{}
	b := apiKeyBroadcaster{gatewayService: NewGatewayEventsService(hub), apiKeyRepo: repo}
	gateways := []*models.Gateway{{UUID: uuid.New()}, {UUID: uuid.New()}, {UUID: uuid.New()}}
	sorted := []string{gateways[0].UUID.String(), gateways[1].UUID.String(), gateways[2].UUID.String()}
	slices.Sort(sorted)
	artifact := uuid.NewString()

	check := func(t *testing.T) {
		t.Helper()
		require.Equal(t, []string{"write"}, steps)
		require.Len(t, hub.published, 3)
		var order []string
		for i, e := range hub.published {
			assert.Same(t, tx, hub.txs[i], "each event is written in the write's transaction")
			order = append(order, e.GatewayID)
		}
		assert.Equal(t, sorted, order, "gateways in ID order")
		steps, hub.published, hub.txs = nil, nil, nil
	}

	_, err := b.broadcastCreateToGateways(gateways, "org", artifact, artifact, &models.CreateAPIKeyRequest{Name: "key-a"})
	require.NoError(t, err)
	t.Run("create", check)

	_, err = b.broadcastRotateToGateways(gateways, "org", artifact, artifact, "key-a", &models.RotateAPIKeyRequest{})
	require.NoError(t, err)
	t.Run("rotate", check)

	require.NoError(t, b.broadcastRevokeToGateways(context.Background(), gateways, artifact, artifact, "key-a"))
	t.Run("revoke", check)

	repo.UpsertFunc = func(*models.StoredAPIKey) error { return errors.New("write failed") }
	_, err = b.broadcastRotateToGateways(gateways, "org", artifact, artifact, "key-a", &models.RotateAPIKeyRequest{})
	require.Error(t, err)
	assert.Empty(t, hub.published, "a failed write sends nothing")
}
