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

package gwctl_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

const (
	agCardURL     = "https://1.1.1.1/.well-known/agent-card.json"
	agHTTPCardURL = "http://1.1.1.1/.well-known/agent-card.json"
)

// agSetCardSource registers the test agent's card URL in the default environment.
func agSetCardSource(t *testing.T, a *routeAPI, url string) {
	t.Helper()
	rec := a.do(http.MethodPut, agAgentPath("/agent-card/source"), map[string]any{"url": url})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
}

// agMakePlatformAgent turns the test agent into a platform-hosted one.
func agMakePlatformAgent(t *testing.T, a *routeAPI) {
	t.Helper()
	require.NoError(t, a.db.Exec(`UPDATE agents SET provisioning_type = 'internal' WHERE org_id = ? AND name = ?`,
		a.org, agAgent).Error)
}

// TestAGRoutesAgentCardSource registers, reads, refreshes and removes an
// external agent's card source.
func TestAGRoutesAgentCardSource(t *testing.T) {
	a, _ := agSetup(t)

	rec := a.do(http.MethodGet, agAgentPath("/agent-card"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "Agent card not found", errorMessage(t, rec))

	agSetCardSource(t, a, " "+agCardURL+" ")
	card := expect(t, a.do(http.MethodGet, agAgentPath("/agent-card"), nil), http.StatusOK)
	assert.Equal(t, "external", card["source"])
	assert.Equal(t, agCardURL, card["sourceUrl"], "the URL is stored trimmed")
	assert.Equal(t, "pending", card["status"], "the card is fetched in the background")
	assert.NotContains(t, card, "card", "no card has been fetched yet")

	// The card worker stores what it fetched; the route serves it as is.
	fetchedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	require.NoError(t, a.db.Exec(`UPDATE mod_ai_gateway_agent_cards SET card = ?, status = 'fetched', fetched_at = ?
		WHERE org_id = ?`, []byte(`{"name":"Weather agent","version":"1.0.0","skills":[{"id":"forecast"}],"capabilities":{"streaming":true},"n":12345678901234567890}`),
		fetchedAt, a.org).Error)
	card = expect(t, a.do(http.MethodGet, agAgentPath("/agent-card"), nil), http.StatusOK)
	assert.Equal(t, "fetched", card["status"])
	servedAt, err := time.Parse(time.RFC3339, card["fetchedAt"].(string))
	require.NoError(t, err)
	assert.True(t, fetchedAt.Equal(servedAt), "fetchedAt is %v", card["fetchedAt"])
	doc := card["card"].(map[string]any)
	assert.Equal(t, "Weather agent", doc["name"])
	assert.Equal(t, []any{map[string]any{"id": "forecast"}}, doc["skills"])
	assert.Contains(t, rec.Body.String()+a.do(http.MethodGet, agAgentPath("/agent-card"), nil).Body.String(),
		"12345678901234567890", "large numbers are served without float rounding")

	rec = a.do(http.MethodPost, agAgentPath("/agent-card/refresh"), nil)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	expect(t, a.do(http.MethodGet, agAgentPath("/agent-card"), nil), http.StatusOK)

	rec = a.do(http.MethodDelete, agAgentPath("/agent-card/source"), nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, agAgentPath("/agent-card"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestAGRoutesAgentCardRejections pins the 400, 404 and 403 answers of the card
// routes.
func TestAGRoutesAgentCardRejections(t *testing.T) {
	a, _ := agSetup(t)
	ghost := func(p string) string { return agentPathIn(agProject, "ghost", "/agent-card"+p) }

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}{
		{"malformed json", http.MethodPut, agAgentPath("/agent-card/source"), "{", http.StatusBadRequest, "Invalid request body"},
		{"no url", http.MethodPut, agAgentPath("/agent-card/source"), map[string]any{"url": "  "}, http.StatusBadRequest, "url is required"},
		{"private host", http.MethodPut, agAgentPath("/agent-card/source"), map[string]any{"url": "http://127.0.0.1/card"}, http.StatusBadRequest, "invalid URL"},
		{"unknown agent card", http.MethodGet, ghost(""), nil, http.StatusNotFound, "Agent not found"},
		{"unknown agent source", http.MethodPut, ghost("/source"), map[string]any{"url": agCardURL}, http.StatusNotFound, "Agent not found"},
		{"unknown agent refresh", http.MethodPost, ghost("/refresh"), nil, http.StatusNotFound, "Agent not found"},
		{"unknown agent delete", http.MethodDelete, ghost("/source"), nil, http.StatusNotFound, "Agent not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(tc.method, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.message)
		})
	}

	// The writes need the deploy scope as well as agent:update.
	noDeploy := withScopes(scope(gwperm.AgentUpdate), scope(gwperm.AgentRead))
	for _, r := range []struct{ method, path string }{
		{http.MethodPut, agAgentPath("/agent-card/source")},
		{http.MethodDelete, agAgentPath("/agent-card/source")},
		{http.MethodPost, agAgentPath("/agent-card/refresh")},
	} {
		rec := a.do(r.method, r.path, map[string]any{"url": agCardURL}, noDeploy)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
	}
	rec := a.do(http.MethodGet, agAgentPath("/agent-card"), nil, withScopes(scope(gwperm.AgentUpdate)))
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, agAgentPath("/agent-card"), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, "the refused writes stored no source: %s", rec.Body.String())
}

// TestAGRoutesAgentCardPlatformAgent pins that a platform-hosted agent's card
// source cannot be set, and that its card cannot be refreshed before it runs.
func TestAGRoutesAgentCardPlatformAgent(t *testing.T) {
	a, _ := agSetup(t)
	agMakePlatformAgent(t, a)

	rec := a.do(http.MethodPut, agAgentPath("/agent-card/source"), map[string]any{"url": agCardURL})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "a card source can be set only for external agents", errorMessage(t, rec))
	rec = a.do(http.MethodDelete, agAgentPath("/agent-card/source"), nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	rec = a.do(http.MethodPost, agAgentPath("/agent-card/refresh"), nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "agent is not an A2A agent", errorMessage(t, rec))
	rec = a.do(http.MethodPut, agAgentPath("/a2a-proxy"), map[string]any{})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "only external agents can be routed through the gateway", errorMessage(t, rec))
}

