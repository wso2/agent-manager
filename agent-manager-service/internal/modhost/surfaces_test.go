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

func TestHooksRunInOrderWithTheirOwnBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var journal []string
	var mu sync.Mutex
	h, g := replica(t, ctx, databases(t)[0], []modkit.Module{hooked("t-a", &journal, &mu, nil), hooked("t-b", &journal, &mu, nil)})
	installAll(t, h, "t-a", "t-b")
	order := h.Active()
	require.Len(t, order, 2)

	ext := map[string]json.RawMessage{"t-a": json.RawMessage(`{"a":1}`)}
	require.NoError(t, h.Check(modkit.ResourceAgent, ext))
	req := modkit.HookRequest{Resource: modkit.ResourceAgent, Action: modkit.ActionCreate}
	require.NoError(t, h.Run(ctx, g, modkit.Validate, req, ext))
	require.NoError(t, h.Run(ctx, g, modkit.Finalize, req, ext))

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

	err := h.Check(modkit.ResourceAgent, map[string]json.RawMessage{"t-z": nil})
	var rejected *modkit.Error
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, http.StatusBadRequest, rejected.Status)
	err = h.Check(modkit.ResourceEnvironment, ext)
	require.ErrorAs(t, err, &rejected, "a module with no environment hooks takes no environment block")
}

func TestHookFailures(t *testing.T) {
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
	h, g := replica(t, ctx, databases(t)[0], []modkit.Module{slow})
	installAll(t, h, "t-slow")
	req := modkit.HookRequest{Resource: modkit.ResourceAgent}

	start := time.Now()
	err := h.Run(ctx, g, modkit.Validate, req, nil)
	var hookErr *modhost.HookError
	require.ErrorAs(t, err, &hookErr)
	assert.Less(t, time.Since(start), modkit.HookTimeout+2*time.Second)

	require.ErrorAs(t, h.Run(ctx, g, modkit.Apply, req, nil), &hookErr)
	assert.Contains(t, hookErr.Error(), "panic")

	var rejected *modkit.Error
	require.ErrorAs(t, h.Run(ctx, g, modkit.Finalize, req, nil), &rejected)
	assert.Equal(t, "t-slow: no", rejected.Message)
}

func TestRegistrationsThatFailActivation(t *testing.T) {
	ok := func(http.ResponseWriter, *http.Request) {}
	cases := map[string]func(rt modkit.Runtime){
		"unknown permission":       func(rt modkit.Runtime) { rt.Handle(http.MethodGet, "/x", "nope:read", ok) },
		"write without permission": func(rt modkit.Runtime) { rt.Handle(http.MethodDelete, "/x", "", ok) },
		"bad method":               func(rt modkit.Runtime) { rt.Handle("TRACE", "/x", "", ok) },
		"internal without scope":   func(rt modkit.Runtime) { rt.HandleInternal(http.MethodGet, "/x", "", ok) },
		"hook on a project":        func(rt modkit.Runtime) { rt.Hook("project", modkit.Validate, nil) },
		"unknown event":            func(rt modkit.Runtime) { rt.Subscribe("nobody.declares", nil) },
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

func TestModulePermissionsAndRoutes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer rbac.SetCurrent(rbac.NewRegistry())
	mf := manifest("t-perm", nil, nil)
	mf.Permissions = []modkit.Permission{{Name: "thing:read"}}
	mf.RoleGrants = map[string][]string{rbac.RoleDeveloper: {"thing:read"}}
	mf.Events = []string{"t-perm.happened"}
	var published error
	m := &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
		rt.Handle(http.MethodGet, "/things", "thing:read", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
		rt.Handle(http.MethodGet, "/agents", string(rbac.AgentRead), func(http.ResponseWriter, *http.Request) {})
		published = rt.Publish(ctx, "t-perm.undeclared", nil)
		return nil
	}}
	var mounted []modhost.Route
	h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{m})
	h.Attach(modhost.Deps{Mount: func(mux *http.ServeMux, r modhost.Route) {
		mounted = append(mounted, r)
		mux.HandleFunc(r.Pattern(), r.Handler)
	}})
	installAll(t, h, "t-perm")
	require.Error(t, published, "an event the manifest does not list cannot be published")

	owner, ok := h.Permissions().Owner("t-perm:thing:read")
	assert.True(t, ok)
	assert.Equal(t, "t-perm", owner)
	assert.Contains(t, rbac.RoleGrants(rbac.RoleDeveloper), rbac.Permission("t-perm:thing:read"))
	perms := map[string]rbac.Permission{}
	for _, r := range mounted {
		perms[r.Path] = r.Permission
	}
	assert.Equal(t, map[string]rbac.Permission{"/things": "t-perm:thing:read", "/agents": rbac.AgentRead}, perms)

	rec := httptest.NewRecorder()
	h.ServeAPI().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/m/t-perm/things", nil))
	assert.Equal(t, http.StatusTeapot, rec.Code)

	require.NoError(t, h.Disable(ctx, "t-perm"))
	rec = httptest.NewRecorder()
	h.ServeAPI().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/m/t-perm/things", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rbac.RoleGrants(rbac.RoleDeveloper), rbac.Permission("t-perm:thing:read"))
}
