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

package controllers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestHandleCommonErrorsEnvironmentHasNoGatewayIs409WithItsCode(t *testing.T) {
	w := httptest.NewRecorder()

	handleCommonErrors(w, fmt.Errorf("%w: environment dev has no ingress gateway", utils.ErrEnvironmentHasNoGateway), "fallback")

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"ENVIRONMENT_HAS_NO_GATEWAY"`)
	assert.Contains(t, w.Body.String(), "environment dev has no ingress gateway")
}

func TestHandleCommonErrorsEntitlementDeniedIs402WithTheCheckersMessage(t *testing.T) {
	w := httptest.NewRecorder()

	handleCommonErrors(w, fmt.Errorf("create agent: %w", &utils.EntitlementDeniedError{Message: "Quota limit reached for platform_agents."}), "fallback")

	assert.Equal(t, http.StatusPaymentRequired, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"ENTITLEMENT_DENIED"`)
	assert.Contains(t, w.Body.String(), `"message":"Quota limit reached for platform_agents."`)
}

func TestHandleCommonErrorsEntitlementDeniedWithoutMessageIs402(t *testing.T) {
	w := httptest.NewRecorder()

	handleCommonErrors(w, fmt.Errorf("%w: refused by the control plane", utils.ErrEntitlementDenied), "fallback")

	assert.Equal(t, http.StatusPaymentRequired, w.Code)
	assert.Contains(t, w.Body.String(), "Upgrade your subscription to continue.")
}
