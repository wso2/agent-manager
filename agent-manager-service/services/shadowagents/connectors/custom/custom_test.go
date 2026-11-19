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

package custom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

const testKey = "inv-key-1234567890"

// noRetryClient is the production client; tests use it as is.
func newConnector() *Connector {
	return New(sa.NewHTTPClient(nil), nil).(*Connector)
}

func conn(u, key string) sa.Connection {
	return sa.Connection{
		ID: uuid.New(), OUID: "org", Name: "inventory", Provider: models.ShadowProviderCustom,
		Config: models.ShadowConnectionConfig{AgentsURL: u},
		Secret: models.ShadowConnectionSecret{APIKey: key},
	}
}

// inventory serves pages of agents, recording each request's query.
type inventory struct {
	pages   []string // JSON bodies, in order; page i answers cursor "c<i>"
	queries []string
	auth    []string
}

func (inv *inventory) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/agents", r.URL.Path)
		inv.queries = append(inv.queries, r.URL.RawQuery)
		inv.auth = append(inv.auth, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"invalid API key"}`))
			return
		}
		i := 0
		if c := r.URL.Query().Get("cursor"); c != "" {
			n, err := strconv.Atoi(strings.TrimPrefix(c, "c"))
			require.NoError(t, err)
			i = n
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(inv.pages[i]))
	}
}

func TestScanReadsEveryPageWithTheContractFields(t *testing.T) {
	inv := &inventory{pages: []string{
		`{"agents":[
			{"id":"inv-1","name":"Invoice triage","description":"Sorts invoices","source":"LangGraph Platform",
			 "url":"https://user:pw@agents.example.com/invoice?token=secret#frag","framework":"LangGraph","model":"gpt-4.1",
			 "version":"1.4.0","status":"running","scope":"finance","owner":{"name":"Jane Doe","email":"jane@example.com"},
			 "createdAt":"2026-05-04T09:30:00Z","labels":{"team":"finance"},
			 "instructions":"You are a secret prompt","apiKey":"sk-should-never-be-read"},
			{"id":"inv-2","name":""},
			{"id":"","name":"No id"},
			{"id":"inv-3","name":"Wrong type","version":7},
			{"id":"inv-1","name":"Repeated"}
		],"nextCursor":"c1"}`,
		`{"agents":[{"id":"inv-4","name":"Support assistant"}],"nextCursor":null}`,
	}}
	srv := httptest.NewServer(inv.handler(t))
	defer srv.Close()

	res := newConnector().Scan(context.Background(), conn(srv.URL+"/v1/agents", testKey))

	assert.Equal(t, []string{"limit=100", "cursor=c1&limit=100"}, inv.queries, "limit on every page, then the cursor")
	assert.Equal(t, []string{"Bearer " + testKey, "Bearer " + testKey}, inv.auth)
	require.Len(t, res.Findings, 2)
	byID := map[string]*sa.Finding{}
	for _, f := range res.Findings {
		byID[f.ExternalID] = f
	}
	f := byID["inv-1"]
	require.NotNil(t, f)
	assert.Equal(t, models.ShadowKindPlatformAgent, f.Kind)
	assert.Equal(t, "Invoice triage", f.DisplayName, "the first occurrence of a repeated id")
	assert.Equal(t, "finance", f.Scope)
	assert.Equal(t, "running", f.State)
	assert.Equal(t, "Jane Doe", f.OwnerHint.Name)
	assert.Equal(t, "jane@example.com", f.OwnerHint.Email)
	require.NotNil(t, f.ProviderCreatedAt)
	assert.Equal(t, "2026-05-04T09:30:00Z", f.ProviderCreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	assert.Equal(t, map[string]any{
		"description": "Sorts invoices", "source": "LangGraph Platform",
		"agentUrl": "https://agents.example.com/invoice", "framework": "LangGraph", "model": "gpt-4.1",
		"agentVersion": "1.4.0", "labels": map[string]string{"team": "finance"},
	}, f.Attributes, "only the contract's fields; the URL without user info, query and fragment")
	stored, _ := json.Marshal(res.Findings)
	assert.NotContains(t, string(stored), "secret prompt")
	assert.NotContains(t, string(stored), "sk-should-never-be-read")
	assert.NotContains(t, string(stored), "token=secret")
	require.Len(t, f.Signals, 1)
	assert.Equal(t, sa.SigCustomAgent, f.Signals[0].Code)
	assert.Equal(t, callAgents, f.Signals[0].Source)
	assert.Equal(t, "Support assistant", byID["inv-4"].DisplayName)

	require.Len(t, res.Calls, 1)
	call := res.Calls[0]
	assert.Equal(t, callAgents, call.Call)
	assert.Equal(t, models.ShadowCallOK, call.Outcome)
	assert.Equal(t, 2, call.Items)
	assert.Contains(t, call.Message, "3 agent(s) left out")
	assert.Contains(t, call.Message, "1 repeated agent id(s) listed once")
}

func TestScanReportsTheServersError(t *testing.T) {
	inv := &inventory{pages: []string{`{"agents":[]}`}}
	srv := httptest.NewServer(inv.handler(t))
	defer srv.Close()

	res := newConnector().Scan(context.Background(), conn(srv.URL+"/v1/agents", "wrong-key"))
	assert.Empty(t, res.Findings)
	require.Len(t, res.Calls, 1)
	assert.Equal(t, models.ShadowCallAuthFailed, res.Calls[0].Outcome)
	assert.Contains(t, res.Calls[0].Message, "invalid API key")
	assert.Len(t, inv.queries, 1, "a 401 is not retried")
}

func TestScanRefusesAResponseWithoutAgents(t *testing.T) {
	inv := &inventory{pages: []string{`{"items":[]}`}}
	srv := httptest.NewServer(inv.handler(t))
	defer srv.Close()

	res := newConnector().Scan(context.Background(), conn(srv.URL+"/v1/agents", testKey))
	require.Len(t, res.Calls, 1)
	assert.Equal(t, models.ShadowCallUnsupported, res.Calls[0].Outcome)
	assert.Contains(t, res.Calls[0].Message, "no agents list")
}

func TestScanStopsWhenTheCursorDoesNotChange(t *testing.T) {
	inv := &inventory{pages: []string{
		`{"agents":[{"id":"a","name":"A"}],"nextCursor":"c1"}`,
		`{"agents":[{"id":"b","name":"B"}],"nextCursor":"c1"}`,
	}}
	srv := httptest.NewServer(inv.handler(t))
	defer srv.Close()

	res := newConnector().Scan(context.Background(), conn(srv.URL+"/v1/agents", testKey))
	assert.Len(t, inv.queries, 2)
	assert.Len(t, res.Findings, 2, "the agents read so far are kept")
	require.Len(t, res.Calls, 1)
	assert.Equal(t, models.ShadowCallUnsupported, res.Calls[0].Outcome)
	assert.Contains(t, res.Calls[0].Message, "nextCursor did not change")
}

func TestScanStopsAtThePageLimit(t *testing.T) {
	pages := make([]string, maxPages+1)
	for i := range pages {
		pages[i] = `{"agents":[{"id":"a` + strconv.Itoa(i) + `","name":"A"}],"nextCursor":"c` + strconv.Itoa(i+1) + `"}`
	}
	inv := &inventory{pages: pages}
	srv := httptest.NewServer(inv.handler(t))
	defer srv.Close()

	res := newConnector().Scan(context.Background(), conn(srv.URL+"/v1/agents", testKey))
	assert.Len(t, inv.queries, maxPages, "at most 100 pages")
	assert.Len(t, res.Findings, maxPages)
	require.Len(t, res.Calls, 2)
	assert.Equal(t, models.ShadowCallOK, res.Calls[0].Outcome)
	assert.Equal(t, callAgents+suffixPageLimit, res.Calls[1].Call)
	assert.Equal(t, models.ShadowCallFailed, res.Calls[1].Outcome, "an incomplete listing is a failed call")
}

func TestScanWithoutAKeyOrWithABadURLCallsNothing(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()

	res := newConnector().Scan(context.Background(), conn(srv.URL+"/v1/agents", " "))
	require.Len(t, res.Calls, 1)
	assert.Equal(t, models.ShadowCallSkipped, res.Calls[0].Outcome)

	res = newConnector().Scan(context.Background(), conn(srv.URL+"/v1/agents?x=1", testKey))
	require.Len(t, res.Calls, 1)
	assert.Equal(t, models.ShadowCallFailed, res.Calls[0].Outcome)
	assert.False(t, called)
}

func TestConnectionTestReadsOneAgent(t *testing.T) {
	inv := &inventory{pages: []string{`{"agents":[{"id":"a","name":"A"}],"nextCursor":"c1"}`}}
	srv := httptest.NewServer(inv.handler(t))
	defer srv.Close()
	c := newConnector()

	info := c.Test(context.Background(), conn(srv.URL+"/v1/agents", testKey))
	assert.True(t, info.OK)
	require.Len(t, info.Checks, 1)
	assert.Equal(t, models.ShadowCallOK, info.Checks[0].Outcome)
	assert.Equal(t, []string{sa.SigCustomAgent}, info.Checks[0].Unlocks)
	assert.Equal(t, []string{"limit=1"}, inv.queries)

	info = c.Test(context.Background(), conn(srv.URL+"/v1/agents", "wrong"))
	assert.False(t, info.OK)
	assert.Equal(t, models.ShadowCallAuthFailed, info.Checks[0].Outcome)

	info = c.Test(context.Background(), conn(srv.URL+"/v1/agents", ""))
	assert.False(t, info.OK)
	assert.Equal(t, models.ShadowCallSkipped, info.Checks[0].Outcome)
}

func TestValidateAgentsURL(t *testing.T) {
	for _, ok := range []string{"https://agents.example.com/v1/agents", "http://host.docker.internal:8085/agents", "https://10.0.0.5"} {
		assert.NoError(t, sa.ValidateAgentsURL(ok), ok)
	}
	for _, bad := range []string{
		"", "agents.example.com", "ftp://agents.example.com", "https://agents.example.com/a?x=1",
		"https://agents.example.com/a?", "https://agents.example.com/a#f", "https://u:p@agents.example.com/a",
		"https://" + strings.Repeat("a", 2100) + ".com",
	} {
		assert.Error(t, sa.ValidateAgentsURL(bad), bad)
	}
}

// The downloadable contract and the decoder must not drift: the agent, owner
// and page properties, the required fields, the query parameters, the bearer
// scheme and the documented bounds are checked against the code.
func TestContractMatchesTheConnector(t *testing.T) {
	var doc struct {
		OpenAPI string `yaml:"openapi"`
		Info    struct {
			Description string `yaml:"description"`
		} `yaml:"info"`
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name     string `yaml:"name"`
				In       string `yaml:"in"`
				Required bool   `yaml:"required"`
				Schema   struct {
					Minimum int `yaml:"minimum"`
					Maximum int `yaml:"maximum"`
				} `yaml:"schema"`
			} `yaml:"parameters"`
			Security []map[string][]string `yaml:"security"`
		} `yaml:"paths"`
		Components struct {
			SecuritySchemes map[string]struct {
				Type   string `yaml:"type"`
				Scheme string `yaml:"scheme"`
			} `yaml:"securitySchemes"`
			Schemas map[string]struct {
				Required   []string               `yaml:"required"`
				Properties map[string]interface{} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(sa.CustomPlatformOpenAPI), &doc))
	assert.Equal(t, "3.0.3", doc.OpenAPI)

	props := func(schema string) []string {
		s, ok := doc.Components.Schemas[schema]
		require.True(t, ok, schema)
		out := make([]string, 0, len(s.Properties))
		for k := range s.Properties {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	assert.Equal(t, jsonTags(agent{}), props("Agent"))
	assert.Equal(t, jsonTags(owner{}), props("Owner"))
	assert.Equal(t, jsonTags(agentPage{}), props("AgentPage"))
	assert.Equal(t, []string{"id", "name"}, doc.Components.Schemas["Agent"].Required)
	assert.Equal(t, []string{"agents"}, doc.Components.Schemas["AgentPage"].Required)

	require.Len(t, doc.Paths, 1)
	get, ok := doc.Paths["/agents"]["get"]
	require.True(t, ok)
	params := map[string]bool{}
	for _, p := range get.Parameters {
		assert.Equal(t, "query", p.In, p.Name)
		params[p.Name] = p.Required
		if p.Name == "limit" {
			assert.Equal(t, 1, p.Schema.Minimum)
			assert.Equal(t, pageSize, p.Schema.Maximum)
		}
	}
	assert.Equal(t, map[string]bool{"limit": true, "cursor": false}, params)
	require.Len(t, get.Security, 1)
	for name := range get.Security[0] {
		s := doc.Components.SecuritySchemes[name]
		assert.Equal(t, "http", s.Type)
		assert.Equal(t, "bearer", s.Scheme)
	}
	assert.Contains(t, doc.Info.Description, "at most "+strconv.Itoa(maxPages)+" pages")
	assert.Contains(t, doc.Info.Description, "Authorization: Bearer <API key>")
}

func jsonTags(v any) []string {
	tp := reflect.TypeOf(v)
	out := make([]string, 0, tp.NumField())
	for i := 0; i < tp.NumField(); i++ {
		out = append(out, strings.Split(tp.Field(i).Tag.Get("json"), ",")[0])
	}
	sort.Strings(out)
	return out
}
