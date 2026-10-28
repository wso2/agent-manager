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

package api

import (
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// RegisterPlatformGatewayHealthRoutes registers the cross-organization gateway
// health surface.
//
// Deliberately not under /orgs/{orgName}: every other route in this table is
// tenant-scoped and takes its org from the token, and a reader has to be able to
// see at a glance that this one is not. The /platform prefix says so, and
// HandleFuncWithValidationAndPlatformAdminAuthz enforces it — it panics at
// startup on an org-scoped pattern.
//
// rbac.GatewayRead is declared because the route needs a permission for its
// audit action label and for the route-table invariants, and the scope check it
// implies always runs. It is not the tenant boundary, though: gateway:read is
// an ordinary org-level grant that any tenant's admin holds, so the
// platform-admin OU check is what actually keeps other tenants out. See
// middleware.RequirePlatformAdminOU.
func RegisterPlatformGatewayHealthRoutes(rr *middleware.RouteRegistrar, ctrl controllers.GatewayController) {
	rr.HandleFuncWithValidationAndPlatformAdminAuthz(
		"GET /platform/gateways/failure-summary", rbac.GatewayRead, ctrl.GetPlatformGatewayFailureSummary,
	)
}
