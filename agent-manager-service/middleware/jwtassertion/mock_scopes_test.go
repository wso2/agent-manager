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

package jwtassertion_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	_ "github.com/wso2/agent-manager/agent-manager-service/modules/aigateway" // declares ai-gateway:*
)

// A mocked token reaches the routes of the linked modules too, not only the
// core's catalog.
func TestMockTokenCarriesModuleScopes(t *testing.T) {
	var scope string
	h := jwtassertion.NewMockMiddleware(t)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		scope = jwtassertion.GetTokenClaims(r.Context()).Scope
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/orgs/acme/gateways", nil))
	scopes := strings.Fields(scope)
	require.Contains(t, scopes, "amp:ai-gateway:gateways.read")
	require.Contains(t, scopes, "amp:agent:read")
}
