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

package gwctl

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// EnvironmentGatewayController serves the gateways of an environment.
type EnvironmentGatewayController struct {
	environmentService *gwsvc.EnvironmentGatewayService
}

// NewEnvironmentGatewayController creates the controller.
func NewEnvironmentGatewayController(environmentService *gwsvc.EnvironmentGatewayService) *EnvironmentGatewayController {
	return &EnvironmentGatewayController{environmentService: environmentService}
}

// GetEnvironmentGateways handles GET /orgs/{orgName}/environments/{envID}/gateways.
func (c *EnvironmentGatewayController) GetEnvironmentGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envID := r.PathValue("envID")

	gatewayList, err := c.environmentService.GetEnvironmentGateways(ctx, ouID, envID)
	if err != nil {
		log.Error("GetEnvironmentGateways: failed to get gateways", "error", err)
		controllers.HandleEnvironmentErrors(w, err, "Failed to get environment gateways")
		return
	}

	// Convert to spec responses
	specGateways := make([]gwspec.GatewayResponse, len(gatewayList))
	for i, gw := range gatewayList {
		specGateways[i] = convertToSpecGatewayResponse(&gw)
	}

	response := gwspec.GetEnvironmentGateways200Response{
		Gateways: specGateways,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// convertToSpecGatewayResponse converts internal gateway response to spec response
func convertToSpecGatewayResponse(gw *gwmodels.GatewayResponse) gwspec.GatewayResponse {
	response := gwspec.GatewayResponse{
		Uuid:        gw.UUID,
		Name:        gw.Name,
		DisplayName: gw.DisplayName,
		GatewayType: canonicalGatewayType(gw.GatewayType),
		Hosting:     gwspec.GatewayHosting(gw.Hosting),
		Vhost:       gw.VHost,
		IsCritical:  gw.IsCritical,
		Status:      gwspec.GatewayStatus(gw.Status),
	}

	return response
}
