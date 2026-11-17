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
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
)

// testAdvisoryLockNamespace keeps these tests' keys apart from every real lock user's.
const testAdvisoryLockNamespace = 4799

const lostLockLogMessage = "Advisory lock was lost before release"

// syncBuffer is a bytes.Buffer safe for the concurrent writes a slog handler may make.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureDefaultLog routes the default slog logger into a buffer for the rest of the test.
func captureDefaultLog(t *testing.T) *syncBuffer {
	t.Helper()
	logs := &syncBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return logs
}

// cappedPool opens a pool on the test database that allows at most maxOpen
// connections, standing in for a deployment that sets DB_MAX_OPEN_COUNT.
func cappedPool(t *testing.T, maxOpen int) *gorm.DB {
	t.Helper()
	p := config.GetConfig().POSTGRESQL
	params := url.Values{}
	if p.SSLMode != "" {
		params.Set("sslmode", p.SSLMode)
	}
	dsn := (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.User, p.Password),
		Host:     fmt.Sprintf("%s:%d", p.Host, p.Port),
		Path:     "/" + p.DBName,
		RawQuery: params.Encode(),
	}).String()
	pool, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := pool.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(maxOpen)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return pool
}

// advisoryLockHolderPID returns the backend PID holding the two-key advisory lock on (namespace, key).
func advisoryLockHolderPID(t *testing.T, namespace int, key string) int {
	t.Helper()
	var pids []int
	require.NoError(t, db.GetDB().Raw(
		`SELECT pid FROM pg_locks
		 WHERE locktype = 'advisory' AND granted AND classid = ? AND objid = hashtext(?)::oid AND objsubid = 2`,
		namespace, key,
	).Scan(&pids).Error)
	require.Len(t, pids, 1, "exactly one session must hold the lock")
	return pids[0]
}

// terminateBackend ends a Postgres session and waits until it is gone.
func terminateBackend(t *testing.T, pid int) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec("SELECT pg_terminate_backend(?)", pid).Error)
	require.Eventually(t, func() bool {
		var alive int64
		require.NoError(t, db.GetDB().Raw("SELECT count(*) FROM pg_stat_activity WHERE pid = ?", pid).Scan(&alive).Error)
		return alive == 0
	}, 5*time.Second, 20*time.Millisecond, "the terminated session must go away")
}

func TestHoldAdvisoryLock_HoldsOnLockPoolNotCallersPool(t *testing.T) {
	key := uuid.NewString()
	release, err := holdAdvisoryLock(context.Background(), db.GetLockDB(), testAdvisoryLockNamespace, key)
	require.NoError(t, err)
	defer release()

	var mainPoolPID int
	require.NoError(t, db.GetDB().Raw("SELECT pg_backend_pid()").Scan(&mainPoolPID).Error)
	assert.NotEqual(t, mainPoolPID, advisoryLockHolderPID(t, testAdvisoryLockNamespace, key),
		"the lock must be held on its own connection")
}

func TestHoldAdvisoryLock_ReleaseOfHeldLockLogsNothing(t *testing.T) {
	logs := captureDefaultLog(t)
	release, err := holdAdvisoryLock(context.Background(), db.GetLockDB(), testAdvisoryLockNamespace, uuid.NewString())
	require.NoError(t, err)

	release()

	assert.NotContains(t, logs.String(), lostLockLogMessage)
}

// TestHoldAdvisoryLock_LogsWhenLockLostBeforeRelease covers a session that
// Postgres ends while the lock is held, as idle_in_transaction_session_timeout
// or a failover would: the lock is gone, so release must say so.
func TestHoldAdvisoryLock_LogsWhenLockLostBeforeRelease(t *testing.T) {
	logs := captureDefaultLog(t)
	key := uuid.NewString()
	release, err := holdAdvisoryLock(context.Background(), db.GetLockDB(), testAdvisoryLockNamespace, key)
	require.NoError(t, err)

	terminateBackend(t, advisoryLockHolderPID(t, testAdvisoryLockNamespace, key))

	other, err := holdAdvisoryLock(context.Background(), db.GetLockDB(), testAdvisoryLockNamespace, key)
	require.NoError(t, err, "with the holder's session gone, the lock is free for another holder")
	other()

	release()
	assert.Contains(t, logs.String(), lostLockLogMessage, "release must report that the lock was lost")
	assert.Contains(t, logs.String(), key)
}

// TestHoldAdvisoryLock_HolderKeepsUsingCappedPool reproduces the review
// concern behind the lock pool: on a pool capped at one connection, a lock
// held on that pool leaves its holder no connection for its own work, while a
// lock held on the lock pool leaves the capped pool free.
func TestHoldAdvisoryLock_HolderKeepsUsingCappedPool(t *testing.T) {
	pool := cappedPool(t, 1)
	queryWithin := func(d time.Duration) error {
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		var one int
		return pool.WithContext(ctx).Raw("SELECT 1").Scan(&one).Error
	}

	starving, err := holdAdvisoryLock(context.Background(), pool, testAdvisoryLockNamespace, uuid.NewString())
	require.NoError(t, err)
	require.Error(t, queryWithin(300*time.Millisecond),
		"a lock held on the capped pool takes its only connection, so the holder's own query cannot run")
	starving()

	release, err := holdAdvisoryLock(context.Background(), db.GetLockDB(), testAdvisoryLockNamespace, uuid.NewString())
	require.NoError(t, err)
	defer release()
	require.NoError(t, queryWithin(5*time.Second), "a lock held on the lock pool must leave the capped pool free")
}

// TestHoldAdvisoryLock_GivesUpWaitingForConnectionWhenCtxDone guards that ctx
// also bounds getting a connection, not only waiting for the lock: a caller
// that gives up must not stay blocked on a pool with no free connection.
func TestHoldAdvisoryLock_GivesUpWaitingForConnectionWhenCtxDone(t *testing.T) {
	pool := cappedPool(t, 1)
	holder, err := holdAdvisoryLock(context.Background(), pool, testAdvisoryLockNamespace, uuid.NewString())
	require.NoError(t, err)
	defer holder()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		release, err := holdAdvisoryLock(ctx, pool, testAdvisoryLockNamespace, uuid.NewString())
		if err == nil {
			release()
		}
		result <- err
	}()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.DeadlineExceeded, "the caller must give up once its context ends")
	case <-time.After(5 * time.Second):
		holder() // frees the pool's only connection so the goroutine can finish
		t.Fatal("waiting for a connection must end when the caller's context does")
	}
}
