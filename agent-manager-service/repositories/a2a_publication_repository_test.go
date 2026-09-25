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

func newTestPublication(agentName string) *models.A2APublication {
	return &models.A2APublication{
		OUID:            "ou-" + uuid.New().String()[:8],
		ProjectName:     "checkout",
		AgentName:       agentName,
		EnvironmentName: "Development",
		EnvironmentUUID: uuid.New(),
		ArtifactUUID:    uuid.New(),
	}
}

func cleanupPublication(t *testing.T, repo A2APublicationRepository, pub *models.A2APublication) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.DeleteForAgent(context.Background(), pub.OUID, pub.ProjectName, pub.AgentName)
	})
}

// A redeploy must re-publish, and a pair that exhausted its budget must get
// another chance — so a second enqueue resets the existing row rather than
// adding a second one for the same pair.
func TestA2APublicationEnqueueResetsTheExistingRow(t *testing.T) {
	repo := NewA2APublicationRepository(db.GetDB())
	ctx := context.Background()

	pub := newTestPublication("trip-planner-" + uuid.New().String()[:8])
	cleanupPublication(t, repo, pub)
	require.NoError(t, repo.Enqueue(ctx, pub))
	require.NoError(t, repo.MarkFailed(ctx, *pub, "gave up"))

	requeued := newTestPublication(pub.AgentName)
	requeued.OUID = pub.OUID
	require.NoError(t, repo.Enqueue(ctx, requeued))

	due, err := repo.FindDue(ctx, time.Now(), 100)
	require.NoError(t, err)

	var found []models.A2APublication
	for _, row := range due {
		if row.AgentName == pub.AgentName {
			found = append(found, row)
		}
	}
	require.Len(t, found, 1, "one row per agent-environment pair, not one per deploy")
	assert.Equal(t, models.A2APublicationStatusPending, found[0].Status)
	assert.Equal(t, 0, found[0].AttemptCount, "the attempt budget is fresh")
	assert.Empty(t, found[0].LastError)
	assert.Equal(t, requeued.ArtifactUUID, found[0].ArtifactUUID, "the newest deploy's artifact wins")
}

// A row whose retry is scheduled for later must not be handed out until then;
// otherwise the backoff has no effect and the reconciler spins.
func TestA2APublicationFindDueRespectsBackoff(t *testing.T) {
	repo := NewA2APublicationRepository(db.GetDB())
	ctx := context.Background()

	pub := newTestPublication("backoff-" + uuid.New().String()[:8])
	cleanupPublication(t, repo, pub)
	require.NoError(t, repo.Enqueue(ctx, pub))
	require.NoError(t, repo.MarkAttemptFailed(ctx, *pub, "binding not ready", time.Now().Add(time.Hour)))

	due, err := repo.FindDue(ctx, time.Now(), 100)
	require.NoError(t, err)
	for _, row := range due {
		assert.NotEqual(t, pub.AgentName, row.AgentName, "a backed-off row is not due yet")
	}

	later, err := repo.FindDue(ctx, time.Now().Add(2*time.Hour), 100)
	require.NoError(t, err)
	var attempts int
	for _, row := range later {
		if row.AgentName == pub.AgentName {
			attempts = row.AttemptCount
		}
	}
	assert.Equal(t, 1, attempts, "the failed attempt was counted")
}

// A published row is done: leaving it due would republish the same agent on
// every tick forever.
func TestA2APublicationMarkPublishedRemovesItFromTheQueue(t *testing.T) {
	repo := NewA2APublicationRepository(db.GetDB())
	ctx := context.Background()

	pub := newTestPublication("published-" + uuid.New().String()[:8])
	cleanupPublication(t, repo, pub)
	require.NoError(t, repo.Enqueue(ctx, pub))
	require.NoError(t, repo.MarkPublished(ctx, *pub))

	due, err := repo.FindDue(ctx, time.Now(), 100)
	require.NoError(t, err)
	for _, row := range due {
		assert.NotEqual(t, pub.AgentName, row.AgentName, "a published row is no longer due")
	}
}

