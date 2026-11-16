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

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// harness is a connector wired to a mock server, with captured logs and
// recorded sleeps.
type harness struct {
	m      *mockServer
	c      *Connector
	logs   *bytes.Buffer
	mu     sync.Mutex
	sleeps []time.Duration
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{m: newMock(t), logs: &bytes.Buffer{}}
	logger := slog.New(slog.NewTextHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h.c = New(h.m.srv.Client(), logger).(*Connector)
	h.c.sleep = func(_ context.Context, d time.Duration) error {
		h.mu.Lock()
		h.sleeps = append(h.sleeps, d)
		h.mu.Unlock()
		return nil
	}
	return h
}

// conn has workspace A's key.
func (h *harness) conn() sa.Connection {
	return sa.Connection{
		Provider: models.ShadowProviderAnthropic,
		Config: models.ShadowConnectionConfig{
			Endpoints: map[string]string{sa.EPAnthropicAPI: h.m.srv.URL},
		},
		Secret: models.ShadowConnectionSecret{ScopedKeys: map[string]string{wsA: wsKey}},
	}
}

func (h *harness) scan(conn sa.Connection) *sa.Result {
	return h.c.Scan(context.Background(), conn)
}

func findingOf(t *testing.T, res *sa.Result, kind, id string) *sa.Finding {
	t.Helper()
	for _, f := range res.Findings {
		if f.Kind == kind && f.ExternalID == id {
			return f
		}
	}
	t.Fatalf("no %s finding %s", kind, id)
	return nil
}

func ids(res *sa.Result) []string {
	out := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		out = append(out, f.ExternalID)
	}
	sort.Strings(out)
	return out
}

