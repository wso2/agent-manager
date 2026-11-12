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

package dbx_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
)

// postgresBaseline is a fresh database next to the test database, holding the PostgreSQL baseline.
func postgresBaseline(t *testing.T) *gorm.DB {
	t.Helper()
	if config.GetConfig().Database.Driver != config.DBDriverPostgres {
		t.Skip("needs DB_DRIVER=postgres")
	}
	admin := db.DB(context.Background())
	name := "dbx_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, admin.Exec(`CREATE DATABASE `+name).Error)
	cfg := config.GetConfig()
	pg := cfg.POSTGRESQL
	pg.DBName = name
	scratch, err := db.Open(config.DatabaseConfig{Driver: config.DBDriverPostgres}, pg)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := scratch.DB(); err == nil {
			_ = sqlDB.Close()
		}
		require.NoError(t, admin.Exec(`DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`).Error)
	})
	require.NoError(t, dbmigrations.MigrateBaseline(scratch, config.DBDriverPostgres))
	return scratch
}

func TestPostgres(t *testing.T) {
	cases := map[string]func(*testing.T, *gorm.DB){
		"insert if absent and upsert": runInsertIfAbsentAndUpsert,
		"concurrent claims":           runConcurrentClaims,
		"composite key claim":         runCompositeKeyClaim,
		"locks":                       runLocks,
		"lock row and keyset":         runLockRowAndKeyset,
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) { run(t, postgresBaseline(t)) })
	}
}
