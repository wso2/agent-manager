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

package framework

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewPersonaProvisionerScopesSystemTokenToThunderResource(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("scope"); got != "system" {
			t.Errorf("scope = %q, want system", got)
		}
		if got, want := r.Form.Get("resource"), "http://thunder.test/mcp"; got != want {
			t.Errorf("resource = %q, want %q", got, want)
		}
		_, _ = io.WriteString(w, `{"access_token":"system-token","expires_in":300}`)
	})

	cfg := &Config{
		ThunderAdminURL:           "http://thunder.test",
		ThunderSystemResource:     "http://thunder.test/mcp",
		ThunderSystemClientID:     "system-id",
		ThunderSystemClientSecret: "system-secret",
	}
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		recorder := newResponseRecorder()
		handler.ServeHTTP(recorder, r)
		return recorder.result(r), nil
	})}
	provisioner, err := newPersonaProvisionerWithHTTPClient(context.Background(), cfg, httpClient)
	if err != nil {
		t.Fatalf("NewPersonaProvisioner() error = %v", err)
	}
	if provisioner.systemToken != "system-token" {
		t.Fatalf("systemToken = %q, want system-token", provisioner.systemToken)
	}
}

func TestRolePersonaLifecycle(t *testing.T) {
	t.Parallel()

	assigned := false
	deleted := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oauth2/token":
			clientID, clientSecret, ok := r.BasicAuth()
			if !ok {
				t.Error("token request omitted basic authentication")
			}
			if clientID != "persona-id" || clientSecret != "persona-key" || !assigned {
				http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
				return
			}
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"expires_in":300}`,
				unsignedTestJWT("amp:project:create amp:project:read"))
		case r.Method == http.MethodGet && r.URL.Path == "/organization-units/tree/default":
			assertBearer(t, r, "system-token")
			_, _ = fmt.Fprint(w, `{"id":"ou-1"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/roles":
			assertBearer(t, r, "system-token")
			_, _ = fmt.Fprint(w, `{"roles":[{"id":"role-1","ouId":"ou-1","name":"developer"}],"totalResults":1}`)
		case r.Method == http.MethodGet && r.URL.Path == "/roles/role-1":
			assertBearer(t, r, "system-token")
			// Second group's scope must be excluded from RolePermissions.
			_, _ = fmt.Fprint(w, `{"id":"role-1","ouId":"ou-1","name":"developer","permissions":[`+
				`{"resourceServerId":"amp-resource-server","permissions":["amp:project:create","amp:project:read"]},`+
				`{"resourceServerId":"amp-agent-manager-mcp-resource-srv","permissions":["amp:project:read"]}`+
				`]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/applications":
			assertBearer(t, r, "system-token")
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode application request: %v", err)
			}
			if got := payload["type"]; got != "m2m" {
				t.Errorf("application type = %v, want m2m", got)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"id":"app-1","clientId":"persona-id","inboundAuthConfig":[{"config":{"clientSecret":"persona-key"}}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/roles/role-1/assignments/add":
			assertBearer(t, r, "system-token")
			assigned = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/roles/role-1/assignments/remove":
			assertBearer(t, r, "system-token")
			assigned = false
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/applications/app-1":
			assertBearer(t, r, "system-token")
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	})
	cfg := &Config{
		ThunderAdminURL:           "http://thunder.test",
		ThunderSystemClientID:     "system-id",
		ThunderSystemClientSecret: "system-key",
	}
	provisioner := &PersonaProvisioner{
		cfg: cfg,
		httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			recorder := newResponseRecorder()
			handler.ServeHTTP(recorder, r)
			return recorder.result(r), nil
		})},
		systemToken: "system-token",
	}
	persona, err := provisioner.CreateRolePersona(context.Background(), "developer")
	if err != nil {
		t.Fatalf("CreateRolePersona() error = %v", err)
	}
	if persona.RoleName != "developer" || len(persona.RolePermissions) != 2 || !assigned {
		t.Fatalf("unexpected provisioned persona: %+v, assigned=%v", persona, assigned)
	}
	if _, err := provisioner.RefreshRolePersonaToken(context.Background(), persona); err != nil {
		t.Fatalf("RefreshRolePersonaToken() error = %v", err)
	}
	if err := provisioner.DeleteRolePersona(context.Background(), persona); err != nil {
		t.Fatalf("DeleteRolePersona() error = %v", err)
	}
	if assigned || !deleted {
		t.Fatalf("persona cleanup incomplete: assigned=%v deleted=%v", assigned, deleted)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// responseRecorder is the small part of httptest.ResponseRecorder needed here,
// kept local so the unit test does not need to bind a TCP port.
type responseRecorder struct {
	header http.Header
	body   strings.Builder
	status int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{header: make(http.Header), status: http.StatusOK}
}

func (r *responseRecorder) Header() http.Header { return r.header }

func (r *responseRecorder) Write(body []byte) (int, error) { return r.body.Write(body) }

func (r *responseRecorder) WriteHeader(status int) { r.status = status }

func (r *responseRecorder) result(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: r.status,
		Header:     r.header,
		Body:       io.NopCloser(strings.NewReader(r.body.String())),
		Request:    req,
	}
}

func assertBearer(t *testing.T, r *http.Request, token string) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+token {
		t.Errorf("Authorization = %q, want bearer token", got)
	}
}

func unsignedTestJWT(scopes string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"scope":%q}`, scopes)))
	return strings.Join([]string{header, payload, "signature"}, ".")
}
