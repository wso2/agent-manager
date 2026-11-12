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

// Package dbx holds the query patterns that replace PostgreSQL-only SQL (schema rule 10): every helper is plain SQL
// that runs unchanged on SQLite, PostgreSQL, SQL Server and Oracle.
package dbx

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/db/dberr"
)

// MaxInList is the most values one IN list may hold (Oracle's limit; SQL Server caps parameters at 2,100).
const MaxInList = 1000

// Now is the current time as every database stores it: UTC, truncated to microseconds, so a value written and read
// back compares equal everywhere.
func Now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

// InsertIfAbsent inserts value, leaving out the omit fields (e.g. associations), and reports whether it did. A
// duplicate key is not an error: it returns false. The insert runs in a savepoint, because a failed statement aborts
// the surrounding transaction on PostgreSQL. The statement is not logged: a duplicate is expected, and any other
// error is returned.
func InsertIfAbsent(db *gorm.DB, value any, omit ...string) (bool, error) {
	db = db.Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Silent)})
	err := db.Transaction(func(sp *gorm.DB) error {
		if len(omit) > 0 {
			sp = sp.Omit(omit...)
		}
		return sp.Create(value).Error
	})
	if err == nil {
		return true, nil
	}
	if dberr.IsDuplicateKey(err) {
		return false, nil
	}
	return false, err
}

// Upsert inserts value, or runs update when a row with the same key exists. It reports whether it inserted.
func Upsert(db *gorm.DB, value any, update func(db *gorm.DB) error) (bool, error) {
	inserted, err := InsertIfAbsent(db, value)
	if err != nil || inserted {
		return inserted, err
	}
	return false, update(db)
}