// TestAGRoutesA2AProxyLifecycle routes an external agent through the
// environment's gateway, sets its upstream credential, refreshes and removes it.
func TestAGRoutesA2AProxyLifecycle(t *testing.T) {
	a, _ := agSetup(t)

	rec := a.do(http.MethodGet, agAgentPath("/a2a-proxy"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "the environment does not route this agent through its gateway", errorMessage(t, rec))
	rec = a.do(http.MethodPut, agAgentPath("/a2a-proxy"), map[string]any{})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, errorMessage(t, rec), "register the agent's card URL")

	agSetCardSource(t, a, agCardURL)
	rec = a.do(http.MethodPut, agAgentPath("/a2a-proxy"), map[string]any{})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	proxy := expect(t, a.do(http.MethodGet, agAgentPath("/a2a-proxy"), nil), http.StatusOK)
	assert.Equal(t, "https://gw.example.com/"+agAgent, proxy["gatewayUrl"])
	assert.Equal(t, "DEPLOYING", proxy["status"])
	assert.Empty(t, proxy["upstreamAuthHeader"])

	rec = a.do(http.MethodPut, agAgentPath("/a2a-proxy"),
		map[string]any{"upstreamAuth": map[string]any{"header": "X-Upstream-Key", "value": "s3cret"}})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	proxy = expect(t, a.do(http.MethodGet, agAgentPath("/a2a-proxy"), nil), http.StatusOK)
	assert.Equal(t, "X-Upstream-Key", proxy["upstreamAuthHeader"])
	assert.NotContains(t, rec.Body.String()+a.do(http.MethodGet, agAgentPath("/a2a-proxy"), nil).Body.String(), "s3cret")

	// The stored credential must not follow the card to another origin.
	rec = a.do(http.MethodPut, agAgentPath("/agent-card/source"), map[string]any{"url": "https://1.1.1.1/card.json"})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, errorMessage(t, rec), "the card URL cannot be routed through the gateway")
	rec = a.do(http.MethodPut, agAgentPath("/agent-card/source"),
		map[string]any{"url": "https://8.8.8.8/.well-known/agent-card.json"})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	card := expect(t, a.do(http.MethodGet, agAgentPath("/agent-card"), nil), http.StatusOK)
	assert.Equal(t, agCardURL, card["sourceUrl"], "the refused change kept the old source")

	rec = a.do(http.MethodPost, agAgentPath("/a2a-proxy/refresh"), nil)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	rec = a.do(http.MethodPost, agAgentPath("/agent-card/refresh"), nil)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	rec = a.do(http.MethodDelete, agAgentPath("/a2a-proxy"), nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, agAgentPath("/a2a-proxy"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = a.do(http.MethodPost, agAgentPath("/a2a-proxy/refresh"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	expect(t, a.do(http.MethodGet, agAgentPath("/agent-card"), nil), http.StatusOK)
}

// TestAGRoutesA2AProxyRejections pins the 400s for an upstream credential the
// gateway must not send, and the 403 without the deploy scope.
func TestAGRoutesA2AProxyRejections(t *testing.T) {
	a, _ := agSetup(t)
	agSetCardSource(t, a, agHTTPCardURL)

	tests := []struct {
		name    string
		body    any
		message string
	}{
		{"malformed json", "{", "Invalid request body"},
		{"reserved header", map[string]any{"upstreamAuth": map[string]any{"header": "Host", "value": "v"}}, "the Host header cannot be set upstream"},
		{"bad header", map[string]any{"upstreamAuth": map[string]any{"header": "X Key", "value": "v"}}, "header may contain only"},
		{"empty value", map[string]any{"upstreamAuth": map[string]any{"header": "X-Key", "value": ""}}, "value must not be empty"},
		{"secret over http", map[string]any{"upstreamAuth": map[string]any{"header": "X-Key", "value": "v"}}, "must use https"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPut, agAgentPath("/a2a-proxy"), tc.body)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.message)
		})
	}
	rec := a.do(http.MethodGet, agAgentPath("/a2a-proxy"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, "no refused request routed the agent: %s", rec.Body.String())

	noDeploy := withScopes(scope(gwperm.AgentUpdate), scope(gwperm.AgentRead))
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		rec := a.do(method, agAgentPath("/a2a-proxy"), map[string]any{}, noDeploy)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s: %s", method, rec.Body.String())
	}
	rec = a.do(http.MethodPost, agAgentPath("/a2a-proxy/refresh"), nil, noDeploy)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, agAgentPath("/a2a-proxy"), nil, withScopes(scope(gwperm.AgentUpdate)))
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

