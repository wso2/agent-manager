//go:build integration

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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/api"
	"github.com/wso2/agent-manager/agent-manager-service/app"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/tests/modules/testmod"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// moduleAuth signs every request in as a member of org holding the Admin
// role's permissions plus extra scopes.
func moduleAuth(org, handle string, extra ...string) jwtassertion.Middleware {
	scopes := extra
	for _, p := range rbac.PredefinedRolePermissions[rbac.RoleAdmin] {
		scopes = append(scopes, p.Scope())
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := &jwtassertion.TokenClaims{
				Sub: "module-tester", Scope: strings.Join(scopes, " "), OuId: org, OuHandle: handle,
				RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
			}
			ctx := jwtassertion.ContextWithTokenClaimsAndScope(r.Context(), claims)
			ctx = jwtassertion.ContextWithJWT(ctx, "test-token")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type moduleWorld struct {
	host     *modhost.Host
	full     http.Handler // signed in with the module's permissions
	limited  http.Handler // signed in without them
	internal http.Handler
	signer   services.WorkloadTokenSigner
	// freshOrg signs in to an organization nobody has used yet.
	freshOrg func() (http.Handler, string)
	org      string
	handle   string
}

func (w moduleWorld) call(t *testing.T, h http.Handler, method, path string, body any, header ...string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	return rec.Code, out
}

func newModuleWorld(t *testing.T) moduleWorld {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	database := db.DB(context.Background())
	log := slog.New(slog.DiscardHandler)
	host := modhost.NewWith(database, config.GetConfig().Database.Driver, log,
		[]modkit.Module{testmod.Module(), testmod.Consumer()})
	host.PollInterval = 50 * time.Millisecond
	host.EventRetryBase = 200 * time.Millisecond
	_, err := host.Migrate(ctx)
	require.NoError(t, err)

	org := uuid.Must(uuid.NewV7()).String()
	// A fresh organization each run, so the test can rerun on a kept database.
	handle := "mod-" + org[len(org)-8:]
	params := func(auth jwtassertion.Middleware) *wiring.AppParams {
		p, err := wiring.InitializeTestAppParamsWithClientMocks(config.GetConfig(), database, auth,
			wiring.TestClients{PlatformRepository: repositories.NewPlatformRepo(database)})
		require.NoError(t, err)
		return p
	}
	full := params(moduleAuth(org, handle, "amp:testmod:widget-read", "amp:testmod:widget-write"))
	limited := params(moduleAuth(org, handle))

	stop, err := app.StartModules(ctx, host, database, full)
	require.NoError(t, err)
	t.Cleanup(func() {
		stop()
		cancel()
		coreext.Set(nil)
		rbac.SetCurrent(rbac.NewRegistry())
		// Leave the shared database as the core binary expects it.
		for _, m := range []string{testmod.ConsumerName, testmod.Name} {
			_ = host.Uninstall(context.Background(), m)
		}
	})
	return moduleWorld{
		host:     host,
		full:     api.MakeHTTPHandlerWithModules(full, nil, nil, host, nil),
		limited:  api.MakeHTTPHandlerWithModules(limited, nil, nil, host, nil),
		internal: api.MakeInternalHTTPHandlerWithModules(full, host, nil),
		signer:   full.AgentTokenManagerService.(services.WorkloadTokenSigner),
		freshOrg: func() (http.Handler, string) {
			id := uuid.Must(uuid.NewV7()).String()
			h := "mod-" + id[len(id)-8:]
			return api.MakeHTTPHandlerWithModules(params(moduleAuth(id, h)), nil, nil, host, nil), h
		},
		org:    org,
		handle: handle,
	}
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestModuleContract drives a module through every contract surface over the
// HTTP API, on whichever database the integration tier runs on.
func TestModuleContract(t *testing.T) {
	w := newModuleWorld(t)
	ctx := context.Background()
	project := "modproj"
	agentsPath := "/api/v1/orgs/" + w.handle + "/projects/" + project + "/agents"
	agentBody := func(name string, ext map[string]any) map[string]any {
		b := map[string]any{
			"name": name, "displayName": name,
			"provisioning": map[string]any{"type": "external"},
			"agentType":    map[string]any{"type": "external-agent-api"},
		}
		if ext != nil {
			b["extensions"] = ext
		}
		return b
	}

	t.Run("routes of an uninstalled module are not found", func(t *testing.T) {
		code, _ := w.call(t, w.full, http.MethodGet, "/m/testmod/whoami", nil)
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("an extension block for an inactive module is rejected", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, "/api/v1/orgs/"+w.handle+"/projects",
			map[string]any{"name": project, "displayName": "Module project"})
		require.Equal(t, http.StatusAccepted, code, string(body))
		code, body = w.call(t, w.full, http.MethodPost, agentsPath, agentBody("early", map[string]any{"testmod": map[string]any{"tag": "x"}}))
		assert.Equal(t, http.StatusBadRequest, code, string(body))
		assert.Contains(t, string(body), "testmod")
		code, _ = w.call(t, w.full, http.MethodPost, agentsPath, agentBody("early", map[string]any{"nosuch": map[string]any{}}))
		assert.Equal(t, http.StatusBadRequest, code)
	})

	_, err := w.host.Install(ctx, testmod.Name, false)
	require.NoError(t, err)
	_, err = w.host.Install(ctx, testmod.ConsumerName, false)
	require.NoError(t, err)

	t.Run("routes need sign-in and the module's permission", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodGet, "/m/testmod/whoami", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var who map[string]any
		require.NoError(t, json.Unmarshal(body, &who))
		assert.Equal(t, w.org, who["orgId"])
		assert.Equal(t, true, who["canWrite"])
		assert.Equal(t, true, who["canReadAgents"])
		assert.EqualValues(t, 1, who["environments"])
		assert.Equal(t, []any{project}, who["projects"])
		assert.Equal(t, true, who["foreignNotFound"])

		code, body = w.call(t, w.limited, http.MethodGet, "/m/testmod/whoami", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		require.NoError(t, json.Unmarshal(body, &who))
		assert.Equal(t, false, who["canWrite"])

		code, _ = w.call(t, w.limited, http.MethodGet, "/m/testmod/tags", nil)
		assert.Equal(t, http.StatusForbidden, code)
		// A route can need several permissions, any of several, or the
		// platform-admin organization.
		for _, c := range []struct {
			method, path  string
			full, limited int
		}{
			{http.MethodPost, "/m/testmod/widgets/w1/stamp", http.StatusOK, http.StatusForbidden},
			{http.MethodGet, "/m/testmod/either", http.StatusOK, http.StatusForbidden},
			{http.MethodGet, "/m/testmod/platform/widgets", http.StatusForbidden, http.StatusForbidden},
		} {
			code, body = w.call(t, w.full, c.method, c.path, nil)
			assert.Equal(t, c.full, code, "%s %s: %s", c.method, c.path, body)
			code, _ = w.call(t, w.limited, c.method, c.path, nil)
			assert.Equal(t, c.limited, code, "%s %s without the module's permissions", c.method, c.path)
		}
		code, _ = w.call(t, w.full, http.MethodGet, "/m/testmod/nosuch", nil)
		assert.Equal(t, http.StatusNotFound, code)
		// Module APIs live at the root, with no alias under the core's prefix,
		// and workload routes only on the internal server.
		code, _ = w.call(t, w.full, http.MethodGet, "/api/v1/m/testmod/whoami", nil)
		assert.Equal(t, http.StatusNotFound, code)
		code, _ = w.call(t, w.full, http.MethodGet, "/m/testmod/internal/ping", nil)
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("hooks validate, apply and finalize inside the core transaction", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, agentsPath, agentBody("bad", map[string]any{"testmod": map[string]any{"reject": true}}))
		assert.Equal(t, http.StatusBadRequest, code, string(body))
		assert.Contains(t, string(body), "rejected on request")
		code, _ = w.call(t, w.full, http.MethodPost, agentsPath, agentBody("bad", map[string]any{"testmod": map[string]any{"tag": 7}}))
		assert.Equal(t, http.StatusBadRequest, code)
		code, _ = w.call(t, w.full, http.MethodGet, agentsPath+"/bad", nil)
		assert.Equal(t, http.StatusNotFound, code, "a rejected create leaves no agent")

		// The apply hook also publishes from inside the transaction and
		// changes its own copy of the object.
		code, body = w.call(t, w.full, http.MethodPost, agentsPath, agentBody("tagged",
			map[string]any{"testmod": map[string]any{"tag": "blue", "announce": true}}))
		require.Equal(t, http.StatusAccepted, code, string(body))
		code, body = w.call(t, w.full, http.MethodGet, "/m/testmod/tags", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, `[{"agent":"tagged","tag":"blue"}]`, string(body))
	})

	t.Run("the core looks resources up by name and can lock an agent from a hook", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, agentsPath, agentBody("locked",
			map[string]any{"testmod": map[string]any{"tag": "green", "lock": true}}))
		require.Equal(t, http.StatusAccepted, code, string(body))
		code, body = w.call(t, w.full, http.MethodGet, "/m/testmod/lookup?project="+project+"&agent=locked&environment=default", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var ids map[string]string
		require.NoError(t, json.Unmarshal(body, &ids))
		for _, k := range []string{"project", "agent", "environment"} {
			_, err := uuid.Parse(ids[k])
			assert.NoError(t, err, "%s looked up by name", k)
		}
		// (Lite has one environment: a route's environment parameter always
		// names it, so only the project and agent can miss.)
		code, body = w.call(t, w.full, http.MethodGet, "/m/testmod/lookup?project=nosuch&agent=locked", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.NotContains(t, string(body), `"project"`)
		assert.NotContains(t, string(body), `"agent"`)
		code, body = w.call(t, w.full, http.MethodDelete, agentsPath+"/locked", nil)
		require.Equal(t, http.StatusNoContent, code, string(body))
	})

	t.Run("a module can create the caller's organization", func(t *testing.T) {
		h, _ := w.freshOrg()
		code, body := w.call(t, h, http.MethodPost, "/m/testmod/org", nil)
		// The fresh org's token has no testmod permission.
		require.Equal(t, http.StatusForbidden, code, string(body))
		code, body = w.call(t, w.full, http.MethodPost, "/m/testmod/org", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var org map[string]string
		require.NoError(t, json.Unmarshal(body, &org))
		assert.Equal(t, w.org, org["id"])
		assert.Equal(t, w.handle, org["handle"])
	})

	t.Run("secrets stay within the module and the organization", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPut, "/m/testmod/secrets/n1", map[string]string{"value": "v1"})
		require.Equal(t, http.StatusNoContent, code, string(body))
		code, body = w.call(t, w.full, http.MethodGet, "/m/testmod/secrets/n1", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, `{"value":"v1"}`, string(body))
		code, _ = w.call(t, w.full, http.MethodPut, "/m/testmod/secrets/n1", map[string]string{"value": "v", "org": uuid.NewString()})
		assert.Equal(t, http.StatusNotFound, code, "a module cannot write another organization's secret from a request")
		code, _ = w.call(t, w.full, http.MethodPut, "/m/testmod/secrets/..", map[string]string{"value": "v"})
		assert.NotEqual(t, http.StatusNoContent, code)
		code, _ = w.call(t, w.full, http.MethodDelete, "/m/testmod/secrets/n1", nil)
		require.Equal(t, http.StatusNoContent, code)
		code, _ = w.call(t, w.full, http.MethodGet, "/m/testmod/secrets/n1", nil)
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("a project finalize hook can refuse a project delete", func(t *testing.T) {
		projects := "/api/v1/orgs/" + w.handle + "/projects"
		code, body := w.call(t, w.full, http.MethodPost, projects, map[string]any{"name": "pinned", "displayName": "Pinned"})
		require.Equal(t, http.StatusAccepted, code, string(body))
		code, body = w.call(t, w.full, http.MethodPost, "/m/testmod/projects/pinned/pin", nil)
		require.Equal(t, http.StatusNoContent, code, string(body))
		code, body = w.call(t, w.full, http.MethodDelete, projects+"/pinned", nil)
		assert.Equal(t, http.StatusConflict, code, string(body))
		assert.Contains(t, string(body), "pinned by testmod")
		code, _ = w.call(t, w.full, http.MethodGet, projects+"/pinned", nil)
		assert.Equal(t, http.StatusOK, code, "the refused delete keeps the project")
		// The hook's own cleanup was rolled back with the delete: the pin is
		// still there and still refuses.
		code, _ = w.call(t, w.full, http.MethodDelete, projects+"/pinned", nil)
		assert.Equal(t, http.StatusConflict, code)

		code, _ = w.call(t, w.full, http.MethodDelete, "/m/testmod/projects/pinned/pin", nil)
		require.Equal(t, http.StatusNoContent, code)
		code, body = w.call(t, w.full, http.MethodDelete, projects+"/pinned", nil)
		assert.Less(t, code, 300, string(body))
		code, _ = w.call(t, w.full, http.MethodGet, projects+"/pinned", nil)
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("route options name the audit action and the webhook event", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, "/m/testmod/widgets/w1/archive", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		code, body = w.call(t, w.full, http.MethodGet, "/api/v1/orgs/"+w.handle+"/events/event-types", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.Contains(t, string(body), testmod.WidgetArchived, "a module's webhook event is in the catalog while it is active")
		assert.NotContains(t, string(body), testmod.WidgetCreated, "an event without a scope is not")
	})

	t.Run("workers, MCP tools and fixed internal routes run while the module is active", func(t *testing.T) {
		eventually(t, "the worker", func() bool { return testmod.WorkerRunning.Load() == 1 })
		var tools []string
		for _, tool := range w.host.MCPTools() {
			tools = append(tools, tool.Tool.Name)
		}
		assert.Contains(t, tools, testmod.Tool)

		code, _ := w.call(t, w.internal, http.MethodGet, testmod.LegacyPath, nil)
		assert.Equal(t, http.StatusUnauthorized, code, "the route checks its own credential")
		code, body := w.call(t, w.internal, http.MethodGet, testmod.LegacyPath, nil, "api-key", testmod.LegacyKey)
		require.Equal(t, http.StatusOK, code, string(body))
		code, body = w.call(t, w.internal, http.MethodPost, testmod.LegacyPath, nil, "api-key", testmod.LegacyKey)
		require.Equal(t, http.StatusOK, code, string(body))
		code, _ = w.call(t, w.full, http.MethodGet, testmod.LegacyPath, nil, "api-key", testmod.LegacyKey)
		assert.Equal(t, http.StatusNotFound, code, "only on the internal server")
	})

	t.Run("a module refuses a delete before anything is torn down", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPut, agentsPath+"/tagged",
			map[string]any{"displayName": "Tagged", "description": "", "extensions": map[string]any{"testmod": map[string]any{"tag": "undeletable"}}})
		require.Equal(t, http.StatusOK, code, string(body))
		code, body = w.call(t, w.full, http.MethodDelete, agentsPath+"/tagged", nil)
		assert.Equal(t, http.StatusConflict, code, string(body))
		code, _ = w.call(t, w.full, http.MethodGet, agentsPath+"/tagged", nil)
		assert.Equal(t, http.StatusOK, code, "a refused delete keeps the agent")
		code, body = w.call(t, w.full, http.MethodGet, "/m/testmod/tags", nil)
		require.Equal(t, http.StatusOK, code)
		assert.JSONEq(t, `[{"agent":"tagged","tag":"undeletable"}]`, string(body), "and everything it owns")
	})

	t.Run("a failing finalize hook cannot block a delete", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPut, agentsPath+"/tagged",
			map[string]any{"displayName": "Tagged", "description": "", "extensions": map[string]any{"testmod": map[string]any{"tag": "finalize-fails"}}})
		require.Equal(t, http.StatusOK, code, string(body))
		code, body = w.call(t, w.full, http.MethodDelete, agentsPath+"/tagged", nil)
		require.Equal(t, http.StatusNoContent, code, string(body))
		code, _ = w.call(t, w.full, http.MethodGet, agentsPath+"/tagged", nil)
		assert.Equal(t, http.StatusNotFound, code)
		var n int64
		require.NoError(t, db.DB(ctx).Raw(`SELECT count(*) FROM mod_testmod_agent_tags WHERE tag = ? AND org_id = ?`, "finalize-fails", w.org).Scan(&n).Error)
		assert.EqualValues(t, 1, n, "the failed finalize's own delete is undone with its savepoint")
	})

	t.Run("a finalize hook that hangs mid-statement cannot break a delete", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, agentsPath, agentBody("hangs", map[string]any{"testmod": map[string]any{"tag": "finalize-hangs"}}))
		require.Equal(t, http.StatusAccepted, code, string(body))
		start := time.Now()
		code, body = w.call(t, w.full, http.MethodDelete, agentsPath+"/hangs", nil)
		require.Equal(t, http.StatusNoContent, code, string(body))
		assert.Less(t, time.Since(start), modkit.OperationTimeout+2*time.Second)
		code, _ = w.call(t, w.full, http.MethodGet, agentsPath+"/hangs", nil)
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("a delete's validate hook cannot publish", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, agentsPath, agentBody("quiet", map[string]any{"testmod": map[string]any{"tag": "publish-on-delete"}}))
		require.Equal(t, http.StatusAccepted, code, string(body))
		code, body = w.call(t, w.full, http.MethodDelete, agentsPath+"/quiet", nil)
		require.Equal(t, http.StatusNoContent, code, string(body))
		perr := testmod.DeletePublishErr.Load()
		require.NotNil(t, perr, "the validate hook ran")
		require.Error(t, *perr)
		assert.Contains(t, (*perr).Error(), "cannot write")
	})

	t.Run("a hook that runs out of time leaves nothing behind", func(t *testing.T) {
		start := time.Now()
		code, body := w.call(t, w.full, http.MethodPost, agentsPath, agentBody("stalled", map[string]any{"testmod": map[string]any{"stall": true}}))
		assert.Equal(t, http.StatusInternalServerError, code, string(body))
		assert.Less(t, time.Since(start), modkit.OperationTimeout+2*time.Second)
		code, _ = w.call(t, w.full, http.MethodGet, agentsPath+"/stalled", nil)
		assert.Equal(t, http.StatusNotFound, code)
		time.Sleep(500 * time.Millisecond) // let the stalled hook try its late write
		var n int64
		require.NoError(t, db.DB(ctx).Raw(`SELECT count(*) FROM mod_testmod_agent_tags WHERE tag IN (?, ?)`,
			testmod.StalledTag, "late").Scan(&n).Error)
		assert.Zero(t, n, "the hook's write rolled back with the transaction")
		assert.False(t, testmod.AfterTimeout.Load(), "the hook could not write after its deadline")
	})

	t.Run("events reach every subscriber after commit, with retries", func(t *testing.T) {
		for _, name := range []string{"w1", "flaky"} {
			code, body := w.call(t, w.full, http.MethodPost, "/m/testmod/widgets", map[string]any{"name": name})
			require.Equal(t, http.StatusCreated, code, string(body))
		}
		want := map[string]bool{
			modkit.EventAgentCreated + "/tagged": false, modkit.EventAgentDeleted + "/tagged": false,
			testmod.WidgetCreated + "/w1": false, testmod.WidgetCreated + "/from-hook": false,
			// fails its first delivery, so arrives only by retry
			testmod.WidgetCreated + "/flaky": false,
		}
		eventually(t, "module events", func() bool {
			code, body := w.call(t, w.full, http.MethodGet, "/m/testmod/received", nil)
			if code != http.StatusOK {
				return false
			}
			var got []struct {
				EventType string  `json:"eventType"`
				Subject   *string `json:"subject"`
			}
			_ = json.Unmarshal(body, &got)
			for _, e := range got {
				if e.Subject == nil {
					continue
				}
				if _, ok := want[e.EventType+"/"+*e.Subject]; ok {
					want[e.EventType+"/"+*e.Subject] = true
				}
			}
			for _, seen := range want {
				if !seen {
					return false
				}
			}
			return true
		})
		var failed int64
		require.NoError(t, db.DB(ctx).Raw(`SELECT count(*) FROM module_event_deliveries WHERE failed_at IS NOT NULL`).Scan(&failed).Error)
		assert.Zero(t, failed)
	})

	for _, tc := range []struct {
		name string
		mode int32
	}{{"fails", testmod.EnvironmentFail}, {"hangs mid-statement", testmod.EnvironmentHang}} {
		t.Run("an environment hook that "+tc.name+" cannot block a new organization", func(t *testing.T) {
			testmod.EnvironmentMode.Store(tc.mode)
			defer testmod.EnvironmentMode.Store(testmod.EnvironmentOK)
			fresh, handle := w.freshOrg()
			start := time.Now()
			code, body := w.call(t, fresh, http.MethodGet, "/api/v1/orgs/"+handle+"/projects", nil)
			require.Equal(t, http.StatusOK, code, string(body))
			assert.Less(t, time.Since(start), modkit.OperationTimeout+2*time.Second)
			code, body = w.call(t, fresh, http.MethodGet, "/api/v1/orgs/"+handle+"/projects", nil)
			require.Equal(t, http.StatusOK, code, string(body))
			var n int64
			require.NoError(t, db.DB(ctx).Raw(`SELECT count(*) FROM mod_testmod_agent_tags WHERE tag = ?`, testmod.EnvironmentTag).Scan(&n).Error)
			assert.Zero(t, n, "the failed hook's write is undone")
		})
	}

	t.Run("a capability resolves through the provider", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodGet, "/m/testcap/echo?s=hi", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, `{"echo":"echo:hi"}`, string(body))
	})

	t.Run("workload tokens open the module's internal routes", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, "/m/testmod/workloads", nil)
		require.Equal(t, http.StatusCreated, code, string(body))
		var issued struct{ Token, CredentialID, Secret string }
		require.NoError(t, json.Unmarshal(body, &issued))
		ping := func(token string) (int, []byte) {
			if token == "" {
				return w.call(t, w.internal, http.MethodGet, "/m/testmod/internal/ping", nil)
			}
			return w.call(t, w.internal, http.MethodGet, "/m/testmod/internal/ping", nil, "Authorization", "Bearer "+token)
		}
		heartbeat := func(credential, protocol string) (int, []byte) {
			return w.call(t, w.internal, http.MethodPost, "/api/internal/v1/workloads/heartbeat",
				map[string]string{"credential": credential, "protocol": protocol, "instance": "pinger-0", "version": "test"})
		}

		code, body = ping(issued.Token)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, fmt.Sprintf(`{"subject":"run:direct","orgId":%q,"workload":"","instance":""}`, w.org), string(body))

		code, _ = ping("")
		assert.Equal(t, http.StatusUnauthorized, code)
		code, _ = ping("not-a-token")
		assert.Equal(t, http.StatusUnauthorized, code)

		// Valid workload tokens that do not grant this route.
		other, _, err := w.signer.IssueWorkload(modkit.Workload{Module: testmod.ConsumerName, Subject: "run:other", Scopes: []string{testmod.PingScope}}, time.Minute)
		require.NoError(t, err)
		code, _ = ping(other)
		assert.Equal(t, http.StatusForbidden, code, "a token for another module")
		wrong, _, err := w.signer.IssueWorkload(modkit.Workload{Module: testmod.Name, Subject: "run:wrong", Scopes: []string{"not-ping"}}, time.Minute)
		require.NoError(t, err)
		code, _ = ping(wrong)
		assert.Equal(t, http.StatusForbidden, code, "a token without the route's scope")

		// An agent token is signed with the same key, but is no workload token.
		code, body = w.call(t, w.full, http.MethodPost, agentsPath, agentBody("tokened", nil))
		require.Equal(t, http.StatusAccepted, code, string(body))
		code, body = w.call(t, w.full, http.MethodPost, agentsPath+"/tokened/token", map[string]any{})
		require.Equal(t, http.StatusOK, code, string(body))
		var agentToken struct {
			Token string `json:"token"`
		}
		require.NoError(t, json.Unmarshal(body, &agentToken))
		code, _ = ping(agentToken.Token)
		assert.Equal(t, http.StatusUnauthorized, code, "an agent token")

		code, body = heartbeat(issued.CredentialID+".wrong", testmod.PingerProtocol)
		assert.Equal(t, http.StatusUnauthorized, code)
		assert.Contains(t, string(body), `"error":"invalid_credential"`)
		code, body = heartbeat(issued.Secret, "testmod-ping/v0")
		assert.Equal(t, http.StatusConflict, code, "a protocol the module does not accept")
		assert.Contains(t, string(body), `"error":"unsupported_protocol"`)
		code, body = heartbeat(issued.Secret, testmod.OldPingerProtocol)
		require.Equal(t, http.StatusOK, code, string(body))
		var hb modhost.HeartbeatResponse
		require.NoError(t, json.Unmarshal(body, &hb))
		assert.Equal(t, testmod.Name, hb.Module)
		assert.Equal(t, testmod.Pinger, hb.Workload)
		assert.Equal(t, map[string]string{
			"rest":   "http://example.com/m/testmod/internal",
			"events": "ws://example.com/m/testmod/internal/events",
		}, hb.Endpoints)
		code, body = ping(hb.Token)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, `{"subject":"pinger:issued","orgId":"`+w.org+`","workload":"pinger","instance":"pinger-0"}`, string(body))

		hl, err := w.host.Health(ctx, testmod.Name)
		require.NoError(t, err)
		assert.Equal(t, modhost.HealthOK, hl.Status, hl.Reasons)
		assert.Equal(t, map[string]any{"pings": "counted elsewhere"}, hl.Detail)

		code, body = w.call(t, w.full, http.MethodPost, "/m/testmod/workloads/revoke", map[string]string{"credentialId": issued.CredentialID})
		require.Equal(t, http.StatusNoContent, code, string(body))
		code, body = heartbeat(issued.Secret, testmod.PingerProtocol)
		assert.Equal(t, http.StatusUnauthorized, code, "a revoked credential")
		assert.Contains(t, string(body), `"error":"invalid_credential"`)
		hl, err = w.host.Health(ctx, testmod.Name)
		require.NoError(t, err)
		assert.Equal(t, modhost.HealthDegraded, hl.Status, "the required pinger is gone")

		assertIssuerSeparation(t, w, hb.Token)
	})

	t.Run("disabling a module removes its routes at once", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, "/m/testmod/workloads", nil)
		require.Equal(t, http.StatusCreated, code, string(body))
		var issued struct{ Token, CredentialID, Secret string }
		require.NoError(t, json.Unmarshal(body, &issued))

		require.NoError(t, w.host.Disable(ctx, testmod.ConsumerName))
		code, _ = w.call(t, w.full, http.MethodGet, "/m/testcap/echo?s=hi", nil)
		assert.Equal(t, http.StatusNotFound, code)
		require.NoError(t, w.host.Disable(ctx, testmod.Name))
		code, _ = w.call(t, w.full, http.MethodGet, "/m/testmod/whoami", nil)
		assert.Equal(t, http.StatusNotFound, code)
		code, body = w.call(t, w.full, http.MethodPost, agentsPath, agentBody("late", map[string]any{"testmod": map[string]any{"tag": "x"}}))
		assert.Equal(t, http.StatusBadRequest, code, string(body))

		code, body = w.call(t, w.internal, http.MethodGet, "/m/testmod/internal/ping", nil, "Authorization", "Bearer "+issued.Token)
		assert.Equal(t, http.StatusUnauthorized, code, "workloads of an inactive module stand by")
		assert.Contains(t, string(body), `"error":"module_inactive"`)
		code, _ = w.call(t, w.internal, http.MethodGet, testmod.LegacyPath, nil, "api-key", testmod.LegacyKey)
		assert.Equal(t, http.StatusNotFound, code, "a disabled module's fixed internal routes are gone")
		assert.Zero(t, testmod.WorkerRunning.Load(), "and its workers stopped")
		assert.Empty(t, w.host.MCPTools(), "and its MCP tools")
		code, body = w.call(t, w.full, http.MethodGet, "/api/v1/orgs/"+w.handle+"/events/event-types", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.NotContains(t, string(body), testmod.WidgetArchived, "and its webhook events")
		code, body = w.call(t, w.internal, http.MethodPost, "/api/internal/v1/workloads/heartbeat",
			map[string]string{"credential": issued.Secret, "protocol": testmod.PingerProtocol})
		assert.Equal(t, http.StatusUnauthorized, code, "a credential of an inactive module")
		assert.Contains(t, string(body), `"error":"module_inactive"`)
	})
}

