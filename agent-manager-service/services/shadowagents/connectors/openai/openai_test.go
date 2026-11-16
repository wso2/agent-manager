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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

func TestProvider(t *testing.T) {
	assert.Equal(t, models.ShadowProviderOpenAI, New(nil, nil).Provider())
}

func TestScanRequests(t *testing.T) {
	_, m, _ := scanWith(t, nil, nil)

	t.Run("only the agents list is called", func(t *testing.T) {
		require.NotEmpty(t, m.requests())
		for _, r := range m.requests() {
			assert.Equal(t, "/v1/agents", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
		}
	})
	t.Run("agents with each project key and the beta header, paged", func(t *testing.T) {
		rs := m.requestsTo("/v1/agents")
		require.Len(t, rs, 3, "alpha: one page; beta: two pages")
		byKey := map[string]int{}
		for _, r := range rs {
			assert.Equal(t, "agents=v1", r.Header.Get("OpenAI-Beta"))
			assert.Equal(t, "desc", r.URL.Query().Get("order"))
			assert.Equal(t, "100", r.URL.Query().Get("limit"))
			byKey[r.Header.Get("Authorization")]++
		}
		assert.Equal(t, map[string]int{"Bearer " + alphaKey: 1, "Bearer " + betaKey: 2}, byKey)
		var afters []string
		for _, r := range rs {
			afters = append(afters, r.URL.Query().Get("after"))
		}
		assert.Contains(t, afters, "agent_b1", "the second beta page follows last_id")
	})
}

func TestScanCallResults(t *testing.T) {
	res, _, _ := scanWith(t, nil, nil)
	a := callResult(t, res, callAgents)
	assert.Equal(t, models.ShadowCallOK, a.Outcome)
	assert.Equal(t, 3, a.Items)
	assert.Len(t, res.Calls, 1)
}

func TestScanAgentObject(t *testing.T) {
	res, _, _ := scanWith(t, nil, nil)
	assert.ElementsMatch(t, []string{"agent_alpha", "agent_b1", "agent_b2"}, findingIDs(res))
	for _, f := range res.Findings {
		assert.Equal(t, models.ShadowKindPlatformAgent, f.Kind, "every finding is an agent")
		require.Len(t, f.Signals, 1)
		assert.Equal(t, sa.SigOpenAIAgentObject, f.Signals[0].Code)
		_, ok := sa.Def(f.Signals[0].Code)
		assert.True(t, ok)
	}

	f := finding(t, res, models.ShadowKindPlatformAgent, "agent_alpha")
	assert.Equal(t, "Support agent", f.DisplayName)
	assert.Equal(t, "proj_alpha", f.Scope)
	assert.Equal(t, stateActive, f.State)
	assert.Equal(t, "gpt-x", f.Attributes["model"])
	assert.Equal(t, int64(4), f.Attributes["toolCount"])
	assert.Equal(t, []sa.ToolRef{
		{Type: "function", Name: "lookup_order"},
		{Type: "mcp", Name: "github"},
		{Type: "mcp", Name: "docs"},
		{Type: "web_search"},
	}, f.Attributes["tools"])
	assert.Equal(t, []sa.MCPServerRef{
		{Label: "github", Origin: "https://mcp.example.com"},
		{Label: "docs", Origin: "https://tools.example.org"},
	}, f.Attributes["mcpServers"])
	assert.Equal(t, time.Unix(1793404800, 0).UTC(), *f.ProviderCreatedAt)
	assert.Equal(t, models.ShadowOwnerHint{}, f.OwnerHint, "the agents list names no creator")
	sig, ok := signal(f, sa.SigOpenAIAgentObject, callAgents)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"toolCount": int64(4), "mcpServerCount": int64(2)}, sig.Observed)
	for _, k := range []string{"instructions", "metadata", "sessionCount", "lastSessionAt", "project", "archived"} {
		_, has := f.Attributes[k]
		assert.False(t, has, k)
	}

	b2 := finding(t, res, models.ShadowKindPlatformAgent, "agent_b2")
	assert.Equal(t, "agent_b2", b2.DisplayName, "an unnamed agent shows its ID")
	assert.Equal(t, "proj_beta", b2.Scope)
	assert.Equal(t, []sa.ToolRef{{Type: "computer_use"}}, b2.Attributes["tools"])
	b1 := finding(t, res, models.ShadowKindPlatformAgent, "agent_b1")
	_, has := b1.Attributes["tools"]
	assert.False(t, has, "no tools, no tools attribute")
	assert.Equal(t, int64(0), b1.Attributes["toolCount"])
}

func TestScanProjectsWithoutKey(t *testing.T) {
	res, m, _ := scanWith(t, func(c *sa.Connection) { delete(c.Secret.ScopedKeys, "proj_beta") }, nil)
	assert.Len(t, m.requestsTo("/v1/agents"), 1, "a project without a key is not called")
	assert.Len(t, res.Calls, 1)
	assert.Equal(t, []string{"agent_alpha"}, findingIDs(res))
}

