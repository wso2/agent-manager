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
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func cleanupEnvThunderURL(t *testing.T, repo EnvThunderURLRepository, ouID, env string) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.Delete(context.Background(), ouID, env)
	})
}

// TestEnvThunderURLRepo_InsertNeverOverwritesAnExistingRow is Insert's core
// correctness property: it must be insert-only, with NO
// "ON CONFLICT ... DO UPDATE" — a second Insert for the SAME (ouID, env) with
// a DIFFERENT handle must be rejected, and the original row's handle must
// survive completely untouched. Thunder's issuer is immutable once minted, so
// the row that wins first must never be silently moved to a different
// hostname by a later write for the same environment.
func TestEnvThunderURLRepo_InsertNeverOverwritesAnExistingRow(t *testing.T) {
	repo := NewEnvThunderURLRepo(db.GetDB())
	const ouID, env = "ou-test-insert-no-overwrite", "env-thunder-insert-no-overwrite"
	cleanupEnvThunderURL(t, repo, ouID, env)

	first := &models.EnvThunderURL{OUID: ouID, EnvName: env, ThunderHandle: strPtr("firsthandle"), ThunderURL: "http://firsthandle.amp.localhost:8080"}
	require.NoError(t, repo.Insert(context.Background(), first))

	second := &models.EnvThunderURL{OUID: ouID, EnvName: env, ThunderHandle: strPtr("secondhandle"), ThunderURL: "http://secondhandle.amp.localhost:8080"}
	err := repo.Insert(context.Background(), second)
	require.Error(t, err, "a second Insert for the same (ouID, env) must be rejected, not silently accepted")
	assert.ErrorIs(t, err, utils.ErrEnvThunderURLAlreadyClaimed,
		"a conflict on (ou_id, env_name) — a losing race for the SAME environment — must be reported distinctly from a handle collision")

	got, getErr := repo.Get(context.Background(), ouID, env)
	require.NoError(t, getErr)
	require.NotNil(t, got.ThunderHandle)
	assert.Equal(t, "firsthandle", *got.ThunderHandle, "the FIRST insert's handle must survive completely untouched")

	var count int64
	require.NoError(t, db.GetDB().Model(&models.EnvThunderURL{}).
		Where("ou_id = ? AND env_name = ?", ouID, env).Count(&count).Error)
	assert.Equal(t, int64(1), count, "must never create a second row for the same (ouID, env)")
}

// TestEnvThunderURLRepo_ConcurrentFirstInsertsOnlyOneWins runs two goroutines
// racing to insert-claim the SAME (ouID, env) for the first time. Exactly one
// must win; the loser must get ErrEnvThunderURLAlreadyClaimed (never silently
// overwrite), and the row left behind must be one of the two attempted
// values, never a third, corrupted, or empty one.
func TestEnvThunderURLRepo_ConcurrentFirstInsertsOnlyOneWins(t *testing.T) {
	repo := NewEnvThunderURLRepo(db.GetDB())
	const ouID, env = "ou-test-concurrent-race", "env-thunder-concurrent-race"
	cleanupEnvThunderURL(t, repo, ouID, env)

	handleA, handleB := "racehandlea", "racehandleb"
	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results[0] = repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouID, EnvName: env, ThunderHandle: strPtr(handleA), ThunderURL: "http://" + handleA + ".amp.localhost:8080"})
	}()
	go func() {
		defer wg.Done()
		results[1] = repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouID, EnvName: env, ThunderHandle: strPtr(handleB), ThunderURL: "http://" + handleB + ".amp.localhost:8080"})
	}()
	wg.Wait()

	successes, claims := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, utils.ErrEnvThunderURLAlreadyClaimed):
			claims++
		default:
			t.Fatalf("unexpected error from a racing Insert: %v", err)
		}
	}
	assert.Equal(t, 1, successes, "exactly one of the two racing inserts must win")
	assert.Equal(t, 1, claims, "the loser must be told it lost the claim, not silently succeed or overwrite")

	got, err := repo.Get(context.Background(), ouID, env)
	require.NoError(t, err)
	require.NotNil(t, got.ThunderHandle)
	assert.Contains(t, []string{handleA, handleB}, *got.ThunderHandle,
		"the surviving row must be exactly one of the two attempted values")
}

// TestEnvThunderURLRepo_DifferentEnvironmentsCannotShareAHandle is the OTHER
// unique constraint: a handle is globally unique across every org/environment
// (every env-Thunder's HTTPRoute attaches to the same shared cluster-wide
// Gateway), distinct from — and reported distinctly from — the same-environment
// race above.
func TestEnvThunderURLRepo_DifferentEnvironmentsCannotShareAHandle(t *testing.T) {
	repo := NewEnvThunderURLRepo(db.GetDB())
	const ouA, envA = "ou-test-handle-collision-a", "env-thunder-handle-collision-a"
	const ouB, envB = "ou-test-handle-collision-b", "env-thunder-handle-collision-b"
	cleanupEnvThunderURL(t, repo, ouA, envA)
	cleanupEnvThunderURL(t, repo, ouB, envB)

	const sharedHandle = "sharedhandle"
	require.NoError(t, repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouA, EnvName: envA, ThunderHandle: strPtr(sharedHandle), ThunderURL: "http://" + sharedHandle + ".amp.localhost:8080"}))

	err := repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouB, EnvName: envB, ThunderHandle: strPtr(sharedHandle), ThunderURL: "http://different-origin-but-same-handle.amp.localhost:8080"})
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrThunderHandleTaken,
		"a DIFFERENT environment claiming an already-registered handle string is a genuine collision, distinct from a same-environment race")

	_, getErr := repo.Get(context.Background(), ouB, envB)
	assert.True(t, errors.Is(getErr, gorm.ErrRecordNotFound), "the rejected insert for envB must not have created a row")
}

