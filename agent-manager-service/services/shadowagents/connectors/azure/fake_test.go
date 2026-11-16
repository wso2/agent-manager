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

package azure

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Test identities and markers. The markers are planted in fixture fields
// that must never reach a finding, a call result or a log line.
const (
	testTenant   = "tenant-1"
	testClient   = "client-1"
	testSecret   = "client-secret-value-SECRETSECRET"
	testSub      = "00000000-1111-2222-3333-444444444444"
	armToken     = "arm-token-abc"
	aiToken      = "ai-token-xyz"
	accountsPath = "/subscriptions/" + testSub + "/providers/Microsoft.CognitiveServices/accounts"
	aif1ID       = "/subscriptions/" + testSub + "/resourceGroups/rg1/providers/Microsoft.CognitiveServices/accounts/aif1"
)

var retentionMarkers = []string{
	"SECRET-PROMPT-MARKER", "sk-test-SECRETSECRETSECRET", "CLAIMS-PII-MARKER", "203.0.113.77",
	testSecret, armToken, aiToken, "token=",
}

// fakeAzure is one TLS server playing Entra, ARM and the Foundry data plane.
// Routes are keyed (e.g. "agents:proj1"); each key serves fixture pages,
// and an override can replace any route. A route the connector no longer
// calls (deployments, Azure Monitor metrics, stored responses) fails the
// test.
type fakeAzure struct {
	t   *testing.T
	srv *httptest.Server

	mu        sync.Mutex
	hits      map[string]int
	reqs      map[string][]*http.Request
	override  map[string]func(w http.ResponseWriter, r *http.Request) bool
	forbidden map[string]bool
	tokenFail bool
	scopes    []string
}

func newFakeAzure(t *testing.T) *fakeAzure {
	t.Helper()
	f := &fakeAzure{
		t: t, hits: map[string]int{}, reqs: map[string][]*http.Request{},
		override: map[string]func(http.ResponseWriter, *http.Request) bool{}, forbidden: map[string]bool{},
	}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAzure) client() *http.Client {
	c := f.srv.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

func (f *fakeAzure) count(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[key]
}

func (f *fakeAzure) requests(key string) []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*http.Request(nil), f.reqs[key]...)
}

// conn returns a connection pointed at the fake server.
func (f *fakeAzure) conn(provider string) sa.Connection {
	return sa.Connection{
		Name: "test", Provider: provider,
		Config: models.ShadowConnectionConfig{
			Endpoints:        map[string]string{sa.EPAzureLogin: f.srv.URL, sa.EPAzureARM: f.srv.URL},
			TenantID:         testTenant,
			ClientID:         testClient,
			AllSubscriptions: true,
		},
		Secret: models.ShadowConnectionSecret{ClientSecret: testSecret},
	}
}

// routeKey classifies a request path.
func routeKey(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	switch {
	case strings.HasSuffix(p, "/.well-known/openid-configuration"):
		return "openid"
	case strings.HasSuffix(p, "/oauth2/v2.0/token"):
		return "token"
	case p == "/subscriptions":
		return "subscriptions"
	case strings.EqualFold(p, accountsPath):
		return "accounts"
	case strings.HasPrefix(p, "/api/projects/"):
		proj := parts[2]
		rest := strings.Join(parts[3:], "/")
		switch rest {
		case "agents":
			return "agents:" + proj
		case "assistants":
			return "assistants:" + proj
		}
	case len(parts) >= 9 && strings.EqualFold(parts[6], "accounts"):
		acct := parts[7]
		rest := strings.Join(parts[8:], "/")
		if rest == "projects" {
			return "projects:" + acct
		}
	}
	return "unknown:" + p
}

