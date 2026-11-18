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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// hooked is a module that registers one hook per phase on agents, logging
// each call into a shared journal.
func hooked(name string, journal *[]string, mu *sync.Mutex, fail error) *testModule {
	note := func(phase modkit.Phase) modkit.HookFunc {
		return func(_ context.Context, req modkit.HookRequest) error {
			mu.Lock()
			*journal = append(*journal, name+":"+string(phase)+":"+string(req.Extension))
			mu.Unlock()
			return fail
		}
	}
	return &testModule{
		manifest: manifest(name, nil, nil),
		activate: func(rt modkit.Runtime) error {
			for _, p := range []modkit.Phase{modkit.Validate, modkit.Apply, modkit.Finalize} {
				rt.Hook(modkit.ResourceAgent, p, note(p))
			}
			return nil
		},
	}
}

func installAll(t *testing.T, h *modhost.Host, names ...string) {
	t.Helper()
	for _, n := range names {
		_, err := h.Install(context.Background(), n, false)
		require.NoError(t, err)
	}
}

// inTx runs fn in a transaction on g, as the core runs hooks.
func inTx(g *gorm.DB, fn func(tx *gorm.DB) error) error { return g.Transaction(fn) }

// eachDatabase runs a test on SQLite and, in the integration tier, PostgreSQL.
func eachDatabase(t *testing.T, test func(t *testing.T, d database)) {
	for _, d := range databases(t) {
		t.Run(d.driver, func(t *testing.T) { test(t, d) })
	}
}

func TestHooksRunInOrderWithTheirOwnBlock(t *testing.T) { eachDatabase(t, hooksRunInOrder) }

func hooksRunInOrder(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var journal []string
	var mu sync.Mutex
	h, g := replica(t, ctx, d, []modkit.Module{hooked("t-a", &journal, &mu, nil), hooked("t-b", &journal, &mu, nil)})
	installAll(t, h, "t-a", "t-b")
	order := h.Active()
	require.Len(t, order, 2)

	ext := map[string]json.RawMessage{"t-a": json.RawMessage(`{"a":1}`)}
	op := h.Begin(ctx, modkit.ResourceAgent)
	require.NoError(t, op.Check(ext))
	req := modkit.HookRequest{Action: modkit.ActionCreate}
	require.NoError(t, inTx(g, func(tx *gorm.DB) error { return op.Run(ctx, tx, modkit.Validate, req, ext) }))
	require.NoError(t, inTx(g, func(tx *gorm.DB) error { return op.Run(ctx, tx, modkit.Finalize, req, ext) }))

	block := func(n string) string {
		if n == "t-a" {
			return `{"a":1}`
		}
		return ""
	}
	assert.Equal(t, []string{
		order[0] + ":validate:" + block(order[0]), order[1] + ":validate:" + block(order[1]),
		order[1] + ":finalize:" + block(order[1]), order[0] + ":finalize:" + block(order[0]),
	}, journal, "validate runs in activation order, finalize in reverse, each with only its own block")

	err := op.Check(map[string]json.RawMessage{"t-z": nil})
	var rejected *modkit.Error
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, http.StatusBadRequest, rejected.Status)
	err = h.Begin(ctx, modkit.ResourceEnvironment).Check(ext)
	require.ErrorAs(t, err, &rejected, "a module with no environment hooks takes no environment block")
}

func TestHookFailures(t *testing.T) { eachDatabase(t, hookFailures) }

