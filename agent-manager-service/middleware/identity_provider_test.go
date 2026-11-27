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

package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

func serveThroughIdentityGate(t *testing.T, authMode, path string) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	rec := httptest.NewRecorder()
	RequireIdentityProvider(authMode)(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// With file sign-in there is no identity provider, so its users, groups, roles and profiles are refused with a
// reason rather than passed on to fail there.
func TestRequireIdentityProviderRefusesIdentityRoutesWithFileSignIn(t *testing.T) {
	for _, path := range []string{
		"/identities/users",
		"/identities/users/u1/profile",
		"/identities/groups/g1/members",
		"/identities/roles",
		"/%69dentities/roles",
		// Agent identities live in the provider too.
		"/agents/support-bot/identities",
		"/agents/support-bot/identities/retry",
		// The router keeps an encoded slash inside a segment, so the gate must too.
		"/agents/a%2Fb/identities",
		// The gate runs before routing, so a path no route serves is refused the same way, not 404:
		// there is no provider to answer it either.
		"/identities/no-such-thing",
	} {
		rec := serveThroughIdentityGate(t, config.AuthModeFile, path)
		require.Equal(t, http.StatusConflict, rec.Code, path)
		var body struct{ Message, Code string }
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, ErrCodeIdentityProviderRequired, body.Code)
		assert.Contains(t, body.Message, "identity provider")
	}
}

func TestRequireIdentityProviderPassesEverythingElse(t *testing.T) {
	for _, c := range []struct{ mode, path string }{
		{config.AuthModeIDP, "/identities/users"},
		{config.AuthModeIDP, "/agents/support-bot/identities"},
		{config.AuthModeFile, "/projects"},
		{config.AuthModeFile, "/agents"},
		{config.AuthModeFile, "/agents/identities"},
		{config.AuthModeFile, "/agents/support-bot/token"},
		{config.AuthModeFile, "/projects/identities/agents"},
	} {
		assert.Equal(t, http.StatusTeapot, serveThroughIdentityGate(t, c.mode, c.path).Code, "%s %s", c.mode, c.path)
	}
}
