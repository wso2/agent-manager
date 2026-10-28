//
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

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

// A component created before routePath existed: spec.parameters carries the
// build-time keys and nothing else.
const legacyComponentJSON = `{
  "metadata": {"name": "legacy-agent"},
  "spec": {
    "componentType": {"name": "internal-agent/agent-api"},
    "owner": {"projectName": "proj"},
    "parameters": {"exposed": true, "basePath": "/old", "port": 8080},
    "workflow": {"parameters": {}}
  }
}`

// UpdateComponentBuildParameters must never introduce routePath. A component
// without it renders the legacy "<component>-<endpoint>" path, which is the URL
// its owners already have; adding the parameter here would mean the first
// build-parameters edit silently re-points a live agent's public URL.
func TestUpdateComponentBuildParameters_DoesNotIntroduceRoutePath(t *testing.T) {
	var putParameters map[string]any

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(legacyComponentJSON))
			require.NoError(t, err)
			return
		}

		var sent struct {
			Spec struct {
				Parameters map[string]any `json:"parameters"`
			} `json:"spec"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		putParameters = sent.Spec.Parameters

		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(legacyComponentJSON))
		require.NoError(t, err)
	}))

	err := client.UpdateComponentBuildParameters(context.Background(), "acme", "proj", "legacy-agent",
		UpdateComponentBuildParametersRequest{
			AgentType:      AgentTypeConfig{Type: "agent-api", SubType: "custom-api"},
			InputInterface: &InputInterfaceConfig{Type: "HTTP", Port: 9090, BasePath: "/new"},
		})

	require.NoError(t, err)
	require.NotNil(t, putParameters, "expected the update to PUT the component back")
	// The build-parameter writes we do expect, so a passing test can't be a
	// false negative from the update silently doing nothing.
	assert.Equal(t, "/new", putParameters["basePath"])
	assert.NotContains(t, putParameters, "routePath")
}

// ListBuilds must follow the pagination cursor to the end. The OpenChoreo list
// API caps a response at one page and Kubernetes returns items in name-ascending
// order, which for "<component>-<timestamp>" build names is oldest-first. Stopping
// at the first page therefore returns only the OLDEST builds: past the page size
// every newly triggered build becomes invisible in the console, and `total` under-
// reports how many exist so no client can page to them either.
func TestListBuilds_FollowsPaginationCursor(t *testing.T) {
	// Two pages, oldest first, mirroring the server's ordering.
	pages := map[string]string{
		"": `{"items":[
			{"metadata":{"name":"agent-1000","creationTimestamp":"2026-01-01T00:00:00Z"}},
			{"metadata":{"name":"agent-2000","creationTimestamp":"2026-01-02T00:00:00Z"}}
		],"pagination":{"nextCursor":"page2"}}`,
		"page2": `{"items":[
			{"metadata":{"name":"agent-3000","creationTimestamp":"2026-01-03T00:00:00Z"}}
		],"pagination":{}}`,
	}

	var cursors []string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		body, ok := pages[cursor]
		require.True(t, ok, "unexpected cursor %q", cursor)
		cursors = append(cursors, cursor)

		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(body))
		require.NoError(t, err)
	}))

	builds, err := client.ListBuilds(context.Background(), "acme", "proj", "agent")
	require.NoError(t, err)

	assert.Equal(t, []string{"", "page2"}, cursors, "expected the second page to be fetched with the returned cursor")
	require.Len(t, builds, 3, "every build must be returned, not just the first page")
	// Newest first: the most recent build is what the console shows at the top.
	assert.Equal(t, []string{"agent-3000", "agent-2000", "agent-1000"},
		[]string{builds[0].Name, builds[1].Name, builds[2].Name})
}

// A server that keeps handing back a fresh cursor must not spin forever. Each
// page advances the cursor, so this exercises the page cap rather than the
// non-advancing-cursor guard below.
func TestListBuilds_StopsAtMaxPages(t *testing.T) {
	calls := 0
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		body := fmt.Sprintf(
			`{"items":[{"metadata":{"name":"agent-%d"}}],"pagination":{"nextCursor":"page-%d"}}`,
			calls, calls)
		_, err := w.Write([]byte(body))
		require.NoError(t, err)
	}))

	builds, err := client.ListBuilds(context.Background(), "acme", "proj", "agent")
	require.NoError(t, err)
	assert.Equal(t, maxListPages, calls)
	assert.Len(t, builds, maxListPages)
}

// A cursor pointing back at the page just fetched means the server's pagination
// is broken. Following it would re-append the same builds until the page cap,
// handing the caller duplicates and an inflated count — so it must be an error,
// not a quietly wrong list.
func TestListBuilds_RejectsNonAdvancingCursor(t *testing.T) {
	calls := 0
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(
			`{"items":[{"metadata":{"name":"agent-1"}}],"pagination":{"nextCursor":"stuck"}}`))
		require.NoError(t, err)
	}))

	builds, err := client.ListBuilds(context.Background(), "acme", "proj", "agent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not advance")
	assert.Nil(t, builds)
	// Page 1 returns "stuck"; page 2 is requested with it and returns it again,
	// which is where the guard trips.
	assert.Equal(t, 2, calls)
}

// newTestClientRawHeaders builds a client whose stub server does NOT stamp
// application/json, letting a handler pick the content type per response. That
// is the only way to reach ListBuilds' `resp.JSON200 == nil` branch: the
// generated parser populates JSON200 solely for a JSON-typed 200.
func newTestClientRawHeaders(t *testing.T, handler http.Handler) *openChoreoClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	gc, err := gen.NewClientWithResponses(srv.URL)
	require.NoError(t, err)
	return &openChoreoClient{ocClient: gc, defaultNamespace: "default"}
}

// captureDefaultLogger redirects slog.Default() into a buffer for the duration
// of a test. ListBuilds logs truncation through logger.GetLogger(ctx), which
// falls back to slog.Default() when the context carries no request logger.
func captureDefaultLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

// A non-200 from the list endpoint must surface as an error. Returning an empty
// slice instead would render as "no builds yet" in the console, which is the
// same thing a brand-new agent shows — an auth or outage failure would be
// indistinguishable from success.
func TestListBuilds_ReturnsErrorOnNonOKStatus(t *testing.T) {
	calls := 0
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte(`{"message":"boom"}`))
		require.NoError(t, err)
	}))

	builds, err := client.ListBuilds(context.Background(), "acme", "proj", "agent")
	require.Error(t, err)
	assert.Nil(t, builds)
	assert.Equal(t, 1, calls, "a failed page must abort the walk, not keep paging")
}

// A transport failure (server gone) must propagate rather than be mistaken for
// an empty build list.
func TestListBuilds_ReturnsErrorOnTransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // refuse every connection

	gc, err := gen.NewClientWithResponses(srv.URL)
	require.NoError(t, err)
	client := &openChoreoClient{ocClient: gc, defaultNamespace: "default"}

	builds, err := client.ListBuilds(context.Background(), "acme", "proj", "agent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list builds")
	assert.Nil(t, builds)
}

// An agent with no builds is an ordinary state, not an error, and must come back
// as an empty (non-nil) slice so callers can range over it safely.
func TestListBuilds_ReturnsEmptyWhenNoBuilds(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"items":[],"pagination":{}}`))
		require.NoError(t, err)
	}))

	builds, err := client.ListBuilds(context.Background(), "acme", "proj", "agent")
	require.NoError(t, err)
	require.NotNil(t, builds)
	assert.Empty(t, builds)
}

