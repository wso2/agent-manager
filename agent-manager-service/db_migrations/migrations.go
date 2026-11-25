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

	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
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

// Migrate brings the configured database to this build's schema: the generated baseline, then any later SQL
// migrations for its driver.
func Migrate() error {
	return MigrateBaseline(db.DB(context.Background()), config.GetConfig().Database.Driver)
}

// WaitForLatest blocks until this build's latest migration is recorded, so a
// server never starts against an older schema while a separate migration job
// is still running. It returns ctx's error if that does not happen first.
func WaitForLatest(ctx context.Context, interval time.Duration) error {
	list, err := sqlMigrationsFor(config.GetConfig().Database.Driver)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return fmt.Errorf("no SQL migrations for %s", config.GetConfig().Database.Driver)
	}
	return waitFor(ctx, db.DB(ctx), list[len(list)-1].ID, interval)
}

// waitFor polls the migration history until version is recorded.
func waitFor(ctx context.Context, dbConn *gorm.DB, targetID string, interval time.Duration) error {
	for {
		applied, err := isApplied(ctx, dbConn, targetID)
		if err != nil {
			// The deadline can expire mid-query; report it as the timeout.
			if ctx.Err() != nil {
				return timeoutError(dbConn, targetID, ctx.Err())
			}
			return err
		}
		if applied {
			slog.Info("dbmigrations:schema is up to date", "latest", targetID)
			return nil
		}
		slog.Info("dbmigrations:waiting for migrations", "latest", targetID)
		select {
		case <-ctx.Done():
			return timeoutError(dbConn, targetID, ctx.Err())
		case <-time.After(interval):
		}
	}
}

func timeoutError(dbConn *gorm.DB, targetID string, cause error) error {
	return fmt.Errorf("waiting for migration %s (latest applied: %s): %w",
		targetID, latestApplied(dbConn), cause)
}

// HasHistory reports whether dbConn holds a migrated install: its migration
// history table exists and records at least one migration.
func HasHistory(ctx context.Context, dbConn *gorm.DB) (bool, error) {
	conn := dbConn.WithContext(ctx)
	exists, err := historyTableExists(conn)
	if err != nil || !exists {
		return false, err
	}
	var count int64
	if err := conn.Table(migrateOptions.TableName).Count(&count).Error; err != nil {
		return false, fmt.Errorf("reading migration history: %w", err)
	}
	return count > 0, nil
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

// historyTableExists asks the catalog directly: Migrator().HasTable returns false for a failed query, which would make
// a database error look like a schema that has not been migrated yet.
func historyTableExists(conn *gorm.DB) (bool, error) {
	// to_regclass resolves the name through search_path, as the history queries do.
	query := "SELECT CASE WHEN to_regclass(?) IS NULL THEN 0 ELSE 1 END"
	if conn.Name() == "sqlite" {
		query = "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?"
	}
	var count int64
	if err := conn.Raw(query, migrateOptions.TableName).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("checking for migration history: %w", err)
	}
	return count > 0, nil
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
