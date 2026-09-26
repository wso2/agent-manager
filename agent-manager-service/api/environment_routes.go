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
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func registerEnvironmentRoutes(rr *middleware.RouteRegistrar, ctrl controllers.EnvironmentController) {
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/environments", rbac.EnvironmentCreate,
		growthanalytics.Track("amp.deployment-ops.create-environment", actionDims("created-environment"), ctrl.CreateEnvironment))
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/environments", ctrl.ListEnvironments,
		rbac.EnvironmentRead, rbac.LLMProviderRead, rbac.LLMProxyRead, rbac.GatewayRead)
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/environments/{envID}", ctrl.GetEnvironment,
		rbac.EnvironmentRead, rbac.LLMProviderRead, rbac.LLMProxyRead, rbac.GatewayRead)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/environments/{envID}", rbac.EnvironmentUpdate,
		growthanalytics.Track("amp.deployment-ops.create-environment", actionDims("updated-environment"), ctrl.UpdateEnvironment))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/environments/{envID}", rbac.EnvironmentDelete,
		growthanalytics.Track("amp.deployment-ops.create-environment", actionDims("deleted-environment"), ctrl.DeleteEnvironment))
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/environments/{envID}/gateways", ctrl.GetEnvironmentGateways,
		rbac.EnvironmentRead, rbac.GatewayRead)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/thunder-instances", rbac.EnvironmentRead, ctrl.ListThunderInstances)
	// Bootstrap-only: add/remove-environment-thunder.sh use these to store/remove
	// the env-Thunder system-client credential AMS uses to reach that Thunder.
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/environments/{envID}/thunder-system-client",
		rbac.OrgManageServiceAccount, ctrl.SetThunderSystemClient)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/environments/{envID}/thunder-system-client",
		rbac.OrgManageServiceAccount, ctrl.DeleteThunderSystemClient)
	// Bootstrap-only: add/remove-environment-thunder.sh use these to register/free
	// the unguessable URL handle that replaces the predictable <org>-<env> pattern.
	// GET is used by add-environment.sh to learn the actual (possibly
	// server-generated) handle for wiring the gateway's ThunderKeyManager.
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/environments/{envID}/thunder-url",
		rbac.OrgManageServiceAccount, ctrl.SetThunderURL)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envID}/thunder-url",
		rbac.OrgManageServiceAccount, ctrl.GetThunderURL)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/environments/{envID}/thunder-url",
		rbac.OrgManageServiceAccount, ctrl.DeleteThunderURL)
	// Advisory pre-flight check for the console's Create Environment drawer —
	// same permission as actually creating an environment, since it's the
	// browser (not a bootstrap script) calling this one. The handle is
	// globally unique, not scoped to a single environment, so this route
	// intentionally has no {envID}.
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/thunder-url-availability",
		rbac.EnvironmentCreate, ctrl.CheckThunderURLAvailability)
}