func callOf(t *testing.T, res *sa.Result, call string) models.ShadowScanCallResult {
	t.Helper()
	for _, c := range res.Calls {
		if c.Call == call {
			return c
		}
	}
	t.Fatalf("call %q not recorded; calls: %+v", call, res.Calls)
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

func TestProvider(t *testing.T) {
	if p := New(nil, nil).Provider(); p != models.ShadowProviderAnthropic {
		t.Errorf("Provider() = %q", p)
	}
}

func TestScanCalls(t *testing.T) {
	h := newHarness(t)
	res := h.scan(h.conn())
	ag := callOf(t, res, callAgents)
	if ag.Outcome != models.ShadowCallOK || ag.Items != 2 {
		t.Errorf("agents = %+v", ag)
	}
	for _, r := range h.m.all() {
		if r.Route != "/v1/agents" {
			t.Errorf("unexpected route %s", r.Route)
		}
	}
	if len(res.Calls) != 1 {
		t.Errorf("calls = %+v; only the agents list is called", res.Calls)
	}
	if got := ids(res); fmt.Sprint(got) != fmt.Sprint([]string{agent1, agentArchived}) {
		t.Errorf("agents = %v", got)
	}
}

func TestRequestBuilding(t *testing.T) {
	h := newHarness(t)
	h.scan(h.conn())

	ag := h.m.requestsTo("/v1/agents")
	if len(ag) != 2 {
		t.Fatalf("agent pages = %d, want 2", len(ag))
	}
	for i, r := range ag {
		q := queryOf(t, r.RawQuery)
		if q.Get("include_archived") != "true" || q.Get("limit") != "100" {
			t.Errorf("agents page %d query = %v", i, q)
		}
		if r.WS != wsA {
			t.Errorf("agents page %d workspace = %q", i, r.WS)
		}
	}
	if p := queryOf(t, ag[1].RawQuery).Get("page"); p != "page_2" {
		t.Errorf("second agents page = %q, want next_page", p)
	}
}

func TestAgentAttributes(t *testing.T) {
	h := newHarness(t)
	res := h.scan(h.conn())
	for _, f := range res.Findings {
		if f.Kind != models.ShadowKindPlatformAgent {
			t.Errorf("finding %s has kind %s; every finding is an agent", f.ExternalID, f.Kind)
		}
		if len(f.Signals) != 1 || f.Signals[0].Code != sa.SigAnthropicManagedAgent || f.Signals[0].Source != callAgents {
			t.Errorf("finding %s evidence = %+v", f.ExternalID, f.Signals)
		}
		if _, ok := sa.Def(f.Signals[0].Code); !ok {
			t.Errorf("evidence code %s is not in the catalog", f.Signals[0].Code)
		}
		if f.OwnerHint != (models.ShadowOwnerHint{}) {
			t.Errorf("owner = %+v; the agents list names no creator", f.OwnerHint)
		}
	}
	ag := findingOf(t, res, models.ShadowKindPlatformAgent, agent1)
	if ag.DisplayName != "My First Agent" || ag.State != "active" || ag.Scope != "workspace:"+wsA ||
		ag.ProviderCreatedAt == nil || !ag.ProviderCreatedAt.Equal(time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("agent = %+v", ag)
	}
	if ag.Signals[0].Observed["archived"] != false {
		t.Errorf("observed = %v", ag.Signals[0].Observed)
	}
	a := ag.Attributes
	if a["model"] != "claude-opus-5-5" || a["workspace"] != wsA || a["agentVersion"] != int64(3) || a["archived"] != false ||
		a["state"] != "active" || a["description"] != "A general-purpose starter agent." || a["toolCount"] != 3 {
		t.Errorf("agent attributes = %v", a)
	}
	if _, ok := a["roleArn"]; ok {
		t.Error("a service account identity has no role")
	}
	tools, _ := a["tools"].([]sa.ToolRef)
	wantTools := []sa.ToolRef{{Type: "agent_toolset_20260401", Name: "bash"}, {Type: "mcp_toolset", Name: "example-mcp"}, {Type: "custom", Name: "lookup_order"}}
	if fmt.Sprint(tools) != fmt.Sprint(wantTools) {
		t.Errorf("tools = %v, want %v (disabled toolset configs left out)", tools, wantTools)
	}
	mcp, _ := a["mcpServers"].([]sa.MCPServerRef)
	if len(mcp) != 1 || mcp[0] != (sa.MCPServerRef{Label: "example-mcp", Origin: "https://example-server.modelcontextprotocol.io"}) {
		t.Errorf("mcpServers = %v", mcp)
	}
	if skills, _ := a["skills"].([]string); fmt.Sprint(skills) != "[xlsx skill_011CZkZFNu9hAbo3jZPRgTmx]" {
		t.Errorf("skills = %v", a["skills"])
	}
	for _, k := range []string{"sessionCount", "lastSessionAt", "repositoryUrl", "models", "products"} {
		if _, ok := a[k]; ok {
			t.Errorf("attribute %s is not read any more", k)
		}
	}

	old := findingOf(t, res, models.ShadowKindPlatformAgent, agentArchived)
	if old.State != "archived" || old.Attributes["archived"] != true || old.Attributes["roleArn"] != "arn:aws:iam::123456789012:role/agent-exec" {
		t.Errorf("archived agent = %+v", old)
	}
	if old.Signals[0].Observed["archived"] != true {
		t.Errorf("archived observed = %v", old.Signals[0].Observed)
	}
	if _, ok := old.Attributes["tools"]; ok {
		t.Error("an agent without tools has no tools attribute")
	}
	if _, ok := old.Attributes["description"]; ok {
		t.Error("a null description is not stored")
	}
}

func TestTwoWorkspaces(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	conn.Secret.ScopedKeys[wsB] = wsKeyB
	res := h.scan(conn)
	if got := ids(res); fmt.Sprint(got) != fmt.Sprint([]string{agent1, agentArchived, agentB}) {
		t.Errorf("agents = %v", got)
	}
	b := findingOf(t, res, models.ShadowKindPlatformAgent, agentB)
	if b.Scope != "workspace:"+wsB || b.Attributes["workspace"] != wsB || b.DisplayName != "Batch helper" {
		t.Errorf("workspace B agent = %+v", b)
	}
	if c := callOf(t, res, callAgents); c.Items != 3 || c.Outcome != models.ShadowCallOK {
		t.Errorf("agents call = %+v", c)
	}
}

func TestNoWorkspaceKeys(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	conn.Secret.ScopedKeys = nil
	res := h.scan(conn)
	if n := len(h.m.requestsTo("/v1/agents")); n != 0 {
		t.Errorf("agents called %d times without a workspace key", n)
	}
	if c := callOf(t, res, callAgents); c.Outcome != models.ShadowCallSkipped || c.Message != "no workspace keys" {
		t.Errorf("agents = %+v", c)
	}
	if len(h.m.all()) != 0 {
		t.Error("no request may be made without a workspace key")
	}
	if len(res.Findings) != 0 {
		t.Errorf("findings = %v", ids(res))
	}
}

func TestWorkspaceKeyRejected(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	conn.Secret.ScopedKeys[wsB] = wsKeyB
	h.m.override["/v1/agents"] = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("anthropic-workspace-id") == wsB {
			anthropicError(w, http.StatusUnauthorized, "authentication_error", "invalid key")
			return true
		}
		return false
	}
	res := h.scan(conn)
	c := callOf(t, res, callAgents)
	if c.Outcome != models.ShadowCallAuthFailed || !strings.Contains(c.Message, "1 of 2 scopes failed") {
		t.Errorf("agents = %+v", c)
	}
	if got := ids(res); fmt.Sprint(got) != fmt.Sprint([]string{agent1, agentArchived}) {
		t.Errorf("agents = %v; workspace A is still listed", got)
	}
}

