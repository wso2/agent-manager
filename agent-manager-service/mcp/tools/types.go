package tools

import (
	"context"
	// "time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

type Toolsets struct {
	ProjectToolset ProjectToolsetHandler
	AgentToolset   AgentToolsetHandler
}

type ProjectToolsetHandler interface {
	ListProjects(ctx context.Context, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error)
	CreateProject(ctx context.Context, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error)
	GenerateName(ctx context.Context, orgName string, req spec.ResourceNameRequest) (string, error)
}

type AgentToolsetHandler interface {
	ListAgents(ctx context.Context, orgName string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error)
	GenerateName(ctx context.Context, orgName string, payload spec.ResourceNameRequest) (string, error)
	GenerateToken(ctx context.Context, orgName string, projectName string, agentName string, environment string, expiresIn string) (*spec.TokenResponse, error)
	CreateAgent(ctx context.Context, orgName string, projectName string, req *spec.CreateAgentRequest) error
	GetAgent(ctx context.Context, orgName string, projectName string, agentName string) (*models.AgentResponse, error)
}
