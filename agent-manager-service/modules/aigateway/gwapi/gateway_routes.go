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

package gwapi

import (
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

func RegisterGatewayRoutes(rr Router, ctrl gwctl.GatewayController) {
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/gateways", gwperm.Create,
		growthanalytics.Track("amp.deployment-ops.register-gateway", actionDims("registered-gateway"), ctrl.RegisterGateway))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways", gwperm.Read, ctrl.ListGateways)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}", gwperm.Read, ctrl.GetGateway)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/gateways/{gatewayID}", gwperm.Update,
		growthanalytics.Track("amp.deployment-ops.register-gateway", actionDims("updated-gateway"), ctrl.UpdateGateway))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/gateways/{gatewayID}", gwperm.Delete,
		growthanalytics.Track("amp.deployment-ops.register-gateway", actionDims("deleted-gateway"), ctrl.DeleteGateway))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", gwperm.Update,
		growthanalytics.Track("amp.deployment-ops.register-gateway.assign-environment", actionDims("assigned-gateway-to-environment"), ctrl.AssignGatewayToEnvironment))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", gwperm.Update,
		growthanalytics.Track("amp.deployment-ops.register-gateway.assign-environment", actionDims("removed-gateway-from-environment"), ctrl.RemoveGatewayFromEnvironment))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}/environments", gwperm.Read, ctrl.GetGatewayEnvironments)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}/health", gwperm.Read, ctrl.CheckGatewayHealth)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}/tokens", gwperm.TokenManage, ctrl.ListGatewayTokens)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/gateways/{gatewayID}/tokens", gwperm.TokenManage,
		growthanalytics.Track("amp.deployment-ops.register-gateway.token", actionDims("rotated-gateway-token"), ctrl.RotateGatewayToken))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}", gwperm.TokenManage,
		growthanalytics.Track("amp.deployment-ops.register-gateway.token", actionDims("revoked-gateway-token"), ctrl.RevokeGatewayToken))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/status", gwperm.Read, ctrl.GetGatewayStatus)

	// Self-hosted gateways: the same operations, restricted to gateways the customer runs.
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/self-hosted-gateways", gwperm.Read, ctrl.ListSelfHostedGateways)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/self-hosted-gateways", gwperm.Create,
		growthanalytics.Track("amp.deployment-ops.register-gateway", actionDims("registered-gateway"), ctrl.RegisterSelfHostedGateway))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/self-hosted-gateways/{gatewayID}", gwperm.Read, ctrl.GetSelfHostedGateway)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/self-hosted-gateways/{gatewayID}", gwperm.Update,
		growthanalytics.Track("amp.deployment-ops.register-gateway", actionDims("updated-gateway"), ctrl.UpdateSelfHostedGateway))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/self-hosted-gateways/{gatewayID}", gwperm.Delete,
		growthanalytics.Track("amp.deployment-ops.register-gateway", actionDims("deleted-gateway"), ctrl.DeleteSelfHostedGateway))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/self-hosted-gateways/{gatewayID}/tokens", gwperm.TokenManage, ctrl.ListSelfHostedGatewayTokens)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/self-hosted-gateways/{gatewayID}/tokens", gwperm.TokenManage,
		growthanalytics.Track("amp.deployment-ops.register-gateway.token", actionDims("rotated-gateway-token"), ctrl.RotateSelfHostedGatewayToken))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/self-hosted-gateways/{gatewayID}/tokens/{tokenID}", gwperm.TokenManage,
		growthanalytics.Track("amp.deployment-ops.register-gateway.token", actionDims("revoked-gateway-token"), ctrl.RevokeSelfHostedGatewayToken))

	// Identity providers (token issuers mirrored from the gateway runtime config)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identity-providers", gwperm.Read, ctrl.ListIdentityProviders)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identity-providers/discover", gwperm.Update, ctrl.DiscoverOidcConfiguration)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/gateways/{gatewayID}/identity-providers", gwperm.Read, ctrl.ListGatewayIdentityProviders)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}", gwperm.Update,
		growthanalytics.Track("amp.deployment-ops.register-gateway.identity-provider", actionDims("configured-gateway-identity-provider"), ctrl.UpsertGatewayIdentityProvider))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}", gwperm.Update,
		growthanalytics.Track("amp.deployment-ops.register-gateway.identity-provider", actionDims("removed-gateway-identity-provider"), ctrl.DeleteGatewayIdentityProvider))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{environmentId}/identity-providers", gwperm.Read, ctrl.ListEnvironmentIdentityProviders)
}