func TestAgentsPageLimit(t *testing.T) {
	h := newHarness(t)
	n := 0
	h.m.override["/v1/agents"] = func(w http.ResponseWriter, _ *http.Request) bool {
		n++
		writeJSON(w, fmt.Sprintf(`{"data":[{"id":"agent_%03d","name":"a","model":{"id":"m"},"tools":[]}],"next_page":"page_%d"}`, n, n+1))
		return true
	}
	res := h.scan(h.conn())
	if n != maxAgentPages {
		t.Errorf("pages read = %d, want %d", n, maxAgentPages)
	}
	c := callOf(t, res, callAgents)
	if c.Outcome != models.ShadowCallFailed || !strings.Contains(c.Message, "incomplete") || c.Items != maxAgentPages {
		t.Errorf("agents = %+v; a cut listing is incomplete", c)
	}
	if len(res.Findings) != maxAgentPages {
		t.Errorf("findings = %d; what was read is kept", len(res.Findings))
	}
}

func TestSameCursorTwiceIsUnsupported(t *testing.T) {
	h := newHarness(t)
	h.m.override["/v1/agents"] = func(w http.ResponseWriter, _ *http.Request) bool {
		writeJSON(w, `{"data":[],"next_page":"page_2"}`)
		return true
	}
	res := h.scan(h.conn())
	if c := callOf(t, res, callAgents); c.Outcome != models.ShadowCallUnsupported {
		t.Errorf("agents = %+v", c)
	}
}

func TestRateLimitedThenOK(t *testing.T) {
	h := newHarness(t)
	var mu sync.Mutex
	first := true
	h.m.override["/v1/agents"] = func(w http.ResponseWriter, _ *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		if first {
			first = false
			w.Header().Set("Retry-After", "0")
			anthropicError(w, http.StatusTooManyRequests, "rate_limit_error", "slow down")
			return true
		}
		return false
	}
	res := h.scan(h.conn())
	if c := callOf(t, res, callAgents); c.Outcome != models.ShadowCallOK || c.Items != 2 {
		t.Errorf("agents = %+v", c)
	}
	if len(h.m.requestsTo("/v1/agents")) != 3 {
		t.Errorf("agents requests = %d, want 3 (one retried)", len(h.m.requestsTo("/v1/agents")))
	}
}

func TestMalformedJSON(t *testing.T) {
	h := newHarness(t)
	h.m.override["/v1/agents"] = func(w http.ResponseWriter, _ *http.Request) bool {
		writeJSON(w, `{"data":[{"id":`)
		return true
	}
	res := h.scan(h.conn())
	if c := callOf(t, res, callAgents); c.Outcome != models.ShadowCallUnsupported {
		t.Errorf("agents = %+v, want unsupported_response", c)
	}
	if len(res.Findings) != 0 {
		t.Errorf("findings = %v", ids(res))
	}
}

func TestBadEndpoint(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	// An unfilled placeholder cannot be expanded (an empty value would fall
	// back to the official endpoint, which tests must never call).
	conn.Config.Endpoints = map[string]string{sa.EPAnthropicAPI: "https://{host}"}
	res := h.scan(conn)
	if c := callOf(t, res, callAgents); c.Outcome != models.ShadowCallFailed {
		t.Errorf("agents = %+v", c)
	}
	if len(h.m.all()) != 0 {
		t.Error("no request may be made")
	}
	info := h.c.Test(context.Background(), conn)
	if info.OK || len(info.Checks) != 1 || info.Checks[0].Outcome != models.ShadowCallFailed {
		t.Errorf("test = %+v", info)
	}
}

