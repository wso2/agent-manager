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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// configured is replica with the host's settings changed before it boots.
func configured(t *testing.T, ctx context.Context, d database, mods []modkit.Module, set func(*modhost.Host)) *modhost.Host {
	t.Helper()
	g := d.open(t)
	closeAtCleanup(t, g)
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	h := modhost.NewWith(g, d.driver, quiet, mods)
	h.PollInterval = 20 * time.Millisecond
	set(h)
	_, err := h.Migrate(ctx)
	require.NoError(t, err)
	require.NoError(t, h.Boot(ctx))
	return h
}

// booted is replica, with its database closed before the test's temporary
// directory is removed, so the host's poller cannot write to it meanwhile.
func booted(t *testing.T, ctx context.Context, d database, mods []modkit.Module) (*modhost.Host, *gorm.DB) {
	t.Helper()
	h, g := replica(t, ctx, d, mods)
	closeAtCleanup(t, g)
	return h, g
}

func closeAtCleanup(t *testing.T, g *gorm.DB) {
	t.Helper()
	t.Cleanup(func() {
		if sqlDB, err := g.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

// counter is a worker that counts its runs and how many copies are running.
type counter struct{ runs, running atomic.Int32 }

func (c *counter) work(ctx context.Context) error {
	c.runs.Add(1)
	c.running.Add(1)
	defer c.running.Add(-1)
	<-ctx.Done()
	return nil
}

func TestWorkersFollowTheModule(t *testing.T) { eachDatabase(t, workersFollowTheModule) }

func workersFollowTheModule(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c counter
	var flaky atomic.Int32
	worker := &testModule{manifest: manifest("t-work", nil, nil), activate: func(rt modkit.Runtime) error {
		rt.Go("count", c.work)
		rt.Go("flaky", func(context.Context) error {
			if flaky.Add(1) < 3 {
				return errors.New("failing on purpose")
			}
			panic("and once more")
		})
		return nil
	}}
	other := &testModule{manifest: manifest("t-other", nil, nil)}
	h := configured(t, ctx, d, []modkit.Module{worker, other}, func(h *modhost.Host) { h.WorkerRetryBase = 10 * time.Millisecond })
	defer h.StopWorkers()

	assert.Zero(t, c.runs.Load(), "an uninstalled module runs no workers")
	installAll(t, h, "t-work")
	waitFor(t, "the worker to start", func() bool { return c.running.Load() == 1 })
	waitFor(t, "the failing worker to be restarted", func() bool { return flaky.Load() >= 4 })

	installAll(t, h, "t-other")
	time.Sleep(50 * time.Millisecond)
	assert.EqualValues(t, 1, c.runs.Load(), "another module's change keeps the running worker")
	assert.EqualValues(t, 1, c.running.Load())

	require.NoError(t, h.Disable(ctx, "t-work"))
	assert.Zero(t, c.running.Load(), "disabling the module stops its worker before Disable returns")

	require.NoError(t, h.Enable(ctx, "t-work"))
	waitFor(t, "the worker to start again", func() bool { return c.running.Load() == 1 })
	assert.EqualValues(t, 2, c.runs.Load())

	h.StopWorkers()
	assert.Zero(t, c.running.Load(), "stopping the server stops the workers")
}

func TestWorkerThatIgnoresCancelIsAbandoned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	defer close(release)
	m := &testModule{manifest: manifest("t-stuck", nil, nil), activate: func(rt modkit.Runtime) error {
		rt.Go("stuck", func(context.Context) error {
			<-release
			return nil
		})
		return nil
	}}
	h := configured(t, ctx, databases(t)[0], []modkit.Module{m}, func(h *modhost.Host) { h.WorkerDrainTimeout = 50 * time.Millisecond })
	installAll(t, h, "t-stuck")
	start := time.Now()
	require.NoError(t, h.Disable(ctx, "t-stuck"))
	assert.Less(t, time.Since(start), 2*time.Second, "a stuck worker holds a disable no longer than the drain timeout")
}

func TestPassiveHostRunsNoWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c counter
	m := &testModule{manifest: manifest("t-work", nil, nil), activate: func(rt modkit.Runtime) error {
		rt.Go("count", c.work)
		return nil
	}}
	p := configured(t, ctx, databases(t)[0], []modkit.Module{m}, func(h *modhost.Host) { h.Passive = true })
	installAll(t, p, "t-work")
	time.Sleep(50 * time.Millisecond)
	assert.Zero(t, c.runs.Load(), "a CLI command that installs a module does not run its workers")
}

func toolModule(name, tool string) *testModule {
	mf := manifest(name, nil, nil)
	mf.Permissions = []modkit.Permission{{Name: "use"}}
	return &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
		rt.MCPTool(modkit.MCPTool{Name: tool, Description: "A tool", ReadOnly: true}, "use",
			func(context.Context, json.RawMessage) (any, error) { return "ok", nil })
		return nil
	}}
}

