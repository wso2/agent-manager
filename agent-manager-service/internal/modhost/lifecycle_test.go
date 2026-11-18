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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/db_migrations/schema"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/sqlitedialect"
)

// testModule is a module that exists only in these tests.
type testModule struct {
	manifest   modkit.Manifest
	migrations fs.FS
	activate   func(rt modkit.Runtime) error
}

func (m *testModule) Manifest() modkit.Manifest { return m.manifest }
func (m *testModule) Migrations() fs.FS         { return m.migrations }
func (m *testModule) UI() fs.FS                 { return nil }

func (m *testModule) Activate(_ context.Context, rt modkit.Runtime) error {
	if m.activate != nil {
		return m.activate(rt)
	}
	return nil
}

func manifest(name string, requires, provides []string) modkit.Manifest {
	return modkit.Manifest{Name: name, Version: "1.0.0", CoreAPI: modkit.CoreAPIVersion, Requires: requires, Provides: provides}
}

// rendered renders module schema files with the same generator `make schema` uses.
func rendered(t *testing.T, module string, files map[string]string) fs.FS {
	t.Helper()
	var in []schema.ModuleFile
	for name, body := range files {
		in = append(in, schema.ModuleFile{Path: "modules/test/schema/" + name, Data: []byte(body)})
	}
	migrations, err := schema.RenderModule(module, in)
	require.NoError(t, err)
	out := fstest.MapFS{}
	for _, m := range migrations {
		for dialect, sql := range m.SQL {
			out[dialect+"/"+m.Version+".sql"] = &fstest.MapFile{Data: []byte(sql)}
		}
	}
	return out
}

// rawSQL is a hand-written migration, the same on every database.
func rawSQL(files map[string]string) fs.FS {
	out := fstest.MapFS{}
	for name, body := range files {
		for _, d := range schema.Dialects {
			out[d+"/"+name] = &fstest.MapFile{Data: []byte(body)}
		}
	}
	return out
}

const storeSchema = `
module: t-store
tables:
  - name: mod_t_store_items
    columns:
      - {name: id, type: int}
      - {name: label, type: text, size: 32}
    primary_key: [id]
    seed:
      - {id: "1", label: one}
      - {id: "2", label: two}
`

// store provides a capability backed by its own table.
type store struct{ db *gorm.DB }

func (s store) Count() (int64, error) {
	var n int64
	err := s.db.Raw(`SELECT count(*) FROM mod_t_store_items`).Scan(&n).Error
	return n, err
}

// world is the set of modules one test binary links.
type world struct {
	store, consumer, builtin *testModule
	consumed                 chan store
}

func newWorld(t *testing.T) *world {
	w := &world{consumed: make(chan store, 16)}
	w.store = &testModule{
		manifest:   manifest("t-store", nil, []string{"store/v1"}),
		migrations: rendered(t, "t-store", map[string]string{"001_items.yaml": storeSchema}),
		activate: func(rt modkit.Runtime) error {
			rt.Provide("store/v1", store{db: rt.DB()})
			return nil
		},
	}
	w.consumer = &testModule{
		manifest: manifest("t-consumer", []string{"store/v1"}, nil),
		migrations: rendered(t, "t-consumer", map[string]string{"001_notes.yaml": `
module: t-consumer
tables:
  - name: mod_t_consumer_notes
    columns: [{name: id, type: id}, {name: body, type: longtext}]
    primary_key: [id]
`}),
		activate: func(rt modkit.Runtime) error {
			s, ok := modkit.Lookup[store](rt, "store/v1")
			if !ok {
				return fmt.Errorf("store/v1 not resolvable")
			}
			w.consumed <- s
			return nil
		},
	}
	w.builtin = &testModule{manifest: modkit.Manifest{Name: "t-builtin", Version: "1.0.0", CoreAPI: modkit.CoreAPIVersion, Builtin: true}}
	return w
}

func (w *world) modules() []modkit.Module {
	return []modkit.Module{w.store, w.consumer, w.builtin}
}

type database struct {
	driver string
	open   func(t *testing.T) *gorm.DB
}

// postgresDatabase is set by the integration tier, which runs the lifecycle on
// PostgreSQL too when the test environment points at one.
var postgresDatabase func(t *testing.T) (database, bool)

