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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

const shadowTestAdminKey = "sk-admin-SHADOWTESTSECRET-0123456789"

// Project keys of the fake: each lists its own project's agents.
var shadowTestProjectKeys = map[string]string{
	"sk-proj-SHADOWTESTSECRET-alpha-0123": "proj_alpha",
	"sk-proj-SHADOWTESTSECRET-beta-45678": "proj_beta",
}

const (
	shadowAlphaKey = "sk-proj-SHADOWTESTSECRET-alpha-0123"
	shadowBetaKey  = "sk-proj-SHADOWTESTSECRET-beta-45678"
)

// fakeOpenAI serves the OpenAI connector's fixtures (official response
// shapes, see the connector's testdata): GET /v1/agents with a project key;
// page 2 of a list when the request carries a cursor. Any other call fails
// the test.
func fakeOpenAI(t *testing.T) *httptest.Server {
	t.Helper()
	dir := filepath.Join("..", "services", "shadowagents", "connectors", "openai", "testdata")
	serve := func(w http.ResponseWriter, name string) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"not found","type":"invalid_request_error"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}
	unauthorized := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`))
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := "1"
		if r.URL.Query().Get("after") != "" {
			page = "2"
		}
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch r.URL.Path {
		case "/v1/agents":
			proj, ok := shadowTestProjectKeys[auth]
			if !ok {
				unauthorized(w)
				return
			}
			if r.Header.Get("OpenAI-Beta") != "agents=v1" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			serve(w, "agents."+proj+"."+page+".json")
		default:
			t.Errorf("unexpected OpenAI call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

type shadowAPI struct {
	t   *testing.T
	app http.Handler
	org string
}

func (a shadowAPI) do(method, path string, body any) (int, []byte) {
	a.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(a.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, fmt.Sprintf("/api/v1/orgs/%s/shadow-agents%s", a.org, path), &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	a.app.ServeHTTP(rr, req)
	return rr.Code, rr.Body.Bytes()
}

func decodeInto[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(b, &v), string(b))
	return v
}

// runShadowScans runs the queued scans the way the worker does.
func runShadowScans(t *testing.T, ctx context.Context, logs *bytes.Buffer) {
	t.Helper()
	key, err := hex.DecodeString(config.GetConfig().EncryptionKey)
	require.NoError(t, err)
	repo := repositories.NewShadowAgentRepository(db.GetDB())
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	services.NewShadowAgentScanner(repo, wiring.ProvideShadowConnectors(logger), key, logger).RunOnce(ctx)
}

func (a shadowAPI) agents(query string) spec.ShadowFindingListResponse {
	a.t.Helper()
	code, body := a.do(http.MethodGet, "/findings"+query, nil)
	require.Equal(a.t, http.StatusOK, code, string(body))
	return decodeInto[spec.ShadowFindingListResponse](a.t, body)
}

func byExternalID(list spec.ShadowFindingListResponse) map[string]spec.ShadowFindingSummary {
	out := map[string]spec.ShadowFindingSummary{}
	for _, f := range list.Findings {
		out[f.ExternalId] = f
	}
	return out
}

func TestShadowAgentDiscoveryAPI_Catalog(t *testing.T) {
	ouID := "ou-" + uuid.NewString()
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: apitestutils.CreateMockOpenChoreoClient()},
		jwtassertion.NewMockMiddlewareWithOUID(t, ouID))
	api := shadowAPI{t: t, app: app, org: "shadow-org-" + uuid.NewString()[:6]}

	code, body := api.do(http.MethodGet, "/providers", nil)
	require.Equal(t, http.StatusOK, code, string(body))
	assert.NotContains(t, string(body), `"scoring"`)
	assert.NotContains(t, string(body), `"weight"`)
	providers := decodeInto[spec.ShadowProviderListResponse](t, body)
	var names []string
	for _, p := range providers.Providers {
		names = append(names, p.Provider)
		assert.NotEmpty(t, p.Capabilities, p.Provider)
		switch p.Provider {
		case "openai", "anthropic":
			assert.Nil(t, p.CredentialChoice, p.Provider)
			for _, f := range p.Fields {
				switch f.Key {
				case "scopedKeys":
					assert.True(t, f.Required, p.Provider)
				case "apiKey":
					assert.False(t, f.Required, p.Provider)
				default:
					t.Errorf("%s has an unexpected field %s", p.Provider, f.Key)
				}
			}
		case "bedrock":
			require.NotNil(t, p.CredentialChoice, "Bedrock needs at least one service")
			assert.Equal(t, []string{"scanAgentCore", "scanAgentRegistry", "scanClassicAgents"}, p.CredentialChoice.Fields)
		}
	}
	assert.Equal(t, []string{"openai", "anthropic", "bedrock", "google", "foundry", "github", "azure_devops"}, names)
	var codes []string
	for _, d := range providers.Signals {
		codes = append(codes, d.Code)
		assert.NotEmpty(t, d.Condition, d.Code)
	}
	assert.Equal(t, []string{
		"openai.agent_object", "anthropic.managed_agent",
		"bedrock.agentcore_runtime", "bedrock.registry_agent_record", "bedrock.classic_agent",
		"google.agent_runtime_engine", "google.registry_agent",
		"foundry.agent_object", "foundry.classic_agent", "foundry.external_possible_duplicate",
	}, codes)
}

// TestShadowAgentDiscoveryAPI covers the whole flow against a fake OpenAI:
// platforms, tests, scans of one platform and of all, the deduplicated
// unregistered agents with the platform filter, review status, agents no
// longer listed, clearing scans, retention and isolation.
func TestShadowAgentDiscoveryAPI(t *testing.T) {
	ctx := context.Background()
	org := "shadow-org-" + uuid.NewString()[:6]
	ouID := "ou-" + uuid.NewString()
	oc := apitestutils.CreateMockOpenChoreoClient()
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc},
		jwtassertion.NewMockMiddlewareWithOUID(t, ouID))
	api := shadowAPI{t: t, app: app, org: org}
	fake := fakeOpenAI(t)
	defer fake.Close()
	logs := &bytes.Buffer{}

	// A project key is required, and there is no Admin key setting: an Admin
	// key cannot list agents.
	code, body := api.do(http.MethodPost, "/connections", map[string]any{
		"name": "no key", "provider": "openai", "endpoints": map[string]string{"api": fake.URL + "/v1"},
		"settings": map[string]any{},
	})
	require.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), "Project keys needs at least one ID and key")
	code, body = api.do(http.MethodPost, "/connections", map[string]any{
		"name": "admin", "provider": "openai", "endpoints": map[string]string{"api": fake.URL + "/v1"},
		"settings": map[string]any{"apiKey": shadowTestAdminKey, "scopedKeys": map[string]any{"proj_alpha": shadowAlphaKey}},
	})
	require.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(body), `unknown setting \"apiKey\"`)
	// Removed settings are refused.
	code, _ = api.do(http.MethodPost, "/connections", map[string]any{
		"name": "deep", "provider": "openai", "endpoints": map[string]string{"api": fake.URL + "/v1"},
		"settings": map[string]any{"scopedKeys": map[string]any{"proj_alpha": shadowAlphaKey}, "deepInspection": true},
	})
	assert.Equal(t, http.StatusBadRequest, code)

	// Platform 1: both projects.
	connReq := map[string]any{
		"name": "OpenAI org", "provider": "openai", "endpoints": map[string]string{"api": fake.URL + "/v1"},
		"settings": map[string]any{
			"scopedKeys": map[string]any{"proj_alpha": shadowAlphaKey, "proj_beta": shadowBetaKey},
		},
	}
	code, body = api.do(http.MethodPost, "/connections", connReq)
	require.Equal(t, http.StatusCreated, code, string(body))
	assert.NotContains(t, string(body), "SHADOWTESTSECRET", "a secret is never returned")
	conn1 := decodeInto[spec.ShadowConnectionResponse](t, body)
	assert.NotContains(t, conn1.SecretHints, "apiKey")
	assert.Contains(t, conn1.SecretHints, "scopedKeys.proj_alpha")
	assert.Contains(t, conn1.SecretHints, "scopedKeys.proj_beta")
	require.Len(t, conn1.EndpointWarnings, 1, "a local endpoint is not an official host")
	code, _ = api.do(http.MethodPost, "/connections", connReq)
	assert.Equal(t, http.StatusConflict, code, "duplicate name")
	var stored models.ShadowConnection
	require.NoError(t, db.GetDB().WithContext(ctx).Where("id = ?", conn1.Id).First(&stored).Error)
	assert.NotContains(t, string(stored.SecretEncrypted), "SHADOWTESTSECRET", "the credential is stored encrypted")

	// Test it: one agent listing per project key, nothing else.
	code, body = api.do(http.MethodPost, "/connections/"+conn1.Id+"/test", nil)
	require.Equal(t, http.StatusOK, code, string(body))
	test := decodeInto[spec.ShadowConnectionTest](t, body)
	assert.True(t, test.Ok, string(body))
	var checks []string
	for _, ch := range test.Checks {
		checks = append(checks, ch.Name)
		assert.Equal(t, "ok", ch.Outcome, ch.Name)
	}
	assert.Equal(t, []string{"Agents (project proj_alpha)", "Agents (project proj_beta)"}, checks)

	// Scan this platform only.
	code, body = api.do(http.MethodPost, "/scans", map[string]any{"connectionIds": []string{conn1.Id}})
	require.Equal(t, http.StatusAccepted, code, string(body))
	scan1 := decodeInto[spec.ShadowScanResponse](t, body)
	assert.Equal(t, []string{conn1.Id}, scan1.ConnectionIds)
	assert.NotContains(t, string(body), "lookbackDays")
	code, _ = api.do(http.MethodPost, "/scans", map[string]any{})
	assert.Equal(t, http.StatusConflict, code, "one active scan per org")
	runShadowScans(t, ctx, logs)
	code, body = api.do(http.MethodGet, "/scans/"+scan1.Id, nil)
	require.Equal(t, http.StatusOK, code)
	scan1 = decodeInto[spec.ShadowScanResponse](t, body)
	require.Equal(t, "succeeded", scan1.Status, string(body))
	require.Len(t, scan1.Results, 1)
	assert.EqualValues(t, 3, scan1.Results[0].FindingsCount)
	for _, c := range scan1.Results[0].Calls {
		assert.Equal(t, "GET /v1/agents", c.Call, "only the agent listing is called")
	}

	list := api.agents("")
	assert.EqualValues(t, 3, list.Total)
	agents := byExternalID(list)
	require.Contains(t, agents, "agent_alpha")
	alpha := agents["agent_alpha"]
	assert.Equal(t, "Support agent", alpha.DisplayName)
	assert.Equal(t, "platform_agent", alpha.Kind)
	assert.Equal(t, "openai", alpha.Provider)
	assert.True(t, alpha.Present)
	require.Len(t, alpha.FoundBy, 1)
	assert.Equal(t, "OpenAI org", alpha.FoundBy[0].ConnectionName)
	assert.Contains(t, agents, "agent_b1")
	assert.Contains(t, agents, "agent_b2", "both pages of proj_beta's agents")

	// Platform 2: only proj_alpha's key. Scan every platform.
	code, body = api.do(http.MethodPost, "/connections", map[string]any{
		"name": "OpenAI alpha team", "provider": "openai", "endpoints": map[string]string{"api": fake.URL + "/v1"},
		"settings": map[string]any{"scopedKeys": map[string]any{"proj_alpha": shadowAlphaKey}},
	})
	require.Equal(t, http.StatusCreated, code, string(body))
	conn2 := decodeInto[spec.ShadowConnectionResponse](t, body)
	code, body = api.do(http.MethodPost, "/connections/"+conn2.Id+"/test", nil)
	require.Equal(t, http.StatusOK, code, string(body))
	test = decodeInto[spec.ShadowConnectionTest](t, body)
	assert.True(t, test.Ok, string(body))
	require.Len(t, test.Checks, 1)
	assert.Equal(t, "Agents (project proj_alpha)", test.Checks[0].Name)
	code, body = api.do(http.MethodPost, "/scans", map[string]any{})
	require.Equal(t, http.StatusAccepted, code, string(body))
	scan2 := decodeInto[spec.ShadowScanResponse](t, body)
	assert.ElementsMatch(t, []string{conn1.Id, conn2.Id}, scan2.ConnectionIds)
	runShadowScans(t, ctx, logs)

	// The same agent found through two platforms is one agent.
	list = api.agents("")
	assert.EqualValues(t, 3, list.Total, "agent_alpha is listed once")
	alpha = byExternalID(list)["agent_alpha"]
	require.Len(t, alpha.FoundBy, 2)
	assert.Equal(t, "OpenAI alpha team", alpha.FoundBy[0].ConnectionName)
	assert.Equal(t, "OpenAI org", alpha.FoundBy[1].ConnectionName)
	code, body = api.do(http.MethodGet, "/summary", nil)
	require.Equal(t, http.StatusOK, code)
	summary := decodeInto[spec.ShadowAgentSummary](t, body)
	assert.EqualValues(t, 3, summary.TotalFindings)
	assert.EqualValues(t, map[string]int32{"openai": 3}, summary.ByProvider)
	assert.EqualValues(t, 2, summary.Connections)
	assert.NotContains(t, string(body), "byVerdict")

	// The platform filter.
	assert.EqualValues(t, 3, api.agents("?provider=openai").Total)
	assert.EqualValues(t, 0, api.agents("?provider=bedrock").Total)
	code, _ = api.do(http.MethodGet, "/findings?provider=acme", nil)
	assert.Equal(t, http.StatusBadRequest, code)

	// The agent's detail: evidence, found by both platforms.
	code, body = api.do(http.MethodGet, "/findings/"+alpha.Id, nil)
	require.Equal(t, http.StatusOK, code, string(body))
	detail := decodeInto[spec.ShadowFindingDetail](t, body)
	require.Len(t, detail.Signals, 1, "the same evidence from two platforms is shown once")
	assert.Equal(t, "openai.agent_object", detail.Signals[0].Code)
	assert.Equal(t, "Agent object", detail.Signals[0].Name)
	assert.Equal(t, "GET /v1/agents", detail.Signals[0].Source)
	assert.Contains(t, detail.Signals[0].Condition, "GET /v1/agents")
	assert.Len(t, detail.FoundBy, 2)
	assert.Equal(t, "gpt-x", detail.Attributes["model"])
	for _, removed := range []string{"score", "verdict", "usage", "keys", "retirement", "linkedAgent", "requestsInWindow"} {
		assert.NotContains(t, string(body), `"`+removed+`"`, removed)
	}

	// Review: one status for the agent, on every platform that found it.
	code, body = api.do(http.MethodPatch, "/findings/"+alpha.Id, map[string]any{"status": "reviewed"})
	require.Equal(t, http.StatusOK, code, string(body))
	for _, src := range detail.FoundBy {
		code, body = api.do(http.MethodGet, "/findings/"+src.FindingId, nil)
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, "reviewed", decodeInto[spec.ShadowFindingDetail](t, body).Status, src.ConnectionName)
	}
	assert.EqualValues(t, 1, api.agents("?status=reviewed").Total)
	code, _ = api.do(http.MethodPatch, "/findings/"+alpha.Id, map[string]any{"status": "onboarded"})
	assert.Equal(t, http.StatusBadRequest, code)

	// Onboarding and retirement are gone.
	code, _ = api.do(http.MethodPost, "/findings/"+alpha.Id+"/onboarding", map[string]any{"projectName": "p", "agentName": "a"})
	assert.GreaterOrEqual(t, code, 400)
	code, _ = api.do(http.MethodPost, "/findings/"+alpha.Id+"/retirement/confirm", nil)
	assert.GreaterOrEqual(t, code, 400)

	// Platform 1 drops proj_beta: its complete listing no longer returns
	// proj_beta's agents, so they are no longer listed.
	code, body = api.do(http.MethodPut, "/connections/"+conn1.Id, map[string]any{
		"name": "OpenAI org", "provider": "openai", "settings": map[string]any{"scopedKeys": map[string]any{"proj_alpha": ""}},
	})
	require.Equal(t, http.StatusOK, code, string(body))
	code, body = api.do(http.MethodPost, "/scans", map[string]any{"connectionIds": []string{conn1.Id}})
	require.Equal(t, http.StatusAccepted, code, string(body))
	runShadowScans(t, ctx, logs)
	list = api.agents("")
	assert.EqualValues(t, 1, list.Total)
	assert.Equal(t, "agent_alpha", list.Findings[0].ExternalId)
	// The first scan's own list still shows them, as no longer listed.
	scanList := api.agents("?scanId=" + scan1.Id)
	require.EqualValues(t, 3, scanList.Total)
	for id, a := range byExternalID(scanList) {
		assert.Equal(t, id == "agent_alpha", a.Present, id)
	}

	// Clear the scans: the agents stay.
	code, body = api.do(http.MethodDelete, "/scans", nil)
	require.Equal(t, http.StatusOK, code, string(body))
	assert.EqualValues(t, 3, decodeInto[spec.ShadowScanClearResponse](t, body).Deleted)
	code, body = api.do(http.MethodGet, "/scans", nil)
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, decodeInto[spec.ShadowScanListResponse](t, body).Scans)
	assert.EqualValues(t, 1, api.agents("").Total)

	// Retention: no secret or instruction anywhere in the stored data or logs.
	for _, table := range []string{"shadow_connections", "shadow_scans", "shadow_findings", "shadow_finding_signals"} {
		var dump string
		require.NoError(t, db.GetDB().WithContext(ctx).Raw(
			fmt.Sprintf("SELECT COALESCE(string_agg(t::text, ' '), '') FROM %s t WHERE ou_id = ?", table), ouID).Scan(&dump).Error)
		assert.NotContains(t, dump, "SHADOWTESTSECRET", table)
		assert.NotContains(t, dump, "SECRET-PROMPT-MARKER", table)
	}
	assert.NotContains(t, logs.String(), "SHADOWTESTSECRET")

	// Another org sees nothing of this one.
	other := shadowAPI{t: t, org: "other-" + uuid.NewString()[:6], app: apitestutils.MakeAppClientWithDeps(t,
		wiring.TestClients{OpenChoreoClient: oc}, jwtassertion.NewMockMiddlewareWithOUID(t, "ou-"+uuid.NewString()))}
	code, _ = other.do(http.MethodGet, "/findings/"+alpha.Id, nil)
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = other.do(http.MethodGet, "/connections/"+conn1.Id, nil)
	assert.Equal(t, http.StatusNotFound, code)
	assert.EqualValues(t, 0, other.agents("").Total)
	code, _ = other.do(http.MethodDelete, "/findings/"+alpha.Id, nil)
	assert.Equal(t, http.StatusNotFound, code, "another org cannot delete the agent")
	code, body = other.do(http.MethodDelete, "/findings", nil)
	require.Equal(t, http.StatusOK, code, string(body))
	assert.EqualValues(t, 0, decodeInto[spec.ShadowFindingClearResponse](t, body).Deleted)
	require.EqualValues(t, 1, api.agents("").Total, "another org's delete all leaves this org's agents")

	// Delete one agent: it goes for both platforms that found it.
	code, _ = api.do(http.MethodDelete, "/findings/"+alpha.Id, nil)
	assert.Equal(t, http.StatusNoContent, code)
	code, _ = api.do(http.MethodGet, "/findings/"+alpha.Id, nil)
	assert.Equal(t, http.StatusNotFound, code)
	assert.EqualValues(t, 0, api.agents("").Total)
	code, _ = api.do(http.MethodDelete, "/findings/"+alpha.Id, nil)
	assert.Equal(t, http.StatusNotFound, code, "already deleted")
	code, _ = api.do(http.MethodDelete, "/findings/not-a-uuid", nil)
	assert.Equal(t, http.StatusNotFound, code, "as GET: a malformed ID names no agent")

	// A later scan lists it again, because the platform still has it.
	code, body = api.do(http.MethodPost, "/scans", map[string]any{})
	require.Equal(t, http.StatusAccepted, code, string(body))
	runShadowScans(t, ctx, logs)
	list = api.agents("")
	require.EqualValues(t, 1, list.Total)
	assert.Equal(t, "agent_alpha", list.Findings[0].ExternalId)
	assert.Equal(t, "new", list.Findings[0].Status)
	assert.Len(t, list.Findings[0].FoundBy, 2)

	// Delete all: every agent goes; platforms and scans stay.
	code, body = api.do(http.MethodDelete, "/findings", nil)
	require.Equal(t, http.StatusOK, code, string(body))
	// agent_alpha (found by two platforms, counted once) and proj_beta's two
	// agents, which platform 1 no longer lists but are still recorded.
	assert.EqualValues(t, 3, decodeInto[spec.ShadowFindingClearResponse](t, body).Deleted)
	assert.EqualValues(t, 0, api.agents("?scanId="+scan1.Id).Total, "no longer listed agents are deleted too")
	assert.EqualValues(t, 0, api.agents("").Total)
	code, body = api.do(http.MethodGet, "/connections", nil)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, decodeInto[spec.ShadowConnectionListResponse](t, body).Connections, 2)
	code, body = api.do(http.MethodGet, "/scans", nil)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, decodeInto[spec.ShadowScanListResponse](t, body).Scans, 1)

	// Delete the platforms: their agents go with them.
	for _, id := range []string{conn1.Id, conn2.Id} {
		code, _ = api.do(http.MethodDelete, "/connections/"+id, nil)
		assert.Equal(t, http.StatusNoContent, code)
	}
	assert.EqualValues(t, 0, api.agents("?scanId="+scan1.Id).Total)
	assert.EqualValues(t, 0, api.agents("").Total)
}
