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

package modhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db_migrations/schema"
)

// migrationLockKey is the Postgres advisory lock that serialises module
// migrations across processes started together. SQLite needs none: its
// transactions take the write lock when they begin.
const migrationLockKey int64 = 0x616d706d6f64 // "ampmod"

const driverPostgres = "postgres"

// migration is one module migration file for the current database.
type migration struct {
	Version    string
	Statements []string
	Checksum   string
}

// tableRefs find the tables a statement creates, changes, writes to or references.
var tableRefs = func() []*regexp.Regexp {
	const ifExists = `(?:IF\s+(?:NOT\s+)?EXISTS\s+)?`
	const name = `"?([A-Za-z_][A-Za-z0-9_]*)`
	var out []*regexp.Regexp
	for _, p := range []string{
		`\b(?:CREATE|ALTER|DROP)\s+TABLE\s+` + ifExists + name,
		`\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+` + ifExists + `\S+\s+ON\s+` + name,
		`\bINSERT\s+INTO\s+` + name,
		`\bDELETE\s+FROM\s+` + name,
		`\bUPDATE\s+` + name + `\s+SET\b`,
		`\bREFERENCES\s+` + name,
	} {
		out = append(out, regexp.MustCompile(`(?i)`+p))
	}
	return out
}()

// loadMigrations reads a module's migrations for driver from fsys (one
// directory per database) in lexical order, and refuses statements that touch
// a table outside the module's prefix.
func loadMigrations(module, driver string, fsys fs.FS) ([]migration, error) {
	if fsys == nil {
		return nil, nil
	}
	entries, err := fs.ReadDir(fsys, driver)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("module %s migrations: %w", module, err)
	}
	prefix := schema.ModuleTablePrefix(module)
	var out []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := fs.ReadFile(fsys, path.Join(driver, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("module %s migrations: %w", module, err)
		}
		stmts := schema.Statements(string(body))
		for _, s := range stmts {
			for _, re := range tableRefs {
				for _, m := range re.FindAllStringSubmatch(s, -1) {
					if !strings.HasPrefix(strings.ToLower(m[1]), prefix) {
						return nil, fmt.Errorf("module %s migration %s touches table %q; module tables are named %s<table>",
							module, e.Name(), m[1], prefix)
					}
				}
			}
		}
		sum := sha256.Sum256([]byte(strings.Join(stmts, schema.StatementSeparator)))
		out = append(out, migration{
			Version:    strings.TrimSuffix(e.Name(), ".sql"),
			Statements: stmts,
			Checksum:   hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// pending returns the migrations of module not yet applied. It refuses an
// applied migration whose file has changed, a recorded migration this binary
// does not know (the database is newer than the binary), and a new migration
// that sorts before an applied one.
func pending(db *gorm.DB, module string, ms []migration) ([]migration, error) {
	var rows []struct {
		Version  string
		Checksum string
	}
	if err := db.Raw(`SELECT version, checksum FROM module_migrations WHERE module = ?`, module).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("reading module migrations: %w", err)
	}
	applied := map[string]string{}
	last := ""
	for _, r := range rows {
		applied[r.Version] = r.Checksum
		last = max(last, r.Version)
	}
	known := map[string]bool{}
	var out []migration
	for _, m := range ms {
		known[m.Version] = true
		sum, ok := applied[m.Version]
		switch {
		case ok && sum != m.Checksum:
			return nil, fmt.Errorf("module %s migration %s changed after it was applied; add a new migration instead", module, m.Version)
		case ok:
		case m.Version < last:
			return nil, fmt.Errorf("module %s migration %s sorts before the applied migration %s; give it a later number", module, m.Version, last)
		default:
			out = append(out, m)
		}
	}
	for _, v := range slices.Sorted(maps.Keys(applied)) {
		if !known[v] {
			return nil, fmt.Errorf("module %s migration %s is applied but not part of this binary; the database is newer than this build", module, v)
		}
	}
	return out, nil
}

// MigrationResult is what Migrate did for one module.
type MigrationResult struct {
	Module  string `json:"module"`
	Applied int    `json:"applied"`
	Total   int    `json:"total"`
}

// Migrate applies the pending migrations of every compiled-in module,
// installed or not, in module name order. The core's own migrations must have
// run first. It is the only code path that runs module DDL: installing,
// enabling, disabling and uninstalling afterwards are data changes only.
//
// Each module migrates in its own transaction, so a failing module leaves
// nothing of itself half-applied. It also records built-in modules as active
// and moves installed modules' recorded version to the compiled one.
func (h *Host) Migrate(ctx context.Context) ([]MigrationResult, error) {
	if err := h.checkCoreTables(); err != nil {
		return nil, err
	}
	out := make([]MigrationResult, 0, len(h.mods))
	for _, name := range h.names() {
		r, err := h.migrateModule(ctx, name)
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (h *Host) migrateModule(ctx context.Context, name string) (MigrationResult, error) {
	res := MigrationResult{Module: name}
	ms, err := loadMigrations(name, h.driver, h.mods[name].Migrations())
	if err != nil {
		return res, err
	}
	res.Total = len(ms)
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if h.driver == driverPostgres {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, migrationLockKey).Error; err != nil {
				return err
			}
		}
		todo, err := pending(tx, name, ms)
		if err != nil {
			return err
		}
		for _, m := range todo {
			h.log.Info("modhost:applying module migration", "module", name, "version", m.Version)
			for _, s := range m.Statements {
				if err := tx.Exec(s).Error; err != nil {
					return fmt.Errorf("module %s migration %s: %w", name, m.Version, err)
				}
			}
			if err := tx.Exec(`INSERT INTO module_migrations (module, version, checksum, applied_at) VALUES (?, ?, ?, ?)`,
				name, m.Version, m.Checksum, time.Now().UTC()).Error; err != nil {
				return err
			}
		}
		res.Applied = len(todo)
		return h.recordVersion(tx, name)
	})
	return res, err
}

// recordVersion records a built-in module as active, and moves an installed
// module's version to the compiled one. Either bumps the generation.
func (h *Host) recordVersion(tx *gorm.DB, name string) error {
	mf := h.manifests[name]
	now := time.Now().UTC()
	var r *gorm.DB
	if mf.Builtin {
		r = tx.Exec(`INSERT INTO modules (name, version, state, created_at, updated_at) VALUES (?, ?, 'active', ?, ?)
			ON CONFLICT (name) DO UPDATE SET version = excluded.version, state = 'active', updated_at = excluded.updated_at
			WHERE modules.version <> excluded.version OR modules.state <> 'active'`,
			name, mf.Version, now, now)
	} else {
		r = tx.Exec(`UPDATE modules SET version = ?, updated_at = ? WHERE name = ? AND version <> ? AND state <> 'uninstalled'`,
			mf.Version, now, name, mf.Version)
	}
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		return nil
	}
	h.log.Info("modhost:module version recorded", "module", name, "version", mf.Version)
	return bump(tx)
}