// assertIssuerSeparation points the production sign-in middleware at the
// core's own JWKS, as Compose does, and checks that a workload token is
// refused for its issuer, while a token signed the same way with an accepted
// issuer gets through: the signature alone does not decide.
func assertIssuerSeparation(t *testing.T, w moduleWorld, workloadToken string) {
	t.Helper()
	cfg := config.GetConfig()
	keys, err := w.signerJWKS()
	require.NoError(t, err)
	jwks := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(keys)
	}))
	defer jwks.Close()
	saved := cfg.KeyManagerConfigurations
	defer func() { cfg.KeyManagerConfigurations = saved }()
	cfg.KeyManagerConfigurations.JWKSUrl = jwks.URL
	cfg.KeyManagerConfigurations.Issuer = []string{"amp-test-login"}
	cfg.KeyManagerConfigurations.Audience = []string{"amp-test-audience"}

	real := api.MakeHTTPHandler(mustRealAuthParams(t), nil)
	projects := "/api/v1/orgs/" + w.handle + "/projects"
	code, _ := w.call(t, real, http.MethodGet, projects, nil, "Authorization", "Bearer "+workloadToken)
	assert.Equal(t, http.StatusUnauthorized, code, "a workload token is not a sign-in")

	control := signLikeTheCore(t, jwt.MapClaims{
		"iss": "amp-test-login", "aud": "amp-test-audience", "sub": "tester", "ouId": w.org, "ouHandle": w.handle,
		"scope": rbac.ProjectRead.Scope(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	code, body := w.call(t, real, http.MethodGet, projects, nil, "Authorization", "Bearer "+control)
	assert.Equal(t, http.StatusOK, code, "the same key with an accepted issuer is accepted: %s", body)
}

func (w moduleWorld) signerJWKS() (any, error) {
	return mustAgentTokens().GetJWKS(context.Background())
}

func mustAgentTokens() services.AgentTokenManagerService {
	s, err := services.NewAgentTokenManagerService(nil, config.GetConfig().JWTSigning, slog.New(slog.DiscardHandler))
	if err != nil {
		panic(err)
	}
	return s
}

// signLikeTheCore signs claims with the core's active signing key.
func signLikeTheCore(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	cfg := config.GetConfig().JWTSigning
	raw, err := os.ReadFile(cfg.PrivateKeyPath)
	require.NoError(t, err)
	key, err := jwt.ParseRSAPrivateKeyFromPEM(raw)
	require.NoError(t, err)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = cfg.ActiveKeyID
	signed, err := tok.SignedString(key)
	require.NoError(t, err)
	return signed
}

// mustRealAuthParams builds the app with the production JWT middleware.
func mustRealAuthParams(t *testing.T) *wiring.AppParams {
	t.Helper()
	database := db.DB(context.Background())
	p, err := wiring.InitializeTestAppParamsWithClientMocks(config.GetConfig(), database, wiring.ProvideAuthMiddleware(*config.GetConfig()),
		wiring.TestClients{PlatformRepository: repositories.NewPlatformRepo(database)})
	require.NoError(t, err)
	return p
}