// databases are the databases the lifecycle runs on: SQLite always, and
// PostgreSQL in the integration tier.
func databases(t *testing.T) []database {
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") +
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate&_time_format=sqlite"
	out := []database{{driver: "sqlite", open: func(t *testing.T) *gorm.DB {
		g, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
		require.NoError(t, err)
		return g
	}}}
	if postgresDatabase != nil {
		if d, ok := postgresDatabase(t); ok {
			out = append(out, d)
		}
	}
	return out
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// replica opens its own connection, migrates (as `amp serve --migrate` does)
// and boots a host for mods.
func replica(t *testing.T, ctx context.Context, d database, mods []modkit.Module) (*modhost.Host, *gorm.DB) {
	t.Helper()
	g := d.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	h := modhost.NewWith(g, d.driver, quiet, mods)
	h.PollInterval = 20 * time.Millisecond
	_, err := h.Migrate(ctx)
	require.NoError(t, err)
	require.NoError(t, h.Boot(ctx))
	return h, g
}

func count(t *testing.T, g *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, g.Raw(query, args...).Scan(&n).Error)
	return n
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestModuleLifecycle(t *testing.T) {
	for _, d := range databases(t) {
		t.Run(d.driver, func(t *testing.T) { lifecycle(t, d) })
	}
}

func lifecycle(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newWorld(t)
	h1, g := replica(t, ctx, d, w.modules())
	h2, _ := replica(t, ctx, d, w.modules()) // a second replica on the same database

	migrationRows := func() int64 { return count(t, g, `SELECT count(*) FROM module_migrations`) }

	t.Run("migrate prepares every compiled-in module before any install", func(t *testing.T) {
		assert.True(t, g.Migrator().HasTable("mod_t_store_items"))
		assert.True(t, g.Migrator().HasTable("mod_t_consumer_notes"))
		assert.EqualValues(t, 2, migrationRows())
		assert.EqualValues(t, 2, count(t, g, `SELECT count(*) FROM mod_t_store_items`))
		// Only the built-in module is recorded; nothing is installed.
		assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM modules`))
		assert.Equal(t, []string{"t-builtin"}, h1.Active())
	})

	t.Run("built-in modules are always active", func(t *testing.T) {
		assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM modules WHERE name = 't-builtin' AND state = 'active'`))
		for _, err := range []error{h1.Disable(ctx, "t-builtin"), h1.Uninstall(ctx, "t-builtin"), h1.Enable(ctx, "t-builtin")} {
			assert.True(t, modhost.IsResolveError(err), "got %v", err)
		}
	})

	t.Run("required capability must be present", func(t *testing.T) {
		_, err := h1.Install(ctx, "t-consumer", false)
		require.Error(t, err)
		assert.True(t, modhost.IsResolveError(err))
		assert.Contains(t, err.Error(), "store/v1")
	})

	t.Run("install with deps activates providers first, with no migrations", func(t *testing.T) {
		before := migrationRows()
		steps, err := h1.Install(ctx, "t-consumer", true)
		require.NoError(t, err)
		assert.Equal(t, []modhost.Step{{Module: "t-store", Action: "install"}, {Module: "t-consumer", Action: "install"}}, steps)
		assert.Equal(t, before, migrationRows())
		assert.Equal(t, []string{"t-builtin", "t-store", "t-consumer"}, h1.Active())
		// The consumer resolved the store's capability while activating.
		s := <-w.consumed
		n, err := s.Count()
		require.NoError(t, err)
		assert.EqualValues(t, 2, n)
	})

	t.Run("another replica picks up the change without a restart", func(t *testing.T) {
		waitFor(t, "replica 2 to reach replica 1's generation", func() bool { return h2.Generation() >= h1.Generation() })
		assert.Equal(t, h1.Active(), h2.Active())
	})

	t.Run("a change made straight in the database propagates by polling", func(t *testing.T) {
		set := func(state string) {
			t.Helper()
			require.NoError(t, g.Exec(`UPDATE modules SET state = ? WHERE name = 't-consumer'`, state).Error)
			require.NoError(t, g.Exec(`UPDATE module_generation SET value = value + 1 WHERE id = 1`).Error)
			want := count(t, g, `SELECT value FROM module_generation WHERE id = 1`)
			for _, h := range []*modhost.Host{h1, h2} {
				waitFor(t, "replicas to reload", func() bool { return h.Generation() >= want })
			}
		}
		set("disabled")
		assert.NotContains(t, h1.Active(), "t-consumer")
		assert.NotContains(t, h2.Active(), "t-consumer")
		set("active")
		assert.Contains(t, h1.Active(), "t-consumer")
		assert.Contains(t, h2.Active(), "t-consumer")
	})

	t.Run("a provider cannot go while a consumer needs it", func(t *testing.T) {
		assert.True(t, modhost.IsResolveError(h1.Disable(ctx, "t-store")))
		assert.True(t, modhost.IsResolveError(h1.Uninstall(ctx, "t-store")))
	})

	t.Run("a disabled module stops and keeps its data", func(t *testing.T) {
		require.NoError(t, g.Exec(`INSERT INTO mod_t_consumer_notes (id, body) VALUES (?, 'mine')`, uuid.NewString()).Error)
		require.NoError(t, h1.Disable(ctx, "t-consumer"))
		require.NoError(t, h1.Disable(ctx, "t-store"))
		assert.Equal(t, []string{"t-builtin"}, h1.Active())
		// Installing a disabled module with its dependencies re-enables both.
		steps, err := h1.Install(ctx, "t-consumer", true)
		require.NoError(t, err)
		assert.Equal(t, []modhost.Step{{Module: "t-store", Action: "enable"}, {Module: "t-consumer", Action: "enable"}}, steps)
		assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM mod_t_consumer_notes`))
	})

	t.Run("uninstall keeps the tables and data; reinstall needs no migration", func(t *testing.T) {
		before := migrationRows()
		require.NoError(t, h1.Uninstall(ctx, "t-consumer"))
		assert.NotContains(t, h1.Active(), "t-consumer")
		assert.True(t, modhost.IsResolveError(h1.Uninstall(ctx, "t-consumer")))
		steps, err := h1.Install(ctx, "t-consumer", false)
		require.NoError(t, err)
		assert.Equal(t, []modhost.Step{{Module: "t-consumer", Action: "install"}}, steps)
		assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM mod_t_consumer_notes`))
		assert.Equal(t, before, migrationRows())
	})

	t.Run("list describes every compiled-in module", func(t *testing.T) {
		got := map[string]modhost.ModuleInfo{}
		for _, m := range h1.List() {
			got[m.Name] = m
		}
		assert.Equal(t, modhost.StateActive, got["t-builtin"].State)
		assert.Equal(t, modhost.StateActive, got["t-store"].State)
		assert.Equal(t, []string{"t-consumer"}, got["t-store"].RequiredBy)
		assert.True(t, got["t-consumer"].Loaded)
	})
}