// TestAGRoutesAgentAPIKeys issues, lists, rotates and revokes the API keys of
// an agent routed through the gateway, and issues a console test key.
func TestAGRoutesAgentAPIKeys(t *testing.T) {
	a, _ := agSetup(t)
	keys := func(p string) string { return agAgentPath("/api-keys" + p) }

	rec := a.do(http.MethodGet, keys(""), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, "an agent not on the gateway has no keys: %s", rec.Body.String())
	assert.Equal(t, "Agent not found", errorMessage(t, rec))

	agSetCardSource(t, a, agCardURL)
	rec = a.do(http.MethodPut, agAgentPath("/a2a-proxy"), map[string]any{})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	listed := expectInto[[]map[string]any](t, a.do(http.MethodGet, keys(""), nil), http.StatusOK)
	assert.Empty(t, listed)

	created := expect(t, a.do(http.MethodPost, keys(""), map[string]any{"name": "client-one", "displayName": "Client one"}), http.StatusCreated)
	assert.Equal(t, "client-one", created["keyId"])
	first, _ := created["apiKey"].(string)
	require.NotEmpty(t, first)
	byDisplay := expect(t, a.do(http.MethodPost, keys(""), map[string]any{"displayName": "Second Client"}), http.StatusCreated)
	assert.NotEmpty(t, byDisplay["keyId"], "a key without a name is named after its display name")

	listed = expectInto[[]map[string]any](t, a.do(http.MethodGet, keys(""), nil), http.StatusOK)
	require.Len(t, listed, 2)
	names := []any{listed[0]["name"], listed[1]["name"]}
	assert.Contains(t, names, "client-one")
	for _, k := range listed {
		assert.NotContains(t, k, "apiKey", "the list shows masked keys only")
	}

	rotated := expect(t, a.do(http.MethodPut, keys("/client-one"), map[string]any{"displayName": "Client one v2"}), http.StatusOK)
	assert.NotEqual(t, first, rotated["apiKey"])
	rotated = expect(t, a.do(http.MethodPut, keys("/client-one"), nil), http.StatusOK)
	assert.NotEmpty(t, rotated["apiKey"], "the rotation body is optional")

	test := expect(t, a.do(http.MethodPost, keys("/test"), nil), http.StatusOK)
	assert.NotEmpty(t, test["apiKey"])
	assert.NotEmpty(t, test["expiresAt"])
	listed = expectInto[[]map[string]any](t, a.do(http.MethodGet, keys(""), nil), http.StatusOK)
	assert.Len(t, listed, 2, "test keys stay out of the list")

	rec = a.do(http.MethodDelete, keys("/client-one"), nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	listed = expectInto[[]map[string]any](t, a.do(http.MethodGet, keys(""), nil), http.StatusOK)
	require.Len(t, listed, 1)
	assert.NotEqual(t, "client-one", listed[0]["name"])
}

// TestAGRoutesAgentAPIKeyRejections pins the 400, 404, 503 and 403 answers of
// the agent API key routes.
func TestAGRoutesAgentAPIKeyRejections(t *testing.T) {
	a, gw := agSetup(t)
	keys := func(p string) string { return agAgentPath("/api-keys" + p) }
	agSetCardSource(t, a, agCardURL)
	rec := a.do(http.MethodPut, agAgentPath("/a2a-proxy"), map[string]any{})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	tests := []struct {
		name    string
		body    any
		message string
	}{
		{"malformed json", "{", "Invalid request body"},
		{"name not lowercase", map[string]any{"name": "Client One"}, "API key name must be lowercase alphanumeric segments"},
		{"reserved test name", map[string]any{"name": "console-test-abc"}, "reserved for console test keys"},
		{"name too short", map[string]any{"name": "ab"}, "must be at least"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, keys(""), tc.body)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.message)
		})
	}
	rec = a.do(http.MethodPut, keys("/client-one"), "{")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "Invalid request body", errorMessage(t, rec))

	ghost := func(p string) string { return agentPathIn(agProject, "ghost", "/api-keys"+p) }
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, ghost(""), map[string]any{"name": "client-one"}},
		{http.MethodPost, ghost("/test"), nil},
		{http.MethodPut, ghost("/client-one"), nil},
		{http.MethodDelete, ghost("/client-one"), nil},
	} {
		rec := a.do(r.method, r.path, r.body)
		require.Equal(t, http.StatusNotFound, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
		assert.Equal(t, "Agent not found", errorMessage(t, rec))
	}

	rec = a.do(http.MethodGet, keys(""), nil, withScopes(scope(gwperm.AgentRead), scope(gwperm.AgentUpdate)))
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = a.do(http.MethodPost, keys(""), map[string]any{"name": "client-one"}, withScopes(scope(gwperm.AgentUpdate)))
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

	// With no ingress gateway left in the environment there is nowhere to send a key.
	require.NoError(t, a.db.Exec(`UPDATE mod_ai_gateway_gateways SET functionality_type = 'egress' WHERE id = ?`, gw.UUID).Error)
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, keys(""), map[string]any{"name": "client-one"}},
		{http.MethodPost, keys("/test"), nil},
		{http.MethodPut, keys("/client-one"), nil},
		{http.MethodDelete, keys("/client-one"), nil},
	} {
		rec := a.do(r.method, r.path, r.body)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
		assert.Equal(t, "No gateway connections available", errorMessage(t, rec))
	}
	listed := expectInto[[]map[string]any](t, a.do(http.MethodGet, keys(""), nil), http.StatusOK)
	assert.Empty(t, listed, "no refused request issued a key")
}
