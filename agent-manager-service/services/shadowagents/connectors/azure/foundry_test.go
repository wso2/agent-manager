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

// Fixtures (testdata/) and the official example each follows
// (evidence/foundry.md, evidence/foundry_classic.md):
//   projects_aif1_page1.json      ListProjects.json (2025-06-01) shape plus the "AI Foundry API" endpoints key
//                                 (Learn quickstart; mock note in foundry.md Call 0)
//   projects_aif1_page2.json      ListProjects.json second item verbatim (no "AI Foundry API" key) and a project
//                                 whose endpoint is http (rejected)
//   agents_proj1_page1.json       v1 list envelope (data/first_id/last_id/has_more); item 2 is the AgentObject of
//                                 Agents_CreateVoiceAgent_ServerVad_Gen.json verbatim; item 1 the schema-constructed
//                                 prompt agent with MCP (foundry.md Call 1 mock note) plus secret headers/instructions
//   agents_proj1_page2.json       hosted (HostedAgentDefinition), external (ExternalAgentDefinition) and a
//                                 connected-model prompt agent ("<connection>/<model>", Learn connected models)
//   assistants_proj1_page1.json   the schema-constructed classic list of foundry_classic.md C1 (L-MIG object + MCP)
//   assistants_proj1_page2.json   ListAgents_MinimumSet_Gen.json item, with tools emptied and has_more false

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

func foundryConn(f *fakeAzure, classic bool) sa.Connection {
	conn := f.conn(models.ShadowProviderFoundry)
	conn.Config.IncludeClassicAgents = classic
	return conn
}

func codes(f *sa.Finding) []string {
	out := make([]string, 0, len(f.Signals))
	for _, s := range f.Signals {
		out = append(out, s.Code)
	}
	return out
}