func TestFailingMigrationAppliesNothing(t *testing.T) {
	for _, d := range databases(t) {
		t.Run(d.driver, func(t *testing.T) {
			ctx := context.Background()
			g := d.open(t)
			require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
			broken := &testModule{
				manifest: manifest("t-broken", nil, nil),
				migrations: rawSQL(map[string]string{
					"001_ok.sql":   "CREATE TABLE mod_t_broken_ok (id INTEGER PRIMARY KEY);\n",
					"002_oops.sql": "CREATE TABLE mod_t_broken_oops (id INTEGER REFERENCES mod_t_broken_nowhere (id));\nINSERT INTO mod_t_broken_oops (id) VALUES ('x', 'y');\n",
				}),
			}
			h := modhost.NewWith(g, d.driver, quiet, []modkit.Module{broken})
			_, err := h.Migrate(ctx)
			require.Error(t, err)
			assert.False(t, g.Migrator().HasTable("mod_t_broken_ok"), "a module whose migration failed must leave nothing behind")
			assert.EqualValues(t, 0, count(t, g, `SELECT count(*) FROM module_migrations WHERE module = 't-broken'`))
			// A binary whose modules are not migrated refuses to serve.
			err = h.Boot(ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "amp migrate")
		})
	}
}

func TestUpgradeOnMigrate(t *testing.T) {
	for _, d := range databases(t) {
		t.Run(d.driver, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			v1 := map[string]string{"001_notes.sql": "CREATE TABLE mod_t_upgrade_notes (body VARCHAR(32));\nINSERT INTO mod_t_upgrade_notes (body) VALUES ('kept');\n"}
			upgrade := &testModule{manifest: manifest("t-upgrade", nil, nil), migrations: rawSQL(v1)}
			h, g := replica(t, ctx, d, []modkit.Module{upgrade})
			_, err := h.Install(ctx, "t-upgrade", false)
			require.NoError(t, err)

			// The next release: same module, one more migration.
			v2 := map[string]string{
				"001_notes.sql":  v1["001_notes.sql"],
				"002_author.sql": "ALTER TABLE mod_t_upgrade_notes ADD COLUMN author VARCHAR(32) NOT NULL DEFAULT 'unknown';\n",
			}
			next := &testModule{manifest: manifest("t-upgrade", nil, nil), migrations: rawSQL(v2)}
			next.manifest.Version = "1.1.0"
			stale := modhost.NewWith(g, d.driver, quiet, []modkit.Module{next})
			err = stale.Boot(ctx)
			require.Error(t, err, "a binary must not serve before its module migrations ran")
			assert.Contains(t, err.Error(), "t-upgrade: 1")

			res, err := stale.Migrate(ctx)
			require.NoError(t, err)
			assert.Equal(t, []modhost.MigrationResult{{Module: "t-upgrade", Applied: 1, Total: 2}}, res)
			require.NoError(t, stale.Boot(ctx))
			var version string
			require.NoError(t, g.Raw(`SELECT version FROM modules WHERE name = 't-upgrade'`).Scan(&version).Error)
			assert.Equal(t, "1.1.0", version)
			var author string
			require.NoError(t, g.Raw(`SELECT author FROM mod_t_upgrade_notes WHERE body = 'kept'`).Scan(&author).Error)
			assert.Equal(t, "unknown", author)

			// Editing an applied migration is refused.
			edited := &testModule{manifest: next.manifest, migrations: rawSQL(map[string]string{
				"001_notes.sql":  "CREATE TABLE mod_t_upgrade_notes (body VARCHAR(64));\n",
				"002_author.sql": v2["002_author.sql"],
			})}
			err = modhost.NewWith(g, d.driver, quiet, []modkit.Module{edited}).CheckSchema(ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "changed after it was applied")

			// So is an applied migration this binary does not know: the database is newer.
			older := &testModule{manifest: upgrade.manifest, migrations: rawSQL(v1)}
			err = modhost.NewWith(g, d.driver, quiet, []modkit.Module{older}).CheckSchema(ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "newer than this build")
		})
	}
}

