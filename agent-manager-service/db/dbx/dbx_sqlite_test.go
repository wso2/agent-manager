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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
)

func sqliteBaseline(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := db.Open(config.DatabaseConfig{Driver: config.DBDriverSQLite, SQLitePath: filepath.Join(t.TempDir(), "amp.db")}, config.POSTGRESQL{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := gormDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, dbmigrations.MigrateBaseline(gormDB, config.DBDriverSQLite))
	return gormDB
}

func TestSQLite(t *testing.T) {
	cases := map[string]func(*testing.T, *gorm.DB){
		"insert if absent and upsert": runInsertIfAbsentAndUpsert,
		"concurrent claims":           runConcurrentClaims,
		"composite key claim":         runCompositeKeyClaim,
		"locks":                       runLocks,
		"lock row and keyset":         runLockRowAndKeyset,
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) { run(t, sqliteBaseline(t)) })
	}
}
