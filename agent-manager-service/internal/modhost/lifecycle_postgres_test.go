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

package modhost_test

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

func init() { postgresDatabase = scratchPostgres }

// scratchPostgres creates a throwaway database on the configured server, when
// the tier runs on PostgreSQL.
func scratchPostgres(t *testing.T) (database, bool) {
	if config.GetConfig().Database.Driver != config.DBDriverPostgres {
		return database{}, false
	}
	name := "modhost_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin := db.DB(context.Background())
	require.NoError(t, admin.Exec(`CREATE DATABASE `+name).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`).Error })
	cfg := config.GetConfig().POSTGRESQL
	params := url.Values{}
	if cfg.SSLMode != "" {
		params.Set("sslmode", cfg.SSLMode)
	}
	dsn := (&url.URL{
		Scheme: "postgres", User: url.UserPassword(cfg.User, cfg.Password),
		Host: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), Path: "/" + name, RawQuery: params.Encode(),
	}).String()
	return database{driver: config.DBDriverPostgres, open: func(t *testing.T) *gorm.DB {
		g, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
		require.NoError(t, err)
		return g
	}}, true
}
