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
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/aesgcm"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
	"github.com/wso2/agent-manager/agent-manager-service/vault/vaulttest"
)

func secretRow(t *testing.T, id uuid.UUID) models.Secret {
	t.Helper()
	var row models.Secret
	require.NoError(t, db.GetDB().Where("id = ?", id).Take(&row).Error)
	return row
}

func TestSecretStoreSealsValuesIntoTheRow(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, owner, id := newOrg(t), NewID(), NewID()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", owner, id, []byte("first")))
	row := secretRow(t, id)
	assert.Equal(t, vault.ProviderLocal, row.Provider)
	assert.NotNil(t, row.KeyID, "the sealing key is recorded for rotation")
	assert.Nil(t, row.ExternalRef)
	assert.NotContains(t, string(row.Ciphertext), "first")
	assert.Equal(t, 1, row.Version)

	value, found, err := store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "first", string(value))

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", owner, id, []byte("second")))
	assert.Equal(t, 2, secretRow(t, id).Version)
	value, _, err = store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.Equal(t, "second", string(value))

	require.Error(t, store.Write(ctx, gdb, newOrg(t), "test_owner", owner, id, []byte("x")),
		"another organization cannot overwrite the secret")

	require.NoError(t, store.Delete(ctx, gdb, id))
	_, found, err = store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.False(t, found)
	require.NoError(t, store.Delete(ctx, gdb, id), "deleting a missing secret is a no-op")
}

// A ciphertext copied into another row must not open there.
func TestConcurrentFirstWritesOfASecretAllSucceed(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, owner, id := newOrg(t), NewID(), NewID()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })

	const writers = 6
	start := make(chan struct{})
	results := make(chan error, writers)
	for i := range writers {
		go func() {
			<-start
			results <- gdb.Transaction(func(tx *gorm.DB) error {
				return store.Write(ctx, tx, org, "test_owner", owner, id, []byte{byte('a' + i)})
			})
		}()
	}
	close(start)
	// Drain every writer before asserting, so cleanup never runs while one is still writing.
	succeeded := true
	for range writers {
		if !assert.NoError(t, <-results, "a write that loses the insert replaces the value instead of failing") {
			succeeded = false
		}
	}
	if succeeded {
		assert.Equal(t, writers, secretRow(t, id).Version, "every write landed, one after another")
	}
}

func TestSecretStoreBindsACiphertextToItsRow(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, a, b := newOrg(t), NewID(), NewID()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, a); _ = store.Delete(ctx, gdb, b) })

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", a, a, []byte("a-secret")))
	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", b, b, []byte("b-secret")))
	require.NoError(t, gdb.Model(&models.Secret{}).Where("id = ?", b).
		Update("ciphertext", secretRow(t, a).Ciphertext).Error)

	_, _, err := store.Read(ctx, gdb, b)
	require.Error(t, err)
}

// Rows written before the vault hold what the owning service encrypted itself.
func TestSecretStoreReadsRowsWrittenBeforeTheVault(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, id, now := newOrg(t), NewID(), dbx.Now()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })

	legacy, err := aesgcm.Encrypt([]byte("old-value"), vaulttest.Key)
	require.NoError(t, err)
	require.NoError(t, gdb.Create(&models.Secret{
		ID: id, OrgID: org, OwnerKind: "test_owner", OwnerID: id, Provider: vault.ProviderLocal,
		Ciphertext: legacy, Version: 1, CreatedAt: now, UpdatedAt: now,
	}).Error)

	value, err := store.Get(gdb, &id)
	require.NoError(t, err)
	assert.Equal(t, "old-value", value)

	stored, err := store.GetAsStored(gdb, &id)
	require.NoError(t, err)
	assert.Equal(t, string(legacy), stored, "an owner that encrypts itself gets its own form back")

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", id, id, []byte("old-value")))
	assert.NotNil(t, secretRow(t, id).KeyID, "the next write reseals the row under the vault")
	stored, err = store.GetAsStored(gdb, &id)
	require.NoError(t, err)
	assert.Equal(t, "old-value", stored)
}

// externalFixture is a store that seals new values with an external vault, and the runner that does that vault's
// work once a write commits.
type externalFixture struct {
	store    *SecretStore
	runner   *SecretJobRunner
	external *vaulttest.External
	org      string
	// offset moves the runner's clock ahead of real time, past retry backoffs.
	offset time.Duration
}

