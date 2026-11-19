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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// The AI Gateway switches off and on at runtime: disabled, its routes answer
// 404 and its worker closes the gateway it built; enabled, a new one is built.
// Purging it and installing it again restores the lease row its migration
// seeded.
func TestGatewayModuleSwitchesOffAndOn(t *testing.T) {
	ctx := context.Background()
	app, host := apitestutils.MakeAppClientWithHost(t, wiring.TestClients{
		PlatformRepository: apitestutils.CreateMockPlatformRepository(),
		SecretMgmtClient:   apitestutils.CreateMockSecretManagementClient(),
	}, jwtassertion.NewMockMiddleware(t))
	list := func() int {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/m/ai-gateway/orgs/%s/gateways", testOrgName), nil)
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

	require.NoError(t, host.Uninstall(ctx, aigateway.Name, true))
	assert.Equal(t, http.StatusNotFound, list())
	assert.Zero(t, leases(), "purging empties the module's tables")
	_, err := host.Install(ctx, aigateway.Name, true)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, list())
	assert.EqualValues(t, 1, leases(), "installing again restores the drift scan lease")
}
