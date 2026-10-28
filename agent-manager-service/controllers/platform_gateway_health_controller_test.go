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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// TestGetPlatformGatewayFailureSummary_RefusesWithoutThePlatformAdminGuard is
// the only test that exercises the handler's fail-closed guard, because no
// correctly-registered route can reach it.
//
// The handler is called DIRECTLY here, with a context that never passed through
// RequirePlatformAdminOU — which is exactly the state a future refactor produces
// by re-registering this route through the ordinary registrar. That mistake
// compiles, passes every routing test, and serves every tenant's rows to any
// authenticated caller. The guard turns it into a 403, and without this test
// nothing proves the guard still works.
//
// Both repository funcs are left nil: an unconfigured moq method panics, so this
// also asserts the refusal happens BEFORE any cross-org query runs.
func TestGetPlatformGatewayFailureSummary_RefusesWithoutThePlatformAdminGuard(t *testing.T) {
	ctrl := NewGatewayController(
		services.NewPlatformGatewayService(&repomocks.GatewayRepositoryMock{}, nil), nil, nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/platform/gateways/failure-summary", nil)
	rec := httptest.NewRecorder()

	ctrl.GetPlatformGatewayFailureSummary(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("handler reached without the platform-admin guard: want 403, got %d (body: %s)",
			rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); len(body) > 0 && rec.Code == http.StatusOK {
		t.Error("a refused caller must not receive any fleet data")
	}
}
