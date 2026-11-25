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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// fakeAgentKeys accepts the key "good" as an agent of org-a.
type fakeAgentKeys struct{}

func (fakeAgentKeys) VerifyAgentKey(token string) (modkit.AgentKey, error) {
	if token != "good" {
		return modkit.AgentKey{}, errors.New("bad key")
	}
	return modkit.AgentKey{KeyID: "jti-1", OrgID: "org-a", AgentID: "agent-1", AgentName: "bot"}, nil
}

func TestAgentKeyRoutes(t *testing.T) { eachDatabase(t, agentKeyRoutes) }

func agentKeyRoutes(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mf := manifest("t-agent", nil, nil)
	mf.AuditActions = []modkit.AuditAction{{Name: "ingest"}}
	m := &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
		rt.HandleAgent(http.MethodPost, "/in", func(w http.ResponseWriter, r *http.Request) {
			k, _ := modkit.AgentKeyFrom(r.Context())
			_ = json.NewEncoder(w).Encode(map[string]string{
				"org": modkit.OrgID(r.Context()), "agent": k.AgentID, "header": r.Header.Get(modkit.AgentKeyHeader),
			})
		}, modkit.Audit("ingest"))
		rt.Handle(http.MethodGet, "/things", "", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
		return nil
	}}
	var mounted []modhost.Route
	h, _ := replica(t, ctx, d, []modkit.Module{m})
	h.Attach(modhost.Deps{AgentKeys: fakeAgentKeys{}, Mount: func(mux *http.ServeMux, r modhost.Route) {
		mounted = append(mounted, r)
		mux.HandleFunc(r.Pattern(), r.Handler)
	}})
	installAll(t, h, "t-agent")

	req := func(method, path, key string) *http.Request {
		r := httptest.NewRequest(method, path, nil)
		if key != "" {
			r.Header.Set(modkit.AgentKeyHeader, key)
		}
		return r
	}
	serve := func(r *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeAgentAPI().ServeHTTP(rec, r)
		return rec
	}

	assert.True(t, h.AgentRequest(req(http.MethodPost, "/m/t-agent/in", "")), "an agent route skips sign-in")
	assert.False(t, h.AgentRequest(req(http.MethodGet, "/m/t-agent/things", "")), "a user route keeps it")
	assert.True(t, h.AgentRequest(req(http.MethodGet, "/m/t-agent/things", "good")), "a request with a key never reaches sign-in")
	assert.Equal(t, http.StatusNotFound, serve(req(http.MethodGet, "/m/t-agent/things", "good")).Code,
		"a key opens agent routes only")

	assert.Equal(t, http.StatusUnauthorized, serve(req(http.MethodPost, "/m/t-agent/in", "")).Code)
	rec := serve(req(http.MethodPost, "/m/t-agent/in", "bad"))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.JSONEq(t, `{"error":"invalid_agent_key","message":"invalid agent key"}`, rec.Body.String())

	rec = serve(req(http.MethodPost, "/m/t-agent/in", "good"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"org":"org-a","agent":"agent-1","header":""}`, rec.Body.String(),
		"the handler gets the key's claims and org, never the key")

	// The install's trial activation mounts too; check the last one.
	mounted = mounted[len(mounted)-2:]
	assert.Equal(t, "GET /m/t-agent/things", mounted[0].Pattern())
	assert.Equal(t, "POST /m/t-agent/in", mounted[1].Pattern())
	assert.True(t, mounted[1].AgentKey)
	assert.Len(t, h.Routes("t-agent"), 2, "Routes lists agent routes too")

	require.NoError(t, h.Disable(ctx, "t-agent"))
	assert.False(t, h.AgentRequest(req(http.MethodPost, "/m/t-agent/in", "")))
	assert.True(t, h.AgentRequest(req(http.MethodPost, "/m/t-agent/in", "good")))
	assert.Equal(t, http.StatusNotFound, serve(req(http.MethodPost, "/m/t-agent/in", "good")).Code,
		"an inactive module's agent route is not found")
}

func TestAgentRoutesThatFailActivation(t *testing.T) {
	ok := func(http.ResponseWriter, *http.Request) {}
	cases := map[string]func(rt modkit.Runtime){
		"write without an audit action": func(rt modkit.Runtime) { rt.HandleAgent(http.MethodPost, "/x", ok) },
		"under /internal":               func(rt modkit.Runtime) { rt.HandleAgent(http.MethodGet, "/internal/x", ok) },
		"with a permission option": func(rt modkit.Runtime) {
			rt.HandleAgent(http.MethodGet, "/x", ok, modkit.AnyPermission("agent:read"))
		},
		"with a platform-admin option": func(rt modkit.Runtime) { rt.HandleAgent(http.MethodGet, "/x", ok, modkit.PlatformAdmin()) },
		"on a user route's path": func(rt modkit.Runtime) {
			rt.Handle(http.MethodGet, "/x", "", ok)
			rt.HandleAgent(http.MethodGet, "/x", ok)
		},
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
			h.Attach(modhost.Deps{Mount: func(mux *http.ServeMux, r modhost.Route) { mux.HandleFunc(r.Pattern(), r.Handler) }})
			_, err := h.Install(ctx, "t-bad", false)
			require.Error(t, err)
			assert.Empty(t, h.Active())
		})
	}
}
