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

package openai

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Fixtures (testdata/) and the official example or schema each follows
// (sources as in evidence/openai.md):
//
//   agents.proj_*.json           DOCS beta agents list 200 example (AgentListResource / AgentResource; MCP item per OAS L101088)
//   error_permission_plain.json  OAS PermissionErrorResponse with a plain string error
//   error_agents_beta.json       Agents errors guide 400 invalid_beta (ErrorBodyResource)
//
// Fixtures carry retention markers in content fields (instructions, tool
// parameters, MCP headers and URLs, metadata): none may appear in a result
// or a log line.

const (
	markerPrompt = "SECRET-PROMPT-MARKER"
	markerKey    = "sk-test-SECRETSECRETSECRET"
	alphaKey     = "sk-test-SECRETSECRETSECRET-ALPHA"
	betaKey      = "sk-test-SECRETSECRETSECRET-BETA"
)

// mockOpenAI serves the fixtures as the OpenAI API under /v1. The first
// page of a listing is <name>.1.json, the page after a cursor <name>.2.json.
// Only the two routes the connector may call exist: any other request fails
// the test.
type mockOpenAI struct {
	t   *testing.T
	mu  sync.Mutex
	req []*http.Request
	// override, when it returns true, has served the request.
	override func(w http.ResponseWriter, r *http.Request) bool
}

func newMock(t *testing.T) (*mockOpenAI, *httptest.Server) {
	t.Helper()
	m := &mockOpenAI{t: t}
	srv := httptest.NewTLSServer(m)
	t.Cleanup(srv.Close)
	return m, srv
}

func (m *mockOpenAI) requests() []*http.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*http.Request(nil), m.req...)
}

// requestsTo returns the requests whose path is p.
func (m *mockOpenAI) requestsTo(p string) []*http.Request {
	var out []*http.Request
	for _, r := range m.requests() {
		if r.URL.Path == p {
			out = append(out, r)
		}
	}
	return out
}

func (m *mockOpenAI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.req = append(m.req, r.Clone(context.Background()))
	m.mu.Unlock()
	if m.override != nil && m.override(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	auth := r.Header.Get("Authorization")
	page := "1"
	if r.URL.Query().Get("after") != "" {
		page = "2"
	}
	project := ""
	switch auth {
	case "Bearer " + alphaKey:
		project = "proj_alpha"
	case "Bearer " + betaKey:
		project = "proj_beta"
	}
	switch path {
	case "/agents":
		if project == "" {
			writeStatus(w, http.StatusUnauthorized, `{"error":{"type":"invalid_request_error","code":"invalid_api_key","message":"Authentication or project context was missing","param":null}}`)
			return
		}
		m.serve(w, "agents."+project+"."+page+".json")
	default:
		m.t.Errorf("unexpected call %s %s: the connector lists agents only", r.Method, r.URL.Path)
		writeStatus(w, http.StatusNotFound, `{"error":{"message":"not found","type":"invalid_request_error","param":null,"code":null}}`)
	}
}

func (m *mockOpenAI) serve(w http.ResponseWriter, name string) {
	m.serveStatus(w, http.StatusOK, name)
}

func (m *mockOpenAI) serveStatus(w http.ResponseWriter, status int, name string) {
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		m.t.Errorf("missing fixture %s", name)
		writeStatus(w, http.StatusNotFound, `{"error":{"message":"no fixture","type":"invalid_request_error","param":null,"code":null}}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func writeStatus(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// testConn has project keys for proj_alpha and proj_beta.
func testConn(srvURL string) sa.Connection {
	return sa.Connection{
		ID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		OUID:     "ou",
		Name:     "openai",
		Provider: models.ShadowProviderOpenAI,
		Config: models.ShadowConnectionConfig{
			Endpoints: map[string]string{sa.EPOpenAIAPI: srvURL + "/v1"},
		},
		Secret: models.ShadowConnectionSecret{
			ScopedKeys: map[string]string{"proj_alpha": alphaKey, "proj_beta": betaKey},
		},
	}
}

// newTestConnector returns a connector over srv's client and the buffer
// its debug logs go to.
func newTestConnector(srv *httptest.Server) (sa.Connector, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return New(srv.Client(), logger), &buf
}

// scanWith scans conn (edited by edit, when given) against a fresh mock.
func scanWith(t *testing.T, edit func(*sa.Connection), override func(w http.ResponseWriter, r *http.Request) bool) (*sa.Result, *mockOpenAI, *bytes.Buffer) {
	t.Helper()
	m, srv := newMock(t)
	m.override = override
	c, logs := newTestConnector(srv)
	conn := testConn(srv.URL)
	if edit != nil {
		edit(&conn)
	}
	return c.Scan(context.Background(), conn), m, logs
}

func callResult(t *testing.T, res *sa.Result, call string) models.ShadowScanCallResult {
	t.Helper()
	for _, c := range res.Calls {
		if c.Call == call {
			return c
		}
	}
	t.Fatalf("no call result for %q; have %+v", call, res.Calls)
	return models.ShadowScanCallResult{}
}

func hasCall(res *sa.Result, call string) bool {
	for _, c := range res.Calls {
		if c.Call == call {
			return true
		}
	}
	return false
}

func finding(t *testing.T, res *sa.Result, kind, id string) *sa.Finding {
	t.Helper()
	for _, f := range res.Findings {
		if f.Kind == kind && f.ExternalID == id {
			return f
		}
	}
	t.Fatalf("no %s finding %q", kind, id)
	return nil
}

func signal(f *sa.Finding, code, source string) (sa.Signal, bool) {
	for _, s := range f.Signals {
		if s.Code == code && (source == "" || s.Source == source) {
			return s, true
		}
	}
	return sa.Signal{}, false
}

func findingIDs(res *sa.Result) []string {
	out := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		out = append(out, f.ExternalID)
	}
	return out
}
