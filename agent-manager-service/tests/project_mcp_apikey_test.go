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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// keyTestScopesHeader names the scopes the request's token carries; absent, it carries every admin scope.
const keyTestScopesHeader = "X-Test-Scopes"

// apiKeyHash is the stored hash of a key value.
func apiKeyHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// scopedTokenMiddleware is the mock JWT middleware with the scopes a request names, so the route's permission gate
// is exercised for real.
func scopedTokenMiddleware(t *testing.T) jwtassertion.Middleware {
	t.Helper()
	all := make([]string, 0)
	for _, p := range rbac.PredefinedRolePermissions[rbac.RoleAdmin] {
		all = append(all, p.Scope())
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes := strings.Join(all, " ")
			if named, ok := r.Header[keyTestScopesHeader]; ok {
				scopes = strings.Join(named, " ")
			}
			org := ""
			parts := strings.Split(r.URL.Path, "/")
			for i, part := range parts {
				if part == "orgs" && i+1 < len(parts) {
					org = parts[i+1]
					break
				}
			}
			claims := &jwtassertion.TokenClaims{
				Scope:            scopes,
				OuId:             jwtassertion.MockOUID,
				OuHandle:         org,
				RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
			}
			next.ServeHTTP(w, r.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(r.Context(), claims)))
		})
	}
}