// SchemaBehindError means the database has not been migrated for this binary.
type SchemaBehindError struct{ Pending map[string]int }

func (e *SchemaBehindError) Error() string {
	parts := make([]string, 0, len(e.Pending))
	for _, n := range slices.Sorted(maps.Keys(e.Pending)) {
		parts = append(parts, fmt.Sprintf("%s: %d", n, e.Pending[n]))
	}
	return "the database schema is behind this binary (pending module migrations: " + strings.Join(parts, ", ") +
		"); run `amp migrate` or start the server with --migrate"
}

// CheckSchema verifies that the module tables exist and that every migration
// of every compiled-in module has been applied unchanged.
func (h *Host) CheckSchema(ctx context.Context) error {
	if err := h.checkCoreTables(); err != nil {
		return err
	}
	behind := map[string]int{}
	for _, name := range h.names() {
		ms, err := loadMigrations(name, h.driver, h.mods[name].Migrations())
		if err != nil {
			return err
		}
		p, err := pending(h.db.WithContext(ctx), name, ms)
		if err != nil {
			return err
		}
		if len(p) > 0 {
			behind[name] = len(p)
		}
	}
	if len(behind) > 0 {
		return &SchemaBehindError{Pending: behind}
	}
	return nil
}

func (h *Host) checkCoreTables() error {
	for _, t := range []string{"modules", "module_generation", "module_migrations"} {
		if !h.db.Migrator().HasTable(t) {
			return fmt.Errorf("table %s is missing: the database was created before modules existed or was never migrated; "+
				"run `amp migrate` (a local database from an earlier development build has to be recreated)", t)
		}
	}
	return nil
}

// schemaReady is the install/enable precondition: the module's migrations
// must already have been applied by Migrate.
func (h *Host) schemaReady(tx *gorm.DB, name string) error {
	ms, err := loadMigrations(name, h.driver, h.mods[name].Migrations())
	if err != nil {
		return err
	}
	p, err := pending(tx, name, ms)
	if err != nil {
		return err
	}
	if len(p) > 0 {
		return &ResolveError{
			Msg:  fmt.Sprintf("%s has %d migration(s) not applied to this database", name, len(p)),
			Hint: "run `amp migrate` with this binary first; module tables are created at migrate time, not at install",
		}
	}
	return nil
}
