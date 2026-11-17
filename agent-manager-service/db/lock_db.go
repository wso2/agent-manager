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

package db

import (
	"sync"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// lockPoolMaxIdle keeps a couple of connections warm for the next lock without
// holding many open while no lock is held.
const lockPoolMaxIdle int64 = 2

var (
	lockDB     *gorm.DB
	lockDBOnce sync.Once
)

// GetLockDB returns the pool that advisory lock holders take their
// connections from, separate from GetDB's.
//
// A holder keeps its lock's connection idle inside a transaction while it does
// its work through GetDB's pool. If both came from one capped pool, enough
// concurrent holders could take every connection and then each wait forever
// for one more. This pool has no open-connection cap, so a held lock never
// costs GetDB's pool a connection, and a lock request never waits on this pool.
// Each held lock still uses one database connection, so Postgres's
// max_connections bounds how many can be held at once; past it, taking a lock
// fails with an error instead of hanging.
//
// Like GetDB, it connects on first use and exits the process if the database
// is unreachable, so app.Run calls it at startup to fail fast there instead of
// on the first lock.
func GetLockDB() *gorm.DB {
	lockDBOnce.Do(func() {
		cfg := config.GetConfig().POSTGRESQL
		cfg.DbConfigs = lockPoolConfigs(cfg.DbConfigs)
		lockDB = initDbConn(cfg)
	})
	return lockDB
}

// lockPoolConfigs derives the lock pool's settings from the main pool's: the
// same lifetimes and logging, no open-connection cap, and few idle connections.
func lockPoolConfigs(main config.DbConfigs) config.DbConfigs {
	cfg := main
	cfg.MaxOpenCount = nil
	maxIdle := lockPoolMaxIdle
	cfg.MaxIdleCount = &maxIdle
	return cfg
}