func (f *fakeAzure) serve(w http.ResponseWriter, r *http.Request) {
	key := routeKey(r.URL.Path)
	f.mu.Lock()
	f.hits[key]++
	f.reqs[key] = append(f.reqs[key], r.Clone(context.Background()))
	ov := f.override[key]
	forbidden := f.forbidden[key]
	f.mu.Unlock()
	if forbidden || strings.HasPrefix(key, "unknown:") {
		f.t.Errorf("route %s must not be called", key)
		http.Error(w, "forbidden route", http.StatusTeapot)
		return
	}
	switch key {
	case "openid":
		tenant := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
		base := f.srv.URL + "/" + tenant
		writeJSON(w, http.StatusOK, fmt.Sprintf(`{"authorization_endpoint":%q,"token_endpoint":%q,"issuer":%q}`,
			base+"/oauth2/v2.0/authorize", base+"/oauth2/v2.0/token", base+"/v2.0"))
		return
	case "token":
		f.token(w, r)
		return
	}
	if ov != nil && ov(w, r) {
		return
	}
	want := "Bearer " + armToken
	if strings.HasPrefix(r.URL.Path, "/api/projects/") {
		want = "Bearer " + aiToken
	}
	if r.Header.Get("Authorization") != want {
		writeJSON(w, http.StatusUnauthorized, `{"error":{"code":"InvalidAuthenticationToken","message":"The access token is invalid."}}`)
		return
	}
	file := f.fixture(key, r.URL.Query())
	if file == "" {
		writeJSON(w, http.StatusNotFound, `{"error":{"code":"ResourceNotFound","message":"not found"}}`)
		return
	}
	writeJSON(w, http.StatusOK, f.load(file))
}

func (f *fakeAzure) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, `{"error":"invalid_request"}`)
		return
	}
	f.mu.Lock()
	f.scopes = append(f.scopes, r.PostForm.Get("scope"))
	fail := f.tokenFail
	f.mu.Unlock()
	if fail || r.PostForm.Get("client_secret") != testSecret || r.PostForm.Get("client_id") != testClient ||
		r.PostForm.Get("grant_type") != "client_credentials" {
		writeJSON(w, http.StatusUnauthorized, `{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided.","error_codes":[7000215]}`)
		return
	}
	var tok string
	switch {
	case strings.Contains(r.PostForm.Get("scope"), sa.AzureARMScope):
		tok = armToken
	case strings.Contains(r.PostForm.Get("scope"), sa.AzureFoundryScope):
		tok = aiToken
	default:
		writeJSON(w, http.StatusBadRequest, `{"error":"invalid_scope"}`)
		return
	}
	writeJSON(w, http.StatusOK, fmt.Sprintf(`{"token_type":"Bearer","expires_in":3599,"ext_expires_in":3599,"access_token":%q}`, tok))
}

// fixture picks the fixture for a route and page.
func (f *fakeAzure) fixture(key string, q url.Values) string {
	page2 := q.Get("$skiptoken") == "page2" || q.Get("after") != ""
	pick := func(p1, p2 string) string {
		if page2 {
			return p2
		}
		return p1
	}
	switch key {
	case "subscriptions":
		return pick("subscriptions_page1.json", "subscriptions_page2.json")
	case "accounts":
		return pick("accounts_page1.json", "accounts_page2.json")
	case "projects:aif1":
		return pick("projects_aif1_page1.json", "projects_aif1_page2.json")
	case "agents:proj1":
		return pick("agents_proj1_page1.json", "agents_proj1_page2.json")
	case "assistants:proj1":
		return pick("assistants_proj1_page1.json", "assistants_proj1_page2.json")
	}
	return ""
}

func (f *fakeAzure) load(name string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		f.t.Fatalf("reading fixture %s: %v", name, err)
	}
	return strings.ReplaceAll(string(b), "{{BASE}}", f.srv.URL)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// newLogger returns a debug logger writing to buf.
func newLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func newTestFoundry(f *fakeAzure, logs *bytes.Buffer) *Foundry {
	c, _ := NewFoundry(f.client(), newLogger(logs)).(*Foundry)
	return c
}