func TestFoundryScan(t *testing.T) {
	f := newFakeAzure(t)
	var logs bytes.Buffer
	res := newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, true))

	for _, call := range []string{callSubscriptions, callAccounts, callProjects, callAgents, callAssistants} {
		r := callResult(res, call)
		require.NotNil(t, r, call)
		assert.Equal(t, models.ShadowCallOK, r.Outcome, "%s: %s", call, r.Message)
	}
	// Only the agent listings and the ARM calls that reach them are made.
	for _, r := range res.Calls {
		assert.NotContains(t, r.Call, "metrics", r.Call)
		assert.NotContains(t, r.Call, "responses", r.Call)
		assert.NotContains(t, r.Call, "deployments", r.Call)
	}
	// Projects: only proj1 has an https "AI Foundry API" endpoint.
	ep := callResult(res, callProjectEP)
	assert.Equal(t, models.ShadowCallUnsupported, ep.Outcome)
	assert.Equal(t, 1, ep.Items)
	assert.Contains(t, ep.Message, "2 of 3 scopes failed")
	assert.Equal(t, 2, f.count("projects:aif1"))
	assert.Equal(t, 2, f.count("agents:proj1"))
	assert.Equal(t, 2, f.count("assistants:proj1"))
	// Only the AIServices account is scanned for projects.
	assert.Equal(t, 0, f.count("projects:aoai1"))

	// 5 agents + 2 classic agents.
	assert.Len(t, res.Findings, 7)
	for _, fd := range res.Findings {
		assert.Equal(t, models.ShadowKindPlatformAgent, fd.Kind, fd.DisplayName)
		assert.Empty(t, fd.OwnerHint, "the listings do not say who created an agent")
		require.Len(t, fd.Signals, 1, fd.DisplayName)
		d, ok := sa.Def(fd.Signals[0].Code)
		if assert.True(t, ok, fd.Signals[0].Code) {
			assert.Equal(t, models.ShadowProviderFoundry, d.Provider)
		}
		// Every key is on the allow-list (empty values are dropped later, on storage).
		assert.Equal(t, fd.Attributes, sa.AllowedAttributes(fd.Attributes), "%s has a disallowed attribute", fd.DisplayName)
	}
	proj1 := f.srv.URL + "/api/projects/proj1"

	travel := findingByName(t, res, "travel-agent")
	assert.Equal(t, proj1+"/agents/agent_travel", travel.ExternalID)
	assert.Equal(t, aif1ID+"/projects/proj1", travel.Scope)
	assert.Equal(t, "enabled", travel.State)
	attrs := sa.SanitizeAttributes(travel.Attributes)
	assert.Equal(t, "prompt", attrs["agentKind"])
	assert.Equal(t, "gpt-4.1", attrs["model"])
	assert.Equal(t, "gpt-4.1", attrs["deployment"])
	assert.Equal(t, aif1ID, attrs["resourceId"])
	assert.Equal(t, "proj1", attrs["project"])
	assert.Equal(t, "3", attrs["agentVersion"])
	assert.Equal(t, "Books trips", attrs["description"])
	assert.Equal(t, []string{"book-trip"}, attrs["skills"])
	assert.Equal(t, []sa.ToolRef{{Type: "mcp", Name: "github"}, {Type: "mcp", Name: "jira"}, {Type: "function", Name: "get_weather"}, {Type: "code_interpreter"}}, attrs["tools"])
	assert.Equal(t, []sa.MCPServerRef{{Label: "github", Origin: "https://api.githubcopilot.com"}, {Label: "jira"}}, attrs["mcpServers"])
	assert.Equal(t, 4, attrs["toolCount"])
	assert.Equal(t, []string{sa.SigFoundryAgentObject}, codes(travel))
	obj := signal(travel, sa.SigFoundryAgentObject)
	assert.Equal(t, callAgents, obj.Source)
	assert.Equal(t, true, obj.Observed["enabled"])

	// Voice agent (official example): no tools.
	voice := findingByName(t, res, "server-vad-agent")
	assert.Equal(t, "voice", voice.Attributes["agentKind"])
	assert.NotContains(t, voice.Attributes, "tools")
	assert.Equal(t, []string{sa.SigFoundryAgentObject}, codes(voice))

	hosted := findingByName(t, res, "hosted-agent")
	assert.Equal(t, "disabled", hosted.State)
	assert.Equal(t, false, signal(hosted, sa.SigFoundryAgentObject).Observed["enabled"])
	assert.Equal(t, "myregistry.azurecr.io/hosted-agent:2", hosted.Attributes["image"])
	assert.Nil(t, hosted.Attributes["model"])

	ext := findingByName(t, res, "support-bot")
	assert.Equal(t, "external", ext.Attributes["agentKind"])
	assert.Equal(t, "support-bot-otel", ext.Attributes["otelAgentId"])
	assert.Nil(t, signal(ext, sa.SigFoundryPossibleDupOf), "cross-provider matching is the service's job")

	conn := findingByName(t, res, "connected-agent")
	assert.Equal(t, "otherconn/gpt-4o", conn.Attributes["model"])
	assert.Nil(t, conn.Attributes["deployment"], "a connected model is not a deployment of this account")
	assert.NotContains(t, conn.Attributes, "tools", "tools is empty")

	classic := findingByName(t, res, "my-agent")
	assert.Equal(t, proj1+"/assistants/asst_AVKrdr2KJthDnZiJ51nca1jy", classic.ExternalID)
	assert.Equal(t, "classic", classic.Attributes["agentKind"])
	assert.Equal(t, []string{sa.SigFoundryClassicAgent}, codes(classic))
	assert.Equal(t, callAssistants, classic.Signals[0].Source)
	assert.Equal(t, []sa.ToolRef{{Type: "code_interpreter"}, {Type: "mcp", Name: "github"}, {Type: "connected_agent", Name: "stock_price_bot"}, {Type: "function", Name: "lookup"}},
		classic.Attributes["tools"])
	require.NotNil(t, classic.ProviderCreatedAt)
	assert.Equal(t, time.Unix(1762218496, 0).UTC(), *classic.ProviderCreatedAt)
	// Classic page 2 without last_id: the last item's id is the cursor.
	reqs := f.requests("assistants:proj1")
	assert.Equal(t, "asst_AVKrdr2KJthDnZiJ51nca1jy", reqs[1].URL.Query().Get("after"))
	minimal := findingByName(t, res, "mldxpytbtdtrukktpciqtkcttk")
	assert.NotContains(t, minimal.Attributes, "tools")

	// Token scopes: ARM and the Foundry data plane.
	foundryScope := false
	for _, s := range f.scopes {
		foundryScope = foundryScope || strings.HasPrefix(s, sa.AzureFoundryScope)
	}
	assert.True(t, foundryScope, "a token for %s was requested", sa.AzureFoundryScope)
	assertNoMarkers(t, res, logs.String())
}

