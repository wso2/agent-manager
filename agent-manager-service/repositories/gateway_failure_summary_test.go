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

// insertFailureFixture creates one gateway and then forces its liveness columns
// with raw SQL.
//
// The second write is not redundant: GORM owns is_active and updated_at on
// create (CreateTx forces is_active false, and the UpdatedAt field is
// autoUpdateTime-tracked), so the only way to pin a row to "disconnected an hour
// ago" is to set the columns after the insert. Soft-deletion is applied the same
// way, since deleted_at is what the query under test is meant to skip.
func insertFailureFixture(
	t *testing.T, ouID, name string, isActive bool, updatedAt time.Time, softDeleted bool,
) uuid.UUID {
	t.Helper()
	gormDB := db.GetDB()

	id := uuid.New()
	gw := &models.Gateway{
		UUID:                     id,
		OUID:                     ouID,
		Name:                     name,
		DisplayName:              name,
		Vhost:                    name + ".test.local",
		GatewayFunctionalityType: models.GatewayRoleEgress,
		// NOT NULL with a '{}' default the ORM overrides with an explicit NULL.
		Properties: map[string]interface{}{},
	}
	require.NoError(t, gormDB.Create(gw).Error, "insert fixture %s", name)

	t.Cleanup(func() {
		// Unscoped so the soft-deleted fixture is really removed rather than
		// left behind to skew the next run's baseline.
		_ = gormDB.Unscoped().Where("uuid = ?", id).Delete(&models.Gateway{}).Error
	})

	require.NoError(t, gormDB.Exec(
		"UPDATE gateways SET is_active = ?, updated_at = ? WHERE uuid = ?",
		isActive, updatedAt, id,
	).Error, "pin liveness columns for %s", name)

	if softDeleted {
		require.NoError(t, gormDB.Exec(
			"UPDATE gateways SET deleted_at = ? WHERE uuid = ?", time.Now(), id,
		).Error,
			"soft-delete %s", name)
	}
	return id
}

// TestGatewayRepo_FailureSummaryAllOrgs covers the cross-organization sweep.
//
// Assertions are deltas around a baseline taken first, not absolute counts:
// these two queries deliberately carry no ou_id predicate, so they see every
// row the rest of the integration suite has left in the database. An absolute
// count here would pass or fail depending on test ordering, which is exactly
// the kind of green that hides a real regression.
func TestGatewayRepo_FailureSummaryAllOrgs(t *testing.T) {
	repo := NewGatewayRepo(db.GetDB())
	ctx := context.Background()

	// Well clear of every fixture's updated_at, so "stale", "fresh" and
	// "abandoned" are not decided by how long the test itself takes.
	now := time.Now()
	window := GatewayFailureWindow{
		StaleBefore:  now.Add(-5 * time.Minute),
		NotOlderThan: now.Add(-7 * 24 * time.Hour),
	}

	baseline, err := repo.CountFailureSummaryAllOrgs(ctx, window)
	require.NoError(t, err)

	const orgA, orgB = "ou-failure-summary-a", "ou-failure-summary-b"
	hourAgo := time.Now().Add(-time.Hour)

	// Counted: disconnected, and disconnected since before the cutoff.
	inactiveStale := insertFailureFixture(t, orgA, "gw-inactive-stale", false, hourAgo, false)
	// Counted, and the reason this endpoint exists: a second organization's
	// failure has to show up in the same sweep.
	otherOrgStale := insertFailureFixture(t, orgB, "gw-other-org-stale", false, time.Now().Add(-2*time.Hour), false)
	// Not counted: disconnected, but not for long enough yet.
	insertFailureFixture(t, orgA, "gw-inactive-fresh", false, time.Now().Add(-10*time.Second), false)
	// Not counted: an old updated_at on a connected gateway is a long-lived
	// healthy connection, not a failure.
	insertFailureFixture(t, orgA, "gw-active-stale", true, hourAgo, false)
	// Counted in neither total nor failed: raw SQL bypasses GORM's soft-delete
	// scope, so deleted_at IS NULL has to be spelled out in the query — this
	// fixture is what proves it was.
	insertFailureFixture(t, orgA, "gw-soft-deleted-stale", false, hourAgo, true)
	// ABANDONED, not failed: down for eight days, past the window's far edge.
	// In total, out of the denominator.
	insertFailureFixture(t, orgA, "gw-abandoned", false, time.Now().Add(-8*24*time.Hour), false)
	// NOT abandoned despite an equally ancient updated_at, because it is still
	// connected. This is the fixture that catches an age-only abandoned
	// predicate: nothing writes updated_at while a connection holds, so a
	// gateway up for eight days straight looks exactly as old as a dead one.
	insertFailureFixture(t, orgB, "gw-long-lived-healthy", true, time.Now().Add(-8*24*time.Hour), false)

	got, err := repo.CountFailureSummaryAllOrgs(ctx, window)
	require.NoError(t, err)

	assert.Equal(t, baseline.Total+6, got.Total,
		"six live fixtures were inserted; the soft-deleted one must not be in total, "+
			"but the abandoned one must — it is excluded from the denominator, not from the fleet")
	assert.Equal(t, baseline.Failed+2, got.Failed,
		"only the two inactive gateways inside the window are failures")
	assert.Equal(t, baseline.Abandoned+1, got.Abandoned,
		"the gateway down for eight days is abandoned, not failed")

	// The detail list must agree with the count on which rows are failing.
	failed, err := repo.ListFailedGatewaysAllOrgs(ctx, window, 1000)
	require.NoError(t, err)

	byID := make(map[uuid.UUID]*models.Gateway, len(failed))
	byName := make(map[string]bool, len(failed))
	for _, gw := range failed {
		byID[gw.UUID] = gw
		byName[gw.Name] = true
	}

	require.Contains(t, byID, inactiveStale, "the inactive-and-stale gateway must be listed")
	require.Contains(t, byID, otherOrgStale, "a second organization's failure must be listed")
	assert.Equal(t, orgB, byID[otherOrgStale].OUID,
		"each row must carry its own organization, since the caller is cross-org")

	assert.False(t, byName["gw-inactive-fresh"], "a recently disconnected gateway is not yet failed")
	assert.False(t, byName["gw-active-stale"], "a connected gateway is not failed")
	assert.False(t, byName["gw-soft-deleted-stale"], "a soft-deleted gateway must be excluded")
	assert.False(t, byName["gw-abandoned"], "a gateway down for eight days is abandoned, not failed")
	assert.False(t, byName["gw-long-lived-healthy"], "a connected gateway is never failed, however old its row")
}