func TestMigrationsStayInsideTheModulePrefix(t *testing.T) {
	g := databases(t)[0]
	db := g.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(db, g.driver))
	for name, sql := range map[string]string{
		"core table":      "CREATE TABLE agents_extra (id INTEGER);\n",
		"other module":    "CREATE TABLE mod_t_other_x (id INTEGER);\n",
		"index elsewhere": "CREATE TABLE mod_t_sneaky_x (id INTEGER);\nCREATE INDEX idx_x ON agents (name);\n",
		"write elsewhere": "INSERT INTO organizations (id) VALUES ('x');\n",
	} {
		t.Run(name, func(t *testing.T) {
			m := &testModule{manifest: manifest("t-sneaky", nil, nil), migrations: rawSQL(map[string]string{"001_x.sql": sql})}
			_, err := modhost.NewWith(db, g.driver, quiet, []modkit.Module{m}).Migrate(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "module tables are named mod_t_sneaky_")
		})
	}
}

func TestInstallRefusesAModuleThatCannotActivate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	early := &testModule{
		manifest: manifest("t-early", nil, []string{"x/v1"}),
		activate: func(rt modkit.Runtime) error {
			rt.Provide("x/v1", struct{}{})
			// A state-changing route must name a permission.
			rt.Handle("POST", "/things", "", func(http.ResponseWriter, *http.Request) {})
			return nil
		},
	}
	h, g := replica(t, ctx, databases(t)[0], []modkit.Module{early})
	_, err := h.Install(ctx, "t-early", false)
	require.Error(t, err)
	assert.True(t, modhost.IsResolveError(err))
	assert.Contains(t, err.Error(), "needs a permission")
	assert.Empty(t, h.Active())
	assert.EqualValues(t, 0, count(t, g, `SELECT count(*) FROM modules WHERE name = 't-early'`), "a refused install commits nothing")
}