func TestFoundryRequestBuilding(t *testing.T) {
	f := newFakeAzure(t)
	var logs bytes.Buffer
	newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, true))

	p := f.requests("projects:aif1")[0]
	assert.Equal(t, aif1ID+"/projects", p.URL.Path)
	assert.Equal(t, "2025-06-01", p.URL.Query().Get("api-version"))
	assert.Equal(t, "Bearer "+armToken, p.Header.Get("Authorization"))

	a := f.requests("agents:proj1")
	q := a[0].URL.Query()
	assert.Equal(t, http.MethodGet, a[0].Method)
	assert.Equal(t, "v1", q.Get("api-version"))
	assert.Equal(t, "100", q.Get("limit"))
	assert.Equal(t, "asc", q.Get("order"))
	assert.Empty(t, q.Get("after"))
	assert.Empty(t, a[0].Header.Get("Foundry-Features"))
	assert.Equal(t, "Bearer "+aiToken, a[0].Header.Get("Authorization"))
	assert.Equal(t, "agent_server_vad", a[1].URL.Query().Get("after"))

	cl := f.requests("assistants:proj1")[0].URL.Query()
	assert.Equal(t, "v1", cl.Get("api-version"))
	assert.Equal(t, "asc", cl.Get("order"))
}

func TestFoundryClassicOff(t *testing.T) {
	f := newFakeAzure(t)
	f.forbidden["assistants:proj1"] = true
	var logs bytes.Buffer
	res := newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, false))
	assert.Len(t, res.Findings, 5)
	r := callResult(res, callAssistants)
	require.NotNil(t, r)
	assert.Equal(t, models.ShadowCallSkipped, r.Outcome)
	assert.Equal(t, "classic agents are not included in this connection", r.Message)
	for _, fd := range res.Findings {
		assert.Nil(t, signal(fd, sa.SigFoundryClassicAgent), fd.DisplayName)
	}
}

func TestFoundryClassicUnavailable(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		f := newFakeAzure(t)
		f.override["assistants:proj1"] = func(w http.ResponseWriter, _ *http.Request) bool {
			writeJSON(w, status, `{"error":{"code":"NotFound","message":"classic retired"}}`)
			return true
		}
		var logs bytes.Buffer
		res := newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, true))
		r := callResult(res, callAssistants)
		require.NotNil(t, r)
		assert.Equal(t, models.ShadowCallSkipped, r.Outcome)
		assert.Equal(t, "classic agents not available", r.Message)
		assert.Len(t, res.Findings, 5)
	}
}

func TestFoundryProjectEndpoint(t *testing.T) {
	https := mustURL(t, "https://management.azure.com")
	httpARM := mustURL(t, "http://localhost:8080")
	p := func(eps map[string]string) project {
		var pr project
		pr.Properties.Endpoints = eps
		return pr
	}
	got, err := projectEndpoint(p(map[string]string{"AI Foundry API": "https://a.services.ai.azure.com/api/projects/p/?x=1"}), https)
	require.NoError(t, err)
	assert.Equal(t, "https://a.services.ai.azure.com/api/projects/p", got)
	_, err = projectEndpoint(p(map[string]string{"ai foundry api": "https://a/api/projects/p"}), https)
	require.NoError(t, err, "key matched case-insensitively")
	_, err = projectEndpoint(p(map[string]string{"OpenAI Dall-E API": "https://a"}), https)
	assert.Equal(t, models.ShadowCallNotFound, sa.OutcomeOf(err))
	_, err = projectEndpoint(p(map[string]string{"AI Foundry API": "http://a/api/projects/p"}), https)
	assert.Equal(t, models.ShadowCallUnsupported, sa.OutcomeOf(err))
	_, err = projectEndpoint(p(map[string]string{"AI Foundry API": "http://a/api/projects/p"}), httpARM)
	require.NoError(t, err, "http allowed only when the configured ARM endpoint is http")
	_, err = projectEndpoint(p(map[string]string{"AI Foundry API": "https://user:pw@a/api/projects/p"}), https)
	assert.Error(t, err)
}

