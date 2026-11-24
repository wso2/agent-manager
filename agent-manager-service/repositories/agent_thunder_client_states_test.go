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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestAgentThunderClientRepo_FindByOuAndEnvironment(t *testing.T) {
	ctx := context.Background()
	repo := NewAgentThunderClientRepo(db.GetDB())
	ou := newOrg(t)
	for _, b := range []struct{ project, agent, env string }{
		{"zeta", "z1", "dev"}, {"alpha", "b-agent", "dev"}, {"alpha", "a-agent", "dev"}, {"alpha", "x", "prod"},
	} {
		c := newTestAgentThunderClient(t, ou, b.project, b.agent, b.env)
		require.NoError(t, repo.Upsert(ctx, c))
	}

	rows, err := repo.FindByOuAndEnvironment(ctx, ou, "dev")
	require.NoError(t, err)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.ProjectName+"/"+r.AgentName)
		assert.Equal(t, "dev", r.EnvironmentName)
		assert.Equal(t, models.AgentProvisioningTypeExternal, r.ProvisioningType)
	}
	assert.Equal(t, []string{"alpha/a-agent", "alpha/b-agent", "zeta/z1"}, got, "sorted by project, then agent")

	other, err := repo.FindByOuAndEnvironment(ctx, newOrg(t), "dev")
	require.NoError(t, err)
	assert.Empty(t, other)
	none, err := repo.FindByOuAndEnvironment(ctx, ou, "missing-env")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestAgentThunderClientRepo_UpsertNeverClobbers(t *testing.T) {
	ctx := context.Background()
	repo := NewAgentThunderClientRepo(db.GetDB())
	ou := newOrg(t)
	first := newTestAgentThunderClient(t, ou, "p", "a", "dev")
	require.NoError(t, repo.Upsert(ctx, first))
	tid := "thunder-agent"
	require.NoError(t, repo.UpdateAfterAttempt(ctx, first.ID, AgentThunderAttemptUpdate{
		Status: models.AgentThunderStatusCompleted, ThunderAgentID: &tid,
	}))

	again := newTestAgentThunderClient(t, ou, "p", "a", "dev")
	require.NoError(t, repo.Upsert(ctx, again), "a duplicate write-ahead is not an error")
	stored, err := repo.Get(ctx, ou, "p", "a", "dev")
	require.NoError(t, err)
	assert.Equal(t, first.ID, stored.ID)
	assert.Equal(t, models.AgentThunderStatusCompleted, stored.Status, "the completed identity survives")
	assert.Equal(t, "thunder-agent", stored.ThunderAgentID)
}

func TestAgentThunderClientRepo_ClaimForAttempt(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	repo := NewAgentThunderClientRepo(gdb)
	ou := newOrg(t)
	c := newTestAgentThunderClient(t, ou, "p", "claim", "dev")
	require.NoError(t, repo.Upsert(ctx, c))

	claimed, err := repo.ClaimForAttempt(ctx, c.ID)
	require.NoError(t, err)
	assert.True(t, claimed)
	claimed, err = repo.ClaimForAttempt(ctx, c.ID)
	require.NoError(t, err)
	assert.False(t, claimed, "a live claim cannot be taken twice")

	// An abandoned attempt (its lease passed) can be claimed again.
	require.NoError(t, gdb.Model(&models.AgentThunderClient{}).Where("id = ?", c.ID).
		Update("claimed_until", time.Now().Add(-time.Minute).UTC()).Error)
	claimed, err = repo.ClaimForAttempt(ctx, c.ID)
	require.NoError(t, err)
	assert.True(t, claimed)

	require.NoError(t, repo.UpdateAfterAttempt(ctx, c.ID, AgentThunderAttemptUpdate{Status: models.AgentThunderStatusCompleted}))
	claimed, err = repo.ClaimForAttempt(ctx, c.ID)
	require.NoError(t, err)
	assert.False(t, claimed, "a completed binding is not claimed")
	claimed, err = repo.ClaimForAttempt(ctx, uuid.New())
	require.NoError(t, err)
	assert.False(t, claimed)
}

func TestAgentThunderClientRepo_ResetFailedForRetry(t *testing.T) {
	ctx := context.Background()
	repo := NewAgentThunderClientRepo(db.GetDB())
	ou := newOrg(t)
	c := newTestAgentThunderClient(t, ou, "p", "retry", "dev")
	require.NoError(t, repo.Upsert(ctx, c))

	reset, err := repo.ResetFailedForRetry(ctx, c.ID)
	require.NoError(t, err)
	assert.False(t, reset, "only a failed binding is reset")

	for range 3 {
		require.NoError(t, repo.UpdateAfterAttempt(ctx, c.ID, AgentThunderAttemptUpdate{
			Status: models.AgentThunderStatusFailed, LastError: "thunder down",
		}))
	}
	failed, err := repo.Get(ctx, ou, "p", "retry", "dev")
	require.NoError(t, err)
	assert.Equal(t, 3, failed.AttemptCount)

	reset, err = repo.ResetFailedForRetry(ctx, c.ID)
	require.NoError(t, err)
	assert.True(t, reset)
	reset, err = repo.ResetFailedForRetry(ctx, c.ID)
	require.NoError(t, err)
	assert.False(t, reset, "a second concurrent retry loses")

	fresh, err := repo.Get(ctx, ou, "p", "retry", "dev")
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusPending, fresh.Status)
	assert.Zero(t, fresh.AttemptCount, "a fresh attempt budget")
	assert.Empty(t, fresh.LastError)

	due, err := repo.FindDue(ctx, time.Now().Add(time.Second), 1000)
	require.NoError(t, err)
	found := false
	for _, d := range due {
		found = found || d.ID == c.ID
	}
	assert.True(t, found, "the reset binding is due now")
}

func TestAgentThunderClientRepo_ReleaseClaimAndLiteNoOps(t *testing.T) {
	ctx := context.Background()
	repo := NewAgentThunderClientRepo(db.GetDB())
	ou := newOrg(t)
	c := newTestAgentThunderClient(t, ou, "p", "rel", "dev")
	require.NoError(t, repo.Upsert(ctx, c))

	next := time.Now().Add(time.Hour)
	require.ErrorIs(t, repo.ReleaseClaim(ctx, c.ID, next, "waiting"), ErrAgentThunderClientNotFound, "not in progress")

	claimed, err := repo.ClaimForAttempt(ctx, c.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, repo.ReleaseClaim(ctx, c.ID, next, "needs the source environment"))
	got, err := repo.Get(ctx, ou, "p", "rel", "dev")
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusPending, got.Status)
	assert.Equal(t, "needs the source environment", got.LastError)
	assert.Zero(t, got.AttemptCount, "releasing does not consume an attempt")

	// Lite stores no secret reference and has no promotions: these are no-ops.
	require.NoError(t, repo.UpdateSecretRef(ctx, c.ID, "path"))
	completed, err := repo.FindRecentlyCompletedInternal(ctx, time.Time{}, nil, 10)
	require.NoError(t, err)
	assert.Empty(t, completed)
	require.NoError(t, repo.MarkSyncAfter(ctx, []uuid.UUID{c.ID}, time.Now()))
	syncDue, err := repo.FindSyncDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Empty(t, syncDue)
	ok, err := repo.ClaimSync(ctx, c.ID, time.Now(), time.Now())
	require.NoError(t, err)
	assert.False(t, ok)
	done, err := repo.FinishSync(ctx, c.ID, time.Now(), nil)
	require.NoError(t, err)
	assert.True(t, done)
}
