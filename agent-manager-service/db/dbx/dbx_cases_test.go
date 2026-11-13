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

package dbx_test

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

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
)

// The cases run against the baseline schema on every database: see dbx_sqlite_test.go and dbx_postgres_test.go.

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func seedOrg(t *testing.T, db *gorm.DB) string {
	t.Helper()
	org, now := newID(), dbx.Now()
	require.NoError(t, db.Exec("INSERT INTO organizations (id, handle, display_name, created_at, updated_at) VALUES (?, ?, 'Org', ?, ?)",
		org, "org-"+org[len(org)-8:], now, now).Error)
	return org
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
	org := seedOrg(t, db)
	now := dbx.Now()
	gateway := newID()
	require.NoError(t, db.Exec(`INSERT INTO gateways (id, org_id, name, display_name, functionality_type, hosting, vhost, is_critical, is_active, properties, created_at, updated_at)
		VALUES (?, ?, 'gw', 'GW', 'both', 'managed', 'h', ?, ?, '{}', ?, ?)`, gateway, org, false, true, now, now).Error)
	for i := 0; i < 3; i++ {
		artifact := newID()
		require.NoError(t, db.Exec(`INSERT INTO artifacts (id, org_id, kind, handle, name, version, in_catalog, created_at, updated_at)
			VALUES (?, ?, 'LlmProvider', ?, ?, 'v1', ?, ?, ?)`, artifact, org, "h"+artifact, "n"+artifact, false, now, now).Error)
		require.NoError(t, db.Exec(`INSERT INTO artifact_gateway_status (artifact_id, gateway_id, org_id, status, status_desired, performed_at, next_attempt_at, attempt_count, updated_at)
			VALUES (?, ?, ?, 'PENDING', 'DEPLOYED', ?, ?, 0, ?)`, artifact, gateway, org, now, now, now).Error)
	}
	claim := dbx.Claim{
		Table: "artifact_gateway_status", Key: []string{"artifact_id", "gateway_id"}, Limit: 2, Now: now, Lease: time.Minute,
		Due: func(q *gorm.DB) *gorm.DB { return q.Where("gateway_id = ? AND next_attempt_at <= ?", gateway, now) },
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
	ok, err := dbx.TryLock(ctx, db, "gateway.drift_scan", "a", now, time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = dbx.TryLock(ctx, db, "gateway.drift_scan", "b", now, time.Minute)
	require.NoError(t, err)
	assert.False(t, ok, "held by a")
	ok, err = dbx.TryLock(ctx, db, "gateway.drift_scan", "a", now.Add(30*time.Second), time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "the owner renews")
	ok, err = dbx.TryLock(ctx, db, "gateway.drift_scan", "b", now.Add(5*time.Minute), time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "an expired lease is taken over")
	require.NoError(t, dbx.Unlock(ctx, db, "gateway.drift_scan", "b"))
	ok, err = dbx.TryLock(ctx, db, "gateway.drift_scan", "c", now, time.Minute)
	require.NoError(t, err)
	assert.True(t, ok, "free after unlock")

	_, err = dbx.TryLock(ctx, db, "no.such.lock", "a", now, time.Minute)
	assert.ErrorIs(t, err, dbx.ErrLockNotSeeded)
}

func runLockRowAndKeyset(t *testing.T, db *gorm.DB) {
	org := seedOrg(t, db)
	now := dbx.Now()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return dbx.LockRow(tx, "organizations", "id", "id = ?", org)
	}))
	assert.ErrorIs(t, dbx.LockRow(db, "organizations", "id", "id = ?", newID()), gorm.ErrRecordNotFound)

	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, db.Exec("INSERT INTO projects (id, org_id, name, display_name, created_at, updated_at) VALUES (?, ?, ?, 'P', ?, ?)",
			newID(), org, name, now, now).Error)
	}
	var names []string
	q := dbx.After(db.Table("projects").Where("org_id = ?", org), []string{"org_id", "name"}, []any{org, "a"})
	require.NoError(t, q.Order("org_id, name").Pluck("name", &names).Error)
	assert.Equal(t, []string{"b", "c"}, names)
}