func TestFoundryErrors(t *testing.T) {
	t.Run("data plane 401 stops the data-plane calls", func(t *testing.T) {
		f := newFakeAzure(t)
		f.override["agents:proj1"] = func(w http.ResponseWriter, _ *http.Request) bool {
			writeJSON(w, http.StatusUnauthorized, `{"error":{"code":"PermissionDenied","message":"no"}}`)
			return true
		}
		f.forbidden["assistants:proj1"] = true
		var logs bytes.Buffer
		res := newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, true))
		assert.Equal(t, models.ShadowCallAuthFailed, callResult(res, callAgents).Outcome)
		assert.Empty(t, res.Findings)
		assert.NotNil(t, callResult(res, callAgents+" (remaining projects)"))
	})
	t.Run("403 on agents, classic still runs", func(t *testing.T) {
		f := newFakeAzure(t)
		f.override["agents:proj1"] = func(w http.ResponseWriter, _ *http.Request) bool {
			writeJSON(w, http.StatusForbidden, `{"error":{"code":"PermissionDenied","message":"agents/read missing"}}`)
			return true
		}
		var logs bytes.Buffer
		res := newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, true))
		assert.Equal(t, models.ShadowCallPermissionDenied, callResult(res, callAgents).Outcome)
		assert.Len(t, res.Findings, 2)
		findingByName(t, res, "my-agent")
	})
	t.Run("malformed agents page", func(t *testing.T) {
		f := newFakeAzure(t)
		f.override["agents:proj1"] = func(w http.ResponseWriter, _ *http.Request) bool {
			writeJSON(w, http.StatusOK, `{"data": {"not": "a list"}, "has_more": false}`)
			return true
		}
		var logs bytes.Buffer
		res := newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, false))
		assert.Equal(t, models.ShadowCallUnsupported, callResult(res, callAgents).Outcome)
	})
	t.Run("429 then 200 on agents", func(t *testing.T) {
		f := newFakeAzure(t)
		n := 0
		f.override["agents:proj1"] = func(w http.ResponseWriter, _ *http.Request) bool {
			n++
			if n == 1 {
				w.Header().Set("Retry-After", "0")
				writeJSON(w, http.StatusTooManyRequests, `{"error":{"code":"rate_limit_exceeded","message":"slow down"}}`)
				return true
			}
			return false
		}
		var logs bytes.Buffer
		res := newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, false))
		assert.Equal(t, models.ShadowCallOK, callResult(res, callAgents).Outcome)
		assert.Len(t, res.Findings, 5)
	})
	t.Run("401 on the first ARM call stops the scan", func(t *testing.T) {
		f := newFakeAzure(t)
		f.override["subscriptions"] = func(w http.ResponseWriter, _ *http.Request) bool {
			writeJSON(w, http.StatusUnauthorized, `{"error":{"code":"InvalidAuthenticationToken","message":"bad"}}`)
			return true
		}
		f.forbidden["accounts"] = true
		var logs bytes.Buffer
		res := newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, false))
		assert.Equal(t, models.ShadowCallAuthFailed, callResult(res, callSubscriptions).Outcome)
		assert.Empty(t, res.Findings)
	})
	t.Run("sign-in fails", func(t *testing.T) {
		f := newFakeAzure(t)
		f.tokenFail = true
		var logs bytes.Buffer
		res := newTestFoundry(f, &logs).Scan(t.Context(), foundryConn(f, true))
		assert.Empty(t, res.Findings)
		for _, r := range res.Calls {
			assert.NotEqual(t, models.ShadowCallOK, r.Outcome, r.Call)
		}
		assertNoMarkers(t, res, logs.String())
	})
}

