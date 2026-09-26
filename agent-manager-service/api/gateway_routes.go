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

func RegisterGatewayRoutes(rr *middleware.RouteRegistrar, ctrl controllers.GatewayController) {
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/gateways", rbac.GatewayCreate,
		growthanalytics.Track("amp.deployment-ops.register-gateway", actionDims("registered-gateway"), ctrl.RegisterGateway))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways", rbac.GatewayRead, ctrl.ListGateways)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}", rbac.GatewayRead, ctrl.GetGateway)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/gateways/{gatewayID}", rbac.GatewayUpdate,
		growthanalytics.Track("amp.deployment-ops.register-gateway", actionDims("updated-gateway"), ctrl.UpdateGateway))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/gateways/{gatewayID}", rbac.GatewayDelete,
		growthanalytics.Track("amp.deployment-ops.register-gateway", actionDims("deleted-gateway"), ctrl.DeleteGateway))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", rbac.GatewayUpdate,
		growthanalytics.Track("amp.deployment-ops.register-gateway.assign-environment", actionDims("assigned-gateway-to-environment"), ctrl.AssignGatewayToEnvironment))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", rbac.GatewayUpdate,
		growthanalytics.Track("amp.deployment-ops.register-gateway.assign-environment", actionDims("removed-gateway-from-environment"), ctrl.RemoveGatewayFromEnvironment))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}/environments", rbac.GatewayRead, ctrl.GetGatewayEnvironments)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}/health", rbac.GatewayRead, ctrl.CheckGatewayHealth)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}/tokens", rbac.GatewayTokenManage, ctrl.ListGatewayTokens)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/gateways/{gatewayID}/tokens", rbac.GatewayTokenManage,
		growthanalytics.Track("amp.deployment-ops.register-gateway.token", actionDims("rotated-gateway-token"), ctrl.RotateGatewayToken))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}", rbac.GatewayTokenManage,
		growthanalytics.Track("amp.deployment-ops.register-gateway.token", actionDims("revoked-gateway-token"), ctrl.RevokeGatewayToken))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/status", rbac.GatewayRead, ctrl.GetGatewayStatus)

	// Identity providers (token issuers mirrored from the gateway runtime config)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identity-providers", rbac.GatewayRead, ctrl.ListIdentityProviders)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identity-providers/discover", rbac.GatewayUpdate, ctrl.DiscoverOidcConfiguration)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}/identity-providers", rbac.GatewayRead, ctrl.ListGatewayIdentityProviders)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}", rbac.GatewayUpdate,
		growthanalytics.Track("amp.deployment-ops.register-gateway.identity-provider", actionDims("configured-gateway-identity-provider"), ctrl.UpsertGatewayIdentityProvider))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}", rbac.GatewayUpdate,
		growthanalytics.Track("amp.deployment-ops.register-gateway.identity-provider", actionDims("removed-gateway-identity-provider"), ctrl.DeleteGatewayIdentityProvider))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{environmentId}/identity-providers", rbac.GatewayRead, ctrl.ListEnvironmentIdentityProviders)
}
