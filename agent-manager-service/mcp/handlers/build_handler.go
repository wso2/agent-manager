package handlers

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

type BuildHandler struct {
	agentSvc services.AgentManagerService
}

func NewBuildHandler(agentSvc services.AgentManagerService) *BuildHandler {
	return &BuildHandler{agentSvc: agentSvc}
}

func (h *BuildHandler) ListAgentBuilds(ctx context.Context, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error) {
	return h.agentSvc.ListAgentBuilds(ctx, orgName, projectName, agentName, limit, offset)
}

func (h *BuildHandler) GetBuild(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error) {
	return h.agentSvc.GetBuild(ctx, orgName, projectName, agentName, buildName)
}

func (h *BuildHandler) BuildAgent(ctx context.Context, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error) {
	return h.agentSvc.BuildAgent(ctx, orgName, projectName, agentName, commitId)
}

func (h *BuildHandler) GetBuildLogs(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.LogsResponse, error) {
	return h.agentSvc.GetBuildLogs(ctx, orgName, projectName, agentName, buildName)
}
