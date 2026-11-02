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

// Use an isolated transaction/schema so claims never consume other tests' work.
func TestEventOutboxLeaseRecovery(t *testing.T) {
	database := db.GetDB()
	schema := "outbox_" + uuid.New().String()[:8]
	require.NoError(t, database.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { assert.NoError(t, database.Exec("DROP SCHEMA "+schema+" CASCADE").Error) })
	tx := database.Begin()
	defer tx.Rollback()
	require.NoError(t, tx.Exec("SET LOCAL search_path TO "+schema+", public").Error)
	require.NoError(t, tx.Exec(`CREATE TABLE event_outbox (
 id TEXT PRIMARY KEY, body JSONB NOT NULL, lease TEXT NOT NULL DEFAULT '',
 available_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`).Error)
	repo := NewEventOutboxRepository(tx)
	ctx := context.Background()
	e := events.New(uuid.NewString(), events.TypeMonitorRunFailed, events.ScopeAgent, "ou-1", "p", "a", time.Now())
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
	require.NoError(t, tx.Exec("UPDATE event_outbox SET available_at = NOW() - INTERVAL '1 second'").Error)
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
	require.NoError(t, tx.Exec("UPDATE event_outbox SET available_at = NOW() - INTERVAL '1 second'").Error)
	next, err = repo.Claim(ctx)
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.NoError(t, repo.Complete(ctx, next[0]))
	next, err = repo.Claim(ctx)
	require.NoError(t, err)
	assert.Empty(t, next)
}
