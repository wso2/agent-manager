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

package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The committed migrations must match lite.yaml, and be the only ones. On failure, run `make schema`.
func TestBaselinesAreGenerated(t *testing.T) {
	s, err := Lite()
	require.NoError(t, err)
	for _, dialect := range Dialects {
		var want []string
		for _, m := range s.Files() {
			ddl, err := s.RenderMigration(dialect, m.ID)
			require.NoError(t, err)
			path := MigrationPath(dialect, m)
			got, err := os.ReadFile(filepath.Join("..", "..", path))
			require.NoError(t, err)
			assert.Equal(t, ddl, string(got), "%s is stale: run `make schema`", path)
			want = append(want, filepath.Base(path))
		}
		files, err := filepath.Glob(filepath.Join("..", "sql", dialect, "*.sql"))
		require.NoError(t, err)
		var have []string
		for _, f := range files {
			have = append(have, filepath.Base(f))
		}
		assert.Equal(t, want, have, "%s has a migration lite.yaml does not render", dialect)
	}
}

// A table a later migration adds is left out of the baseline and rendered in that migration alone.
func TestLaterMigrationsHoldTheirTables(t *testing.T) {
	s, err := Parse([]byte(`
migrations:
  - {id: "002", name: later}
tables:
  - name: first
    columns: [{name: id, type: id}]
    primary_key: [id]
  - name: second
    migration: "002"
    columns: [{name: id, type: id}, {name: first_id, type: id}]
    primary_key: [id]
    foreign_keys: [{columns: [first_id], references: first, ref_columns: [id]}]`))
	require.NoError(t, err)
	baseline, err := s.Render(DialectSQLite)
	require.NoError(t, err)
	assert.Contains(t, baseline, "CREATE TABLE first")
	assert.NotContains(t, baseline, "CREATE TABLE second")
	later, err := s.RenderMigration(DialectSQLite, "002")
	require.NoError(t, err)
	assert.Contains(t, later, "CREATE TABLE second")
	assert.NotContains(t, later, "CREATE TABLE first")
	assert.Equal(t, "db_migrations/sql/sqlite/002_later.sql", MigrationPath(DialectSQLite, s.Files()[1]))
}

// A migration's seed adds rows to a table an earlier migration created, in that migration alone.
func TestLaterMigrationsSeedEarlierTables(t *testing.T) {
	s, err := Parse([]byte(`
migrations:
  - {id: "002", name: more_locks, seed: {locks: [{name: second}]}}
tables:
  - name: locks
    columns: [{name: name, type: text, size: 128}]
    primary_key: [name]
    seed: [{name: first}]`))
	require.NoError(t, err)
	baseline, err := s.Render(DialectSQLite)
	require.NoError(t, err)
	assert.Contains(t, baseline, "INSERT INTO locks (name) VALUES ('first')")
	assert.NotContains(t, baseline, "'second'")
	later, err := s.RenderMigration(DialectPostgres, "002")
	require.NoError(t, err)
	assert.Contains(t, later, "INSERT INTO locks (name) VALUES ('second')")
	assert.NotContains(t, later, "CREATE TABLE")
}

func TestValidateRejectsRuleBreaks(t *testing.T) {
	tests := map[string]string{
		"nullable unique key": `
tables:
  - name: t
    columns: [{name: id, type: id}, {name: code, type: text, size: 10, nullable: true}]
    primary_key: [id]
    unique: [[code]]`,
		"two cascades": `
tables:
  - name: p
    columns: [{name: id, type: id}]
    primary_key: [id]
  - name: c
    columns: [{name: id, type: id}, {name: a, type: id}, {name: b, type: id}]
    primary_key: [id]
    foreign_keys:
      - {columns: [a], references: p, ref_columns: [id], on_delete: cascade}
      - {columns: [b], references: p, ref_columns: [id], on_delete: cascade}`,
		"foreign key to a column that is not a key": `
tables:
  - name: p
    columns: [{name: id, type: id}, {name: other, type: id}]
    primary_key: [id]
  - name: c
    columns: [{name: id, type: id}, {name: p_other, type: id}]
    primary_key: [id]
    foreign_keys: [{columns: [p_other], references: p, ref_columns: [other]}]`,
		"indexed long text": `
tables:
  - name: t
    columns: [{name: id, type: id}, {name: body, type: longtext}]
    primary_key: [id]
    indexes: [[body]]`,
		"undeclared migration": `
tables:
  - name: t
    migration: "002"
    columns: [{name: id, type: id}]
    primary_key: [id]`,
		"migration ids out of order": `
migrations:
  - {id: "003", name: a}
  - {id: "002", name: b}
tables:
  - name: t
    columns: [{name: id, type: id}]
    primary_key: [id]`,
		"foreign key to a table a later migration creates": `
migrations:
  - {id: "002", name: later}
tables:
  - name: p
    migration: "002"
    columns: [{name: id, type: id}]
    primary_key: [id]
  - name: c
    columns: [{name: id, type: id}, {name: p_id, type: id}]
    primary_key: [id]
    foreign_keys: [{columns: [p_id], references: p, ref_columns: [id]}]`,
		"seed of a table the same migration creates": `
migrations:
  - {id: "002", name: later, seed: {t: [{id: x}]}}
tables:
  - name: t
    migration: "002"
    columns: [{name: id, type: text, size: 10}]
    primary_key: [id]`,
		"seed of an unknown table": `
migrations:
  - {id: "002", name: later, seed: {nosuch: [{id: x}]}}
tables:
  - name: t
    columns: [{name: id, type: text, size: 10}]
    primary_key: [id]`,
		"migration seed of an unknown column": `
migrations:
  - {id: "002", name: later, seed: {t: [{other: x}]}}
tables:
  - name: t
    columns: [{name: id, type: text, size: 10}]
    primary_key: [id]`,
		"unknown field": `
tables:
  - name: t
    columns: [{name: id, type: id, null: true}]
    primary_key: [id]`,
	}
	for name, doc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(doc))
			assert.Error(t, err)
		})
	}
}

