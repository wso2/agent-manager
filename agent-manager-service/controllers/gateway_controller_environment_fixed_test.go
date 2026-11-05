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

func TestHandleGatewayErrorsEnvironmentFixedIs409WithItsCode(t *testing.T) {
	w := httptest.NewRecorder()

	handleGatewayErrors(w, fmt.Errorf("%w: gateway is mapped to another environment", utils.ErrGatewayEnvironmentFixed), "fallback")

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"GATEWAY_ENVIRONMENT_FIXED"`)
}

func TestHandleGatewayErrors_Status(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"token limit", fmt.Errorf("rotate: %w", utils.ErrGatewayTokenLimitReached), http.StatusConflict, "Revoke"},
		{"token not found", utils.ErrGatewayTokenNotFound, http.StatusNotFound, "Token not found"},
		{
			"validation error",
			utils.NewInvalidInputError("Gateway name must be 3 to 64 characters", "gateway name must not exceed 64 characters"),
			http.StatusBadRequest, "Gateway name must be 3 to 64 characters",
		},
		{"bare invalid input", fmt.Errorf("internal detail: %w", utils.ErrInvalidInput), http.StatusBadRequest, "Invalid input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			handleGatewayErrors(w, tc.err, "fallback")

			assert.Equal(t, tc.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tc.wantBody)
			assert.NotContains(t, w.Body.String(), "internal detail")
		})
	}
}