func hookFailures(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slow := &testModule{
		manifest: manifest("t-slow", nil, nil),
		activate: func(rt modkit.Runtime) error {
			rt.Hook(modkit.ResourceAgent, modkit.Validate, func(ctx context.Context, _ modkit.HookRequest) error {
				<-ctx.Done()
				return ctx.Err()
			})
			rt.Hook(modkit.ResourceAgent, modkit.Apply, func(context.Context, modkit.HookRequest) error { panic("boom") })
			rt.Hook(modkit.ResourceAgent, modkit.Finalize, func(context.Context, modkit.HookRequest) error {
				return modkit.Reject("no")
			})
			return nil
		},
	}
	h, g := replica(t, ctx, d, []modkit.Module{slow})
	installAll(t, h, "t-slow")
	req := modkit.HookRequest{}

	start := time.Now()
	err := inTx(g, func(tx *gorm.DB) error {
		return h.Begin(ctx, modkit.ResourceAgent).Run(ctx, tx, modkit.Validate, req, nil)
	})
	var hookErr *modhost.HookError
	require.ErrorAs(t, err, &hookErr)
	assert.Less(t, time.Since(start), modkit.HookTimeout+2*time.Second)

	require.ErrorAs(t, inTx(g, func(tx *gorm.DB) error {
		return h.Begin(ctx, modkit.ResourceAgent).Run(ctx, tx, modkit.Apply, req, nil)
	}), &hookErr)
	assert.Contains(t, hookErr.Error(), "panic")

	assert.NoError(t, inTx(g, func(tx *gorm.DB) error {
		return h.Begin(ctx, modkit.ResourceAgent).Run(ctx, tx, modkit.Finalize, req, nil)
	}),
		"finalize cannot refuse")
}

// The hooks of one operation share a deadline: two slow modules cannot hold
// the transaction for twice the per-hook limit.
func TestOneDeadlinePerOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slow := func(name string) *testModule {
		return &testModule{manifest: manifest(name, nil, nil), activate: func(rt modkit.Runtime) error {
			rt.Hook(modkit.ResourceAgent, modkit.Apply, func(ctx context.Context, _ modkit.HookRequest) error {
				select {
				case <-ctx.Done():
				case <-time.After(modkit.HookTimeout - 500*time.Millisecond):
				}
				return nil
			})
			return nil
		}}
	}
	h, g := replica(t, ctx, databases(t)[0], []modkit.Module{slow("t-s1"), slow("t-s2"), slow("t-s3")})
	installAll(t, h, "t-s1", "t-s2", "t-s3")
	start := time.Now()
	err := inTx(g, func(tx *gorm.DB) error {
		return h.Begin(ctx, modkit.ResourceAgent).Run(ctx, tx, modkit.Apply, modkit.HookRequest{}, nil)
	})
	require.Error(t, err, "three hooks of 4.5s each overrun the operation's deadline")
	assert.Less(t, time.Since(start), modkit.OperationTimeout+time.Second)
}

// An operation keeps the snapshot it began with, and each hook gets its own
// copy of the object; a transaction handle stops working once its hook returns.
func TestOperationSnapshotCopiesAndGuard(t *testing.T) {
	eachDatabase(t, operationSnapshotCopiesAndGuard)
}

func operationSnapshotCopiesAndGuard(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var seen []string
	var kept *gorm.DB
	var mu sync.Mutex
	mutator := func(name string) *testModule {
		return &testModule{manifest: manifest(name, nil, nil), activate: func(rt modkit.Runtime) error {
			rt.Hook(modkit.ResourceAgent, modkit.Apply, func(_ context.Context, req modkit.HookRequest) error {
				mu.Lock()
				defer mu.Unlock()
				seen = append(seen, name+"="+req.Object["name"].(string))
				req.Object["name"] = "changed-by-" + name
				kept = req.Tx
				return req.Tx.Exec(`SELECT 1`).Error
			})
			return nil
		}}
	}
	h, g := replica(t, ctx, d, []modkit.Module{mutator("t-m1"), mutator("t-m2")})
	installAll(t, h, "t-m1", "t-m2")
	op := h.Begin(ctx, modkit.ResourceAgent)
	require.NoError(t, h.Disable(ctx, "t-m2"))

	object := map[string]any{"name": "original"}
	require.NoError(t, g.Transaction(func(tx *gorm.DB) error {
		return op.Run(ctx, tx, modkit.Apply, modkit.HookRequest{Object: object}, nil)
	}))
	assert.ElementsMatch(t, []string{"t-m1=original", "t-m2=original"}, seen,
		"both modules ran, from the operation's snapshot, and neither saw the other's change")
	assert.Equal(t, "original", object["name"])
	assert.Error(t, kept.Exec(`SELECT 1`).Error, "a hook's transaction handle is closed after the hook")
}

