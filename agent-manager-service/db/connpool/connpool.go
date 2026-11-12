// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package connpool

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"
)

var WaitFunc = func(d time.Duration) {
	<-time.After(d)
}

type BackoffFunc func(failedCount int) time.Duration

type RetryParams struct {
	MaxRetries  int
	BackoffFunc BackoffFunc
}

// a wrapper to add a retry mechanism to sql.DB methods
type connPool struct {
	*sql.DB
	retryParams RetryParams
}

var (
	_ gorm.ConnPool         = &connPool{}
	_ gorm.GetDBConnector   = &connPool{}
	_ gorm.ConnPoolBeginner = &connPool{}
	_ gorm.TxCommitter      = &txPool{}
)

func New(db *sql.DB, retryParams RetryParams) gorm.ConnPool {
	if retryParams.BackoffFunc == nil {
		slog.Info("setting default backoff function", "log_type", "connPool")
		retryParams.BackoffFunc = func(failedCount int) time.Duration {
			return time.Millisecond * 200
		}
	}
	return &connPool{db, retryParams}
}

func (c *connPool) GetDBConn() (*sql.DB, error) {
	return c.DB, nil
}

func (c *connPool) Conn(ctx context.Context) (*sql.Conn, error) {
	slog.Debug("connPool:Conn", "log_type", "connPool")
	return c.DB.Conn(ctx)
}

// BeginTx returns the transaction wrapped, so its statements get the same argument handling as the pool's.
func (c *connPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	slog.Debug("connPool:BeginTx", "log_type", "connPool", "opts", opts)
	var tx *sql.Tx
	var err error
	retryErr := c.retry(ctx, "BeginTx", func() error {
		// `tx` contains *sql.DB, retries won't work for queries run on `tx`
		tx, err = c.DB.BeginTx(ctx, opts)
		return err
	})
	if err == nil {
		err = retryErr
	}
	if err != nil {
		return nil, err
	}
	return &txPool{Tx: tx, db: c.DB}, nil
}

func (c *connPool) PrepareContext(ctx context.Context, query string) (stmt *sql.Stmt, err error) {
	slog.Debug("connPool:PrepareContext", "log_type", "connPool", "query", query)
	retryErr := c.retry(ctx, "PrepareContext", func() error {
		// `stmt` contains *sql.DB, retries won't work for queries run on `stmt`
		stmt, err = c.DB.PrepareContext(ctx, query)
		return err
	})
	if err == nil {
		err = retryErr
	}
	return stmt, err
}

func (c *connPool) ExecContext(ctx context.Context, query string, args ...interface{}) (result sql.Result, err error) {
	slog.Debug("connPool:ExecContext", "log_type", "connPool", "query", query)
	retryErr := c.retry(ctx, "ExecContext", func() error {
		result, err = c.DB.ExecContext(ctx, query, utcArgs(args)...)
		return err
	})
	if err == nil {
		err = retryErr
	}
	return result, err
}

func (c *connPool) QueryContext(ctx context.Context, query string, args ...interface{}) (rows *sql.Rows, err error) {
	slog.Debug("connPool:QueryContext", "log_type", "connPool", "query", query)
	retryErr := c.retry(ctx, "QueryContext", func() error {
		rows, err = c.DB.QueryContext(ctx, query, utcArgs(args)...)
		return err
	})
	if err == nil {
		err = retryErr
	}
	return rows, err
}

func (c *connPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) (val *sql.Row) {
	slog.Debug("connPool:QueryRowContext", "log_type", "connPool", "query", query)
	_ = c.retry(ctx, "QueryRowContext", func() error {
		val = c.DB.QueryRowContext(ctx, query, utcArgs(args)...)
		return val.Err()
	})
	if val == nil {
		// Never run because ctx ended: the row carries ctx's error.
		val = c.DB.QueryRowContext(ctx, query, utcArgs(args)...)
	}
	return val
}

// retry runs op until it succeeds, fails for good or the retries run out. It returns ctx's error when ctx ends before
// op ran, so a caller never mistakes a skipped operation for one that succeeded with no result.
func (c *connPool) retry(ctx context.Context, fn string, op func() error) error {
	var err error
	maxAttempts := c.retryParams.MaxRetries + 1
	for attempts := 1; attempts <= maxAttempts; attempts++ {
		// Check context before attempting operation
		select {
		// Context was cancelled (timeout/user cancellation)
		case <-ctx.Done():
			slog.Warn("connPool operation canceled",
				"log_type", "connPool",
				"fn", fn,
				"context_error", ctx.Err())
			if err == nil {
				err = ctx.Err()
			}
			return err
		default:
		}

		err = op()
		if err == nil || !isRetryableError(err) {
			return err
		}

		slog.Error("connPool operation failed",
			"log_type", "connPool",
			"error", err,
			"attempt", attempts,
			"fn", fn)

		if attempts < maxAttempts {
			backoffDuration := c.retryParams.BackoffFunc(attempts)

			select {
			case <-time.After(backoffDuration):
				// Wait completed, continue to next retry
			case <-ctx.Done():
				slog.Warn("connPool operation canceled during backoff",
					"log_type", "connPool",
					"fn", fn,
					"context_error", ctx.Err())
				return err
			}
		}
	}
	return err
}

func isRetryableError(err error) bool {
	msg := err.Error()
	// read tcp 172.17.1.242:33978->172.17.64.21:1433: i/o timeout
	if strings.Contains(msg, "read tcp") && strings.Contains(msg, "i/o timeout") {
		return true
	}
	for _, s := range []string{dbIsTimeout, dbIsClosed, dbIsFailedRPC} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// https://github.com/wso2-enterprise/choreo-connect-global-adapter/pull/201/files
const (
	dbIsClosed    = "is closed"
	dbIsTimeout   = "timed out"
	dbIsFailedRPC = "failed to send RPC"
)

// txPool is a transaction begun on the pool.
type txPool struct {
	*sql.Tx
	db *sql.DB
}

func (t *txPool) GetDBConn() (*sql.DB, error) { return t.db, nil }

func (t *txPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, query, utcArgs(args)...)
}

func (t *txPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, query, utcArgs(args)...)
}

func (t *txPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.Tx.QueryRowContext(ctx, query, utcArgs(args)...)
}

// utcArgs converts time arguments to UTC. Every database stores the same instant either way, but SQLite keeps times
// as text with their offset and compares them as text, which orders correctly only when every value is in one zone.
func utcArgs(args []interface{}) []interface{} {
	out := args
	for i, arg := range args {
		var utc interface{}
		switch v := arg.(type) {
		case time.Time:
			utc = v.UTC()
		case *time.Time:
			if v == nil {
				continue
			}
			u := v.UTC()
			utc = &u
		default:
			continue
		}
		if &out[0] == &args[0] {
			out = append([]interface{}(nil), args...)
		}
		out[i] = utc
	}
	return out
}
