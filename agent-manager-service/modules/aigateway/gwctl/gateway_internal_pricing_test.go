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

package gwctl

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// TestPushPricingRecordsWhatItReceivedAndAdded pins the audit record of a price push: the
// gateway, how many models the file listed and how many were new.
func TestPushPricingRecordsWhatItReceivedAndAdded(t *testing.T) {
	gdb := db.GetDB()
	require.NoError(t, gdb.Exec("DELETE FROM mod_ai_gateway_model_prices").Error)
	t.Cleanup(func() { require.NoError(t, gdb.Exec("DELETE FROM mod_ai_gateway_model_prices").Error) })
	ouID := dbtest.Org(t, gdb)
	gateway := gwtest.GatewayIn(t, gdb, ouID, gwmodels.GatewayRoleBoth)
	gatewayService := gwsvc.NewPlatformGatewayService(gwrepo.NewGatewayRepo(gdb))
	token, err := gatewayService.RotateToken(gateway.UUID.String(), ouID)
	require.NoError(t, err)
	ctrl := &gatewayInternalController{
		gatewayService: gatewayService,
		pricingService: gwsvc.NewPricingService(gwrepo.NewModelPriceRepository(gdb)),
	}

	push := func(body string) (int, audit.Event) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.SetPathValue("gatewayId", gateway.UUID.String())
		req.Header.Set("api-key", token.Token)
		ctx, _ := audit.NewRequestScope(req.Context())
		w := httptest.NewRecorder()
		ctrl.PushPricing(w, req.WithContext(ctx))
		return w.Code, audit.BuildEvent(ctx, "ai-gateway:gateways.push-pricing")
	}

	code, event := push(`{"a": {}, "b": {}, "c": 5}`)
	require.Equal(t, http.StatusNoContent, code)
	assert.Equal(t, "gateway", event.ResourceType)
	assert.Equal(t, gateway.UUID.String(), event.ResourceID)
	assert.EqualValues(t, 3, event.Details["modelCount"])
	assert.EqualValues(t, 2, event.Details["addedCount"])

	code, event = push(`{"a": {}, "b": {}, "d": {}}`)
	require.Equal(t, http.StatusNoContent, code)
	assert.EqualValues(t, 3, event.Details["modelCount"])
	assert.EqualValues(t, 1, event.Details["addedCount"], "a, b are already stored")

	code, event = push(`[]`)
	require.Equal(t, http.StatusBadRequest, code)
	assert.EqualValues(t, 0, event.Details["addedCount"], "a refused file adds nothing, and is still recorded")
	assert.Equal(t, gateway.UUID.String(), event.ResourceID)
}
