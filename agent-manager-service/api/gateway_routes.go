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
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware"
)

func RegisterGatewayRoutes(mux *http.ServeMux, ctrl controllers.GatewayController) {
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/gateways", ctrl.RegisterGateway)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/gateways", ctrl.ListGateways)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/gateways/{gatewayID}", ctrl.GetGateway)
	middleware.HandleFuncWithValidation(mux, "PUT /orgs/{orgName}/gateways/{gatewayID}", ctrl.UpdateGateway)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/gateways/{gatewayID}", ctrl.DeleteGateway)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", ctrl.AssignGatewayToEnvironment)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", ctrl.RemoveGatewayFromEnvironment)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/gateways/{gatewayID}/environments", ctrl.GetGatewayEnvironments)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/gateways/{gatewayID}/health", ctrl.CheckGatewayHealth)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/gateways/{gatewayID}/tokens", ctrl.RotateGatewayToken)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}", ctrl.RevokeGatewayToken)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/gateways/status", ctrl.GetGatewayStatus)
}
