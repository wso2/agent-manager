package handlers

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// ProjectHandler bridges MCP project tools to the infra resource manager service.
type ProjectHandler struct {
	infraSvc services.InfraResourceManager
	agentMgr services.AgentManagerService
}

func NewProjectHandler(infraSvc services.InfraResourceManager, agentMgr services.AgentManagerService) *ProjectHandler {
	return &ProjectHandler{infraSvc: infraSvc, agentMgr: agentMgr}
}

func (h *ProjectHandler) GenerateName(ctx context.Context, orgName string, req spec.ResourceNameRequest) (string, error) {
	return h.agentMgr.GenerateName(ctx, orgName, req)
}
func (h *ProjectHandler) ListProjects(ctx context.Context, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error) {
	return h.infraSvc.ListProjects(ctx, orgName, limit, offset)
}

func (h *ProjectHandler) CreateProject(ctx context.Context, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	return h.infraSvc.CreateProject(ctx, orgName, payload)
}
