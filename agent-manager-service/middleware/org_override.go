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
	"strings"
	"sync/atomic"

	"github.com/google/uuid"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
)

// OrgOverrideResolver decides whether a request may act on an organization
// other than the one its token names, and which one. It runs after
// authentication and RequireOrgMatch, so the verified token claims are on the
// request context (jwtassertion.GetTokenClaims).
//
// Returning ok=false keeps the token's organization. An implementation must
// only return ok=true for callers it trusts to act across organizations —
// typically a deployment's own control-plane service token — because the
// handler then reads and writes the returned organization's data.
type OrgOverrideResolver func(r *http.Request) (ouID string, ok bool)

var orgOverrideResolver atomic.Pointer[OrgOverrideResolver]

// SetOrgOverrideResolver installs the deployment's resolver. It is called once
// at startup, before the HTTP server begins serving. nil (the open-source
// default) disables overrides: the token stays the sole source of org identity.
func SetOrgOverrideResolver(resolver OrgOverrideResolver) {
	if resolver == nil {
		orgOverrideResolver.Store(nil)
		return
	}
	orgOverrideResolver.Store(&resolver)
}

// OrgOverride returns the organization a request should act on instead of its
// token's, when the installed resolver allows one. With no resolver installed
// it always returns ok=false, whatever headers the caller sent.
//
// Only the handlers that a deployment's control plane drives on a tenant's
// behalf consult this; every other route uses the token's organization alone.
func OrgOverride(r *http.Request) (string, bool) {
	resolver := orgOverrideResolver.Load()
	if resolver == nil {
		return "", false
	}
	return (*resolver)(r)
}

// ImpersonatedOrgHeader returns the organization UUID from the
// X-Impersonate-Org header, if the caller sent a well-formed one. It performs
// no authorization: an OrgOverrideResolver uses it to read the header only
// after deciding the caller is allowed to impersonate.
func ImpersonatedOrgHeader(r *http.Request) (string, bool) {
	raw := strings.TrimSpace(r.Header.Get(occlient.HeaderImpersonateOrg))
	if raw == "" {
		return "", false
	}
	if _, err := uuid.Parse(raw); err != nil {
		return "", false
	}
	return raw, true
}
