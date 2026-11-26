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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// coreWorld is the core API over the real platform repository, signed in to a
// fresh organization nobody else uses, so a test can rerun on a kept database.
type coreWorld struct {
	org    string // the OU id the tokens carry
	handle string // the org handle the paths use
	// admin holds every Admin permission; reader holds only the read ones;
	// stranger is an admin of another organization calling the same paths.
	admin, reader, stranger http.Handler
}

// readerScopes are the read permissions of the core routes.
var readerScopes = []rbac.Permission{
	rbac.OrgView, rbac.ProjectRead, rbac.AgentRead, rbac.EventsRead, rbac.EnvironmentRead,
}

func adminScopes() []rbac.Permission {
	return rbac.PredefinedRolePermissions[rbac.RoleAdmin]
}

// signedIn signs every request in as a member of the org holding perms.
func signedIn(ouID, handle string, perms []rbac.Permission) jwtassertion.Middleware {
	scopes := make([]string, 0, len(perms))
	for _, p := range perms {
		scopes = append(scopes, p.Scope())
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := &jwtassertion.TokenClaims{
				Sub: "core-tester", Scope: strings.Join(scopes, " "), OuId: ouID, OuHandle: handle,
				RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
			}
			ctx := jwtassertion.ContextWithTokenClaimsAndScope(r.Context(), claims)
			ctx = jwtassertion.ContextWithJWT(ctx, "test-token")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func freshOrg() (ouID, handle string) {
	ouID = uuid.Must(uuid.NewV7()).String()
	return ouID, "core-" + ouID[len(ouID)-8:]
}

func newCoreWorld(t *testing.T) coreWorld {
	t.Helper()
	org, handle := freshOrg()
	other, otherHandle := freshOrg()
	build := func(auth jwtassertion.Middleware) http.Handler {
		return apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{
			PlatformRepository: repositories.NewPlatformRepo(db.GetDB()),
			SecretMgmtClient:   apitestutils.CreateMockSecretManagementClient(),
		}, auth)
	}
	return coreWorld{
		org:      org,
		handle:   handle,
		admin:    build(signedIn(org, handle, adminScopes())),
		reader:   build(signedIn(org, handle, readerScopes)),
		stranger: build(signedIn(other, otherHandle, adminScopes())),
	}
}

// path is an API path under the world's organization.
func (w coreWorld) path(format string, a ...any) string {
	return "/api/v1/orgs/" + w.handle + fmt.Sprintf(format, a...)
}

// projectsPath is a project API path; the organization comes from the caller's token.
func projectsPath(format string, a ...any) string {
	return "/api/v1/projects" + fmt.Sprintf(format, a...)
}

// projectDocument is a Project document body.
func projectDocument(name, displayName string, description *string) spec.ProjectDocument {
	return spec.ProjectDocument{
		ApiVersion: "ai-control-plane.choreolabs.com/v1",
		Kind:       "Project",
		Metadata:   spec.DocumentMetadata{Name: name},
		Spec:       spec.ProjectSpec{DisplayName: displayName, Description: description},
	}
}

// agentsPath is an agent API path; the organization comes from the caller's token.
func agentsPath(format string, a ...any) string {
	return "/api/v1/agents" + fmt.Sprintf(format, a...)
}

// call sends body (marshalled unless it is a string) and returns the status
// and the response body.
func call(t *testing.T, h http.Handler, method, path string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		require.NoError(t, err)
		r = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	return rec.Code, out
}

// mustCall is call that fails the test unless the status is want.
func mustCall(t *testing.T, h http.Handler, want int, method, path string, body any) []byte {
	t.Helper()
	status, out := call(t, h, method, path, body)
	require.Equal(t, want, status, "%s %s: %s", method, path, out)
	return out
}

func decodeAs[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(body, &v), string(body))
	return v
}

// apiError is the error body every core route answers with.
type apiError struct {
	Message string `json:"message"`
	Reason  string `json:"reason"`
	Code    string `json:"code"`
}

func errorOf(t *testing.T, body []byte) apiError {
	t.Helper()
	return decodeAs[apiError](t, body)
}
