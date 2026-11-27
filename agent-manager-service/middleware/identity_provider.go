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
	"net/http"
	"net/url"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// ErrCodeIdentityProviderRequired marks a request for an identity provider's users, groups, roles, profiles or agent
// identities made while none is connected.
const ErrCodeIdentityProviderRequired = "IDENTITY_PROVIDER_REQUIRED"

// RequireIdentityProvider refuses the identity routes when sign-in is from a file: /identities/... (users, groups,
// roles, profiles) and /agents/{agent}/identities/... (agent identities). They live in the identity provider, and with
// file sign-in there is none, so the request gets a reason instead of failing against a provider that may not be
// running. It runs before routing, so a path under them that no route serves gets the same 409, not 404.
func RequireIdentityProvider(authMode string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if authMode != config.AuthModeFile {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isIdentityRoute(r.URL) {
				httpx.WriteErrorResponseWithReason(w, http.StatusConflict,
					"Users, groups, roles and agent identities need an identity provider; this deployment signs in from a file (auth.mode: file).",
					"", ErrCodeIdentityProviderRequired)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isIdentityRoute reports whether u is under /identities (users, groups, roles, profiles) or is an agent's
// identity (/agents/{agent}/identities and below). It splits the escaped path, as the router matches segments, so an
// encoded slash stays inside its segment; each segment is unescaped before comparing. A segment that does not unescape
// counts as an identity route, so it is refused rather than passed on.
func isIdentityRoute(u *url.URL) bool {
	raw := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	seg := make([]string, len(raw))
	for i, r := range raw {
		s, err := url.PathUnescape(r)
		if err != nil {
			return true
		}
		seg[i] = s
	}
	switch {
	case len(seg) >= 1 && seg[0] == "identities":
		return true
	case len(seg) >= 3 && seg[0] == "agents" && seg[2] == "identities":
		return true
	}
	return false
}
