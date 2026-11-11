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

package dbmigrations

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
)

// scratchDatabase creates an empty database next to the test database and drops it when the test ends.
func scratchDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	admin := db.DB(context.Background())
	name := "baseline_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, admin.Exec(`CREATE DATABASE `+name).Error)
	scratch, err := gorm.Open(postgres.Open(scratchDSN(name)), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := scratch.DB(); err == nil {
			_ = sqlDB.Close()
		}
		require.NoError(t, admin.Exec(`DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`).Error)
	})
	return scratch
}

func scratchDSN(dbName string) string {
	cfg := config.GetConfig().POSTGRESQL
	params := url.Values{}
	if cfg.SSLMode != "" {
		params.Set("sslmode", cfg.SSLMode)
	}
	if cfg.SSLRootCert != "" {
		params.Set("sslrootcert", cfg.SSLRootCert)
	}
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:     "/" + dbName,
		RawQuery: params.Encode(),
	}).String()
}

func TestPostgresBaselineAppliesAndEnforcesKeys(t *testing.T) {
	scratch := scratchDatabase(t)
	require.NoError(t, MigrateBaseline(scratch, config.DBDriverPostgres))
	require.NoError(t, MigrateBaseline(scratch, config.DBDriverPostgres), "a second run must be a no-op")

	var locks []string
	require.NoError(t, scratch.Raw("SELECT name FROM locks ORDER BY name").Scan(&locks).Error)
	require.Equal(t, []string{"agent_identity.reconciler", "gateway.drift_scan"}, locks)

	assertBaselineKeys(t, scratch)
}
