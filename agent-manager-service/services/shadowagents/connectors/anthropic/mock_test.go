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

package anthropic

// Fixtures in testdata/ copy the official example shapes of the evidence
// (evidence/anthropic.md); values are changed only to build multi-page and
// positive/negative cases, and marker strings sit in content fields that
// must never be stored:
//
//	agents_p1/p2.json      §2.1 List Agents example (data, next_page) + a custom tool and an archived agent (workspace A)
//	agents_wsb_p1.json     §2.1 List Agents example, one agent (workspace B)

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Test credentials. The marker substring must never appear in a result or
// a log line.
const (
	secretMarker  = "sk-test-SECRETSECRETSECRET"
	wsKey         = secretMarker + "-workspace"
	wsKeyB        = secretMarker + "-workspace-b"
	wsA           = "wrkspc_01JwQvzr7rXLA5AGx3HKfFUJ"
	wsB           = "wrkspc_01BBBBBBBBBBBBBBBBBBBBBB"
	agent1        = "agent_011CZkYpogX7uDKUyvBTophP"
	agentArchived = "agent_03ArchivedAgentXXXXXXXXX"
	agentB        = "agent_04BatchHelperXXXXXXXXXXX"
)

// wsKeys are the workspace key each workspace accepts.
var wsKeys = map[string]string{wsA: wsKey, wsB: wsKeyB}

// recorded is one request the mock received.
type recorded struct {
	Route    string
	WS       string
	RawQuery string
	Header   http.Header
}

// mockServer speaks the documented Claude Developer Platform wire protocol
// for the one route the connector may call: the workspace-scoped Managed
// Agents list. Any other route (the Admin API's workspace list included)
// fails the test.
type mockServer struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	requests []recorded
	// override answers a route (e.g. "/v1/agents") instead of the
	// fixtures; returning false falls back to the fixtures.
	override map[string]func(w http.ResponseWriter, r *http.Request) bool
}

func newMock(t *testing.T) *mockServer {
	t.Helper()
	m := &mockServer{t: t, override: map[string]func(http.ResponseWriter, *http.Request) bool{}}
	m.srv = httptest.NewTLSServer(http.HandlerFunc(m.serve))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockServer) requestsTo(route string) []recorded {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []recorded
	for _, r := range m.requests {
		if r.Route == route {
			out = append(out, r)
		}
	}
	return out
}

func (m *mockServer) all() []recorded {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]recorded(nil), m.requests...)
}

func (m *mockServer) serve(w http.ResponseWriter, r *http.Request) {
	rec := recorded{Route: r.URL.Path, WS: r.Header.Get("anthropic-workspace-id"), RawQuery: r.URL.RawQuery, Header: r.Header.Clone()}
	m.mu.Lock()
	m.requests = append(m.requests, rec)
	fn := m.override[rec.Route]
	m.mu.Unlock()
	if rec.Route != "/v1/agents" {
		m.t.Errorf("unexpected call %s %s: the connector lists Managed Agents only", r.Method, rec.Route)
		http.NotFound(w, r)
		return
	}
	m.checkHeaders(rec, r)
	if fn != nil && fn(w, r) {
		return
	}
	m.serveFixture(w, r, rec)
}

// checkHeaders asserts the documented headers of each route.
func (m *mockServer) checkHeaders(rec recorded, r *http.Request) {
	t := m.t
	h := r.Header
	if r.Method != http.MethodGet {
		t.Errorf("%s: method %s", rec.Route, r.Method)
	}
	if got := h.Get("anthropic-version"); got != "2026-10-21" {
		t.Errorf("%s: anthropic-version = %q", rec.Route, got)
	}
	switch rec.Route {
	case "/v1/agents":
		if got := h.Get("anthropic-beta"); got != "managed-agents-2026-04-01" {
			t.Errorf("%s: anthropic-beta = %q", rec.Route, got)
		}
		key, ok := wsKeys[rec.WS]
		if !ok {
			t.Errorf("%s: anthropic-workspace-id = %q", rec.Route, rec.WS)
		}
		if h.Get("x-api-key") != "" {
			t.Errorf("%s: no x-api-key may be sent", rec.Route)
		}
		if h.Get("Authorization") != "Bearer "+key && !strings.HasPrefix(h.Get("Authorization"), "Bearer sk-wrong") {
			t.Errorf("%s: want workspace %s's key as a bearer token", rec.Route, rec.WS)
		}
	}
}

func (m *mockServer) serveFixture(w http.ResponseWriter, r *http.Request, rec recorded) {
	q := r.URL.Query()
	var file string
	switch rec.Route {
	case "/v1/agents":
		switch {
		case rec.WS == wsB:
			file = "agents_wsb_p1.json"
		case q.Get("page") == "":
			file = "agents_p1.json"
		default:
			file = "agents_p2.json"
		}
	}
	b, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		m.t.Errorf("fixture %s: %v", file, err)
		http.Error(w, "no fixture", http.StatusInternalServerError)
		return
	}
	w.Header().Set("request-id", "req_018EeWyXxfu5pfWkrYcMdjWG")
	writeJSON(w, string(b))
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// anthropicError writes the documented error shape (anthropic.md §2.0).
func anthropicError(w http.ResponseWriter, status int, typ, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"type":"error","error":{"type":%q,"message":%q},"request_id":"req_011CSHoEeqs5C35K2UUqR7Fy"}`, typ, msg)
}

func queryOf(t *testing.T, raw string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("query %q: %v", raw, err)
	}
	return q
}
