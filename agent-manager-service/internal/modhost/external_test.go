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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// fakeStore is an external store that keeps its records in memory.
type fakeStore struct {
	mu      sync.Mutex
	records map[string]string
	applied []string
	settled int
	failOn  string
	down    bool
	// reads counts Applied calls; hold, when set, runs in each with its count.
	reads int
	hold  func(n int)
}

// fakeSchema is fakeStore's schema with one binary's files.
type fakeSchema struct {
	store *fakeStore
	files fs.FS
}

func (s fakeSchema) Store() string { return "fake" }
func (s fakeSchema) Files() fs.FS  { return s.files }

func (s fakeSchema) Applied(context.Context) (map[string]string, error) {
	s.store.mu.Lock()
	s.store.reads++
	n, hold := s.store.reads, s.store.hold
	s.store.mu.Unlock()
	if hold != nil {
		hold(n)
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if s.store.down {
		return nil, errors.New("connection refused")
	}
	out := map[string]string{}
	for k, v := range s.store.records {
		out[k] = v
	}
	return out, nil
}

func (s fakeSchema) Apply(_ context.Context, version, checksum string, _ []byte) error {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if version == s.store.failOn {
		return errors.New("syntax error")
	}
	s.store.applied = append(s.store.applied, version)
	s.store.records[version] = checksum
	return nil
}

func (s fakeSchema) Settle(context.Context) error {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.store.settled++
	return nil
}

// externalModule has an external store; canApply is false on a server that
// has the store's reading login but not its migrator login.
type externalModule struct {
	testModule
	store      *fakeStore
	files      fs.FS
	configured bool
	canApply   bool
	// checkErr, when set, is the error of reading the schema to check it.
	checkErr error
}

func (m *externalModule) ExternalSchema(_ func(string) string, apply bool) (modkit.ExternalSchema, error) {
	if !m.configured {
		return nil, fmt.Errorf("%w: no store address", modkit.ErrNotConfigured)
	}
	if !apply && m.checkErr != nil {
		return nil, m.checkErr
	}
	if apply && !m.canApply {
		return nil, fmt.Errorf("%w: no migrator login", modkit.ErrNotConfigured)
	}
	return fakeSchema{store: m.store, files: m.files}, nil
}

func externalFiles(names ...string) fs.FS {
	out := fstest.MapFS{}
	for _, n := range names {
		out[n] = &fstest.MapFile{Data: []byte("create " + n)}
	}
	return out
}

func newExternal(store *fakeStore, files fs.FS, configured, canApply bool) *externalModule {
	return &externalModule{
		testModule: testModule{manifest: manifest("t-ext", nil, nil)},
		store:      store, files: files, configured: configured, canApply: canApply,
	}
}

func TestExternalSchema(t *testing.T) {
	// Cancelled before the database closes, so the booted host's goroutines stop first.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := databases(t)[0]
	g := d.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	store := &fakeStore{records: map[string]string{}}
	files := externalFiles("001_spans.sql", "002_index.sql")
	host := func(m *externalModule) *modhost.Host { return modhost.NewWith(g, d.driver, quiet, []modkit.Module{m}) }

	// The server migrates on start with only its reading login: nothing is
	// applied, and installing is refused until the schema is there.
	server := host(newExternal(store, files, true, false))
	res, err := server.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, &modhost.ExternalResult{Skipped: "modkit: not configured: no migrator login"}, res[0].External)
	assert.Empty(t, store.applied)
	require.NoError(t, server.Boot(ctx))
	_, err = server.Install(ctx, "t-ext", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 fake schema file(s) not applied")
	assert.Contains(t, err.Error(), "amp migrate")

	// amp migrate, with the migrator login, applies both files and settles.
	migrator := host(newExternal(store, files, true, true))
	res, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, &modhost.ExternalResult{Store: "fake", Applied: 2, Total: 2}, res[0].External)
	assert.Equal(t, []string{"001_spans", "002_index"}, store.applied)
	assert.Equal(t, 1, store.settled)

	// Running it again applies nothing, but settles again.
	res, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, res[0].External.Applied)
	assert.Len(t, store.applied, 2)
	assert.Equal(t, 2, store.settled)

	// Now the module installs.
	_, err = server.Install(ctx, "t-ext", false)
	require.NoError(t, err)

	// An edited applied file is refused by migrate.
	edited := fstest.MapFS{
		"001_spans.sql": &fstest.MapFile{Data: []byte("create 001_spans.sql, edited")},
		"002_index.sql": &fstest.MapFile{Data: []byte("create 002_index.sql")},
	}
	_, err = host(newExternal(store, edited, true, true)).Migrate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module t-ext fake schema 001_spans changed after it was applied")
	assert.Len(t, store.applied, 2)

	// So is a store that holds a file this binary does not have.
	_, err = host(newExternal(store, externalFiles("001_spans.sql"), true, true)).Migrate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer than this build")

	// And a new file that sorts before an applied one.
	_, err = host(newExternal(store, externalFiles("001_spans.sql", "002_index.sql", "001_early.sql"), true, true)).Migrate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sorts before the applied migration")

	// And two files of one version.
	_, err = host(newExternal(store, externalFiles("001_spans.sql", "001_spans.txt", "002_index.sql"), true, true)).Migrate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than one file is version 001_spans")
}