// dueRowFor returns the due row for agentName, as the reconciler would have read it.
func dueRowFor(t *testing.T, repo A2APublicationRepository, agentName string) models.A2APublication {
	t.Helper()
	due, err := repo.FindDue(context.Background(), time.Now(), 100)
	require.NoError(t, err)
	for _, row := range due {
		if row.AgentName == agentName {
			return row
		}
	}
	require.FailNow(t, "no due row", "agent %s", agentName)
	return models.A2APublication{}
}

func statusOf(t *testing.T, pub *models.A2APublication) models.A2APublication {
	t.Helper()
	var row models.A2APublication
	require.NoError(t, db.GetDB().Where("id = ?", pub.ID).First(&row).Error)
	return row
}

// A settings save that re-enqueues while the reconciler is publishing the
// config it read earlier must not be swallowed: the reconciler published the
// old config, so the row has to stay pending for the next tick to pick up the
// new one.
func TestA2APublicationMarkPublishedDoesNotSwallowANewerEnqueue(t *testing.T) {
	repo := NewA2APublicationRepository(db.GetDB())
	ctx := context.Background()

	pub := newTestPublication("race-published-" + uuid.New().String()[:8])
	cleanupPublication(t, repo, pub)
	require.NoError(t, repo.Enqueue(ctx, pub))
	read := dueRowFor(t, repo, pub.AgentName)

	requeue := *pub
	require.NoError(t, repo.Enqueue(ctx, &requeue))
	require.ErrorIs(t, repo.MarkPublished(ctx, read), ErrA2APublicationSuperseded)

	assert.Equal(t, models.A2APublicationStatusPending, statusOf(t, pub).Status,
		"the newer enqueue is still owed a publish")
}

// A retry recorded against a row that was re-enqueued meanwhile would push a
// fresh publication's first attempt 30s out and charge it an attempt it never
// made.
func TestA2APublicationMarkAttemptFailedDoesNotTouchANewerEnqueue(t *testing.T) {
	repo := NewA2APublicationRepository(db.GetDB())
	ctx := context.Background()

	pub := newTestPublication("race-retry-" + uuid.New().String()[:8])
	cleanupPublication(t, repo, pub)
	require.NoError(t, repo.Enqueue(ctx, pub))
	read := dueRowFor(t, repo, pub.AgentName)

	requeue := *pub
	require.NoError(t, repo.Enqueue(ctx, &requeue))
	require.ErrorIs(t, repo.MarkAttemptFailed(ctx, read, "binding not ready", time.Now().Add(time.Hour)), ErrA2APublicationSuperseded)

	row := statusOf(t, pub)
	assert.Equal(t, 0, row.AttemptCount, "the fresh attempt budget is untouched")
	require.NotNil(t, row.NextAttemptAt)
	assert.False(t, row.NextAttemptAt.After(time.Now()), "and it is still due now")
}

// Giving up on a row that was re-enqueued meanwhile would fail a publication
// that has not had a single attempt.
func TestA2APublicationMarkFailedDoesNotFailANewerEnqueue(t *testing.T) {
	repo := NewA2APublicationRepository(db.GetDB())
	ctx := context.Background()

	pub := newTestPublication("race-failed-" + uuid.New().String()[:8])
	cleanupPublication(t, repo, pub)
	require.NoError(t, repo.Enqueue(ctx, pub))
	read := dueRowFor(t, repo, pub.AgentName)

	requeue := *pub
	require.NoError(t, repo.Enqueue(ctx, &requeue))
	require.ErrorIs(t, repo.MarkFailed(ctx, read, "gave up"), ErrA2APublicationSuperseded)

	assert.Equal(t, models.A2APublicationStatusPending, statusOf(t, pub).Status,
		"the newer enqueue is still owed a publish")
}
