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

package db_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// The cases run against the baseline schema on every database: see dbx_sqlite_test.go and dbx_postgres_test.go.

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func seedOrg(t *testing.T, db *gorm.DB) string {
	t.Helper()
	return newID()
}

type outboxRow struct {
	ID          string `gorm:"primaryKey"`
	OrgID       string
	Body        string
	AvailableAt time.Time
}

func (outboxRow) TableName() string { return "event_outbox" }

func runInsertIfAbsentAndUpsert(t *testing.T, db *gorm.DB) {
	org := seedOrg(t, db)
	row := outboxRow{ID: newID(), OrgID: org, Body: `{"n":1}`, AvailableAt: dbx.Now()}

	inserted, err := dbx.InsertIfAbsent(db, &row)
	require.NoError(t, err)
	assert.True(t, inserted)
	dup := row
	inserted, err = dbx.InsertIfAbsent(db, &dup)
	require.NoError(t, err)
	assert.False(t, inserted)

	// Inside a transaction the duplicate must not poison it (PostgreSQL aborts a transaction on a failed statement).
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		dup := row
		dup.Body = `{"n":2}`
		inserted, err := dbx.Upsert(tx, &dup, func(tx *gorm.DB) error {
			return tx.Model(&outboxRow{}).Where("id = ?", row.ID).Update("body", dup.Body).Error
		})
		require.NoError(t, err)
		assert.False(t, inserted)
		return tx.Model(&outboxRow{}).Where("id = ?", row.ID).Update("available_at", dbx.Now()).Error
	}))
	var got outboxRow
	require.NoError(t, db.Where("id = ?", row.ID).Take(&got).Error)
	assert.JSONEq(t, `{"n":2}`, got.Body)
}

func runConcurrentClaims(t *testing.T, db *gorm.DB) {
	org := seedOrg(t, db)
	now := dbx.Now()
	const total = 40
	for i := 0; i < total; i++ {
		require.NoError(t, db.Create(&outboxRow{ID: newID(), OrgID: org, Body: "{}", AvailableAt: now.Add(-time.Second)}).Error)
	}
	// Not due yet: must never be claimed.
	require.NoError(t, db.Create(&outboxRow{ID: newID(), OrgID: org, Body: "{}", AvailableAt: now.Add(time.Hour)}).Error)

	var (
		mu    sync.Mutex
		seen  = map[string]int{}
		wg    sync.WaitGroup
		errCh = make(chan error, 5)
	)
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				got, err := dbx.ClaimRows(context.Background(), db, dbx.Claim{
					Table: "event_outbox", Key: []string{"id"}, Limit: 3, Now: now, Lease: time.Minute,
					Due: func(q *gorm.DB) *gorm.DB { return q.Where("available_at <= ?", now).Order("available_at, id") },
				})
				if err != nil {
					errCh <- err
					return
				}
				if len(got.Keys) == 0 {
					return
				}
				mu.Lock()
				for _, k := range got.Keys {
					seen[fmt.Sprint(k[0])]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	assert.Len(t, seen, total, "every due row is claimed")
	for id, n := range seen {
		assert.Equal(t, 1, n, "row %s claimed once", id)
	}

	// Leases expire: a later claim takes the rows back.
	later := now.Add(2 * time.Minute)
	got, err := dbx.ClaimRows(context.Background(), db, dbx.Claim{
		Table: "event_outbox", Key: []string{"id"}, Limit: 100, Now: later, Lease: time.Minute,
		Due: func(q *gorm.DB) *gorm.DB { return q.Where("available_at <= ?", later) },
	})
	require.NoError(t, err)
	assert.Len(t, got.Keys, total)
}

func runCompositeKeyClaim(t *testing.T, db *gorm.DB) {
	now := dbx.Now()
	// A queue keyed by two columns, as the AI Gateway's render queue is.
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS claim_pairs (
		a VARCHAR(36) NOT NULL, b VARCHAR(36) NOT NULL, next_attempt_at TIMESTAMP NOT NULL,
		claimed_by VARCHAR(64), claimed_until TIMESTAMP, PRIMARY KEY (a, b))`).Error)
	require.NoError(t, db.Exec(`DELETE FROM claim_pairs`).Error)
	group := newID()
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Exec(`INSERT INTO claim_pairs (a, b, next_attempt_at) VALUES (?, ?, ?)`, newID(), group, now).Error)
	}
	claim := dbx.Claim{
		Table: "claim_pairs", Key: []string{"a", "b"}, Limit: 2, Now: now, Lease: time.Minute,
		Due: func(q *gorm.DB) *gorm.DB { return q.Where("b = ? AND next_attempt_at <= ?", group, now) },
	}
	first, err := dbx.ClaimRows(context.Background(), db, claim)
	require.NoError(t, err)
	assert.Len(t, first.Keys, 2)
	second, err := dbx.ClaimRows(context.Background(), db, claim)
	require.NoError(t, err)
	assert.Len(t, second.Keys, 1, "only the unclaimed row is left")
}

func runLocks(t *testing.T, db *gorm.DB) {
	ctx, now := context.Background(), dbx.Now()
	ok, err := dbx.TryLock(ctx, db, "locks", "agent_identity.reconciler", "a", now, time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = dbx.TryLock(ctx, db, "locks", "agent_identity.reconciler", "b", now, time.Minute)
	require.NoError(t, err)
	assert.False(t, ok, "held by a")
	ok, err = dbx.TryLock(ctx, db, "locks", "agent_identity.reconciler", "a", now.Add(30*time.Second), time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "the owner renews")
	ok, err = dbx.TryLock(ctx, db, "locks", "agent_identity.reconciler", "b", now.Add(5*time.Minute), time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "an expired lease is taken over")
	require.NoError(t, dbx.Unlock(ctx, db, "locks", "agent_identity.reconciler", "b"))
	ok, err = dbx.TryLock(ctx, db, "locks", "agent_identity.reconciler", "c", now, time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "free after unlock")

	_, err = dbx.TryLock(ctx, db, "locks", "no.such.lock", "a", now, time.Minute)
	assert.ErrorIs(t, err, dbx.ErrLockNotSeeded)
}

func runLockRowAndKeyset(t *testing.T, db *gorm.DB) {
	org := seedOrg(t, db)
	now := dbx.Now()
	locked := newID()
	require.NoError(t, db.Exec("INSERT INTO projects (id, org_id, name, display_name, created_at, updated_at) VALUES (?, ?, 'locked', 'P', ?, ?)",
		locked, seedOrg(t, db), now, now).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return dbx.LockRow(tx, "projects", "id", "id = ?", locked)
	}))
	assert.ErrorIs(t, dbx.LockRow(db, "projects", "id", "id = ?", newID()), gorm.ErrRecordNotFound)

	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, db.Exec("INSERT INTO projects (id, org_id, name, display_name, created_at, updated_at) VALUES (?, ?, ?, 'P', ?, ?)",
			newID(), org, name, now, now).Error)
	}
	var names []string
	q := dbx.After(db.Table("projects").Where("org_id = ?", org), []string{"org_id", "name"}, []any{org, "a"})
	require.NoError(t, q.Order("org_id, name").Pluck("name", &names).Error)
	assert.Equal(t, []string{"b", "c"}, names)
}