func newExternalFixture(t *testing.T) *externalFixture {
	t.Helper()
	vaults, external := vaulttest.LocalAndExternal(t)
	f := &externalFixture{
		store: NewSecretStore(vaults), runner: NewSecretJobRunner(db.GetDB(), vaults, slog.New(slog.DiscardHandler)),
		external: external, org: newOrg(t),
	}
	return f
}

// cleanup removes what the test left for id, its jobs included, so no other test's runner picks them up.
func (f *externalFixture) cleanup(t *testing.T, ids ...uuid.UUID) {
	t.Cleanup(func() {
		for _, id := range ids {
			_ = ReleaseSecret(db.GetDB(), &id)
			_ = db.GetDB().Where("secret_id = ?", id).Delete(&models.SecretJob{}).Error
		}
	})
}

func (f *externalFixture) now() time.Time { return time.Now().UTC().Add(f.offset) }

// run runs the jobs due by the fixture's clock and returns how many ran and failed.
func (f *externalFixture) run(t *testing.T) (int, int) {
	t.Helper()
	ran, failed, err := f.runner.RunOnce(context.Background(), f.now())
	require.NoError(t, err)
	return ran, failed
}

// later moves the fixture's clock past any retry backoff.
func (f *externalFixture) later() { f.offset += 2 * secretJobMaxBackoff }

func (f *externalFixture) write(t *testing.T, id uuid.UUID, value string) {
	t.Helper()
	require.NoError(t, f.store.Write(context.Background(), db.GetDB(), f.org, "test_owner", id, id, []byte(value)))
}

func (f *externalFixture) read(t *testing.T, id uuid.UUID) string {
	t.Helper()
	value, found, err := f.store.Read(context.Background(), db.GetDB(), id)
	require.NoError(t, err)
	require.True(t, found)
	return string(value)
}

func (f *externalFixture) pending(t *testing.T, id uuid.UUID) int64 {
	t.Helper()
	n, err := PendingSecretJobs(context.Background(), db.GetDB(), id)
	require.NoError(t, err)
	return n
}

// errRollback aborts a transaction on purpose.
var errRollback = errors.New("roll back")