// TestProjectMCPBindingAPIKeys drives a project proxy binding's API keys through the HTTP API against the database:
// issue, list, rotate and revoke, the refusals and their codes, the permission gate, the gateway key sync, and the
// keys going with the binding when it is removed.
func TestProjectMCPBindingAPIKeys(t *testing.T) {
	suffix := uuid.NewString()[:8]
	project := "pk-" + suffix
	projectUUID := uuid.NewString()
	devEnv, stgEnv := "dev-"+suffix, "stg-"+suffix
	envs := map[string]uuid.UUID{devEnv: uuid.New(), stgEnv: uuid.New()}

	oc := apitestutils.CreateMockOpenChoreoClient()
	baseProject := oc.GetProjectFunc
	oc.GetProjectFunc = func(ctx context.Context, ns, name string) (*models.ProjectResponse, error) {
		p, err := baseProject(ctx, ns, name)
		if err != nil {
			return nil, err
		}
		p.Name, p.UUID = name, projectUUID
		return p, nil
	}
	oc.GetEnvironmentFunc = func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
		id, ok := envs[name]
		if !ok {
			return nil, utils.ErrNotFound
		}
		return &models.EnvironmentResponse{UUID: id.String(), Name: name}, nil
	}
	oc.GetReleaseBindingEndpointServiceURLFunc = func(_ context.Context, _, _, component, _, env, _ string) (string, error) {
		return fmt.Sprintf("http://%s.dp-%s.svc.cluster.local:8080", component, env), nil
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, scopedTokenMiddleware(t))
	ouID := jwtassertion.MockOUID
	gdb := db.DB(context.Background())
	keyRepo := repositories.NewAPIKeyRepo(gdb)

	// One gateway in the org, not mapped to either environment: issued keys are still sent to it, and the bindings
	// stay unplaced (NO_GATEWAY), which a key does not need.
	gatewayID := uuid.New()
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, "pk-gw-"+suffix, "pk-gw-"+suffix, "gw.example.com", models.GatewayRoleBoth, ouID,
	).Error)
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, ouID, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})

	do := func(method, path string, body any, scopes ...string) *httptest.ResponseRecorder {
		t.Helper()
		var reader *bytes.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			reader = bytes.NewReader(raw)
		} else {
			reader = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, fmt.Sprintf("/api/v1/orgs/%s/projects/%s/mcp-proxies%s", testOrgName, project, path), reader)
		req.Header.Set("Content-Type", "application/json")
		if scopes != nil {
			req.Header[keyTestScopesHeader] = scopes
		}
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		return rr
	}

	handle := "keys-" + suffix
	rr := do(http.MethodPut, "/"+handle, map[string]any{
		"name": "Keys " + suffix, "context": "/keys",
		"source": map[string]string{"componentName": handle, "endpointName": "mcp"},
	})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	for _, env := range []string{devEnv, stgEnv} {
		rr = do(http.MethodPut, "/"+handle+"/environments/"+env, map[string]any{})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	}
	keysPath := func(env string) string { return "/" + handle + "/environments/" + env + "/api-keys" }
	artifactOf := func(t *testing.T, env string) uuid.UUID {
		t.Helper()
		var raw string
		require.NoError(t, gdb.Raw(`SELECT b.artifact_uuid::text FROM mcp_proxy_endpoint_environments b
			JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ? AND b.environment_uuid = ?`,
			handle, ouID, envs[env]).Row().Scan(&raw))
		id, err := uuid.Parse(raw)
		require.NoError(t, err)
		return id
	}
	storedKeys := func(t *testing.T, artifact uuid.UUID) []models.StoredAPIKey {
		t.Helper()
		keys, err := keyRepo.ListByArtifact(context.Background(), artifact.String())
		require.NoError(t, err)
		return keys
	}
	type issued struct {
		Status string `json:"status"`
		KeyID  string `json:"keyId"`
		APIKey string `json:"apiKey"`
	}
	decodeIssued := func(t *testing.T, rr *httptest.ResponseRecorder) issued {
		t.Helper()
		var out issued
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out), rr.Body.String())
		return out
	}
	listNames := func(t *testing.T, env string) []string {
		t.Helper()
		rr := do(http.MethodGet, keysPath(env), nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var list models.ListAPIKeysResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
		names := []string{}
		for _, k := range list.Keys {
			names = append(names, k.Name)
			assert.NotContains(t, rr.Body.String(), "apiKeyHash", "the list never carries the hash")
		}
		return names
	}

	var devValue string
	t.Run("POST issues a key: 201, the value once, stored under the binding's artifact", func(t *testing.T) {
		rr := do(http.MethodPost, keysPath(devEnv), map[string]any{"name": "ip-users", "displayName": "IP users"})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		got := decodeIssued(t, rr)
		assert.Equal(t, "success", got.Status)
		assert.Equal(t, "ip-users", got.KeyID)
		require.NotEmpty(t, got.APIKey)
		devValue = got.APIKey

		keys := storedKeys(t, artifactOf(t, devEnv))
		require.Len(t, keys, 1)
		assert.Equal(t, "ip-users", keys[0].Name)
		assert.Equal(t, "IP users", keys[0].DisplayName)
		assert.Equal(t, models.APIKeyPurposeUserManaged, keys[0].Purpose)
		assert.NotEqual(t, devValue, keys[0].APIKeyHash, "only the hash is stored")
		assert.NotContains(t, keys[0].MaskedAPIKey, devValue)

		assert.Equal(t, []string{"ip-users"}, listNames(t, devEnv))
		assert.Empty(t, listNames(t, stgEnv), "a key is per environment")
	})

	t.Run("a key issued from displayName alone is named from it", func(t *testing.T) {
		rr := do(http.MethodPost, keysPath(stgEnv), map[string]any{"displayName": "Staging Users"})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		assert.NotEmpty(t, decodeIssued(t, rr).KeyID)
		assert.Len(t, listNames(t, stgEnv), 1)
	})

	t.Run("the gateway key sync includes the key, under the binding's artifact", func(t *testing.T) {
		synced, err := keyRepo.ListByArtifactKind(ouID, models.KindMCPMapping)
		require.NoError(t, err)
		dev := artifactOf(t, devEnv)
		found := false
		for _, k := range synced {
			if k.ArtifactUUID == dev && k.Name == "ip-users" {
				found = true
			}
		}
		assert.True(t, found, "a gateway that connects later receives the key")
	})

	t.Run("refusals and their codes", func(t *testing.T) {
		rr := do(http.MethodPost, keysPath(devEnv), map[string]any{"name": "ip-users"})
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeConflict, errorCode(t, rr))
		keys := storedKeys(t, artifactOf(t, devEnv))
		require.Len(t, keys, 1)

		rr = do(http.MethodPost, keysPath(devEnv), map[string]any{})
		require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())

		rr = do(http.MethodPost, keysPath(devEnv), map[string]any{"name": "ab"})
		require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeValidation, errorCode(t, rr))

		rr = do(http.MethodPost, keysPath(devEnv), map[string]any{"name": "dated", "expiresAt": "next tuesday"})
		require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeValidation, errorCode(t, rr))

		rr = do(http.MethodPost, keysPath(devEnv), map[string]any{
			"name": "expired", "expiresAt": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		})
		require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeValidation, errorCode(t, rr))

		rr = do(http.MethodPost, "/"+handle+"/environments/prod-"+suffix+"/api-keys", map[string]any{"name": "ip-users"})
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())

		rr = do(http.MethodPost, "/missing-"+suffix+"/environments/"+devEnv+"/api-keys", map[string]any{"name": "ip-users"})
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())

		rr = do(http.MethodPut, keysPath(devEnv)+"/unknown", nil)
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
		assert.Len(t, storedKeys(t, artifactOf(t, devEnv)), 1, "rotating an unknown key creates nothing")

		rr = do(http.MethodDelete, keysPath(devEnv)+"/unknown", nil)
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	})

	t.Run("the permission gate: list needs read, issue needs api-key-manage", func(t *testing.T) {
		read := rbac.MCPServerRead.Scope()
		rr := do(http.MethodGet, keysPath(devEnv), nil, read)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

		rr = do(http.MethodPost, keysPath(devEnv), map[string]any{"name": "no-scope"}, read, rbac.MCPServerUpdate.Scope())
		require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		rr = do(http.MethodPut, keysPath(devEnv)+"/ip-users", nil, read)
		require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		rr = do(http.MethodDelete, keysPath(devEnv)+"/ip-users", nil, read)
		require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())

		rr = do(http.MethodPost, keysPath(devEnv), map[string]any{"name": "scoped"}, rbac.MCPServerAPIKeyManage.Scope())
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		rr = do(http.MethodDelete, keysPath(devEnv)+"/scoped", nil, rbac.MCPServerAPIKeyManage.Scope())
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	})

	t.Run("PUT rotates: 200, a new value, the stored hash replaced", func(t *testing.T) {
		before := storedKeys(t, artifactOf(t, devEnv))[0].APIKeyHash
		rr := do(http.MethodPut, keysPath(devEnv)+"/ip-users", nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		got := decodeIssued(t, rr)
		require.NotEmpty(t, got.APIKey)
		assert.NotEqual(t, devValue, got.APIKey)
		after := storedKeys(t, artifactOf(t, devEnv))
		require.Len(t, after, 1)
		assert.NotEqual(t, before, after[0].APIKeyHash)
	})

	t.Run("concurrent rotations reach the gateway in the order they were stored", func(t *testing.T) {
		const n = 8
		var wg sync.WaitGroup
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// A rotation that lost the race to another is refused; the winners' order is what is checked.
				do(http.MethodPut, keysPath(devEnv)+"/ip-users", nil)
			}()
		}
		wg.Wait()

		stored := storedKeys(t, artifactOf(t, devEnv))
		var want string
		for _, k := range stored {
			if k.Name == "ip-users" {
				want = k.APIKeyHash
			}
		}
		require.NotEmpty(t, want)

		// The gateway applies its events in seq order, so the last event for the key must carry the stored value.
		rows, err := gdb.Raw(`SELECT event_data FROM eventhub_events WHERE gateway_id = ? AND entity_type IN (?, ?)
			ORDER BY seq`, gatewayID.String(), "apikey.created", "apikey.updated").Rows()
		require.NoError(t, err)
		defer rows.Close()
		var last string
		for rows.Next() {
			var data string
			require.NoError(t, rows.Scan(&data))
			var envelope struct {
				Payload struct {
					APIID        string `json:"apiId"`
					Name         string `json:"name"`
					KeyName      string `json:"keyName"`
					APIKeyHashes string `json:"apiKeyHashes"`
				} `json:"payload"`
			}
			require.NoError(t, json.Unmarshal([]byte(data), &envelope), data)
			event := envelope.Payload
			if event.APIID == artifactOf(t, devEnv).String() && (event.KeyName == "ip-users" || event.Name == "ip-users") {
				last = event.APIKeyHashes
			}
		}
		require.NoError(t, rows.Err())
		assert.Equal(t, fmt.Sprintf(`{"sha256":"%s"}`, want), last, "the gateway ends on the stored value")
	})

	t.Run("PUT with no expiresAt keeps the key's expiry", func(t *testing.T) {
		expiry := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
		rr := do(http.MethodPost, keysPath(devEnv), map[string]any{"name": "dated", "expiresAt": expiry.Format(time.RFC3339)})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		dated := func() models.StoredAPIKey {
			for _, k := range storedKeys(t, artifactOf(t, devEnv)) {
				if k.Name == "dated" {
					return k
				}
			}
			t.Fatal("key dated not stored")
			return models.StoredAPIKey{}
		}
		before := dated().APIKeyHash

		rr = do(http.MethodPut, keysPath(devEnv)+"/dated", nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		after := dated()
		assert.NotEqual(t, before, after.APIKeyHash)
		require.NotNil(t, after.ExpiresAt, "a plain rotate keeps the expiry")
		assert.True(t, expiry.Equal(*after.ExpiresAt), "a plain rotate keeps the expiry")

		rr = do(http.MethodDelete, keysPath(devEnv)+"/dated", nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	})

	// Concurrent requests: each name and test key slot is claimed by one request, so every key handed out is the one
	// stored, never one a concurrent request overwrote.
	t.Run("concurrent creates of one name: one 201, the rest 409, and the stored key is the 201's", func(t *testing.T) {
		const n = 8
		codes, values := make([]int, n), make([]string, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rr := do(http.MethodPost, keysPath(devEnv), map[string]any{"name": "racy"})
				codes[i] = rr.Code
				if rr.Code == http.StatusCreated {
					var out issued
					if json.Unmarshal(rr.Body.Bytes(), &out) == nil {
						values[i] = out.APIKey
					}
				}
			}()
		}
		wg.Wait()
		var winner string
		for i, code := range codes {
			switch code {
			case http.StatusCreated:
				require.Empty(t, winner, "only one create may win the name")
				winner = values[i]
			case http.StatusConflict:
			default:
				t.Fatalf("unexpected status %d", code)
			}
		}
		require.NotEmpty(t, winner)
		key, err := keyRepo.GetByArtifactAndName(artifactOf(t, devEnv).String(), "racy")
		require.NoError(t, err)
		assert.Equal(t, apiKeyHash(winner), key.APIKeyHash, "the stored key is the one handed out")

		rr := do(http.MethodDelete, keysPath(devEnv)+"/racy", nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	})

	t.Run("concurrent test keys for one user fill distinct slots, each stored as handed out", func(t *testing.T) {
		const n = 3 // one per slot: none may overwrite another
		values := make([]string, n)
		ids := make([]string, n)
		codes := make([]int, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rr := do(http.MethodPost, keysPath(devEnv)+"/test", map[string]any{"forUser": "racer-" + suffix})
				codes[i] = rr.Code
				var out issued
				if json.Unmarshal(rr.Body.Bytes(), &out) == nil {
					values[i], ids[i] = out.APIKey, out.KeyID
				}
			}()
		}
		wg.Wait()
		stored := map[string]string{}
		for _, k := range storedKeys(t, artifactOf(t, devEnv)) {
			stored[k.Name] = k.APIKeyHash
		}
		seen := map[string]bool{}
		for i := range n {
			require.Equal(t, http.StatusOK, codes[i])
			assert.False(t, seen[ids[i]], "each request gets its own slot")
			seen[ids[i]] = true
			assert.Equal(t, apiKeyHash(values[i]), stored[ids[i]], "the key handed out is the one stored")
		}

		require.NoError(t, gdb.Exec(`DELETE FROM api_keys WHERE artifact_uuid = ? AND purpose = ?`,
			artifactOf(t, devEnv), models.APIKeyPurposeTest).Error)
	})

	t.Run("security off: issue and rotate are 409 API_KEY_SECURITY_DISABLED, revoke still works", func(t *testing.T) {
		rr := do(http.MethodPut, "/"+handle+"/environments/"+devEnv, map[string]any{"security": map[string]any{"enabled": false}})
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

		rr = do(http.MethodPost, keysPath(devEnv), map[string]any{"name": "after-off"})
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeAPIKeySecurityDisabled, errorCode(t, rr))
		rr = do(http.MethodPut, keysPath(devEnv)+"/ip-users", nil)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeAPIKeySecurityDisabled, errorCode(t, rr))

		rr = do(http.MethodDelete, keysPath(devEnv)+"/ip-users", nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		assert.Empty(t, storedKeys(t, artifactOf(t, devEnv)))
		assert.Empty(t, listNames(t, devEnv))

		// A gateway that missed the revoke event is reconciled by its key sync on reconnect, which no longer
		// carries the key.
		synced, err := keyRepo.ListByArtifactKind(ouID, models.KindMCPMapping)
		require.NoError(t, err)
		for _, k := range synced {
			assert.False(t, k.ArtifactUUID == artifactOf(t, devEnv) && k.Name == "ip-users", "a revoked key is not synced")
		}
		var revokes int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM eventhub_events WHERE gateway_id = ? AND entity_type = ?
			AND event_data LIKE ?`, gatewayID.String(), "apikey.revoked", `%"keyName":"ip-users"%`).Scan(&revokes).Error)
		assert.Positive(t, revokes, "the revoke event is stored for the gateway")
	})

	t.Run("removing the binding deletes its keys", func(t *testing.T) {
		stg := artifactOf(t, stgEnv)
		require.Len(t, storedKeys(t, stg), 1)

		rr := do(http.MethodDelete, "/"+handle+"/environments/"+stgEnv, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		assert.Empty(t, storedKeys(t, stg), "the keys cascade with the binding's artifact")

		rr = do(http.MethodGet, keysPath(stgEnv), nil)
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	})
}
