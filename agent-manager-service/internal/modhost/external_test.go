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
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"

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
}

func (m *externalModule) ExternalSchema(_ func(string) string, apply bool) (modkit.ExternalSchema, error) {
	if !m.configured {
		return nil, fmt.Errorf("%w: no store address", modkit.ErrNotConfigured)
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
	ctx := context.Background()
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

	// An edited applied file is refused, by migrate and by enable.
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