func TestConstraintNamesStayShort(t *testing.T) {
	long := constraintName("idx", "a_rather_long_table_name", []string{"first_long_column", "second_long_column", "third"})
	assert.LessOrEqual(t, len(long), maxNameLength)
	assert.NotEqual(t, long, constraintName("idx", "a_rather_long_table_name", []string{"first_long_column", "second_long_column", "fourth"}))
	assert.Equal(t, "pk_agents", constraintName("pk", "agents", nil))
}

func TestNullableColumnsRender(t *testing.T) {
	s, err := Lite()
	require.NoError(t, err)
	ddl, err := s.Render(DialectPostgres)
	require.NoError(t, err)
	assert.Contains(t, ddl, "    description VARCHAR(1000),\n")
	assert.True(t, strings.Contains(ddl, "    name VARCHAR(63) NOT NULL,\n"))
}

// The committed module migrations must match each module's schema files. On failure, run `make schema`.
func TestModuleMigrationsAreGenerated(t *testing.T) {
	root := filepath.Join("..", "..")
	modules, err := ModuleSchemas(root)
	require.NoError(t, err)
	for _, m := range modules {
		name, err := m.Module()
		require.NoError(t, err)
		migrations, err := RenderModule(name, m.Files)
		require.NoError(t, err)
		for _, mig := range migrations {
			for _, dialect := range Dialects {
				got, err := os.ReadFile(filepath.Join(root, ModuleMigrationPath(m.Dir, dialect, mig.Version)))
				require.NoError(t, err)
				assert.Equal(t, mig.SQL[dialect], string(got), "%s %s is stale: run `make schema`", m.Dir, mig.Version)
			}
		}
	}
}

func TestRenderModule(t *testing.T) {
	file := func(name, body string) ModuleFile {
		return ModuleFile{Path: "modules/x/schema/" + name, Data: []byte(body)}
	}
	items := file("001_items.yaml", `
module: ai-gateway
tables:
  - name: mod_ai_gateway_items
    columns: [{name: id, type: id}, {name: rank, type: int}]
    primary_key: [id]
    seed: [{id: 01900000-0000-7000-8000-000000000001, rank: "3"}]`)

	t.Run("renders each file, referencing earlier ones", func(t *testing.T) {
		got, err := RenderModule("ai-gateway", []ModuleFile{file("002_tags.yaml", `
module: ai-gateway
tables:
  - name: mod_ai_gateway_tags
    columns: [{name: item_id, type: id}, {name: tag, type: text, size: 32}]
    primary_key: [item_id, tag]
    foreign_keys: [{columns: [item_id], references: mod_ai_gateway_items, ref_columns: [id], on_delete: cascade}]`), items})
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "001_items", got[0].Version)
		assert.Equal(t, "002_tags", got[1].Version)
		assert.Contains(t, got[0].SQL[DialectPostgres], "-- Code generated from modules/x/schema/001_items.yaml")
		assert.Contains(t, got[0].SQL[DialectPostgres], "VALUES ('01900000-0000-7000-8000-000000000001', 3)")
		assert.NotContains(t, got[1].SQL[DialectSQLite], "CREATE TABLE mod_ai_gateway_items", "earlier tables are not rendered again")
		assert.Contains(t, got[1].SQL[DialectSQLite], "REFERENCES mod_ai_gateway_items (id) ON DELETE CASCADE")
	})

	for name, f := range map[string]ModuleFile{
		"table without the module prefix": file("002_x.yaml", `
module: ai-gateway
tables: [{name: items, columns: [{name: id, type: id}], primary_key: [id]}]`),
		"another module's prefix": file("002_x.yaml", `
module: ai-gateway
tables: [{name: mod_other_items, columns: [{name: id, type: id}], primary_key: [id]}]`),
		"wrong module name": file("002_x.yaml", `
module: other
tables: [{name: mod_other_items, columns: [{name: id, type: id}], primary_key: [id]}]`),
		"table defined in an earlier file": file("002_x.yaml", `
module: ai-gateway
tables: [{name: mod_ai_gateway_items, columns: [{name: id, type: id}], primary_key: [id]}]`),
		"file name without a version": file("items.yaml", `
module: ai-gateway
tables: [{name: mod_ai_gateway_more, columns: [{name: id, type: id}], primary_key: [id]}]`),
		"a migrations list": file("002_x.yaml", `
module: ai-gateway
migrations: [{id: "002", name: later}]
tables: [{name: mod_ai_gateway_more, columns: [{name: id, type: id}], primary_key: [id]}]`),
		"a table assigned to a later migration": file("002_x.yaml", `
module: ai-gateway
migrations: []
tables: [{name: mod_ai_gateway_more, migration: "002", columns: [{name: id, type: id}], primary_key: [id]}]`),
		"non-integer seed for an int column": file("002_x.yaml", `
module: ai-gateway
tables: [{name: mod_ai_gateway_more, columns: [{name: n, type: int}], primary_key: [n], seed: [{n: "1; DROP TABLE agents"}]}]`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := RenderModule("ai-gateway", []ModuleFile{items, f})
			assert.Error(t, err)
		})
	}

	t.Run("core tables may not use the module prefix", func(t *testing.T) {
		_, err := Parse([]byte(`
tables: [{name: mod_sneaky, columns: [{name: id, type: id}], primary_key: [id]}]`))
		assert.ErrorContains(t, err, "reserved for module tables")
	})
}