func TestRegistrationsThatFailActivation(t *testing.T) {
	ok := func(http.ResponseWriter, *http.Request) {}
	cases := map[string]func(rt modkit.Runtime){
		"unknown permission":           func(rt modkit.Runtime) { rt.Handle(http.MethodGet, "/x", "nope:read", ok) },
		"write without permission":     func(rt modkit.Runtime) { rt.Handle(http.MethodDelete, "/x", "", ok) },
		"bad method":                   func(rt modkit.Runtime) { rt.Handle("TRACE", "/x", "", ok) },
		"internal without scope":       func(rt modkit.Runtime) { rt.HandleInternal(http.MethodGet, "/x", "", ok) },
		"public route under /internal": func(rt modkit.Runtime) { rt.Handle(http.MethodGet, "/internal/x", "", ok) },
		"hook on a project":            func(rt modkit.Runtime) { rt.Hook("project", modkit.Validate, nil) },
		"unknown event":                func(rt modkit.Runtime) { rt.Subscribe("nobody.declares", nil) },
	}
	for name, register := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m := &testModule{manifest: manifest("t-bad", nil, nil), activate: func(rt modkit.Runtime) error {
				register(rt)
				return nil
			}}
			h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{m})
			_, err := h.Install(ctx, "t-bad", false)
			require.Error(t, err)
			assert.Empty(t, h.Active())
		})
	}
}

func TestModulePermissionsAndRoutes(t *testing.T) { eachDatabase(t, modulePermissionsAndRoutes) }

func modulePermissionsAndRoutes(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer rbac.SetCurrent(rbac.NewRegistry())
	mf := manifest("t-perm", nil, nil)
	mf.Permissions = []modkit.Permission{{Name: "thing-read"}}
	mf.RoleGrants = map[string][]string{rbac.RoleDeveloper: {"thing-read"}}
	mf.Events = []string{"t-perm.happened"}
	var published error
	m := &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
		rt.Handle(http.MethodGet, "/things", "thing-read", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
		rt.Handle(http.MethodGet, "/agents", string(rbac.AgentRead), func(http.ResponseWriter, *http.Request) {})
		published = rt.Publish(ctx, "t-perm.undeclared", nil)
		return nil
	}}
	var mounted []modhost.Route
	h, _ := replica(t, ctx, d, []modkit.Module{m})
	h.Attach(modhost.Deps{Mount: func(mux *http.ServeMux, r modhost.Route) {
		mounted = append(mounted, r)
		mux.HandleFunc(r.Pattern(), r.Handler)
	}})
	installAll(t, h, "t-perm")
	require.Error(t, published, "an event the manifest does not list cannot be published")

	owner, ok := h.Permissions().Owner("t-perm:thing-read")
	assert.True(t, ok)
	assert.Equal(t, "t-perm", owner)
	assert.Contains(t, rbac.RoleGrants(rbac.RoleDeveloper), rbac.Permission("t-perm:thing-read"))
	perms := map[string]rbac.Permission{}
	for _, r := range mounted {
		perms[r.Path] = r.Permission
	}
	assert.Equal(t, map[string]rbac.Permission{"/things": "t-perm:thing-read", "/agents": rbac.AgentRead}, perms)

	rec := httptest.NewRecorder()
	h.ServeAPI().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/m/t-perm/things", nil))
	assert.Equal(t, http.StatusTeapot, rec.Code)

	require.NoError(t, h.Disable(ctx, "t-perm"))
	rec = httptest.NewRecorder()
	h.ServeAPI().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/m/t-perm/things", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rbac.RoleGrants(rbac.RoleDeveloper), rbac.Permission("t-perm:thing-read"))
}
