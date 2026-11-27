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
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

func RegisterGatewayRoutes(rr Router, ctrl gwctl.GatewayController) {
	rr.HandleFuncWithValidationAndAuthz("POST /gateways", gwperm.Create,
		rr.Track("amp.deployment-ops.register-gateway", actionDims("registered-gateway"), ctrl.RegisterGateway))
	rr.HandleFuncWithValidationAndAuthz("GET /gateways", gwperm.Read, ctrl.ListGateways)
	rr.HandleFuncWithValidationAndAuthz("GET /gateways/{gatewayID}", gwperm.Read, ctrl.GetGateway)
	rr.HandleFuncWithValidationAndAuthz("PUT /gateways/{gatewayID}", gwperm.Update,
		rr.Track("amp.deployment-ops.register-gateway", actionDims("updated-gateway"), ctrl.UpdateGateway))
	rr.HandleFuncWithValidationAndAuthz("DELETE /gateways/{gatewayID}", gwperm.Delete,
		rr.Track("amp.deployment-ops.register-gateway", actionDims("deleted-gateway"), ctrl.DeleteGateway))
	rr.HandleFuncWithValidationAndAuthz("GET /gateways/{gatewayID}/health", gwperm.Read, ctrl.CheckGatewayHealth)
	rr.HandleFuncWithValidationAndAuthz("GET /gateways/{gatewayID}/tokens", gwperm.TokenManage, ctrl.ListGatewayTokens)
	rr.HandleFuncWithValidationAndAuthz("POST /gateways/{gatewayID}/tokens", gwperm.TokenManage,
		rr.Track("amp.deployment-ops.register-gateway.token", actionDims("rotated-gateway-token"), ctrl.RotateGatewayToken))
	rr.HandleFuncWithValidationAndAuthz("DELETE /gateways/{gatewayID}/tokens/{tokenID}", gwperm.TokenManage,
		rr.Track("amp.deployment-ops.register-gateway.token", actionDims("revoked-gateway-token"), ctrl.RevokeGatewayToken))
	rr.HandleFuncWithValidationAndAuthz("GET /gateways/status", gwperm.Read, ctrl.GetGatewayStatus)

	// Identity providers (token issuers mirrored from the gateway runtime config)
	rr.HandleFuncWithValidationAndAuthz("GET /identity-providers", gwperm.Read, ctrl.ListIdentityProviders)
	rr.HandleFuncWithValidationAndAuthz("GET /identity-providers/discover", gwperm.Update, ctrl.DiscoverOidcConfiguration)
	rr.HandleFuncWithValidationAndAuthz("GET /gateways/{gatewayID}/identity-providers", gwperm.Read, ctrl.ListGatewayIdentityProviders)
	rr.HandleFuncWithValidationAndAuthz("PUT /gateways/{gatewayID}/identity-providers/{name}", gwperm.Update,
		rr.Track("amp.deployment-ops.register-gateway.identity-provider", actionDims("configured-gateway-identity-provider"), ctrl.UpsertGatewayIdentityProvider))
	rr.HandleFuncWithValidationAndAuthz("DELETE /gateways/{gatewayID}/identity-providers/{name}", gwperm.Update,
		rr.Track("amp.deployment-ops.register-gateway.identity-provider", actionDims("removed-gateway-identity-provider"), ctrl.DeleteGatewayIdentityProvider))
}
