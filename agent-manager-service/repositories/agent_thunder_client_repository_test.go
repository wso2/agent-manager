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
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func newTestAgentThunderClient(org, project, agent, env string) *models.AgentThunderClient {
	return &models.AgentThunderClient{
		OUID:             org,
		ProjectName:      project,
		AgentName:        agent,
		EnvironmentName:  env,
		ProvisioningType: models.AgentProvisioningTypeExternal,
		Status:           models.AgentThunderStatusPending,
	}
}

func cleanupAgentThunderClients(t *testing.T, repo AgentThunderClientRepository, org, project, agent string) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.DeleteByAgent(context.Background(), org, project, agent)
	})
}

func TestAgentThunderClientRepo_UpsertAndGet(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-upsert-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))

	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusPending, got.Status)
	assert.Equal(t, models.AgentProvisioningTypeExternal, got.ProvisioningType)

	// Upsert is only ever called to write-ahead a BRAND NEW binding (see
	// ProvisionForAgent) — every real caller already confirmed via Get() that
	// no row exists yet. A second Upsert on the same key models a race between
	// two such callers (e.g. the explicit provision endpoint racing
	// PromoteAgent's hook): it must be a no-op that leaves the winner's row
	// untouched, not an overwrite — silently clobbering an already-completed
	// Thunder identity/secret reference back to blank would orphan it.
	c.ProvisioningType = models.AgentProvisioningTypeInternal
	require.NoError(t, repo.Upsert(context.Background(), c))

	rows, err := repo.FindByAgent(context.Background(), org, project, agent)
	require.NoError(t, err)
	require.Len(t, rows, 1, "a conflicting upsert must not insert a second row")
	assert.Equal(t, models.AgentProvisioningTypeExternal, rows[0].ProvisioningType,
		"a conflicting upsert must leave the existing row's fields untouched, not overwrite them")
}

func TestAgentThunderClientRepo_Upsert_PersistsRequestedBy(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-requested-by-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	c.RequestedBy = "platform-thunder-subject-abc123"
	require.NoError(t, repo.Upsert(context.Background(), c))

	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, "platform-thunder-subject-abc123", got.RequestedBy)

	// A conflicting upsert (see the comment in TestAgentThunderClientRepo_UpsertAndGet
	// for why this must be a no-op) must not overwrite the original requester.
	c.RequestedBy = "platform-thunder-subject-xyz789"
	require.NoError(t, repo.Upsert(context.Background(), c))
	updated, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, "platform-thunder-subject-abc123", updated.RequestedBy)
}

func TestAgentThunderClientRepo_Get_NotFound(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	_, err := repo.Get(context.Background(), "test-org", "test-proj", "atc-missing-agent", "dev")
	assert.True(t, errors.Is(err, ErrAgentThunderClientNotFound))
}

func TestAgentThunderClientRepo_FindByAgent_MultipleEnvironments(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "atc-multi-env-agent"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	for _, env := range []string{"dev", "staging", "prod"} {
		require.NoError(t, repo.Upsert(context.Background(), newTestAgentThunderClient(org, project, agent, env)))
	}

	rows, err := repo.FindByAgent(context.Background(), org, project, agent)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	envs := []string{rows[0].EnvironmentName, rows[1].EnvironmentName, rows[2].EnvironmentName}
	assert.ElementsMatch(t, []string{"dev", "staging", "prod"}, envs)
}