func TestFoundryTest(t *testing.T) {
	checkNames := func(info models.ShadowConnectionTestInfo) []string {
		var out []string
		for _, ch := range info.Checks {
			out = append(out, ch.Name)
		}
		return out
	}
	t.Run("all checks pass", func(t *testing.T) {
		f := newFakeAzure(t)
		var logs bytes.Buffer
		info := newTestFoundry(f, &logs).Test(t.Context(), foundryConn(f, true))
		assert.True(t, info.OK)
		assert.Equal(t, []string{checkSignIn, checkAccounts, checkProjects, checkAgents, checkClassic}, checkNames(info))
		for _, ch := range info.Checks {
			assert.Equal(t, models.ShadowCallOK, ch.Outcome, "%s: %s", ch.Name, ch.Message)
			for _, u := range ch.Unlocks {
				_, ok := sa.Def(u)
				assert.True(t, ok, "%s unlocks %s, an evidence code", ch.Name, u)
			}
		}
		assert.Equal(t, "1", f.requests("agents:proj1")[0].URL.Query().Get("limit"))
		assert.Equal(t, "1", f.requests("assistants:proj1")[0].URL.Query().Get("limit"))
		assertNoMarkers(t, info, logs.String())
	})
	t.Run("classic off: no classic check", func(t *testing.T) {
		f := newFakeAzure(t)
		f.forbidden["assistants:proj1"] = true
		var logs bytes.Buffer
		info := newTestFoundry(f, &logs).Test(t.Context(), foundryConn(f, false))
		assert.True(t, info.OK)
		assert.NotContains(t, checkNames(info), checkClassic)
	})
	t.Run("agents denied", func(t *testing.T) {
		f := newFakeAzure(t)
		f.override["agents:proj1"] = func(w http.ResponseWriter, _ *http.Request) bool {
			writeJSON(w, http.StatusForbidden, `{"error":{"code":"PermissionDenied","message":"agents/read missing"}}`)
			return true
		}
		var logs bytes.Buffer
		info := newTestFoundry(f, &logs).Test(t.Context(), foundryConn(f, false))
		assert.False(t, info.OK)
		found := false
		for _, ch := range info.Checks {
			if ch.Name == checkAgents {
				found = true
				assert.Equal(t, models.ShadowCallPermissionDenied, ch.Outcome)
				assert.Equal(t, []string{sa.SigFoundryAgentObject}, ch.Unlocks)
			}
		}
		assert.True(t, found)
	})
	t.Run("classic unavailable is skipped", func(t *testing.T) {
		f := newFakeAzure(t)
		f.override["assistants:proj1"] = func(w http.ResponseWriter, _ *http.Request) bool {
			writeJSON(w, http.StatusGone, `{"error":{"code":"Gone","message":"retired"}}`)
			return true
		}
		var logs bytes.Buffer
		info := newTestFoundry(f, &logs).Test(t.Context(), foundryConn(f, true))
		assert.True(t, info.OK)
		for _, ch := range info.Checks {
			if ch.Name == checkClassic {
				assert.Equal(t, models.ShadowCallSkipped, ch.Outcome)
				assert.Equal(t, "classic agents not available", ch.Message)
			}
		}
	})
	t.Run("sign-in fails", func(t *testing.T) {
		f := newFakeAzure(t)
		f.tokenFail = true
		var logs bytes.Buffer
		info := newTestFoundry(f, &logs).Test(t.Context(), foundryConn(f, true))
		assert.False(t, info.OK)
		assert.Equal(t, models.ShadowCallAuthFailed, info.Checks[0].Outcome)
		assert.ElementsMatch(t, []string{sa.SigFoundryAgentObject, sa.SigFoundryClassicAgent}, info.Checks[0].Unlocks)
	})
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

// stubGetter serves the same JSON body for every URL.
type stubGetter struct {
	body string
	urls []string
}

func (s *stubGetter) GetJSON(_ context.Context, u string, out any) error {
	s.urls = append(s.urls, u)
	return json.Unmarshal([]byte(s.body), out)
}

func TestCursorListBounds(t *testing.T) {
	// has_more forever with a new cursor each time: stopped at the page limit.
	g := &stubGetter{body: `{"data":[{"id":"a"}],"has_more":true}`}
	n := 0
	_, stop, err := cursorList(t.Context(), &dataPlane{client: g}, "https://p", "/agents", nil, 3,
		func(a agentObject) string { n++; return fmt.Sprintf("c%d", n) },
		func(agentObject) bool { return true })
	require.NoError(t, err)
	assert.Contains(t, stop, "stopped after 3 pages")
	assert.Len(t, g.urls, 3)

	// has_more without any cursor: stopped, never loops on the same page.
	g = &stubGetter{body: `{"data":[{"name":"x"}],"has_more":true}`}
	_, stop, err = cursorList(t.Context(), &dataPlane{client: g}, "https://p", "/agents", nil, 10,
		func(a agentObject) string { return a.ID },
		func(agentObject) bool { return true })
	require.NoError(t, err)
	assert.Contains(t, stop, "no cursor")
	assert.Len(t, g.urls, 1)
}

func TestARMListPageLimit(t *testing.T) {
	g := &stubGetter{body: `{"value":[{"subscriptionId":"s","state":"Enabled"}],"nextLink":"https://arm.example.com/subscriptions?$skiptoken=x"}`}
	arm := &armClient{client: g, base: mustURL(t, "https://arm.example.com")}
	n, stop, err := armList(t.Context(), arm, arm.url("/subscriptions"), 4, func(subscription) {})
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Contains(t, stop, "stopped after 4 pages")
}