func TestMCPToolsFollowTheModule(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, _ := booted(t, ctx, databases(t)[0], []modkit.Module{toolModule("t-tool", "t_tool_run"), toolModule("t-tool2", "t_tool_run")})
	assert.Empty(t, h.MCPTools())
	installAll(t, h, "t-tool")
	tools := h.MCPTools()
	require.Len(t, tools, 1)
	assert.Equal(t, "t_tool_run", tools[0].Tool.Name)
	assert.EqualValues(t, "t-tool:use", tools[0].Permission)
	assert.Equal(t, "t-tool:use", tools[0].Action, "the permission names the audit action")

	// Modules activate in a fixed order; a clash refuses the later one.
	_, err := h.Install(ctx, "t-tool2", false)
	require.Error(t, err, "a second module cannot add a tool of the same name")

	require.NoError(t, h.Disable(ctx, "t-tool"))
	assert.Empty(t, h.MCPTools(), "a disabled module's tools are gone")
}

func TestProjectFinalizeCanRefuse(t *testing.T) { eachDatabase(t, projectFinalizeCanRefuse) }

func projectFinalizeCanRefuse(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var refuse atomic.Bool
	m := &testModule{manifest: manifest("t-proj", nil, nil), activate: func(rt modkit.Runtime) error {
		rt.Hook(modkit.ResourceProject, modkit.Finalize, func(_ context.Context, req modkit.HookRequest) error {
			// A write the refusal must undo.
			if err := req.Tx.Exec(`UPDATE module_generation SET value = value WHERE id = 1`).Error; err != nil {
				return err
			}
			if refuse.Load() {
				return &modkit.Error{Status: http.StatusConflict, Message: "still in use"}
			}
			return nil
		})
		return nil
	}}
	h, g := booted(t, ctx, d, []modkit.Module{m})
	installAll(t, h, "t-proj")
	req := modkit.HookRequest{Action: modkit.ActionDelete, ID: uuid.NewString()}

	refuse.Store(true)
	err := inTx(g, func(tx *gorm.DB) error {
		return h.Begin(ctx, modkit.ResourceProject).Run(ctx, tx, modkit.Finalize, req, nil)
	})
	var refused *modkit.Error
	require.ErrorAs(t, err, &refused, "a project finalize hook's error rolls the delete back")
	assert.Equal(t, http.StatusConflict, refused.Status)

	refuse.Store(false)
	require.NoError(t, inTx(g, func(tx *gorm.DB) error {
		return h.Begin(ctx, modkit.ResourceProject).Run(ctx, tx, modkit.Finalize, req, nil)
	}))
}

// secretsModule stores secrets through rt.Secrets() from its exported fields.
type secretsModule struct {
	*testModule
	rt modkit.Runtime
}

func newSecretsModule(name string) *secretsModule {
	s := &secretsModule{}
	s.testModule = &testModule{manifest: manifest(name, nil, nil), activate: func(rt modkit.Runtime) error {
		s.rt = rt
		return nil
	}}
	return s
}

func TestSecretsStayInTheModule(t *testing.T) { eachDatabase(t, secretsStayInTheModule) }

