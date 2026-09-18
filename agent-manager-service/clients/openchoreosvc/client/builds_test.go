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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
