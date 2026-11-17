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
	_, err := host.Migrate(ctx)
	require.NoError(t, err)

	org := uuid.Must(uuid.NewV7()).String()
	// A fresh organization each run, so the test can rerun on a kept database.
	handle := "mod-" + org[len(org)-8:]
	params := func(auth jwtassertion.Middleware) *wiring.AppParams {
		p, err := wiring.InitializeTestAppParamsWithClientMocks(config.GetConfig(), database, auth,
			wiring.TestClients{PlatformRepository: repositories.NewPlatformRepo(database)}, nil)
		require.NoError(t, err)
		return p
	}
	full := params(moduleAuth(org, handle, "amp:testmod:widget:read", "amp:testmod:widget:write"))
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
		full:     api.MakeHTTPHandlerWithModules(full, nil, nil, host),
		limited:  api.MakeHTTPHandlerWithModules(limited, nil, nil, host),
		internal: api.MakeInternalHTTPHandlerWithModules(full, host),
		org:      org,
		handle:   handle,
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
		code, _ := w.call(t, w.full, http.MethodGet, "/api/v1/m/testmod/whoami", nil)
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
		code, body := w.call(t, w.full, http.MethodGet, "/api/v1/m/testmod/whoami", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var who map[string]any
		require.NoError(t, json.Unmarshal(body, &who))
		assert.Equal(t, w.org, who["orgId"])
		assert.Equal(t, true, who["canWrite"])
		assert.Equal(t, true, who["canReadAgents"])
		assert.EqualValues(t, 1, who["environments"])
		assert.Equal(t, []any{project}, who["projects"])
		assert.Equal(t, true, who["foreignNotFound"])

		code, body = w.call(t, w.limited, http.MethodGet, "/api/v1/m/testmod/whoami", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		require.NoError(t, json.Unmarshal(body, &who))
		assert.Equal(t, false, who["canWrite"])

		code, _ = w.call(t, w.limited, http.MethodGet, "/api/v1/m/testmod/tags", nil)
		assert.Equal(t, http.StatusForbidden, code)
		code, _ = w.call(t, w.full, http.MethodGet, "/api/v1/m/testmod/nosuch", nil)
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

		code, body = w.call(t, w.full, http.MethodPost, agentsPath, agentBody("tagged", map[string]any{"testmod": map[string]any{"tag": "blue"}}))
		require.Equal(t, http.StatusAccepted, code, string(body))
		code, body = w.call(t, w.full, http.MethodGet, "/api/v1/m/testmod/tags", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, `[{"agent":"tagged","tag":"blue"}]`, string(body))

		code, body = w.call(t, w.full, http.MethodPut, agentsPath+"/tagged",
			map[string]any{"displayName": "Tagged", "description": "", "extensions": map[string]any{"testmod": map[string]any{"tag": "undeletable"}}})
		require.Equal(t, http.StatusOK, code, string(body))
		code, body = w.call(t, w.full, http.MethodDelete, agentsPath+"/tagged", nil)
		assert.Equal(t, http.StatusConflict, code, string(body))
		code, _ = w.call(t, w.full, http.MethodGet, agentsPath+"/tagged", nil)
		assert.Equal(t, http.StatusOK, code, "a refused finalize keeps the agent")

		code, body = w.call(t, w.full, http.MethodPut, agentsPath+"/tagged",
			map[string]any{"displayName": "Tagged", "description": "", "extensions": map[string]any{"testmod": map[string]any{"tag": "green"}}})
		require.Equal(t, http.StatusOK, code, string(body))
		code, body = w.call(t, w.full, http.MethodDelete, agentsPath+"/tagged", nil)
		require.Equal(t, http.StatusNoContent, code, string(body))
		code, body = w.call(t, w.full, http.MethodGet, "/api/v1/m/testmod/tags", nil)
		require.Equal(t, http.StatusOK, code)
		assert.JSONEq(t, `[]`, string(body))
	})

	t.Run("events reach subscribers after commit", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, "/api/v1/m/testmod/widgets", map[string]any{"name": "w1"})
		require.Equal(t, http.StatusCreated, code, string(body))
		want := map[string]bool{
			modkit.EventAgentCreated + "/tagged": false, modkit.EventAgentDeleted + "/tagged": false,
			testmod.WidgetCreated: false,
		}
		eventually(t, "module events", func() bool {
			code, body := w.call(t, w.full, http.MethodGet, "/api/v1/m/testmod/received", nil)
			if code != http.StatusOK {
				return false
			}
			var got []struct {
				EventType string  `json:"eventType"`
				Subject   *string `json:"subject"`
			}
			_ = json.Unmarshal(body, &got)
			for _, e := range got {
				key := e.EventType
				if e.Subject != nil && e.EventType != testmod.WidgetCreated {
					key += "/" + *e.Subject
				}
				if _, ok := want[key]; ok {
					want[key] = true
				}
			}
			for _, seen := range want {
				if !seen {
					return false
				}
			}
			return true
		})
	})

	t.Run("a capability resolves through the provider", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodGet, "/api/v1/m/testcap/echo?s=hi", nil)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, `{"echo":"echo:hi"}`, string(body))
	})

	t.Run("workload tokens open the module's internal routes", func(t *testing.T) {
		code, body := w.call(t, w.full, http.MethodPost, "/api/v1/m/testmod/workloads", nil)
		require.Equal(t, http.StatusCreated, code, string(body))
		var issued struct{ Token, CredentialID, Secret string }
		require.NoError(t, json.Unmarshal(body, &issued))

		code, body = w.call(t, w.internal, http.MethodGet, "/api/internal/v1/m/testmod/ping", nil, "Authorization", "Bearer "+issued.Token)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, fmt.Sprintf(`{"subject":"run:direct","orgId":%q}`, w.org), string(body))

		code, _ = w.call(t, w.internal, http.MethodGet, "/api/internal/v1/m/testmod/ping", nil)
		assert.Equal(t, http.StatusUnauthorized, code)
		code, _ = w.call(t, w.internal, http.MethodGet, "/api/internal/v1/m/testmod/ping", nil, "Authorization", "Bearer not-a-token")
		assert.Equal(t, http.StatusUnauthorized, code)

		code, _ = w.call(t, w.internal, http.MethodPost, "/api/internal/v1/workload-tokens",
			map[string]string{"credentialId": issued.CredentialID, "secret": "wrong"})
		assert.Equal(t, http.StatusUnauthorized, code)
		code, body = w.call(t, w.internal, http.MethodPost, "/api/internal/v1/workload-tokens",
			map[string]string{"credentialId": issued.CredentialID, "secret": issued.Secret})
		require.Equal(t, http.StatusOK, code, string(body))
		var exchanged modhost.TokenExchangeResponse
		require.NoError(t, json.Unmarshal(body, &exchanged))
		code, body = w.call(t, w.internal, http.MethodGet, "/api/internal/v1/m/testmod/ping", nil, "Authorization", "Bearer "+exchanged.Token)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.Contains(t, string(body), "run:exchanged")

		// A workload token is not a sign-in: the public API refuses it.
		real := api.MakeHTTPHandler(mustRealAuthParams(t), nil)
		code, _ = w.call(t, real, http.MethodGet, "/api/v1/orgs/"+w.handle+"/projects", nil, "Authorization", "Bearer "+exchanged.Token)
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	t.Run("disabling a module removes its routes at once", func(t *testing.T) {
		require.NoError(t, w.host.Disable(ctx, testmod.ConsumerName))
		code, _ := w.call(t, w.full, http.MethodGet, "/api/v1/m/testcap/echo?s=hi", nil)
		assert.Equal(t, http.StatusNotFound, code)
		require.NoError(t, w.host.Disable(ctx, testmod.Name))
		code, _ = w.call(t, w.full, http.MethodGet, "/api/v1/m/testmod/whoami", nil)
		assert.Equal(t, http.StatusNotFound, code)
		code, body := w.call(t, w.full, http.MethodPost, agentsPath, agentBody("late", map[string]any{"testmod": map[string]any{"tag": "x"}}))
		assert.Equal(t, http.StatusBadRequest, code, string(body))
	})
}

// mustRealAuthParams builds the app with the production JWT middleware.
func mustRealAuthParams(t *testing.T) *wiring.AppParams {
	t.Helper()
	database := db.DB(context.Background())
	p, err := wiring.InitializeTestAppParamsWithClientMocks(config.GetConfig(), database, wiring.ProvideAuthMiddleware(*config.GetConfig()),
		wiring.TestClients{PlatformRepository: repositories.NewPlatformRepo(database)}, nil)
	require.NoError(t, err)
	return p
}