func secretsStayInTheModule(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := newSecretsModule("t-sa"), newSecretsModule("t-sb")
	h, g := booted(t, ctx, d, []modkit.Module{a, b})
	key := make([]byte, 32)
	h.Attach(modhost.Deps{Secrets: repositories.NewModuleSecretStore(g, key)})
	installAll(t, h, "t-sa", "t-sb")

	org := uuid.NewString()
	now := dbx.Now()
	require.NoError(t, g.Exec(`INSERT INTO organizations (id, handle, display_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		org, "o-"+org[:8], "Org", now, now).Error)

	sa, sb := a.rt.Secrets(), b.rt.Secrets()
	require.NoError(t, sa.Put(ctx, org, "providers/p1/api-key", "s3cret"))
	require.NoError(t, sa.Put(ctx, org, "providers/p1/api-key", "s3cret-2"), "a second Put replaces the value")
	got, err := sa.Get(ctx, org, "providers/p1/api-key")
	require.NoError(t, err)
	assert.Equal(t, "s3cret-2", got)

	_, err = sb.Get(ctx, org, "providers/p1/api-key")
	require.ErrorIs(t, err, modkit.ErrNotFound, "another module cannot read the secret at the same path")
	require.NoError(t, sb.Delete(ctx, org, "providers/p1/api-key"))
	_, err = sa.Get(ctx, org, "providers/p1/api-key")
	require.NoError(t, err, "nor delete it")

	assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM secrets WHERE owner_kind = ? AND org_id = ?`, "mod:t-sa", org))
	assert.Zero(t, count(t, g, `SELECT count(*) FROM secrets WHERE org_id = ? AND ciphertext = ?`, org, []byte("s3cret-2")),
		"values are stored encrypted")

	_, err = sa.Get(modkit.WithOrgID(ctx, uuid.NewString()), org, "providers/p1/api-key")
	require.ErrorIs(t, err, modkit.ErrNotFound, "a request of another organization does not reach it")
	for _, bad := range []string{"", "../core", "a//b", "a/./b", "/abs", "white space", strings.Repeat("x", 300)} {
		require.Error(t, sa.Put(ctx, org, bad, "v"), "path %q", bad)
	}
	require.Error(t, sa.Put(ctx, org, "empty", ""))

	rolledBack := errors.New("roll back")
	err = g.Transaction(func(tx *gorm.DB) error {
		if err := sa.In(tx).Put(ctx, org, "in-tx", "v"); err != nil {
			return err
		}
		return rolledBack
	})
	require.ErrorIs(t, err, rolledBack)
	_, err = sa.Get(ctx, org, "in-tx")
	require.ErrorIs(t, err, modkit.ErrNotFound, "a write through In rolls back with the transaction")

	require.NoError(t, sa.Delete(ctx, org, "providers/p1/api-key"))
	_, err = sa.Get(ctx, org, "providers/p1/api-key")
	require.ErrorIs(t, err, modkit.ErrNotFound)
}

func TestHandleInternalAt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mf := manifest("t-at", nil, nil)
	mf.AuditActions = []modkit.AuditAction{{Name: "sync"}}
	m := &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
		rt.HandleInternalAt(http.MethodGet, "/api/internal/v1/things/{id}", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(r.PathValue("id")))
		})
		rt.HandleInternalAt(http.MethodPost, "/api/internal/v1/things", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}, modkit.Audit("sync"))
		return nil
	}}
	clash := &testModule{manifest: manifest("t-clash", nil, nil), activate: func(rt modkit.Runtime) error {
		rt.HandleInternalAt(http.MethodGet, "/api/internal/v1/things/{id}", func(http.ResponseWriter, *http.Request) {})
		return nil
	}}
	h, _ := booted(t, ctx, databases(t)[0], []modkit.Module{m, clash})
	serve := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeInternalAt().ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}
	assert.Equal(t, http.StatusNotFound, serve(http.MethodGet, "/things/7").Code)
	installAll(t, h, "t-at")
	rec := serve(http.MethodGet, "/things/7")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "7", rec.Body.String())
	assert.Equal(t, http.StatusAccepted, serve(http.MethodPost, "/things").Code)

	_, err := h.Install(ctx, "t-clash", false)
	require.Error(t, err, "two modules cannot claim one internal path")

	require.NoError(t, h.Disable(ctx, "t-at"))
	assert.Equal(t, http.StatusNotFound, serve(http.MethodGet, "/things/7").Code, "gone with its module")
}

func TestRouteOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer events.SetModuleTypes(nil)
	defer audit.SetModuleActions(nil)
	mf := manifest("t-opt", nil, nil)
	mf.DisplayName = "Options"
	mf.Permissions = []modkit.Permission{{Name: "key-manage"}}
	mf.AuditActions = []modkit.AuditAction{{Name: "key-rotate", Class: "credential"}, {Name: "key-manage", Class: "credential", Severity: "warning"}}
	mf.Events = []modkit.EventType{{Name: "t-opt.key_rotated", Scope: "agent", Description: "A key was rotated."}, {Name: "t-opt.internal"}}
	ok := func(http.ResponseWriter, *http.Request) {}
	m := &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
		rt.Handle(http.MethodPost, "/keys/{id}/rotate", "key-manage", ok, modkit.Audit("key-rotate"), modkit.EmitsWithoutData("t-opt.key_rotated"))
		rt.Handle(http.MethodGet, "/keys", "key-manage", ok, modkit.Audit("key-manage"))
		rt.Handle(http.MethodGet, "/plain", "", ok)
		return nil
	}}
	var mounted []modhost.Route
	h, _ := booted(t, ctx, databases(t)[0], []modkit.Module{m})
	h.Attach(modhost.Deps{Mount: func(mux *http.ServeMux, r modhost.Route) {
		mounted = append(mounted, r)
		mux.HandleFunc(r.Pattern(), r.Handler)
	}})
	installAll(t, h, "t-opt")
	byPath := map[string]modhost.Route{}
	for _, r := range mounted {
		byPath[r.Path] = r
	}
	rotate := byPath["/keys/{id}/rotate"]
	assert.Equal(t, "t-opt:key-rotate", rotate.AuditAction)
	assert.Equal(t, "t-opt.key_rotated", rotate.Event)
	assert.Equal(t, "agent", rotate.EventScope)
	assert.True(t, rotate.EventWithoutData)
	assert.True(t, byPath["/keys"].AuditRead, "Audit on a read records the read")
	assert.Equal(t, "t-opt:key-manage", byPath["/keys"].AuditAction)
	assert.Empty(t, byPath["/plain"].AuditAction)
	var listed []string
	for _, r := range h.Routes("t-opt") {
		listed = append(listed, r.Pattern())
	}
	assert.Equal(t, []string{"POST /m/t-opt/keys/{id}/rotate", "GET /m/t-opt/keys", "GET /m/t-opt/plain"}, listed)

	typ, found := events.Lookup("t-opt.key_rotated")
	require.True(t, found, "a scoped event is a webhook type while its module is active")
	assert.Equal(t, events.ScopeAgent, typ.Scope)
	assert.Equal(t, "Options", typ.Category)
	_, found = events.Lookup("t-opt.internal")
	assert.False(t, found, "an event without a scope is not a webhook type")
	assert.Equal(t, audit.ClassCredential, audit.Action("t-opt:key-rotate").Class())
	assert.Equal(t, audit.SeverityCritical, audit.Action("t-opt:key-rotate").Severity())
	assert.Equal(t, audit.SeverityWarning, audit.Action("t-opt:key-manage").Severity())

	require.NoError(t, h.Disable(ctx, "t-opt"))
	_, found = events.Lookup("t-opt.key_rotated")
	assert.False(t, found, "gone with its module")
	assert.Nil(t, h.Routes("t-opt"))
}