func TestEnvThunderURLRepo_GetNotFound(t *testing.T) {
	repo := NewEnvThunderURLRepo(db.GetDB())

	_, err := repo.Get(context.Background(), "no-such-ou", "no-such-env")
	require.Error(t, err)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

// TestEnvThunderURLRepo_DeleteIsIdempotent mirrors
// remove-environment-thunder.sh's expectation that deleting an
// already-removed (or never-created) handle is not an error — teardown must
// succeed even on a retry, and frees the handle for reuse afterward.
func TestEnvThunderURLRepo_DeleteIsIdempotent(t *testing.T) {
	repo := NewEnvThunderURLRepo(db.GetDB())
	const ouID, env = "ou-test-delete-idempotent", "env-thunder-url-delete-idempotent"

	require.NoError(t, repo.Delete(context.Background(), ouID, env), "deleting a non-existent row must not be an error")

	require.NoError(t, repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouID, EnvName: env, ThunderHandle: strPtr("deleteidempo"), ThunderURL: "http://deleteidempo.amp.localhost:8080"}))
	require.NoError(t, repo.Delete(context.Background(), ouID, env))
	_, err := repo.Get(context.Background(), ouID, env)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))

	// Deleting again (simulating a retried teardown) must still succeed.
	require.NoError(t, repo.Delete(context.Background(), ouID, env))

	// And the handle must be genuinely free again — a re-insert must succeed.
	require.NoError(t, repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouID, EnvName: env, ThunderHandle: strPtr("deleteidempo"), ThunderURL: "http://deleteidempo.amp.localhost:8080"}))
	require.NoError(t, repo.Delete(context.Background(), ouID, env))
}

// TestEnvThunderURLRepo_CallerSuppliedRowsHaveNoHandleButMustHaveAUniqueURL
// covers the caller-supplied url registration path: a row with no
// thunder_handle at all (NULL — Postgres never conflicts two NULLs on
// uq_env_thunder_urls_handle), but thunder_url is the primary invariant
// there, enforced by uq_env_thunder_urls_url exactly like the handle-based
// path enforces uq_env_thunder_urls_handle.
func TestEnvThunderURLRepo_CallerSuppliedRowsHaveNoHandleButMustHaveAUniqueURL(t *testing.T) {
	repo := NewEnvThunderURLRepo(db.GetDB())
	const ouA, envA = "ou-test-saas-url-a", "env-thunder-saas-url-a"
	const ouB, envB = "ou-test-saas-url-b", "env-thunder-saas-url-b"
	cleanupEnvThunderURL(t, repo, ouA, envA)
	cleanupEnvThunderURL(t, repo, ouB, envB)

	const sharedURL = "https://stage-idp.saas-url-test.example.com"
	require.NoError(t, repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouA, EnvName: envA, ThunderURL: sharedURL}))

	got, err := repo.Get(context.Background(), ouA, envA)
	require.NoError(t, err)
	assert.Empty(t, got.ThunderHandle, "a caller-supplied row has no handle")
	assert.Equal(t, sharedURL, got.ThunderURL)

	err = repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouB, EnvName: envB, ThunderURL: sharedURL})
	require.Error(t, err, "a different environment claiming an already-registered url is a genuine collision")
	assert.ErrorIs(t, err, utils.ErrThunderURLTaken)

	_, getErr := repo.Get(context.Background(), ouB, envB)
	assert.True(t, errors.Is(getErr, gorm.ErrRecordNotFound), "the rejected insert for envB must not have created a row")
}

// TestEnvThunderURLRepo_MultipleCallerSuppliedRowsWithoutHandlesDoNotConflict
// proves Postgres's NULL semantics hold here: two handle-less
// (caller-supplied) rows for different environments and different URLs must
// both succeed — NULL never equals NULL under a unique constraint, so
// uq_env_thunder_urls_handle never fires for either of them.
func TestEnvThunderURLRepo_MultipleCallerSuppliedRowsWithoutHandlesDoNotConflict(t *testing.T) {
	repo := NewEnvThunderURLRepo(db.GetDB())
	const ouA, envA = "ou-test-saas-no-handle-a", "env-thunder-saas-no-handle-a"
	const ouB, envB = "ou-test-saas-no-handle-b", "env-thunder-saas-no-handle-b"
	cleanupEnvThunderURL(t, repo, ouA, envA)
	cleanupEnvThunderURL(t, repo, ouB, envB)

	require.NoError(t, repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouA, EnvName: envA, ThunderURL: "https://idp-a.saas-url-test.example.com"}))
	require.NoError(t, repo.Insert(context.Background(), &models.EnvThunderURL{OUID: ouB, EnvName: envB, ThunderURL: "https://idp-b.saas-url-test.example.com"}))

	gotA, err := repo.Get(context.Background(), ouA, envA)
	require.NoError(t, err)
	assert.Empty(t, gotA.ThunderHandle)

	gotB, err := repo.Get(context.Background(), ouB, envB)
	require.NoError(t, err)
	assert.Empty(t, gotB.ThunderHandle)
}

func strPtr(s string) *string { return &s }
