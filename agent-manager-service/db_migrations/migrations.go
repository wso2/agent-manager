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
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
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
	return migrateTo(db.DB(context.Background()), latestVersion)
}

// migrateTo applies every migration up to and including version.
func migrateTo(dbConn *gorm.DB, version int32) error {
	successCount := 0
	var list []*gormigrate.Migration
	for _, m := range migrations {
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
	targetID := generateIdStr(version)
	slog.Info("dbmigrations:starting migration", "latest", targetID)
	m := gormigrate.New(dbConn, migrateOptions, list)
	if err := m.MigrateTo(targetID); err != nil {
		return err
	}
	slog.Info("dbmigrations:migration completed", "latest", targetID, "successCount", successCount)
	return nil
}

// WaitForLatest blocks until this build's latest migration is recorded, so a
// server never starts against an older schema while a separate migration job
// is still running. It returns ctx's error if that does not happen first.
func WaitForLatest(ctx context.Context, interval time.Duration) error {
	return waitFor(ctx, db.DB(ctx), latestVersion, interval)
}

// waitFor polls the migration history until version is recorded.
func waitFor(ctx context.Context, dbConn *gorm.DB, version int32, interval time.Duration) error {
	targetID := generateIdStr(version)
	for {
		applied, err := isApplied(ctx, dbConn, targetID)
		if err != nil {
			return err
		}
		if applied {
			slog.Info("dbmigrations:schema is up to date", "latest", targetID)
			return nil
		}
		slog.Info("dbmigrations:waiting for migrations", "latest", targetID)
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for migration %s: %w", targetID, ctx.Err())
		case <-time.After(interval):
		}
	}
}

// isApplied checks the history table first, so the wait before the first
// migration run does not log a query error on every poll.
func isApplied(ctx context.Context, dbConn *gorm.DB, id string) (bool, error) {
	conn := dbConn.WithContext(ctx)
	if !conn.Migrator().HasTable(migrateOptions.TableName) {
		return false, nil
	}
	var count int64
	if err := conn.Table(migrateOptions.TableName).Where(migrateOptions.IDColumnName+" = ?", id).Count(&count).Error; err != nil {
		return false, fmt.Errorf("reading migration history: %w", err)
	}
	return count > 0, nil
}

func generateIdStr(id int32) string {
	return fmt.Sprintf("%04d", id)
}
