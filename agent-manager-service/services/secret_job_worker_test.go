//go:build integration

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
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/vault/vaulttest"
)

// A write queues its promotion to the external vault; the worker carries it out as soon as it starts, and stops
// cleanly, more than once.
func TestSecretJobWorkerPromotesQueuedSecrets(t *testing.T) {
	ctx, gdb := context.Background(), db.GetDB()
	vaults, external := vaulttest.LocalAndExternal(t)
	store := repositories.NewSecretStore(vaults)
	org, id := dbtest.Org(t, gdb), repositories.NewID()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })
	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", id, id, []byte("value")))
	require.Zero(t, external.Len(), "nothing is promoted before the worker runs")

	worker := NewSecretJobWorker(repositories.NewSecretJobRunner(gdb, vaults, slog.New(slog.DiscardHandler)), slog.New(slog.DiscardHandler))
	require.NoError(t, worker.Start(ctx))
	require.Eventually(t, func() bool {
		var row models.Secret
		return gdb.Where("id = ?", id).Take(&row).Error == nil && row.Provider == vaulttest.ExternalProvider
	}, 10*time.Second, 20*time.Millisecond, "the worker promotes the secret on its first pass")
	require.NoError(t, worker.Stop())
	require.NoError(t, worker.Stop(), "stopping twice is safe")

	value, found, err := store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "value", string(value))
}

// A cancelled context ends the loop as Stop does, and RunOnce alone runs what is due.
func TestSecretJobWorkerStopsWithItsContext(t *testing.T) {
	gdb := db.GetDB()
	vaults := vaulttest.Local(t)
	worker := NewSecretJobWorker(repositories.NewSecretJobRunner(gdb, vaults, slog.New(slog.DiscardHandler)), slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, worker.Start(ctx))
	cancel()
	done := make(chan struct{})
	go func() {
		_ = worker.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop did not return after the context was cancelled")
	}
	worker.RunOnce(context.Background())
}
