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

// registerShadowAgentRoutes registers Shadow Agent Discovery. It is an org
// admin feature: connections hold platform credentials, and the agents they
// find are the org's unregistered agents.
func registerShadowAgentRoutes(rr *middleware.RouteRegistrar, controller controllers.ShadowAgentController) {
	base := "/orgs/{orgName}/shadow-agents"
	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/providers"), rbac.ShadowAgentRead, controller.ListProviders)
	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/summary"), rbac.ShadowAgentRead, controller.GetSummary)

	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/connections"), rbac.ShadowAgentRead, controller.ListConnections)
	rr.HandleFuncWithValidationAndAuthz(route("POST", base+"/connections"), rbac.ShadowAgentManage, controller.CreateConnection)
	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/connections/{connectionId}"), rbac.ShadowAgentRead, controller.GetConnection)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", base+"/connections/{connectionId}"), rbac.ShadowAgentManage, controller.UpdateConnection)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", base+"/connections/{connectionId}"), rbac.ShadowAgentManage, controller.DeleteConnection)
	rr.HandleFuncWithValidationAndAuthz(route("POST", base+"/connections/{connectionId}/test"), rbac.ShadowAgentManage, controller.TestConnection)

	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/scans"), rbac.ShadowAgentRead, controller.ListScans)
	rr.HandleFuncWithValidationAndAuthz(route("POST", base+"/scans"), rbac.ShadowAgentManage, controller.CreateScan)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", base+"/scans"), rbac.ShadowAgentManage, controller.ClearScans)
	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/scans/{scanId}"), rbac.ShadowAgentRead, controller.GetScan)

	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/findings"), rbac.ShadowAgentRead, controller.ListFindings)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", base+"/findings"), rbac.ShadowAgentManage, controller.DeleteFindings)
	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/findings/{findingId}"), rbac.ShadowAgentRead, controller.GetFinding)
	rr.HandleFuncWithValidationAndAuthz(route("PATCH", base+"/findings/{findingId}"), rbac.ShadowAgentManage, controller.UpdateFinding)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", base+"/findings/{findingId}"), rbac.ShadowAgentManage, controller.DeleteFinding)
}
