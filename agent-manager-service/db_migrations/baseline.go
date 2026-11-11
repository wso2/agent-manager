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
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db_migrations/schema"
)

// sqlMigrations holds one folder of numbered SQL migrations per database, starting with the generated baseline.
//
//go:embed sql
var sqlMigrations embed.FS

// sqlMigrationsFor lists a database's migrations in version order. Each file is named NNN_description.sql and its
// version is the NNN prefix; every database uses the same versions (schema rule 11).
func sqlMigrationsFor(driver string) ([]*gormigrate.Migration, error) {
	dir := path.Join("sql", driver)
	entries, err := fs.ReadDir(sqlMigrations, dir)
	if err != nil {
		return nil, fmt.Errorf("no SQL migrations for %s: %w", driver, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	list := make([]*gormigrate.Migration, 0, len(names))
	for _, name := range names {
		id, _, ok := strings.Cut(name, "_")
		if !ok || id == "" {
			return nil, fmt.Errorf("migration %s/%s is not named NNN_description.sql", dir, name)
		}
		body, err := fs.ReadFile(sqlMigrations, path.Join(dir, name))
		if err != nil {
			return nil, err
		}
		statements := splitStatements(string(body))
		list = append(list, &gormigrate.Migration{
			ID: id,
			Migrate: func(tx *gorm.DB) error {
				slog.Info("dbmigrations:applying migration", "id", id, "file", name)
				for _, stmt := range statements {
					if err := tx.Exec(stmt).Error; err != nil {
						return fmt.Errorf("%s: %w", name, err)
					}
				}
				return nil
			},
		})
	}
	return list, nil
}

// splitStatements splits a migration file on the statement separator the generator writes, dropping comment-only
// chunks. Hand-written migrations follow the same layout: each statement ends with ";" at the end of a line.
func splitStatements(body string) []string {
	var out []string
	for _, chunk := range strings.Split(body, schema.StatementSeparator) {
		var lines []string
		for _, line := range strings.Split(chunk, "\n") {
			if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "--") {
				lines = append(lines, line)
			}
		}
		if len(lines) > 0 {
			out = append(out, strings.Join(lines, "\n"))
		}
	}
	return out
}

// migrateBaseline applies a database's SQL migrations, starting from the baseline.
func migrateBaseline(dbConn *gorm.DB, driver string) error {
	list, err := sqlMigrationsFor(driver)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return fmt.Errorf("no SQL migrations for %s", driver)
	}
	latest := list[len(list)-1].ID
	slog.Info("dbmigrations:starting migration", "driver", driver, "latest", latest)
	if err := gormigrate.New(dbConn, migrateOptions, list).Migrate(); err != nil {
		return err
	}
	slog.Info("dbmigrations:migration completed", "driver", driver, "latest", latest)
	return nil
}