func (f *externalFixture) rolledBack(t *testing.T, fn func(tx *gorm.DB) error) {
	t.Helper()
	err := db.GetDB().Transaction(func(tx *gorm.DB) error {
		require.NoError(t, fn(tx))
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
}

// promoted writes value and promotes it, returning the reference the row holds.
func (f *externalFixture) promoted(t *testing.T, id uuid.UUID, value string) string {
	t.Helper()
	f.write(t, id, value)
	f.run(t)
	row := secretRow(t, id)
	require.Equal(t, vaulttest.ExternalProvider, row.Provider)
	require.NotNil(t, row.ExternalRef)
	return *row.ExternalRef
}

func TestSecretStoreWritesAnExternalVaultOnlyAfterCommit(t *testing.T) {
	f := newExternalFixture(t)
	id := NewID()
	f.cleanup(t, id)

	f.write(t, id, "v1")
	row := secretRow(t, id)
	assert.Equal(t, vault.ProviderLocal, row.Provider, "a write seals into the row until it has committed")
	assert.Zero(t, f.external.Len(), "nothing reaches the external vault inside the transaction")
	assert.Equal(t, "v1", f.read(t, id))
	assert.EqualValues(t, 1, f.pending(t, id))

	ran, failed := f.run(t)
	assert.Positive(t, ran)
	assert.Zero(t, failed)
	row = secretRow(t, id)
	assert.Equal(t, vaulttest.ExternalProvider, row.Provider)
	assert.Empty(t, row.Ciphertext, "nothing of the value stays in the database")
	assert.Nil(t, row.KeyID)
	require.NotNil(t, row.ExternalRef)
	first := *row.ExternalRef
	assert.Equal(t, "v1", f.external.Value(first))
	assert.Equal(t, "v1", f.read(t, id))
	assert.Zero(t, f.pending(t, id))

	// A rotation stages a new version; the one the row pointed to goes only once the new one is in the row.
	f.write(t, id, "v2")
	assert.True(t, f.external.Has(first), "the previous version stays until its replacement is recorded")
	f.run(t)
	second := *secretRow(t, id).ExternalRef
	assert.NotEqual(t, first, second)
	assert.Equal(t, "v2", f.external.Value(second))
	assert.Equal(t, "v2", f.read(t, id))
	f.run(t)
	assert.False(t, f.external.Has(first), "the replaced version is removed after the commit")
	assert.Zero(t, f.pending(t, id))

	require.NoError(t, f.store.Delete(context.Background(), db.GetDB(), id))
	assert.True(t, f.external.Has(second), "a delete removes the external value only through a job")
	f.run(t)
	assert.False(t, f.external.Has(second))
	assert.Zero(t, f.pending(t, id))
}

// The defect this guards against: a delete or replacement inside a transaction that later rolls back must leave the
// value the restored row points to readable.
func TestSecretStoreRollbackKeepsTheExternalValue(t *testing.T) {
	f := newExternalFixture(t)
	id := NewID()
	f.cleanup(t, id)
	ref := f.promoted(t, id, "v1")

	f.rolledBack(t, func(tx *gorm.DB) error { return f.store.Delete(context.Background(), tx, id) })
	f.rolledBack(t, func(tx *gorm.DB) error {
		return f.store.Write(context.Background(), tx, f.org, "test_owner", id, id, []byte("v2"))
	})
	f.later()
	f.run(t)

	assert.True(t, f.external.Has(ref))
	assert.Equal(t, ref, *secretRow(t, id).ExternalRef)
	assert.Equal(t, "v1", f.read(t, id))
	assert.Zero(t, f.pending(t, id), "the rolled-back writes queued nothing")
	assert.Empty(t, f.external.Destroyed)
}

func TestSecretJobsRetryAndNeverRemoveAValueInUse(t *testing.T) {
	f := newExternalFixture(t)
	id, other := NewID(), NewID()
	f.cleanup(t, id, other)

	// A failed promotion leaves the value readable from the row and is retried.
	f.external.FailSeal = 1
	f.write(t, id, "v1")
	_, failed := f.run(t)
	assert.Equal(t, 1, failed)
	assert.Equal(t, vault.ProviderLocal, secretRow(t, id).Provider)
	assert.Equal(t, "v1", f.read(t, id))
	var job models.SecretJob
	require.NoError(t, db.GetDB().Where("secret_id = ?", id).Take(&job).Error)
	assert.Equal(t, 1, job.Attempts)
	require.NotNil(t, job.LastError)
	assert.True(t, job.AvailableAt.After(f.now()), "a failed job waits before its next attempt")
	ran, _ := f.run(t)
	assert.Zero(t, ran, "a job is not retried before its backoff ends")
	f.later()
	f.run(t)
	ref := *secretRow(t, id).ExternalRef

	// A failed removal is retried until it succeeds.
	f.external.FailDestroy = 1
	require.NoError(t, f.store.Delete(context.Background(), db.GetDB(), id))
	_, failed = f.run(t)
	assert.Equal(t, 1, failed)
	assert.True(t, f.external.Has(ref))
	f.later()
	f.run(t)
	assert.False(t, f.external.Has(ref))
	assert.Zero(t, f.pending(t, id))

	// A removal queued for a value a committed row refers to is dropped, and the value kept.
	inUse := f.promoted(t, other, "kept")
	require.NoError(t, queueDestroy(db.GetDB(), f.org, other, "test_owner",
		externalValue{provider: vaulttest.ExternalProvider, ref: inUse}))
	require.NoError(t, queueDestroy(db.GetDB(), f.org, other, "test_owner",
		externalValue{provider: vaulttest.ExternalProvider, ref: inUse}))
	f.run(t)
	assert.True(t, f.external.Has(inUse))
	assert.Equal(t, "kept", f.read(t, other))
	assert.Zero(t, f.pending(t, other))

	// Removing a value that is already gone succeeds, so a retry after a lost acknowledgement finishes.
	require.NoError(t, queueDestroy(db.GetDB(), f.org, other, "test_owner",
		externalValue{provider: vaulttest.ExternalProvider, ref: "amp/gone#1"}))
	_, failed = f.run(t)
	assert.Zero(t, failed)
	assert.Zero(t, f.pending(t, other))
}

func TestSecretPromotionHandsOverWhatItReplaces(t *testing.T) {
	f := newExternalFixture(t)
	id := NewID()
	f.cleanup(t, id)
	first := f.promoted(t, id, "v1")

	// Two writes before the worker runs: the first's promotion is superseded and the second's replaces first.
	f.write(t, id, "v2")
	f.write(t, id, "v3")
	f.run(t)
	f.run(t)
	row := secretRow(t, id)
	require.NotNil(t, row.ExternalRef)
	assert.Equal(t, "v3", f.external.Value(*row.ExternalRef))
	assert.False(t, f.external.Has(first))
	assert.Equal(t, 1, f.external.Len(), "a superseded promotion writes nothing")
	assert.Zero(t, f.pending(t, id))

	// A write then a delete before the worker runs removes the promoted value and writes nothing.
	current := *row.ExternalRef
	f.write(t, id, "v4")
	require.NoError(t, f.store.Delete(context.Background(), db.GetDB(), id))
	f.run(t)
	f.run(t)
	assert.False(t, f.external.Has(current))
	assert.Zero(t, f.external.Len())
	assert.Zero(t, f.pending(t, id))
}

func TestSecretPromotionUsesTheQueuedVaultDuringRollout(t *testing.T) {
	f := newExternalFixture(t)
	id := NewID()
	f.cleanup(t, id)
	f.write(t, id, "v1")

	ctx := context.Background()
	oldWorker := NewSecretJobRunner(db.GetDB(), vaulttest.Local(t), slog.New(slog.DiscardHandler))
	ran, failed, err := oldWorker.RunOnce(ctx, f.now())
	require.NoError(t, err)
	require.Equal(t, 1, ran)
	require.Equal(t, 1, failed)
	assert.Equal(t, int64(1), f.pending(t, id))
	assert.Equal(t, vault.ProviderLocal, secretRow(t, id).Provider)

	f.later()
	f.run(t)
	assert.Zero(t, f.pending(t, id))
	assert.Equal(t, vaulttest.ExternalProvider, secretRow(t, id).Provider)
	assert.Equal(t, "v1", f.read(t, id))
}

// stalledSecretVault stalls whichever seal starts first until its attempt times out, and records when each seal
// started, so a test can tell the runner went on after the timeout whatever order it took the jobs in.
type stalledSecretVault struct {
	*vaulttest.External
	mu       sync.Mutex
	started  []uuid.UUID
	stalled  uuid.UUID
	resumeAt time.Time
	resumed  time.Time
}

func (v *stalledSecretVault) Seal(ctx context.Context, ref vault.Ref, value []byte, current *vault.Sealed) (vault.Sealed, error) {
	v.mu.Lock()
	v.started = append(v.started, ref.SecretID)
	first := len(v.started) == 1
	if first {
		v.stalled = ref.SecretID
	} else if v.resumed.IsZero() {
		v.resumed = time.Now()
	}
	v.mu.Unlock()
	if first {
		<-ctx.Done()
		v.mu.Lock()
		v.resumeAt = time.Now()
		v.mu.Unlock()
		return vault.Sealed{}, ctx.Err()
	}
	return v.External.Seal(ctx, ref, value, current)
}

func TestSecretJobsRetryTimedOutAttemptsAndContinue(t *testing.T) {
	f := newExternalFixture(t)
	a, b := NewID(), NewID()
	f.cleanup(t, a, b)
	local, err := vault.NewLocal(vaulttest.Key)
	require.NoError(t, err)
	adapter := &stalledSecretVault{External: f.external}
	f.runner.vaults = vault.Of(adapter, local)
	f.runner.attemptTimeout = 250 * time.Millisecond
	f.write(t, a, "v1")
	f.write(t, b, "v2")

	ran, failed := f.run(t)
	require.Equal(t, 2, ran)
	require.Equal(t, 1, failed)
	adapter.mu.Lock()
	stalled, started, resumeAt, resumed := adapter.stalled, slices.Clone(adapter.started), adapter.resumeAt, adapter.resumed
	adapter.mu.Unlock()
	require.Len(t, started, 2, "the second job's seal started in the same pass")
	healthy := a
	if stalled == a {
		healthy = b
	}
	assert.False(t, resumed.Before(resumeAt), "the second seal started only once the first had timed out")

	var retry models.SecretJob
	require.NoError(t, db.GetDB().Where("secret_id = ?", stalled).Take(&retry).Error)
	require.NotNil(t, retry.LastError)
	assert.Equal(t, context.DeadlineExceeded.Error(), *retry.LastError)
	assert.Nil(t, retry.ClaimedBy)
	assert.Equal(t, 1, retry.Attempts)
	assert.Zero(t, f.pending(t, healthy))
	assert.Equal(t, vaulttest.ExternalProvider, secretRow(t, healthy).Provider)
}

func TestSecretPromotionRecoversAnInterruptedExternalWrite(t *testing.T) {
	f := newExternalFixture(t)
	id := NewID()
	f.cleanup(t, id)
	f.write(t, id, "v1")

	// Simulate losing the worker after the external write, before publication.
	var job models.SecretJob
	require.NoError(t, db.GetDB().Where("secret_id = ?", id).Take(&job).Error)
	ref := jobRef(&job)
	ref.OperationID = job.ID
	require.NotNil(t, job.StagedRef, "the reference is reserved when the job is queued")
	path := *job.StagedRef
	_, err := f.external.Seal(context.Background(), ref, []byte("v1"), nil)
	require.NoError(t, err)

	f.run(t)
	row := secretRow(t, id)
	require.NotNil(t, row.ExternalRef)
	assert.Equal(t, path, *row.ExternalRef, "the retry reuses the original operation")
	assert.Equal(t, "v1", f.external.Value(path))
	assert.Equal(t, 1, f.external.Len())
	assert.Zero(t, f.pending(t, id))
}

func TestSecretPromotionCannotPublishOrQueueCleanupAfterClaimExpires(t *testing.T) {
	f := newExternalFixture(t)
	id := NewID()
	f.cleanup(t, id)
	f.write(t, id, "v1")

	var job models.SecretJob
	require.NoError(t, db.GetDB().Where("secret_id = ?", id).Take(&job).Error)
	token := "expired-claim"
	now := f.now()
	require.NoError(t, db.GetDB().Model(&job).Updates(map[string]any{
		"claimed_by": token, "claimed_until": now.Add(-time.Second),
	}).Error)
	job.ClaimedBy = &token
	attempt := &secretJobAttempt{SecretJob: &job, now: func() time.Time { return now }}
	ref := jobRef(&job)
	ref.OperationID = job.ID
	sealed, err := f.external.Seal(context.Background(), ref, []byte("v1"), nil)
	require.NoError(t, err)
	require.NoError(t, db.GetDB().Model(&job).Updates(map[string]any{
		"staged_provider": vaulttest.ExternalProvider, "staged_ref": sealed.ExternalRef,
	}).Error)

	row := secretRow(t, id)
	require.ErrorIs(t, f.runner.publish(context.Background(), attempt, &row, sealed, vaulttest.ExternalProvider, nil), errSecretJobClaimLost)
	require.ErrorIs(t, f.runner.finishQueueing(context.Background(), attempt, &externalValue{
		provider: vaulttest.ExternalProvider, ref: sealed.ExternalRef,
	}), errSecretJobClaimLost)
	assert.Equal(t, int64(1), f.pending(t, id))
	assert.True(t, f.external.Has(sealed.ExternalRef))

	f.run(t)
	row = secretRow(t, id)
	require.NotNil(t, row.ExternalRef)
	assert.Equal(t, sealed.ExternalRef, *row.ExternalRef)
	assert.Zero(t, f.pending(t, id))
}

func TestSecretStoreMovesBetweenVaults(t *testing.T) {
	f := newExternalFixture(t)
	local, err := vault.NewLocal(vaulttest.Key)
	require.NoError(t, err)
	localOnly := NewSecretStore(vault.Of(local, f.external))
	id := NewID()
	f.cleanup(t, id)

	// A value the local vault sealed stays readable after the move and goes to the external vault on its next write.
	require.NoError(t, localOnly.Write(context.Background(), db.GetDB(), f.org, "test_owner", id, id, []byte("v1")))
	assert.Zero(t, f.pending(t, id), "the local vault queues nothing")
	assert.Equal(t, "v1", f.read(t, id))
	ref := f.promoted(t, id, "v2")

	// Moving back seals into the row, and the external value goes once that has committed.
	f.rolledBack(t, func(tx *gorm.DB) error {
		return localOnly.Write(context.Background(), tx, f.org, "test_owner", id, id, []byte("v3"))
	})
	f.run(t)
	assert.True(t, f.external.Has(ref))
	require.NoError(t, localOnly.Write(context.Background(), db.GetDB(), f.org, "test_owner", id, id, []byte("v3")))
	row := secretRow(t, id)
	assert.Equal(t, vault.ProviderLocal, row.Provider)
	assert.Nil(t, row.ExternalRef)
	assert.True(t, f.external.Has(ref), "the external value outlives the write until the job runs")
	f.run(t)
	assert.False(t, f.external.Has(ref))
	value, _, err := localOnly.Read(context.Background(), db.GetDB(), id)
	require.NoError(t, err)
	assert.Equal(t, "v3", string(value))
}

// Module secrets, which rt.Secrets() reaches by path, take the same path through the vault as core ones.
func TestModuleSecretsGoThroughTheVault(t *testing.T) {
	f := newExternalFixture(t)
	ctx := context.Background()
	modules := NewModuleSecretStore(db.GetDB(), f.store)
	kind, owner, other := "mod:t-vault", uuid.New(), uuid.New()
	idOf := func(o uuid.UUID) uuid.UUID {
		var ids []uuid.UUID
		require.NoError(t, db.GetDB().Model(&models.Secret{}).
			Where("org_id = ? AND owner_kind = ? AND owner_id = ?", f.org, kind, o).Pluck("id", &ids).Error)
		require.Len(t, ids, 1)
		return ids[0]
	}

	require.NoError(t, modules.Put(ctx, nil, f.org, kind, owner, "v1"))
	require.NoError(t, modules.Put(ctx, nil, f.org, kind, other, "other"))
	id, otherID := idOf(owner), idOf(other)
	f.cleanup(t, id, otherID)
	assert.Zero(t, f.external.Len(), "nothing reaches the external vault before the write commits")

	f.run(t)
	row := secretRow(t, id)
	assert.Equal(t, vaulttest.ExternalProvider, row.Provider)
	require.NotNil(t, row.ExternalRef)
	first := *row.ExternalRef
	value, ok, err := modules.Get(ctx, nil, f.org, kind, owner)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "v1", value)

	require.NoError(t, modules.Put(ctx, nil, f.org, kind, owner, "v2"))
	assert.Equal(t, id, idOf(owner), "a rewrite keeps the owner's one row")
	f.run(t)
	f.run(t)
	assert.False(t, f.external.Has(first), "the replaced version is removed")
	value, _, err = modules.Get(ctx, nil, f.org, kind, owner)
	require.NoError(t, err)
	assert.Equal(t, "v2", value)
	second := *secretRow(t, id).ExternalRef

	// A delete that rolls back leaves the value readable.
	f.rolledBack(t, func(tx *gorm.DB) error { return modules.Delete(ctx, tx, f.org, kind, owner) })
	f.run(t)
	assert.True(t, f.external.Has(second))

	require.NoError(t, modules.Delete(ctx, nil, f.org, kind, owner))
	_, ok, err = modules.Get(ctx, nil, f.org, kind, owner)
	require.NoError(t, err)
	assert.False(t, ok)
	f.run(t)
	assert.False(t, f.external.Has(second), "a delete destroys the external value after it commits")

	// A purge clears every secret of the module, the external values included.
	otherRef := *secretRow(t, otherID).ExternalRef
	require.NoError(t, ReleaseSecretsOfKind(db.GetDB().WithContext(ctx), kind))
	f.run(t)
	assert.False(t, f.external.Has(otherRef))
	assert.Zero(t, f.pending(t, otherID))
}

// A value is bound to the org id as the row stores it, so another spelling of the same id must not lock it out.
func TestSecretStoreAcceptsAnyOrgIDSpelling(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, id := newOrg(t), NewID()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })

	require.NoError(t, store.Write(ctx, gdb, strings.ToUpper(org), "test_owner", id, id, []byte("first")))
	value, found, err := store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "first", string(value))

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", id, id, []byte("second")),
		"the same organization spelled canonically can rewrite it")
	require.NoError(t, store.Write(ctx, gdb, strings.ToUpper(org), "test_owner", id, id, []byte("third")))
	value, _, err = store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.Equal(t, "third", string(value))
}