func TestFailedActivationDropsWhatItProvided(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := &testModule{
		manifest: manifest("t-provider", nil, []string{"x/v1"}),
		activate: func(rt modkit.Runtime) error {
			rt.Provide("x/v1", struct{}{})
			return errors.New("provider broke after providing")
		},
	}
	consumer := &testModule{manifest: manifest("t-needs-x", []string{"x/v1"}, nil)}
	h, g := replica(t, ctx, databases(t)[0], []modkit.Module{provider, consumer})
	// Mark both active behind the host's back, as a restore would.
	for _, n := range []string{"t-provider", "t-needs-x"} {
		require.NoError(t, g.Exec(`INSERT INTO modules (name, version, state, created_at, updated_at) VALUES (?, '1.0.0', 'active', ?, ?)`,
			n, time.Now().UTC(), time.Now().UTC()).Error)
	}
	require.NoError(t, h.Reload(ctx))
	assert.Empty(t, h.Active(), "a consumer must not activate against a capability its failed provider registered")
}

type catchUp struct {
	testModule
	err error
}

func (c *catchUp) OnInstall(context.Context, modkit.Runtime) error { return c.err }

func TestInstallReportsAFailedCatchUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &catchUp{testModule: testModule{manifest: manifest("t-catchup", nil, nil)}, err: errors.New("backfill failed")}
	h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{m})
	steps, err := h.Install(ctx, "t-catchup", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backfill failed")
	assert.Equal(t, []modhost.Step{{Module: "t-catchup", Action: "install"}}, steps, "the module stays installed")
	assert.Equal(t, []string{"t-catchup"}, h.Active())
}

func TestFailedActivationIsRetried(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	flaky := &testModule{
		manifest: manifest("t-flaky", nil, nil),
		activate: func(modkit.Runtime) error {
			if calls.Add(1) < 3 {
				return errors.New("dependency not ready yet")
			}
			return nil
		},
	}
	h, g := replica(t, ctx, databases(t)[0], []modkit.Module{flaky})
	require.NoError(t, g.Exec(`INSERT INTO modules (name, version, state, created_at, updated_at) VALUES ('t-flaky', '1.0.0', 'active', ?, ?)`,
		time.Now().UTC(), time.Now().UTC()).Error)
	require.NoError(t, g.Exec(`UPDATE module_generation SET value = value + 1 WHERE id = 1`).Error)
	// No further generation change: the poller retries on its own.
	waitFor(t, "the flaky module to activate", func() bool { return len(h.Active()) == 1 })
}

func TestGenerationGoingBackwardsReloads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &testModule{manifest: manifest("t-plain", nil, nil)}
	h, g := replica(t, ctx, databases(t)[0], []modkit.Module{m})
	_, err := h.Install(ctx, "t-plain", false)
	require.NoError(t, err)
	// A restored database: the module row is gone and the counter is lower.
	require.NoError(t, g.Exec(`DELETE FROM modules WHERE name = 't-plain'`).Error)
	require.NoError(t, g.Exec(`UPDATE module_generation SET value = 0 WHERE id = 1`).Error)
	waitFor(t, "the replica to follow the restored database", func() bool { return len(h.Active()) == 0 })
	assert.EqualValues(t, 0, h.Generation())
}

func TestPassiveHostActivatesNothingToList(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	m := &testModule{
		manifest: modkit.Manifest{Name: "t-counted", Version: "1.0.0", CoreAPI: modkit.CoreAPIVersion, Builtin: true},
		activate: func(modkit.Runtime) error { calls.Add(1); return nil },
	}
	d := databases(t)[0]
	g := d.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	h := modhost.NewWith(g, d.driver, quiet, []modkit.Module{m})
	h.Passive = true
	_, err := h.Migrate(ctx)
	require.NoError(t, err)
	require.NoError(t, h.Reload(ctx))
	list := h.List()
	require.Len(t, list, 1)
	assert.Equal(t, modhost.StateActive, list[0].State)
	assert.EqualValues(t, 0, calls.Load())
}

func TestMigrationNamesCarryAVersion(t *testing.T) {
	d := databases(t)[0]
	g := d.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	for _, name := range []string{"1_x.sql", "x.sql", "0010_x.sql", "001-x.sql", "001_X.sql"} {
		t.Run(name, func(t *testing.T) {
			m := &testModule{manifest: manifest("t-named", nil, nil), migrations: rawSQL(map[string]string{name: "CREATE TABLE mod_t_named_x (id INTEGER);\n"})}
			_, err := modhost.NewWith(g, d.driver, quiet, []modkit.Module{m}).Migrate(context.Background())
			assert.ErrorContains(t, err, "NNN_description.sql")
		})
	}
}
