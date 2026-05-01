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
