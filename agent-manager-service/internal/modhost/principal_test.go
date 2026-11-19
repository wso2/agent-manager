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

package modhost

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

func TestWithOrgSetsTheCaller(t *testing.T) {
	var got modkit.Principal
	var org string
	h := withOrg(func(_ http.ResponseWriter, r *http.Request) {
		got, org = modkit.PrincipalFrom(r.Context()), modkit.OrgID(r.Context())
	})
	r := httptest.NewRequest(http.MethodGet, "/m/x/things", nil)
	r = r.WithContext(jwtassertion.ContextWithTokenClaims(r.Context(), &jwtassertion.TokenClaims{Sub: "user-1", ClientID: "console", OuId: "org-1"}))
	h(httptest.NewRecorder(), r)
	if got != (modkit.Principal{User: "user-1", ClientID: "console"}) || org != "org-1" {
		t.Errorf("principal %+v org %q, want user-1/console in org-1", got, org)
	}
}
