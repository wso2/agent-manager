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

	"github.com/wso2/agent-manager/agent-manager-service/config"
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
	driver := config.GetConfig().Database.Driver
	if driver == config.DBDriverPostgres {
		return migrateTo(db.DB(context.Background()), latestVersion)
	}
	return migrateBaseline(db.DB(context.Background()), driver)
}

// RequireRunnable refuses to start the server on a driver whose repositories are not ported yet. Such a database can
// already be created with -migrate.
func RequireRunnable(driver string) error {
	if driver == config.DBDriverPostgres {
		return nil
	}
	return fmt.Errorf("the service cannot run on DB_DRIVER=%s yet; use %s", driver, config.DBDriverPostgres)
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
	if err := RequireRunnable(config.GetConfig().Database.Driver); err != nil {
		return err
	}
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
			return fmt.Errorf("waiting for migration %s (latest applied: %s): %w",
				targetID, latestApplied(dbConn), ctx.Err())
		case <-time.After(interval):
		}
	}
}

// isApplied checks the history table first, so the wait before the first
// migration run does not log a query error on every poll.
func isApplied(ctx context.Context, dbConn *gorm.DB, id string) (bool, error) {
	conn := dbConn.WithContext(ctx)
	exists, err := historyTableExists(conn)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	var count int64
	if err := conn.Table(migrateOptions.TableName).Where(migrateOptions.IDColumnName+" = ?", id).Count(&count).Error; err != nil {
		return false, fmt.Errorf("reading migration history: %w", err)
	}
	return count > 0, nil
}

// historyTableExists reports query errors, unlike Migrator().HasTable, which
// returns false for them and would make a database error look like a schema
// that has not been migrated yet.
func historyTableExists(conn *gorm.DB) (bool, error) {
	var exists bool
	if err := conn.Raw("SELECT to_regclass(?) IS NOT NULL", migrateOptions.TableName).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("checking for migration history: %w", err)
	}
	return exists, nil
}

// latestApplied describes the newest recorded migration for a timeout message,
// which tells a migration job running an older image apart from one that never
// ran. It uses its own short context because the caller's has already expired.
func latestApplied(dbConn *gorm.DB) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn := dbConn.WithContext(ctx)
	exists, err := historyTableExists(conn)
	if err != nil {
		return "unknown"
	}
	if !exists {
		return "none"
	}
	var latest *string
	if err := conn.Table(migrateOptions.TableName).Select("MAX(" + migrateOptions.IDColumnName + ")").Scan(&latest).Error; err != nil {
		return "unknown"
	}
	if latest == nil {
		return "none"
	}
	return *latest
}

func generateIdStr(id int32) string {
	return fmt.Sprintf("%04d", id)
}
