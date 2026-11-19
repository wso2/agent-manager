// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"encoding/json"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// AgentGatewayController serves the agent routes that only exist with a
// gateway: an agent's deploy settings and where the gateway serves it.
type AgentGatewayController struct {
	agentService *gwsvc.AgentGatewayService
}

// NewAgentGatewayController creates the controller.
func NewAgentGatewayController(agentService *gwsvc.AgentGatewayService) *AgentGatewayController {
	return &AgentGatewayController{agentService: agentService}
}

func (c *AgentGatewayController) UpdateAgentDeploySettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)

	var payload gwspec.UpdateAgentDeploySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("UpdateAgentDeploySettings: failed to decode request body", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if payload.EnvironmentName == "" {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "environmentName is required")
		return
	}
	events.SetEnvironment(ctx, payload.EnvironmentName)

	if err := c.agentService.UpdateAgentDeploySettings(ctx, ouID, projName, agentName, &payload); err != nil {
		log.Error("UpdateAgentDeploySettings: failed to update deploy settings", "error", err)
		controllers.HandleCommonErrors(w, err, "Failed to update deploy settings")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *AgentGatewayController) GetAgentDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := modkit.OrgID(r.Context())
	projName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)

	deployments, err := c.agentService.GetAgentDeployments(ctx, ouID, projName, agentName)
	if err != nil {
		log.Error("GetAgentDeployments: failed to get deployments", "error", err)
		controllers.HandleCommonErrors(w, err, "Failed to get deployments")
		return
	}

	deploymentResponses := gwconv.ConvertToDeploymentDetailsResponse(deployments)
	httpx.WriteSuccessResponse(w, http.StatusOK, deploymentResponses)
}

// GetAgentConfigurations returns the agent's gateway settings in an environment
// and the provisioning state of the bindings it asked for.
func (c *AgentGatewayController) GetAgentConfigurations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ouID := modkit.OrgID(r.Context())
	projName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
	environment := r.URL.Query().Get("environment")
	if environment == "" {
		environment = models.DefaultEnvironmentName
	}
	cfg, bindings, err := c.agentService.AgentConfigurations(ctx, ouID, projName, agentName, environment)
	if err != nil {
		logger.GetLogger(ctx).Error("GetAgentConfigurations: failed to read configurations", "error", err)
		controllers.HandleCommonErrors(w, err, "Failed to get configurations")
		return
	}
	resp := gwspec.AgentConfigurationResponse{
		ProjectName: projName,
		AgentName:   agentName,
		Environment: environment,
		Bindings:    make([]gwspec.AgentBinding, 0, len(bindings)),
	}
	gwconv.PopulateAgentConfiguration(&resp, cfg)
	for _, b := range bindings {
		item := gwspec.AgentBinding{Kind: b.Kind, Position: int32(b.Position), Target: b.Target, State: b.State, LastError: b.LastError}
		if b.ConfigID != nil {
			item.ConfigId = gwutil.StrAsStrPointer(b.ConfigID.String())
		}
		resp.Bindings = append(resp.Bindings, item)
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}
