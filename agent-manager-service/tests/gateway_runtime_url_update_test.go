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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The gateway extension chart's pre-upgrade hook PUTs runtimeUrl on an already-registered
// gateway, and that PUT is the only mechanism that repairs a NULL runtime_url on an
// existing install. These tests drive it through the real repository and re-read the column
// with a raw query, so a runtime_url dropped from GatewayRepo.UpdateGateway's explicit
// Updates map surfaces as a silent no-op here instead of in production.
func TestUpdateGatewayPersistsRuntimeURLColumn(t *testing.T) {
	tx := runtimeURLTestTx(t)
	id := seedRuntimeURLGateway(t, tx, "api-platform-acme-dev")
	require.False(t, runtimeURLOf(t, tx, id).Valid, "fixture must start from the NULL state")

	svc := services.NewPlatformGatewayService(repositories.NewGatewayRepo(tx), nil)
	want := "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893"

	resp, err := svc.UpdateGateway(id.String(), gatewayOrgOf(t, tx, id), nil, nil, nil, nil, &want)
	require.NoError(t, err)
	require.Equal(t, want, resp.RuntimeURL)

	got := runtimeURLOf(t, tx, id)
	require.True(t, got.Valid, "runtime_url must be written to the column, not just the response")
	require.Equal(t, want, got.String)
}

// An invalid address never reaches the column.
func TestUpdateGatewayRejectsInvalidRuntimeURL(t *testing.T) {
	tx := runtimeURLTestTx(t)
	id := seedRuntimeURLGateway(t, tx, "api-platform-acme-dev")

	svc := services.NewPlatformGatewayService(repositories.NewGatewayRepo(tx), nil)
	bad := "http://exfil.dev:53"

	_, err := svc.UpdateGateway(id.String(), gatewayOrgOf(t, tx, id), nil, nil, nil, nil, &bad)
	require.ErrorIs(t, err, utils.ErrBadRequest)
	require.False(t, runtimeURLOf(t, tx, id).Valid)
}