// TestGatewayRepo_ListFailedGatewaysAllOrgs_Limit pins the cap and the ordering
// it depends on. Oldest failure first is what makes a truncated list useful: it
// keeps the longest-running failures rather than an arbitrary page.
func TestGatewayRepo_ListFailedGatewaysAllOrgs_Limit(t *testing.T) {
	repo := NewGatewayRepo(db.GetDB())
	ctx := context.Background()
	now := time.Now()
	window := GatewayFailureWindow{
		StaleBefore:  now.Add(-5 * time.Minute),
		NotOlderThan: now.Add(-7 * 24 * time.Hour),
	}

	const org = "ou-failure-summary-limit"
	oldest := insertFailureFixture(t, org, "gw-limit-oldest", false, time.Now().Add(-6*24*time.Hour), false)
	insertFailureFixture(t, org, "gw-limit-newer", false, time.Now().Add(-30*time.Minute), false)

	// A limit of 1 over the whole table must return the single oldest failure
	// anywhere, which is this fixture unless another test left something older
	// still — hence the ordering assertion below rather than an identity one.
	one, err := repo.ListFailedGatewaysAllOrgs(ctx, window, 1)
	require.NoError(t, err)
	require.Len(t, one, 1, "limit must be honoured")

	all, err := repo.ListFailedGatewaysAllOrgs(ctx, window, 1000)
	require.NoError(t, err)
	require.NotEmpty(t, all)
	assert.Equal(t, all[0].UUID, one[0].UUID,
		"the capped list must be the head of the full list, not an arbitrary page")
	for i := 1; i < len(all); i++ {
		assert.False(t, all[i].UpdatedAt.Before(all[i-1].UpdatedAt),
			"results must be ordered oldest failure first")
	}

	var oldestSeen bool
	for _, gw := range all {
		if gw.UUID == oldest {
			oldestSeen = true
		}
	}
	assert.True(t, oldestSeen, "the six-day-old failure must appear in the unbounded list")

	// A non-positive limit asks for nothing and must return nothing, rather
	// than falling through to an unbounded scan.
	none, err := repo.ListFailedGatewaysAllOrgs(ctx, window, 0)
	require.NoError(t, err)
	assert.Empty(t, none)
}
