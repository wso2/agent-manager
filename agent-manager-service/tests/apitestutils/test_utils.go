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
	"log/slog"
	"net/http"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/api"
	"github.com/wso2/agent-manager/agent-manager-service/app"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway"
	_ "github.com/wso2/agent-manager/agent-manager-service/modules/aigateway" // the AI Gateway
	_ "github.com/wso2/agent-manager/agent-manager-service/modules/legacy"    // what no module owns yet
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
	// The built-in modules and the AI Gateway, which the full distribution
	// installs by default, run on a module host, as in the server: their
	// routes, capabilities and hooks (the gateway's finalize hooks remove
	// its rows of a deleted agent).
	var mods []modkit.Module
	for _, m := range modkit.Registered() {
		if mf := m.Manifest(); mf.Builtin || mf.Name == aigateway.Name {
			mods = append(mods, m)
		}
	}
	host := modhost.NewWith(db, config.GetConfig().Database.Driver, slog.Default(), mods)
	// The host follows module changes until the test ends.
	ctx, cancel := context.WithCancel(context.Background())
	stop, err := app.StartModules(ctx, host, db, appParams)
	if err != nil {
		cancel()
		t.Fatalf("failed to start the modules: %v", err)
	}
	t.Cleanup(func() { stop(); cancel() })
	if _, err := host.InstallDefaults(ctx, []string{aigateway.Name}); err != nil {
		t.Fatalf("failed to install the default modules: %v", err)
	}

	// Create HTTP handler
	handler := api.MakeHTTPHandlerWithModules(appParams, nil, nil, host)

	// Return the handler instance
	return handler
}