func TestExternalSchemaFailureIsRetried(t *testing.T) {
	ctx := context.Background()
	d := databases(t)[0]
	g := d.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	store := &fakeStore{records: map[string]string{}, failOn: "002_index"}
	m := newExternal(store, externalFiles("001_spans.sql", "002_index.sql"), true, true)
	h := modhost.NewWith(g, d.driver, quiet, []modkit.Module{m})

	_, err := h.Migrate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module t-ext fake schema 002_index: syntax error")
	assert.Equal(t, map[string]string{"001_spans": store.records["001_spans"]}, store.records, "only the file that applied is recorded")
	assert.Zero(t, store.settled)

	store.failOn = ""
	res, err := h.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res[0].External.Applied)
}

func TestExternalSchemaNotConfigured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &fakeStore{records: map[string]string{}}
	m := newExternal(store, externalFiles("001_spans.sql"), false, false)
	h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{m})

	res, err := h.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, &modhost.ExternalResult{Skipped: "modkit: not configured: no store address"}, res[0].External)
	// With no store there is nothing to wait for: the module installs.
	_, err = h.Install(ctx, "t-ext", false)
	require.NoError(t, err)
	assert.Empty(t, store.applied)
}

func TestEnableRefusesAnUnreachableStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &fakeStore{records: map[string]string{}}
	m := newExternal(store, externalFiles("001_spans.sql"), true, true)
	h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{m})
	_, err := h.Install(ctx, "t-ext", false)
	require.NoError(t, err)
	require.NoError(t, h.Disable(ctx, "t-ext"))

	store.down = true
	err = h.Enable(ctx, "t-ext")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read t-ext's fake schema: connection refused")

	store.down = false
	require.NoError(t, h.Enable(ctx, "t-ext"))
}

func TestModulesWithoutAnExternalStoreReportNone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	plain := &testModule{manifest: manifest("t-plain", nil, nil)}
	h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{plain})
	res, err := h.Migrate(ctx)
	require.NoError(t, err)
	assert.Equal(t, []modhost.MigrationResult{{Module: "t-plain"}}, res)
}

// blockFirstRead makes store's first Applied wait for release, after closing
// entered.
func blockFirstRead(store *fakeStore) (entered, release chan struct{}) {
	entered, release = make(chan struct{}), make(chan struct{})
	store.hold = func(n int) {
		if n == 1 {
			close(entered)
			<-release
		}
	}
	return entered, release
}

// While amp migrate waits on the store, the core database takes writes from
// another connection: the migration holds no transaction.
func TestExternalMigrateHoldsNoCoreTransaction(t *testing.T) {
	ctx := context.Background()
	d := databases(t)[0]
	g := d.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	store := &fakeStore{records: map[string]string{}}
	entered, release := blockFirstRead(store)
	h := modhost.NewWith(g, d.driver, quiet, []modkit.Module{newExternal(store, externalFiles("001_spans.sql"), true, true)})

	done := make(chan error, 1)
	go func() {
		_, err := h.Migrate(ctx)
		done <- err
	}()
	<-entered
	other := d.open(t)
	start := time.Now()
	err := other.Exec(`UPDATE module_generation SET value = value WHERE id = 1`).Error
	close(release)
	require.NoError(t, err, "a write while the store is read")
	assert.Less(t, time.Since(start), 2*time.Second, "the write waited on a lock")
	require.NoError(t, <-done)
	assert.Equal(t, []string{"001_spans"}, store.applied)
}

// A second amp migrate waits for the first's lease, then runs.
func TestExternalMigrateWaitsForTheLease(t *testing.T) {
	ctx := context.Background()
	d := databases(t)[0]
	g := d.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	store := &fakeStore{records: map[string]string{}}
	entered, release := blockFirstRead(store)
	files := externalFiles("001_spans.sql")
	migrate := func(done chan<- error) {
		h := modhost.NewWith(g, d.driver, quiet, []modkit.Module{newExternal(store, files, true, true)})
		_, err := h.Migrate(ctx)
		done <- err
	}

	first, second := make(chan error, 1), make(chan error, 1)
	go migrate(first)
	<-entered
	go migrate(second)
	time.Sleep(1500 * time.Millisecond)
	store.mu.Lock()
	reads := store.reads
	store.mu.Unlock()
	assert.Equal(t, 1, reads, "the second migrate read the store while the first held the lease")

	close(release)
	require.NoError(t, <-first)
	require.NoError(t, <-second)
	assert.Equal(t, 2, store.reads)
	assert.Equal(t, []string{"001_spans"}, store.applied, "applied once")
	assert.Equal(t, 2, store.settled)
}

// installLog installs mod on a fresh host and returns what the host logged.
func installLog(t *testing.T, mod modkit.Module, name string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := databases(t)[0]
	g := d.open(t)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	var buf bytes.Buffer
	h := modhost.NewWith(g, d.driver, slog.New(slog.NewTextHandler(&buf, nil)), []modkit.Module{mod})
	_, err := h.Migrate(ctx)
	require.NoError(t, err)
	require.NoError(t, h.Boot(ctx))
	_, err = h.Install(ctx, name, false)
	require.NoError(t, err)
	return buf.String()
}

