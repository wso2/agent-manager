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
	"context"
	"net/http"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/api"
	"github.com/wso2/agent-manager/agent-manager-service/builtin"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway"
	_ "github.com/wso2/agent-manager/agent-manager-service/modules/legacy" // the legacy routes
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// MakeAppClientWithDeps creates an HTTP handler with the provided dependencies for testing
func MakeAppClientWithDeps(t *testing.T, testClients wiring.TestClients, authMiddleware jwtassertion.Middleware) http.Handler {
	// Use wire to initialize the app parameters with test clients
	db := db.DB(context.Background())
	appParams, err := wiring.InitializeTestAppParamsWithClientMocks(config.GetConfig(), db, authMiddleware, testClients)
	if err != nil {
		t.Fatalf("failed to initialize test app params: %v", err)
	}
	// The full distribution's features, built on the core as the server
	// builds them; their workers are not started.
	bundles, err := builtin.Build(context.Background(), appParams)
	if err != nil {
		t.Fatalf("failed to build the built-in features: %v", err)
	}
	// No module host runs here, so the core reaches the gateway's
	// capabilities directly.
	coreext.Set(coreext.Static(aigateway.Capabilities(aigateway.Current())))
	t.Cleanup(func() { coreext.Set(nil) })

	// Create HTTP handler
	handler := api.MakeHTTPHandlerWithModules(appParams, nil, nil, nil, bundles)

	// Return the handler instance
	return handler
}
