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

package repositories

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/events"
)

// Claims take any available row, so the test runs in a transaction it rolls back, after emptying the outbox there.
func TestEventOutboxLeaseRecovery(t *testing.T) {
	database := db.GetDB()
	ou := newOrg(t)
	tx := database.Begin()
	defer tx.Rollback()
	require.NoError(t, tx.Exec("DELETE FROM event_outbox").Error)
	makeAvailable := func() {
		require.NoError(t, tx.Exec("UPDATE event_outbox SET available_at = ?", time.Now().Add(-time.Second)).Error)
	}
	repo := NewEventOutboxRepository(tx)
	ctx := context.Background()
	e := events.New(uuid.NewString(), events.TypeMonitorRunFailed, events.ScopeAgent, ou, "p", "a", time.Now())
	require.NoError(t, repo.Put(ctx, e))
	require.NoError(t, repo.Put(ctx, e))
	first, err := repo.Claim(ctx)
	require.NoError(t, err)
	require.Len(t, first, 1, "duplicate puts create one work item")
	var decoded events.Event
	require.NoError(t, json.Unmarshal(first[0].Body, &decoded))
	assert.Equal(t, e.ID, decoded.ID)
	next, err := repo.Claim(ctx)
	require.NoError(t, err)
	assert.Empty(t, next, "unexpired claims are exclusive")
	makeAvailable()
	recovered, err := repo.Claim(ctx)
	require.NoError(t, err)
	require.Len(t, recovered, 1)
	assert.NotEqual(t, first[0].Lease, recovered[0].Lease)
	require.NoError(t, repo.Complete(ctx, first[0]))
	var count int64
	require.NoError(t, tx.Table("event_outbox").Count(&count).Error)
	assert.EqualValues(t, 1, count, "stale worker cannot delete a recovered claim")
	require.NoError(t, repo.Retry(ctx, recovered[0]))
	next, err = repo.Claim(ctx)
	require.NoError(t, err)
	assert.Empty(t, next, "failed sends back off")
	makeAvailable()
	next, err = repo.Claim(ctx)
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.NoError(t, repo.Complete(ctx, next[0]))
	next, err = repo.Claim(ctx)
	require.NoError(t, err)
	assert.Empty(t, next)
}