func TestScanNoProjectKeys(t *testing.T) {
	res, m, _ := scanWith(t, func(c *sa.Connection) { c.Secret.ScopedKeys = map[string]string{"proj_alpha": "  "} }, nil)
	assert.Empty(t, m.requestsTo("/v1/agents"))
	r := callResult(t, res, callAgents)
	assert.Equal(t, models.ShadowCallSkipped, r.Outcome)
	assert.Equal(t, reasonNoProjectKey, r.Message)
	assert.Empty(t, m.requests())
	assert.Empty(t, res.Findings)
}

func TestNoCredentialAtAll(t *testing.T) {
	res, m, _ := scanWith(t, func(c *sa.Connection) {
		c.Secret.ScopedKeys = nil
	}, nil)
	assert.Empty(t, m.requests())
	assert.Empty(t, res.Findings)
	assert.Equal(t, models.ShadowCallSkipped, callResult(t, res, callAgents).Outcome)
}

func TestScanBadEndpoint(t *testing.T) {
	// An unfilled placeholder cannot be expanded (an empty value would fall
	// back to the official endpoint, which tests must never call).
	res, m, _ := scanWith(t, func(c *sa.Connection) { c.Config.Endpoints = map[string]string{sa.EPOpenAIAPI: "https://{host}/v1"} }, nil)
	assert.Empty(t, m.requests())
	assert.Equal(t, models.ShadowCallFailed, callResult(t, res, callAgents).Outcome)
	assert.Empty(t, res.Findings)
}

func TestSameAgentInTwoProjectsIsOneFinding(t *testing.T) {
	res, _, _ := scanWith(t, nil, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v1/agents" && r.Header.Get("Authorization") == "Bearer "+betaKey {
			writeStatus(w, http.StatusOK, `{"object":"list","data":[{"id":"agent_alpha","name":"Copy","model":"m","tools":[]}],"has_more":false,"last_id":"agent_alpha"}`)
			return true
		}
		return false
	})
	assert.Equal(t, []string{"agent_alpha"}, findingIDs(res))
	assert.Equal(t, "Support agent", res.Findings[0].DisplayName, "the first listing wins")
}

func TestAgentsPageLimit(t *testing.T) {
	res, m, _ := scanWith(t, func(c *sa.Connection) { delete(c.Secret.ScopedKeys, "proj_beta") },
		func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/v1/agents" {
				return false
			}
			n := len(r.URL.Query().Get("after"))
			writeStatus(w, http.StatusOK, fmt.Sprintf(
				`{"object":"list","data":[{"id":"agent_%d","name":"a","model":"m","tools":[]}],"has_more":true,"last_id":"%s"}`,
				n, r.URL.Query().Get("after")+"x"))
			return true
		})
	assert.Len(t, m.requestsTo("/v1/agents"), maxListPages)
	r := callResult(t, res, callAgents+suffixPageLimit)
	assert.Equal(t, models.ShadowCallFailed, r.Outcome, "a cut listing is incomplete, so the call fails")
	assert.Contains(t, r.Message, "stopped after 100 pages")
	assert.Equal(t, models.ShadowCallOK, callResult(t, res, callAgents).Outcome)
	assert.Len(t, res.Findings, maxListPages, "what was read is kept")
}

func TestAgentsBetaRemovedContinues(t *testing.T) {
	m, srv := newMock(t)
	m.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v1/agents" && r.Header.Get("Authorization") == "Bearer "+betaKey {
			m.serveStatus(w, http.StatusBadRequest, "error_agents_beta.json")
			return true
		}
		return false
	}
	c, _ := newTestConnector(srv)
	res := c.Scan(context.Background(), testConn(srv.URL))
	r := callResult(t, res, callAgents)
	assert.Equal(t, models.ShadowCallFailed, r.Outcome)
	assert.Contains(t, r.Message, "invalid_beta")
	assert.Contains(t, r.Message, "1 of 2 scopes failed")
	assert.Equal(t, 1, r.Items)
	assert.Equal(t, []string{"agent_alpha"}, findingIDs(res))
}

func TestRateLimitRetry(t *testing.T) {
	hits := 0
	res, m, _ := scanWith(t, func(c *sa.Connection) { delete(c.Secret.ScopedKeys, "proj_beta") },
		func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/v1/agents" {
				hits++
				if hits == 1 {
					w.Header().Set("Retry-After", "0")
					writeStatus(w, http.StatusTooManyRequests, `{"error":{"message":"Rate limit reached","type":"rate_limit_error","param":null,"code":null}}`)
					return true
				}
			}
			return false
		})
	assert.Equal(t, models.ShadowCallOK, callResult(t, res, callAgents).Outcome)
	assert.Len(t, m.requestsTo("/v1/agents"), 2)
	assert.Equal(t, []string{"agent_alpha"}, findingIDs(res))
}

func TestMalformedJSON(t *testing.T) {
	res, _, _ := scanWith(t, nil, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.URL.Path == "/v1/agents" && r.Header.Get("Authorization") == "Bearer "+betaKey:
			writeStatus(w, http.StatusOK, `{"object":"list","first_id":null}`)
			return true
		}
		return false
	})
	r := callResult(t, res, callAgents)
	assert.Equal(t, models.ShadowCallUnsupported, r.Outcome, "no data list is a shape change")
	assert.Equal(t, []string{"agent_alpha"}, findingIDs(res))
}