// LockRow locks the rows matching where for the rest of the transaction by setting column to itself, which changes no
// data, and fails with gorm.ErrRecordNotFound when no row matches. Call it first in a transaction (schema rule 10).
func LockRow(tx *gorm.DB, table, column, where string, args ...any) error {
	res := tx.Table(table).Where(where, args...).Update(column, gorm.Expr(column))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Chunks splits items into slices of at most MaxInList, for IN lists.
func Chunks[T any](items []T) [][]T {
	var out [][]T
	for len(items) > MaxInList {
		out = append(out, items[:MaxInList])
		items = items[MaxInList:]
	}
	if len(items) > 0 {
		out = append(out, items)
	}
	return out
}

// After adds a keyset condition: rows that sort after vals on cols, all ascending. Order the query by cols too.
func After(q *gorm.DB, cols []string, vals []any) *gorm.DB {
	if len(cols) == 0 || len(cols) != len(vals) {
		return q
	}
	var terms []string
	var args []any
	for i := range cols {
		var parts []string
		for j := range i {
			parts = append(parts, cols[j]+" = ?")
			args = append(args, vals[j])
		}
		parts = append(parts, cols[i]+" > ?")
		args = append(args, vals[i])
		terms = append(terms, "("+strings.Join(parts, " AND ")+")")
	}
	return q.Where("("+strings.Join(terms, " OR ")+")", args...)
}

// Claim describes one claim of due rows from a queue table that has claimed_by and claimed_until columns.
type Claim struct {
	Table string
	// Key is the table's primary key columns.
	Key []string
	// Due narrows and orders the candidates, e.g. q.Where("available_at <= ?", now).Order("available_at, id").
	Due   func(q *gorm.DB) *gorm.DB
	Limit int
	Now   time.Time
	// Lease is how long the claim holds; a row whose lease expired can be claimed again.
	Lease time.Duration
}

// Claimed is the result of a claim: the token written to claimed_by and the keys of the rows it holds.
type Claimed struct {
	Token string
	Keys  [][]any
}

// leaseFree matches rows that no claim holds right now.
const leaseFree = "(claimed_until IS NULL OR claimed_until < ?)"

// ClaimRows claims up to c.Limit due rows whose lease is free or expired. Each row goes to one caller: the update
// re-checks the lease per row, so a concurrent claim of the same row affects nothing.
func ClaimRows(ctx context.Context, db *gorm.DB, c Claim) (Claimed, error) {
	token := uuid.NewString()
	until := c.Now.Add(c.Lease)
	db = db.WithContext(ctx)

	q := db.Table(c.Table).Select(c.Key).Where(leaseFree, c.Now)
	if c.Due != nil {
		q = c.Due(q)
	}
	var rows []map[string]any
	if err := q.Limit(c.Limit).Find(&rows).Error; err != nil {
		return Claimed{}, fmt.Errorf("finding %s rows to claim: %w", c.Table, err)
	}
	if len(rows) == 0 {
		return Claimed{Token: token}, nil
	}
	claim := map[string]any{"claimed_by": token, "claimed_until": until}

	if len(c.Key) == 1 {
		ids := make([]any, len(rows))
		for i, r := range rows {
			ids[i] = r[c.Key[0]]
		}
		for _, chunk := range Chunks(ids) {
			err := db.Table(c.Table).Where(c.Key[0]+" IN ?", chunk).Where(leaseFree, c.Now).Updates(claim).Error
			if err != nil {
				return Claimed{}, fmt.Errorf("claiming %s rows: %w", c.Table, err)
			}
		}
	} else {
		for _, r := range rows {
			q := db.Table(c.Table).Where(leaseFree, c.Now)
			for _, k := range c.Key {
				q = q.Where(k+" = ?", r[k])
			}
			if err := q.Updates(claim).Error; err != nil {
				return Claimed{}, fmt.Errorf("claiming %s rows: %w", c.Table, err)
			}
		}
	}

	var held []map[string]any
	if err := db.Table(c.Table).Select(c.Key).Where("claimed_by = ?", token).Find(&held).Error; err != nil {
		return Claimed{}, fmt.Errorf("reading claimed %s rows: %w", c.Table, err)
	}
	out := Claimed{Token: token, Keys: make([][]any, len(held))}
	for i, r := range held {
		key := make([]any, len(c.Key))
		for j, k := range c.Key {
			key[j] = r[k]
		}
		out.Keys[i] = key
	}
	return out, nil
}

// ErrLockNotSeeded means a singleton lease names a locks row no migration created.
var ErrLockNotSeeded = errors.New("lock row does not exist")

// TryLock takes the singleton lease name for owner until now+ttl. It succeeds when the lease is free, expired or
// already owner's (which renews it). Rows are seeded by migrations, so this never inserts.
func TryLock(ctx context.Context, db *gorm.DB, name, owner string, now time.Time, ttl time.Duration) (bool, error) {
	db = db.WithContext(ctx)
	res := db.Table("locks").
		Where("name = ? AND (owner IS NULL OR owner = ? OR expires_at < ?)", name, owner, now).
		Updates(map[string]any{"owner": owner, "expires_at": now.Add(ttl)})
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 1 {
		return true, nil
	}
	var n int64
	if err := db.Table("locks").Where("name = ?", name).Count(&n).Error; err != nil {
		return false, err
	}
	if n == 0 {
		return false, fmt.Errorf("%w: %s", ErrLockNotSeeded, name)
	}
	return false, nil
}

// Unlock frees name if owner holds it.
func Unlock(ctx context.Context, db *gorm.DB, name, owner string) error {
	return db.WithContext(ctx).Table("locks").Where("name = ? AND owner = ?", name, owner).
		Updates(map[string]any{"owner": nil, "expires_at": nil}).Error
}

// LockRows locks every row matching where for the rest of the transaction, like LockRow, and reports how many it
// locked; none is not an error.
func LockRows(tx *gorm.DB, table, column, where string, args ...any) (int64, error) {
	res := tx.Table(table).Where(where, args...).Update(column, gorm.Expr(column))
	return res.RowsAffected, res.Error
}

// JSON is v encoded as JSON text, for a JSON column set in a map update: GORM applies a field's serializer only when
// it writes the struct, and only some drivers encode a Go struct or map on their own.
func JSON(v any) driver.Valuer { return jsonValue{v} }

type jsonValue struct{ v any }

func (j jsonValue) Value() (driver.Value, error) {
	b, err := json.Marshal(j.v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