// Installing a module whose external store is not configured warns that the
// store was not checked; a module with no external store installs quietly.
func TestInstallWarnsOfAnUncheckedStore(t *testing.T) {
	logged := installLog(t, newExternal(&fakeStore{records: map[string]string{}}, externalFiles("001_spans.sql"), false, false), "t-ext")
	assert.Contains(t, logged, `level=WARN msg="modhost:external store not checked" module=t-ext reason="modkit: not configured: no store address"`)

	logged = installLog(t, &testModule{manifest: manifest("t-plain", nil, nil)}, "t-plain")
	assert.NotContains(t, logged, "external store not checked")
}

// An active module whose store cannot be read is left out when the host
// loads, and comes back once the store can be read.
func TestReloadLeavesOutAnUnreadableStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &fakeStore{records: map[string]string{}}
	setDown := func(down bool) {
		store.mu.Lock()
		store.down = down
		store.mu.Unlock()
	}
	m := newExternal(store, externalFiles("001_spans.sql"), true, true)
	// The handler fails while the store is down, as a span delete would.
	m.activate = func(rt modkit.Runtime) error {
		rt.Subscribe(modkit.EventAgentDeleted, func(context.Context, modkit.Event) error {
			store.mu.Lock()
			defer store.mu.Unlock()
			if store.down {
				return errors.New("connection refused")
			}
			return nil
		})
		return nil
	}
	d := databases(t)[0]
	h, g := replica(t, ctx, d, []modkit.Module{m})
	_, err := h.Install(ctx, "t-ext", false)
	require.NoError(t, err)
	assert.Contains(t, h.Active(), "t-ext")

	setDown(true)
	require.NoError(t, h.Reload(ctx))
	assert.Contains(t, h.Active(), "t-ext", "a loaded module stays loaded while its store is down")
	org := newOrg(t, g)
	require.NoError(t, h.Begin(ctx, modkit.ResourceAgent).Emit(ctx, nil,
		modkit.Event{Type: modkit.EventAgentDeleted, OrgID: org, Subject: "a1", Data: []byte(`{}`)}))
	assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM module_event_deliveries WHERE module = 't-ext'`))

	// A replica that boots while the store is down leaves the module out and retries it.
	h2 := modhost.NewWith(g, d.driver, quiet, []modkit.Module{m})
	h2.PollInterval = 20 * time.Millisecond
	require.NoError(t, h2.Boot(ctx))
	assert.NotContains(t, h2.Active(), "t-ext")
	// Its events still queue on that replica, and are delivered once the store is back.
	require.NoError(t, h2.Begin(ctx, modkit.ResourceAgent).Emit(ctx, nil,
		modkit.Event{Type: modkit.EventAgentDeleted, OrgID: org, Subject: "a2", Data: []byte(`{}`)}))
	held := `SELECT count(*) FROM module_event_deliveries d JOIN module_events e ON e.id = d.event_id
		WHERE d.module = 't-ext' AND e.subject = 'a2'`
	assert.EqualValues(t, 1, count(t, g, held))
	setDown(false)
	waitFor(t, "t-ext to load once its store is back", func() bool { return slices.Contains(h2.Active(), "t-ext") })
	waitFor(t, "the queued delivery to be made", func() bool { return count(t, g, held) == 0 })
}

func TestCheckExternalSchemaWaitsForPendingFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &fakeStore{records: map[string]string{}}
	d := databases(t)[0]
	h, g := replica(t, ctx, d, []modkit.Module{newExternal(store, externalFiles("001_spans.sql"), true, true)})

	// A newer binary with a file the store lacks.
	nm := newExternal(store, externalFiles("001_spans.sql", "002_more.sql"), true, true)
	next := modhost.NewWith(g, d.driver, quiet, []modkit.Module{nm})
	require.NoError(t, next.CheckExternalSchema(ctx), "a module that isn't active is not waited for")

	_, err := h.Install(ctx, "t-ext", false)
	require.NoError(t, err)
	err = next.CheckExternalSchema(ctx)
	require.ErrorIs(t, err, modhost.ErrExternalBehind)
	assert.Contains(t, err.Error(), "t-ext: 1 fake file(s)")

	nm.checkErr = errors.New("store address set but no reader login")
	err = next.CheckExternalSchema(ctx)
	require.Error(t, err)
	assert.NotErrorIs(t, err, modhost.ErrExternalBehind, "a config error is not pending files")
	nm.checkErr = nil

	store.mu.Lock()
	store.down = true
	store.mu.Unlock()
	require.NoError(t, next.CheckExternalSchema(ctx), "a store that can't be read is not waited for")
	store.mu.Lock()
	store.down = false
	store.mu.Unlock()

	_, err = next.Migrate(ctx)
	require.NoError(t, err)
	require.NoError(t, next.CheckExternalSchema(ctx))
}