func TestMCPOrigin(t *testing.T) {
	assert.Equal(t, "https://a.example", mcpOrigin(agentTool{ServerURL: "https://u:p@a.example/x?k=v"}))
	assert.Equal(t, "https://b.example", mcpOrigin(agentTool{Transport: json.RawMessage(`{"url":"https://b.example/sse?t=1"}`)}))
	assert.Equal(t, "https://c.example", mcpOrigin(agentTool{Transport: json.RawMessage(`{"server_url":"https://c.example/p"}`)}))
	assert.Equal(t, "https://d.example", mcpOrigin(agentTool{ConnectionOrigin: json.RawMessage(`"https://d.example/x"`)}))
	assert.Empty(t, mcpOrigin(agentTool{Transport: json.RawMessage(`"streamable_http"`), ConnectionOrigin: json.RawMessage(`"customer"`)}))
}

func TestRetention(t *testing.T) {
	res, _, logs := scanWith(t, nil, nil)
	b, err := json.Marshal(res)
	require.NoError(t, err)
	_, srv := newMock(t)
	c, testLogs := newTestConnector(srv)
	info := c.Test(context.Background(), testConn(srv.URL))
	ib, err := json.Marshal(info)
	require.NoError(t, err)
	for name, text := range map[string]string{
		"result": string(b), "scan logs": logs.String(), "test info": string(ib), "test logs": testLogs.String(),
	} {
		for _, bad := range []string{markerPrompt, markerKey, "token=", "api_key=", "user:pw", "/sse", "cred_"} {
			assert.NotContains(t, text, bad, "%s leaks %q", name, bad)
		}
	}
	assert.Contains(t, logs.String(), "Shadow agent discovery provider call", "debug logs were captured")
}

func checksByName(info models.ShadowConnectionTestInfo) map[string]models.ShadowConnectionCheck {
	out := map[string]models.ShadowConnectionCheck{}
	for _, ch := range info.Checks {
		out[ch.Name] = ch
	}
	return out
}

func TestConnectionTest(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		m, srv := newMock(t)
		c, _ := newTestConnector(srv)
		info := c.Test(context.Background(), testConn(srv.URL))
		assert.True(t, info.OK)
		names := checksByName(info)
		assert.Len(t, names, 2)
		for _, n := range []string{"Agents (project proj_alpha)", "Agents (project proj_beta)"} {
			require.Contains(t, names, n)
			assert.Equal(t, models.ShadowCallOK, names[n].Outcome, n)
		}
		assert.Equal(t, []string{sa.SigOpenAIAgentObject}, names["Agents (project proj_alpha)"].Unlocks)
		for _, r := range m.requests() {
			assert.Equal(t, "1", r.URL.Query().Get("limit"), r.URL.Path)
		}
		ag := m.requestsTo("/v1/agents")
		require.Len(t, ag, 2)
		for _, r := range ag {
			assert.Equal(t, "agents=v1", r.Header.Get("OpenAI-Beta"))
		}
	})
	t.Run("one project key rejected", func(t *testing.T) {
		m, srv := newMock(t)
		m.override = func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path == "/v1/agents" && r.Header.Get("Authorization") == "Bearer "+betaKey {
				writeStatus(w, http.StatusUnauthorized, `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","param":null,"code":"invalid_api_key"}}`)
				return true
			}
			return false
		}
		c, _ := newTestConnector(srv)
		info := c.Test(context.Background(), testConn(srv.URL))
		assert.False(t, info.OK, "a rejected project key fails the test")
		assert.Equal(t, models.ShadowCallAuthFailed, checksByName(info)["Agents (project proj_beta)"].Outcome)
		assert.Equal(t, models.ShadowCallOK, checksByName(info)["Agents (project proj_alpha)"].Outcome)
	})
	t.Run("no project key", func(t *testing.T) {
		m, srv := newMock(t)
		c, _ := newTestConnector(srv)
		conn := testConn(srv.URL)
		conn.Secret.ScopedKeys = nil
		info := c.Test(context.Background(), conn)
		assert.False(t, info.OK, "without a project key no agents can be listed")
		ch := checksByName(info)["Agents"]
		assert.Equal(t, models.ShadowCallSkipped, ch.Outcome)
		assert.Equal(t, reasonNoProjectKey, ch.Message)
		assert.Empty(t, m.requests())
	})
	t.Run("bad endpoint", func(t *testing.T) {
		_, srv := newMock(t)
		c, _ := newTestConnector(srv)
		conn := testConn(srv.URL)
		conn.Config.Endpoints = map[string]string{sa.EPOpenAIAPI: "https://{host}/v1"}
		info := c.Test(context.Background(), conn)
		assert.False(t, info.OK)
		require.Len(t, info.Checks, 1)
		assert.Equal(t, models.ShadowCallFailed, info.Checks[0].Outcome)
	})
}
