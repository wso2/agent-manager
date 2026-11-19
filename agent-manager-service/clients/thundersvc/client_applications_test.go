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

package thundersvc

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These cover the /applications side of the shared findResourceByName /
// regenerateResourceSecret / deleteThunderResource / listResourcePage
// helpers — the /agents twins (agent_client_test.go) already covered these
// paths, but the /applications side itself had no direct test coverage
// before the two were consolidated onto the same code.

func TestEnsureApp_CreatesNewApp_WhenNoneExists(t *testing.T) {
	var createdBody map[string]any
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/applications":
			w.Header().Set("Content-Type", "application/json")
			// Same wrapped-with-extra-fields shape /agents uses — the shared
			// list helper must ignore totalResults/count, not just Applications.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"totalResults": 0,
				"count":        0,
				"applications": []map[string]any{},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/applications":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&createdBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "app-uuid-1",
				"name": createdBody["name"],
				"inboundAuthConfig": []map[string]any{
					{"type": "oauth2", "config": map[string]any{
						"clientId":     "amp-app-acme",
						"clientSecret": "generated-secret",
					}},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	clientID, clientSecret, created, err := c.EnsureApp(context.Background(), "amp-app-acme", "ou-1")

	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "amp-app-acme", clientID)
	assert.Equal(t, "generated-secret", clientSecret)
	assert.Equal(t, "amp-app-acme", createdBody["name"])
}

func TestEnsureApp_ReturnsExisting_WhenAlreadyExists(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/applications", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"applications": []map[string]any{
				{"id": "app-uuid-1", "name": "amp-app-acme", "clientId": "existing-client-id"},
			},
		})
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	clientID, clientSecret, created, err := c.EnsureApp(context.Background(), "amp-app-acme", "ou-1")

	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, "existing-client-id", clientID)
	assert.Empty(t, clientSecret, "an already-existing app's secret is never available")
}

func TestDeleteApp_Success(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/applications":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "app-uuid-1", "name": "amp-app-acme", "clientId": "existing-client-id"},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/applications/app-uuid-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	found, err := c.DeleteApp(context.Background(), "amp-app-acme")
	require.NoError(t, err)
	assert.True(t, found)
}

func TestDeleteApp_NotFound_ReturnsFalseNoError(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/applications", r.URL.Path, "must not attempt a delete when the app can't be found by name")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	found, err := c.DeleteApp(context.Background(), "amp-app-acme")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestRegenerateAppClientSecret_Success(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/applications":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "app-uuid-1", "name": "amp-app-acme", "clientId": "existing-client-id"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/applications/app-uuid-1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "app-uuid-1",
				"name": "amp-app-acme",
				"inboundAuthConfig": []map[string]any{
					{"type": "oauth2", "config": map[string]any{
						"clientId":                "existing-client-id",
						"grantTypes":              []string{"client_credentials"},
						"tokenEndpointAuthMethod": "client_secret_basic",
					}},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/applications/app-uuid-1":
			var putBody map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&putBody))
			_, hasID := putBody["id"]
			assert.False(t, hasID, "id must not be sent in the PUT body — it belongs in the URL")
			inbound := putBody["inboundAuthConfig"].([]any)
			cfg := inbound[0].(map[string]any)["config"].(map[string]any)
			newSecret := cfg["clientSecret"]
			require.NotEmpty(t, newSecret)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "app-uuid-1",
				"name": "amp-app-acme",
				"inboundAuthConfig": []map[string]any{
					{"type": "oauth2", "config": map[string]any{
						"clientId":     "existing-client-id",
						"clientSecret": newSecret,
					}},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	newSecret, err := c.RegenerateAppClientSecret(context.Background(), "amp-app-acme")
	require.NoError(t, err)
	assert.NotEmpty(t, newSecret)
	assert.Len(t, newSecret, 64, "must use the same 384-bit random secret generator as the /agents path")
}

func TestRegenerateAppClientSecret_AppNotFound(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/applications", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	_, err := c.RegenerateAppClientSecret(context.Background(), "amp-app-acme")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// EnsureApp must work for any app name.
func TestEnsureApp_CreatesNewApp_WithArbitraryName(t *testing.T) {
	var createdBody map[string]any
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/applications":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/applications":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&createdBody))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "app-uuid-2",
				"name": createdBody["name"],
				"inboundAuthConfig": []map[string]any{
					{"type": "oauth2", "config": map[string]any{
						"clientId":     "amp-scheduler-acme",
						"clientSecret": "generated-scheduler-secret",
					}},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	clientID, clientSecret, created, err := c.EnsureApp(context.Background(), "amp-scheduler-acme", "ou-1")

	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "amp-scheduler-acme", clientID)
	assert.Equal(t, "generated-scheduler-secret", clientSecret)
	assert.Equal(t, "amp-scheduler-acme", createdBody["name"])
}
