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
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// holdAdvisoryLock takes a transaction-scoped advisory lock on key within
// namespace and holds it until the returned release is called, exactly once.
// It is for work that runs outside the lock's transaction, such as calls to
// other services and writes through other repositories.
//
// The lock lives on a connection from pool, which callers take from
// db.GetLockDB so that held locks never use up the connections the guarded
// work needs; see GetLockDB. Postgres releases the lock if that connection's
// session ends, so a crashed holder never strands it.
//
// ctx bounds getting the connection and waiting for the lock, so a caller that
// gives up is never held by a slow or unreachable database. The transaction
// itself runs on a context that ignores ctx's cancellation, because
// database/sql rolls a transaction back as soon as its context ends, which
// would drop the lock while the caller is still inside its critical section,
// for example after the HTTP request that started it gives up.
func holdAdvisoryLock(ctx context.Context, pool *gorm.DB, namespace int, key string) (func(), error) {
	sqlDB, err := pool.DB()
	if err != nil {
		return nil, fmt.Errorf("advisory lock pool: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("get advisory lock connection: %w", err)
	}
	tx, err := conn.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("begin advisory lock transaction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1, hashtext($2))", namespace, key); err != nil {
		_ = tx.Rollback()
		_ = conn.Close()
		return nil, fmt.Errorf("take advisory lock: %w", err)
	}
	return func() {
		// Rolling back an open transaction fails only when its session is
		// already gone, for example ended by idle_in_transaction_session_timeout,
		// a failover, or a dropped connection. Postgres released the lock at that
		// point, so the work it guarded may have overlapped with another holder.
		if err := tx.Rollback(); err != nil {
			slog.Error("Advisory lock was lost before release; the work it guarded may not have been serialized",
				"lockNamespace", namespace, "lockKey", key, "error", err)
		}
		_ = conn.Close()
	}, nil
}
