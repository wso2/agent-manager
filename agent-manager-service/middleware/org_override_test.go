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
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
)

const testImpersonatedOUID = "01900000-0000-7000-8000-000000000002"

func requestWithImpersonationHeader(value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/acme/gateways", nil)
	if value != "" {
		r.Header.Set(occlient.HeaderImpersonateOrg, value)
	}
	return r
}

func installOrgOverrideResolver(t *testing.T, resolver OrgOverrideResolver) {
	t.Helper()
	SetOrgOverrideResolver(resolver)
	t.Cleanup(func() { SetOrgOverrideResolver(nil) })
}

// With no resolver installed — every on-premises deployment — a caller cannot
// switch organizations, whatever header it sends.
func TestOrgOverride_IgnoredWithoutResolver(t *testing.T) {
	installOrgOverrideResolver(t, nil)

	ouID, ok := OrgOverride(requestWithImpersonationHeader(testImpersonatedOUID))

	assert.False(t, ok)
	assert.Empty(t, ouID)
}

func TestOrgOverride_UsesInstalledResolver(t *testing.T) {
	installOrgOverrideResolver(t, func(r *http.Request) (string, bool) {
		// A real resolver checks the caller's token first; this one trusts a
		// marker header to stand in for that decision.
		if r.Header.Get("X-Test-Trusted") != "yes" {
			return "", false
		}
		return ImpersonatedOrgHeader(r)
	})

	untrusted := requestWithImpersonationHeader(testImpersonatedOUID)
	_, ok := OrgOverride(untrusted)
	assert.False(t, ok, "the resolver decides who may impersonate, not the header")

	trusted := requestWithImpersonationHeader(testImpersonatedOUID)
	trusted.Header.Set("X-Test-Trusted", "yes")
	ouID, ok := OrgOverride(trusted)
	require.True(t, ok)
	assert.Equal(t, testImpersonatedOUID, ouID)
}

func TestImpersonatedOrgHeader(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header string
		want   string
		wantOK bool
	}{
		{name: "absent", header: "", wantOK: false},
		{name: "well-formed UUID", header: testImpersonatedOUID, want: testImpersonatedOUID, wantOK: true},
		{name: "surrounding whitespace is trimmed", header: "  " + testImpersonatedOUID + " ", want: testImpersonatedOUID, wantOK: true},
		{name: "not a UUID", header: "acme", wantOK: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ImpersonatedOrgHeader(requestWithImpersonationHeader(tc.header))
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