func TestAgentThunderClientRepo_FindDue(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "atc-find-due-agent"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	// A brand-new write-ahead row (no next_retry_at yet) is due immediately.
	fresh := newTestAgentThunderClient(org, project, agent, "dev")
	require.NoError(t, repo.Upsert(context.Background(), fresh))

	// A row scheduled in the future is not due yet.
	future := newTestAgentThunderClient(org, project, agent, "staging")
	nextRetry := time.Now().Add(1 * time.Hour)
	future.NextRetryAt = &nextRetry
	require.NoError(t, repo.Upsert(context.Background(), future))

	// A row scheduled in the past is due.
	past := newTestAgentThunderClient(org, project, agent, "prod")
	pastRetry := time.Now().Add(-1 * time.Minute)
	past.NextRetryAt = &pastRetry
	require.NoError(t, repo.Upsert(context.Background(), past))

	// A completed row must never be picked up regardless of next_retry_at.
	completed := newTestAgentThunderClient(org, project, agent, "qa")
	completed.Status = models.AgentThunderStatusCompleted
	require.NoError(t, repo.Upsert(context.Background(), completed))

	due, err := repo.FindDue(context.Background(), time.Now(), 100)
	require.NoError(t, err)

	dueEnvs := make(map[string]bool)
	for _, row := range due {
		if row.OUID == org && row.ProjectName == project && row.AgentName == agent {
			dueEnvs[row.EnvironmentName] = true
		}
	}
	assert.True(t, dueEnvs["dev"], "a fresh row with no next_retry_at must be due immediately")
	assert.True(t, dueEnvs["prod"], "a row whose next_retry_at is in the past must be due")
	assert.False(t, dueEnvs["staging"], "a row scheduled in the future must not be due yet")
	assert.False(t, dueEnvs["qa"], "a completed row must never be due")
}

func TestAgentThunderClientRepo_UpdateAfterAttempt_Success(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-attempt-success-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))
	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)

	err = repo.UpdateAfterAttempt(context.Background(), got.ID, AgentThunderAttemptUpdate{
		Status:          models.AgentThunderStatusCompleted,
		ThunderAgentID:  utils.StrAsStrPointer("thunder-agent-uuid"),
		ThunderClientID: utils.StrAsStrPointer("client-id-123"),
	})
	require.NoError(t, err)

	updated, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusCompleted, updated.Status)
	assert.Equal(t, "thunder-agent-uuid", updated.ThunderAgentID)
	assert.Equal(t, "client-id-123", updated.ThunderClientID)
	assert.Equal(t, 1, updated.AttemptCount)
	assert.Nil(t, updated.NextRetryAt)
	assert.NotNil(t, updated.LastAttemptedAt)
}

func TestAgentThunderClientRepo_UpdateAfterAttempt_FailureSchedulesRetry(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-attempt-fail-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))
	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)

	next := time.Now().Add(1 * time.Minute)
	err = repo.UpdateAfterAttempt(context.Background(), got.ID, AgentThunderAttemptUpdate{
		Status:      models.AgentThunderStatusPending,
		LastError:   "thunder instance unreachable",
		NextRetryAt: &next,
	})
	require.NoError(t, err)

	updated, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusPending, updated.Status)
	assert.Equal(t, "thunder instance unreachable", updated.LastError)
	assert.Equal(t, 1, updated.AttemptCount)
	require.NotNil(t, updated.NextRetryAt)
	assert.WithinDuration(t, next, *updated.NextRetryAt, time.Second)
	// A failed attempt must never clobber an identity already established by an
	// earlier successful attempt on the same row (e.g. Thunder created the agent
	// but a later step failed) — ThunderAgentID/ClientID were empty in this update,
	// so the previously-empty values are simply left as-is here (nothing to lose),
	// but the zero-value guard in UpdateAfterAttempt is what protects a non-empty
	// prior value from being overwritten by an empty one.
	assert.Empty(t, updated.ThunderAgentID)
}

func TestAgentThunderClientRepo_UpdateSecretRef(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-secretref-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))
	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)

	require.NoError(t, repo.UpdateSecretRef(context.Background(), got.ID, "agent-thunder-clients/test-org/test-proj/dev/atc-secretref-agent"))
	updated, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, "agent-thunder-clients/test-org/test-proj/dev/atc-secretref-agent", updated.SecretRefPath)

	// Clearing it back to "" (the revoke case) must work too.
	require.NoError(t, repo.UpdateSecretRef(context.Background(), got.ID, ""))
	cleared, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Empty(t, cleared.SecretRefPath)
}