// Hitting the page cap means builds really were left unread, so the operator
// warning must fire. This is the positive half of the pair below.
func TestListBuilds_WarnsWhenTruncatedAtPageCap(t *testing.T) {
	logs := captureDefaultLogger(t)

	calls := 0
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprintf(w,
			`{"items":[{"metadata":{"name":"agent-%d"}}],"pagination":{"nextCursor":"page-%d"}}`,
			calls, calls)
		require.NoError(t, err)
	}))

	_, err := client.ListBuilds(context.Background(), "acme", "proj", "agent")
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "truncated",
		"stopping at the page cap genuinely drops builds and must be reported")
}

// Ending the walk because a page carried no decodable body is a clean stop: every
// build the server offered was read. Warning "truncated" here would send an
// operator hunting for missing builds that were never missing. Regression test
// for the cursor being left set on this break path.
func TestListBuilds_DoesNotWarnTruncatedOnCleanStop(t *testing.T) {
	logs := captureDefaultLogger(t)

	calls := 0
	client := newTestClientRawHeaders(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(
				`{"items":[{"metadata":{"name":"agent-1"}}],"pagination":{"nextCursor":"page2"}}`))
			require.NoError(t, err)
			return
		}
		// 200 with a body the generated parser leaves undecoded => JSON200 == nil.
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("not json"))
		require.NoError(t, err)
	}))

	builds, err := client.ListBuilds(context.Background(), "acme", "proj", "agent")
	require.NoError(t, err)
	assert.Len(t, builds, 1, "the page that did parse must still be returned")
	assert.NotContains(t, logs.String(), "truncated",
		"a clean end of list must not be reported as a truncated build list")
}

func TestWithRunEnvVars_AddsToRunWithoutTouchingComponent(t *testing.T) {
	userEnv := map[string]interface{}{"name": "LOG_LEVEL", "value": "debug"}
	staleID := map[string]interface{}{"name": EnvVarAgentIDClientID, "value": "old"}
	params := map[string]interface{}{
		workflowParamEnvironmentVariables: []interface{}{userEnv, staleID},
		"repository":                      map[string]interface{}{"url": "https://example.com/repo"},
	}

	got := withRunEnvVars(params, []EnvVar{
		{Key: EnvVarAgentIDClientID, Value: "new"},
		{Key: EnvVarAgentIDClientSecret, ValueFrom: &EnvVarValueFrom{SecretKeyRef: &SecretKeyRef{Name: "agent-secret", Key: "client-secret"}}},
	})

	assert.Equal(t, []interface{}{
		userEnv,
		map[string]any{"name": EnvVarAgentIDClientID, "value": "new"},
		map[string]any{"name": EnvVarAgentIDClientSecret, "valueFrom": map[string]any{
			"secretKeyRef": map[string]any{"name": "agent-secret", "key": "client-secret"},
		}},
	}, got[workflowParamEnvironmentVariables])
	assert.Equal(t, params["repository"], got["repository"])
	// The component's own parameters keep only what the user configured.
	assert.Equal(t, []interface{}{userEnv, staleID}, params[workflowParamEnvironmentVariables])
}

func TestWithRunEnvVars_NoVarsReturnsParamsUnchanged(t *testing.T) {
	params := map[string]interface{}{workflowParamEnvironmentVariables: []interface{}{}}
	assert.Equal(t, params, withRunEnvVars(params, nil))
}
