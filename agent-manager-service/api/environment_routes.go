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

func registerEnvironmentRoutes(rr *middleware.RouteRegistrar, ctrl controllers.EnvironmentController) {
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/environments", rbac.EnvironmentCreate,
		notImplemented)
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/environments", ctrl.ListEnvironments,
		rbac.EnvironmentRead, rbac.LLMProviderRead, rbac.LLMProxyRead, rbac.GatewayRead)
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/environments/{envID}", ctrl.GetEnvironment,
		rbac.EnvironmentRead, rbac.LLMProviderRead, rbac.LLMProxyRead, rbac.GatewayRead)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/environments/{envID}", rbac.EnvironmentUpdate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/environments/{envID}", rbac.EnvironmentDelete,
		notImplemented)
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/environments/{envID}/gateways", ctrl.GetEnvironmentGateways,
		rbac.EnvironmentRead, rbac.GatewayRead)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/thunder-instances", rbac.EnvironmentRead, ctrl.ListThunderInstances)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/environments/{envID}/thunder-system-client",
		rbac.OrgManageServiceAccount, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/environments/{envID}/thunder-system-client",
		rbac.OrgManageServiceAccount, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/environments/{envID}/thunder-url",
		rbac.OrgManageServiceAccount, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envID}/thunder-url",
		rbac.OrgManageServiceAccount, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/environments/{envID}/thunder-url",
		rbac.OrgManageServiceAccount, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/thunder-url-availability",
		rbac.EnvironmentCreate, notImplemented)
}
