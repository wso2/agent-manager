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

package dbmigrations

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
)

var migrateOptions = &gormigrate.Options{
	TableName:                 "migration_history",
	IDColumnName:              "id",
	IDColumnSize:              255,
	UseTransaction:            true, // Controls whether migrations run within database transactions
	ValidateUnknownMigrations: true, // Controls validation of migrations that exist in the database but not in the code
}

type migration struct {
	ID      int32
	Migrate gormigrate.MigrateFunc
}

func Migrate() error {
	dbConn := db.DB(context.Background())

	successCount := 0
	var list []*gormigrate.Migration
	for _, m := range migrations {
		m := m
		id := generateIdStr(m.ID)
		list = append(list, &gormigrate.Migration{
			ID: id,
			Migrate: func(g *gorm.DB) error {
				slog.Info("dbmigrations:applying migration", "id", id)
				if err := m.Migrate(g); err != nil {
					return err
				}
				successCount++
				slog.Info("dbmigrations:migration applied successfully", "id", id)
				return nil
			},
		})
	}
	latestId := generateIdStr(latestVersion)
	slog.Info("dbmigrations:starting migration", "latest", latestId)
	m := gormigrate.New(dbConn, migrateOptions, list)
	if err := m.MigrateTo(latestId); err != nil {
		return err
	}
	slog.Info("dbmigrations:migration completed", "latest", latestId, "successCount", successCount)
	return nil
}

func generateIdStr(id int32) string {
	return fmt.Sprintf("%04d", id)
}
