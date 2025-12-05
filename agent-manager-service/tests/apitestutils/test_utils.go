// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package apitestutils

import (
	"net/http"
	"testing"

	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/api"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/config"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/wiring"
)

// MakeAppClientWithDeps creates an HTTP handler with the provided dependencies for testing
func MakeAppClientWithDeps(t *testing.T, testClients wiring.TestClients, authMiddleware jwtassertion.Middleware) http.Handler {
	// Use wire to initialize the app parameters with test clients
	appParams, err := wiring.InitializeTestAppParamsWithClientMocks(config.GetConfig(), authMiddleware, testClients)
	if err != nil {
		t.Fatalf("failed to initialize test app params: %v", err)
	}

	// Create HTTP handler
	handler := api.MakeHTTPHandler(appParams)

	// Return the handler instance
	return handler
}
