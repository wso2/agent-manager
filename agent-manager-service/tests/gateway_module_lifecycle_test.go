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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gatewaytoken"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// The AI Gateway switches off and on at runtime: disabled, its routes answer
// 404 and its worker closes the gateway it built; enabled, a new one is built.
// Uninstalling and installing it again keeps its data. A purge is not tried
// here: integration packages share one database, and emptying the gateway's
// tables would pull rows from under the other packages' tests (the module
// host's own tests cover purge on a database of their own).
func TestGatewayModuleSwitchesOffAndOn(t *testing.T) {
	// The module's workers act on every organization. On PostgreSQL the
	// packages share one database in parallel, so they would place and render
	// other packages' rows mid-test; SQLite runs them serially.
	if config.GetConfig().Database.Driver != "sqlite" {
		t.Skip("runs the gateway's background workers; runs on SQLite, where packages run serially")
	}
	ctx := context.Background()
	app, host := apitestutils.MakeAppClientWithWorkers(t, wiring.TestClients{
		PlatformRepository: apitestutils.CreateMockPlatformRepository(),
		SecretMgmtClient:   apitestutils.CreateMockSecretManagementClient(),
	}, jwtassertion.NewMockMiddleware(t))
	list := func() int {
		req := httptest.NewRequest(http.MethodGet, "/m/ai-gateway/gateways", nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		return rr.Code
	}
	leases := func() int64 {
		var n int64
		require.NoError(t, db.GetDB().Raw(`SELECT count(*) FROM mod_ai_gateway_locks WHERE name = 'drift_scan'`).Scan(&n).Error)
		return n
	}

	require.Equal(t, http.StatusOK, list())
	first := aigateway.Current()
	require.NotNil(t, first)

	require.NoError(t, host.Disable(ctx, aigateway.Name))
	assert.Equal(t, http.StatusNotFound, list())
	assert.Nil(t, aigateway.Current(), "disabling stops the worker, which closes the gateway")

	require.NoError(t, host.Enable(ctx, aigateway.Name))
	assert.Equal(t, http.StatusOK, list())
	assert.NotSame(t, first, aigateway.Current(), "enabling builds a new gateway")

	// With a default gateway configured, installing must not write inside
	// the install's transaction (on SQLite that waits on itself); the worker
	// registers it once the install is in effect.
	token, err := gatewaytoken.New()
	require.NoError(t, err)
	t.Setenv("MODULE_AI_GATEWAY_DEFAULT_GATEWAY_NAME", "lifecycle-gw")
	t.Setenv("MODULE_AI_GATEWAY_DEFAULT_GATEWAY_VHOST", "http://lifecycle-gw.localhost:19080")
	t.Setenv("MODULE_AI_GATEWAY_DEFAULT_GATEWAY_TOKEN", token)
	require.NoError(t, host.Uninstall(ctx, aigateway.Name, false))
	assert.Equal(t, http.StatusNotFound, list())
	_, err = host.Install(ctx, aigateway.Name, true)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, list())
	assert.EqualValues(t, 1, leases(), "uninstalling without a purge keeps the drift scan lease")
	assert.Eventually(t, func() bool {
		var n int64
		require.NoError(t, db.GetDB().Raw(`SELECT count(*) FROM mod_ai_gateway_gateways WHERE name = 'lifecycle-gw'`).Scan(&n).Error)
		return n == 1
	}, 10*time.Second, 100*time.Millisecond, "the worker registers the configured default gateway")
}