func TestRegistrationsThatFailActivationG1(t *testing.T) {
	ok := func(http.ResponseWriter, *http.Request) {}
	tool := func(context.Context, json.RawMessage) (any, error) { return "ok", nil }
	cases := map[string]func(rt modkit.Runtime){
		"unknown audit action":      func(rt modkit.Runtime) { rt.Handle(http.MethodGet, "/x", "", ok, modkit.Audit("nope")) },
		"event not in the manifest": func(rt modkit.Runtime) { rt.Handle(http.MethodPost, "/x", "p", ok, modkit.Emits("t-bad.nope")) },
		"event without a scope":     func(rt modkit.Runtime) { rt.Handle(http.MethodPost, "/x", "p", ok, modkit.Emits("t-bad.quiet")) },
		"worker without a name":     func(rt modkit.Runtime) { rt.Go("", func(context.Context) error { return nil }) },
		"worker twice":              func(rt modkit.Runtime) { w := func(context.Context) error { return nil }; rt.Go("w", w); rt.Go("w", w) },
		"tool without a permission": func(rt modkit.Runtime) { rt.MCPTool(modkit.MCPTool{Name: "t_x", Description: "x"}, "", tool) },
		"tool with a bad name":      func(rt modkit.Runtime) { rt.MCPTool(modkit.MCPTool{Name: "Bad-Name", Description: "x"}, "p", tool) },
		"tool with a bad schema": func(rt modkit.Runtime) {
			rt.MCPTool(modkit.MCPTool{Name: "t_x", Description: "x", InputSchema: json.RawMessage(`[]`)}, "p", tool)
		},
		"validate hook on a project": func(rt modkit.Runtime) { rt.Hook(modkit.ResourceProject, modkit.Validate, nil) },
		"apply hook on a project":    func(rt modkit.Runtime) { rt.Hook(modkit.ResourceProject, modkit.Apply, nil) },
		"internal-at outside the internal API": func(rt modkit.Runtime) {
			rt.HandleInternalAt(http.MethodGet, "/api/v1/x", ok)
		},
		"internal-at on a core path": func(rt modkit.Runtime) {
			rt.HandleInternalAt(http.MethodPost, "/api/internal/v1/workload-tokens", ok, modkit.Audit("act"))
		},
		"internal-at under module routes": func(rt modkit.Runtime) {
			rt.HandleInternalAt(http.MethodGet, "/api/internal/v1/m/t-bad/x", ok)
		},
		"internal-at write without an audit action": func(rt modkit.Runtime) {
			rt.HandleInternalAt(http.MethodPost, "/api/internal/v1/x", ok)
		},
	}
	for name, register := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mf := manifest("t-bad", nil, nil)
			mf.Permissions = []modkit.Permission{{Name: "p"}}
			mf.AuditActions = []modkit.AuditAction{{Name: "act"}}
			mf.Events = []modkit.EventType{{Name: "t-bad.quiet"}}
			m := &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
				register(rt)
				return nil
			}}
			h, _ := booted(t, ctx, databases(t)[0], []modkit.Module{m})
			_, err := h.Install(ctx, "t-bad", false)
			require.Error(t, err)
			assert.Empty(t, h.Active())
		})
	}
	bad := map[string]func(*modkit.Manifest){
		"audit action with a bad name": func(mf *modkit.Manifest) { mf.AuditActions = []modkit.AuditAction{{Name: "Bad Name"}} },
		"audit action of no class":     func(mf *modkit.Manifest) { mf.AuditActions = []modkit.AuditAction{{Name: "a", Class: "fun"}} },
		"event of another module":      func(mf *modkit.Manifest) { mf.Events = []modkit.EventType{{Name: "core.thing"}} },
		"event of no scope":            func(mf *modkit.Manifest) { mf.Events = []modkit.EventType{{Name: "t-bad.x", Scope: "galaxy"}} },
	}
	for name, change := range bad {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mf := manifest("t-bad", nil, nil)
			change(&mf)
			h, _ := booted(t, ctx, databases(t)[0], []modkit.Module{&testModule{manifest: mf}})
			_, err := h.Install(ctx, "t-bad", false)
			require.Error(t, err)
		})
	}
}

// internalAtUsers are the modules allowed to call Runtime.HandleInternalAt,
// which exists only until the AI Gateway's controllers can register as
// workloads (wso2/agent-manager#2493). A new caller fails this test; when the
// list is empty again, HandleInternalAt goes.
var internalAtUsers = map[string]bool{}

func TestHandleInternalAtUsers(t *testing.T) {
	root := filepath.Join("..", "..", "modules")
	found := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(src), "HandleInternalAt(") {
			rel, _ := filepath.Rel(root, path)
			found[strings.Split(filepath.ToSlash(rel), "/")[0]] = true
		}
		return nil
	})
	require.NoError(t, err)
	for dir := range found {
		assert.True(t, internalAtUsers[dir], "modules/%s calls HandleInternalAt, which is reserved for the AI Gateway until #2493", dir)
	}
	for dir := range internalAtUsers {
		assert.True(t, found[dir], "modules/%s no longer calls HandleInternalAt: drop it from internalAtUsers", dir)
	}
}