func TestRetention(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	conn.Secret.ScopedKeys[wsB] = wsKeyB
	res := h.scan(conn)
	info := h.c.Test(context.Background(), conn)
	out, err := json.Marshal(struct {
		R *sa.Result
		I models.ShadowConnectionTestInfo
	}{res, info})
	if err != nil {
		t.Fatal(err)
	}
	markers := []string{
		secretMarker, "SECRET-PROMPT-MARKER", "SECRET-SCHEMA-MARKER", "SECRET-METADATA-MARKER",
		"You are a general-purpose agent", "token=",
	}
	for _, mk := range markers {
		if bytes.Contains(out, []byte(mk)) {
			t.Errorf("result contains %q", mk)
		}
		if strings.Contains(h.logs.String(), mk) {
			t.Errorf("logs contain %q", mk)
		}
	}
	if !strings.Contains(h.logs.String(), "Shadow agent discovery provider call") {
		t.Error("expected debug call logs to be captured")
	}
	if strings.Contains(h.logs.String(), "include_archived=") {
		t.Error("logs contain a query string")
	}
}

func checkNamed(info models.ShadowConnectionTestInfo, name string) (models.ShadowConnectionCheck, bool) {
	for _, c := range info.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return models.ShadowConnectionCheck{}, false
}

func TestConnectionTest(t *testing.T) {
	agentsCheck := "Workspace " + wsA + ": Managed Agents"
	t.Run("ok", func(t *testing.T) {
		h := newHarness(t)
		info := h.c.Test(context.Background(), h.conn())
		if !info.OK || len(info.Checks) != 1 {
			t.Fatalf("test = %+v", info)
		}
		ag, ok := checkNamed(info, agentsCheck)
		if !ok || ag.Outcome != models.ShadowCallOK || fmt.Sprint(ag.Unlocks) != "[Managed Agent]" {
			t.Errorf("agents check = %+v", ag)
		}
		for _, r := range h.m.all() {
			if queryOf(t, r.RawQuery).Get("limit") != "1" {
				t.Errorf("%s: limit = %q", r.Route, queryOf(t, r.RawQuery).Get("limit"))
			}
		}
	})
	t.Run("no workspace key", func(t *testing.T) {
		h := newHarness(t)
		conn := h.conn()
		conn.Secret.ScopedKeys = nil
		info := h.c.Test(context.Background(), conn)
		if info.OK {
			t.Error("without a workspace key no Managed Agent can be listed")
		}
		if c, _ := checkNamed(info, "Managed Agents"); c.Outcome != models.ShadowCallSkipped || c.Message != "no workspace keys" {
			t.Errorf("check = %+v", c)
		}
		if len(h.m.all()) != 0 {
			t.Error("no request may be made without a workspace key")
		}
	})
	t.Run("a workspace key rejected", func(t *testing.T) {
		h := newHarness(t)
		conn := h.conn()
		conn.Secret.ScopedKeys[wsB] = wsKeyB
		h.m.override["/v1/agents"] = func(w http.ResponseWriter, r *http.Request) bool {
			if r.Header.Get("anthropic-workspace-id") == wsB {
				anthropicError(w, http.StatusUnauthorized, "authentication_error", "invalid key")
				return true
			}
			return false
		}
		info := h.c.Test(context.Background(), conn)
		if info.OK {
			t.Error("every workspace key must list its agents")
		}
		if c, _ := checkNamed(info, "Workspace "+wsB+": Managed Agents"); c.Outcome != models.ShadowCallAuthFailed {
			t.Errorf("workspace B check = %+v", c)
		}
	})
	t.Run("no data list", func(t *testing.T) {
		h := newHarness(t)
		h.m.override["/v1/agents"] = func(w http.ResponseWriter, _ *http.Request) bool {
			writeJSON(w, `{"object":"list"}`)
			return true
		}
		info := h.c.Test(context.Background(), h.conn())
		if c, _ := checkNamed(info, agentsCheck); info.OK || c.Outcome != models.ShadowCallUnsupported {
			t.Errorf("test = %+v", info)
		}
	})
}