func TestAgentThunderClientRepo_DeleteByAgent(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "atc-delete-agent"

	require.NoError(t, repo.Upsert(context.Background(), newTestAgentThunderClient(org, project, agent, "dev")))
	require.NoError(t, repo.Upsert(context.Background(), newTestAgentThunderClient(org, project, agent, "prod")))

	require.NoError(t, repo.DeleteByAgent(context.Background(), org, project, agent))

	rows, err := repo.FindByAgent(context.Background(), org, project, agent)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// TestAgentThunderClientRepo_DeleteByIDs_IgnoresConcurrentRecreate reproduces
// the exact race a blanket delete-by-agent-name is vulnerable to: a same-named
// agent recreated (fresh row, different ID) while a background deletion for
// the old generation is still snapshotted and in flight. Deleting by the
// snapshotted IDs must remove only the old rows and leave the new one intact.
func TestAgentThunderClientRepo_DeleteByIDs_IgnoresConcurrentRecreate(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "atc-delete-by-ids-agent"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	old1 := newTestAgentThunderClient(org, project, agent, "dev")
	require.NoError(t, repo.Upsert(context.Background(), old1))
	old2 := newTestAgentThunderClient(org, project, agent, "prod")
	require.NoError(t, repo.Upsert(context.Background(), old2))

	// Simulates a background DeleteAllBindings snapshotting these two rows
	// before doing the (slow) Thunder-side cleanup.
	snapshotIDs := []uuid.UUID{old1.ID, old2.ID}

	// Simulates the agent being recreated under the same name while that
	// background cleanup is still in flight — a distinct environment so this
	// Upsert lands as a genuinely new row rather than conflicting with old1's
	// still-present "dev" row.
	recreated := newTestAgentThunderClient(org, project, agent, "staging")
	require.NoError(t, repo.Upsert(context.Background(), recreated))
	require.NotEqual(t, uuid.Nil, recreated.ID)

	require.NoError(t, repo.DeleteByIDs(context.Background(), snapshotIDs))

	rows, err := repo.FindByAgent(context.Background(), org, project, agent)
	require.NoError(t, err)
	require.Len(t, rows, 1, "the recreated agent's fresh row must survive a delete scoped to the old snapshot")
	assert.Equal(t, recreated.ID, rows[0].ID)
}

func TestAgentThunderClientRepo_SyncMarkerIsClaimedAndClearedOnlyIfNotMoved(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "atc-sync-agent"
	ctx := context.Background()
	cleanupAgentThunderClients(t, repo, org, project, agent)

	home := newTestAgentThunderClient(org, project, agent, "dev")
	home.Status = models.AgentThunderStatusCompleted
	promoted := newTestAgentThunderClient(org, project, agent, "stage")
	promoted.Status = models.AgentThunderStatusCompleted
	promoted.PromotionSource = "dev"
	pending := newTestAgentThunderClient(org, project, agent, "prod")
	pending.PromotionSource = "stage"
	for _, c := range []*models.AgentThunderClient{home, promoted, pending} {
		require.NoError(t, repo.Upsert(ctx, c))
	}

	past := time.Now().Add(-time.Minute).Truncate(time.Microsecond)
	require.NoError(t, repo.MarkSyncAfter(ctx, []uuid.UUID{home.ID, promoted.ID, pending.ID}, past))
	gotHome, err := repo.Get(ctx, org, project, agent, "dev")
	require.NoError(t, err)
	assert.Nil(t, gotHome.SyncAfter, "a binding created directly is never marked")
	gotPending, err := repo.Get(ctx, org, project, agent, "prod")
	require.NoError(t, err)
	assert.Nil(t, gotPending.SyncAfter, "a pending binding reads the current set when it is attempted")

	due, err := repo.FindSyncDue(ctx, time.Now(), 50)
	require.NoError(t, err)
	var ids []uuid.UUID
	var observed time.Time
	for _, row := range due {
		if row.AgentName == agent {
			ids = append(ids, row.ID)
			observed = *row.SyncAfter
		}
	}
	require.Equal(t, []uuid.UUID{promoted.ID}, ids)

	lease := time.Now().Add(2 * time.Minute).Truncate(time.Microsecond)
	claimed, err := repo.ClaimSync(ctx, promoted.ID, observed, lease)
	require.NoError(t, err)
	assert.True(t, claimed)
	again, err := repo.ClaimSync(ctx, promoted.ID, observed, lease)
	require.NoError(t, err)
	assert.False(t, again, "a second replica cannot take the same value")

	// A newer request moves the marker while the sync runs: finishing it must not clear it.
	newer := time.Now().Add(10 * time.Second).Truncate(time.Microsecond)
	require.NoError(t, repo.MarkSyncAfter(ctx, []uuid.UUID{promoted.ID}, newer))
	done, err := repo.FinishSync(ctx, promoted.ID, lease, nil)
	require.NoError(t, err)
	assert.False(t, done, "the claim was superseded")
	got, err := repo.Get(ctx, org, project, agent, "stage")
	require.NoError(t, err)
	require.NotNil(t, got.SyncAfter)
	assert.True(t, got.SyncAfter.Equal(newer))

	done, err = repo.FinishSync(ctx, promoted.ID, newer, nil)
	require.NoError(t, err)
	assert.True(t, done)
	got, err = repo.Get(ctx, org, project, agent, "stage")
	require.NoError(t, err)
	assert.Nil(t, got.SyncAfter)
}

func TestAgentThunderClientRepo_DeferAndNoteDoNotConsumeAttempts(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "atc-defer-agent"
	ctx := context.Background()
	cleanupAgentThunderClients(t, repo, org, project, agent)
	c := newTestAgentThunderClient(org, project, agent, "stage")
	c.PromotionSource = "dev"
	require.NoError(t, repo.Upsert(ctx, c))
	claimed, err := repo.ClaimForAttempt(ctx, c.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	next := time.Now().Add(5 * time.Minute).Truncate(time.Microsecond)
	require.ErrorIs(t, repo.Defer(ctx, c.ID, next, "Waiting."), ErrAgentThunderClientNotFound,
		"a deferral never touches a binding another attempt has claimed")
	require.NoError(t, repo.ReleaseClaim(ctx, c.ID, next, "Waiting."))
	require.ErrorIs(t, repo.ReleaseClaim(ctx, c.ID, next, "Waiting."), ErrAgentThunderClientNotFound, "only a claimed binding is released")
	got, err := repo.Get(ctx, org, project, agent, "stage")
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusPending, got.Status, "a claimed attempt is released")
	assert.Zero(t, got.AttemptCount)
	assert.Equal(t, "Waiting.", got.LastError)
	require.NotNil(t, got.NextRetryAt)
	assert.True(t, got.NextRetryAt.Equal(next))

	require.NoError(t, repo.SetLastError(ctx, c.ID, ""))
	got, err = repo.Get(ctx, org, project, agent, "stage")
	require.NoError(t, err)
	assert.Empty(t, got.LastError)
	assert.Equal(t, "dev", got.PromotionSource)
}

func TestAgentThunderClientRepo_ListHomeAgentsPagesDistinctAgentsWithACompletedDirectBinding(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project = "test-org-home", "test-proj"
	ctx := context.Background()
	for _, name := range []string{"atc-home-a", "atc-home-b", "atc-home-c"} {
		cleanupAgentThunderClients(t, repo, org, project, name)
		direct := newTestAgentThunderClient(org, project, name, "dev")
		direct.Status = models.AgentThunderStatusCompleted
		promoted := newTestAgentThunderClient(org, project, name, "stage")
		promoted.Status = models.AgentThunderStatusCompleted
		promoted.PromotionSource = "dev"
		require.NoError(t, repo.Upsert(ctx, direct))
		require.NoError(t, repo.Upsert(ctx, promoted))
	}
	pendingOnly := newTestAgentThunderClient(org, project, "atc-home-pending", "dev")
	cleanupAgentThunderClients(t, repo, org, project, "atc-home-pending")
	require.NoError(t, repo.Upsert(ctx, pendingOnly))

	// Start just before this test's organization so rows other tests left do not interfere.
	first, err := repo.ListHomeAgents(ctx, &AgentRef{OUID: org}, 2)
	require.NoError(t, err)
	require.Len(t, first, 2)
	assert.Equal(t, "atc-home-a", first[0].AgentName)
	second, err := repo.ListHomeAgents(ctx, &first[1], 2)
	require.NoError(t, err)
	require.NotEmpty(t, second)
	assert.Equal(t, "atc-home-c", second[0].AgentName, "one entry per agent, in order, and no pending-only agent")
}
