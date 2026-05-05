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

package handlers

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

type DeploymentHandler struct {
	agentSvc services.AgentManagerService
}

func NewDeploymentHandler(agentSvc services.AgentManagerService) *DeploymentHandler {
	return &DeploymentHandler{agentSvc: agentSvc}
}

func (h *DeploymentHandler) GetAgentDeployments(ctx context.Context, orgName string, projectName string, agentName string) ([]*models.DeploymentResponse, error) {
	return h.agentSvc.GetAgentDeployments(ctx, orgName, projectName, agentName)
}

func (h *DeploymentHandler) DeployAgent(ctx context.Context, orgName string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error) {
	return h.agentSvc.DeployAgent(ctx, orgName, projectName, agentName, req)
}

func (h *DeploymentHandler) UpdateDeploymentState(ctx context.Context, orgName string, projectName string, agentName string, environment string, state string) error {
	return h.agentSvc.UpdateAgentDeploymentState(ctx, orgName, projectName, agentName, environment, state)
}
