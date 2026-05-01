package tools

import (
	"context"
	// "time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

type Toolsets struct {
	ProjectToolset    ProjectToolsetHandler
	AgentToolset      AgentToolsetHandler
	BuildToolset      BuildToolsetHandler
	DeploymentToolset DeploymentToolsetHandler
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

type BuildToolsetHandler interface {
	ListAgentBuilds(ctx context.Context, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error)
	GetBuild(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error)
	BuildAgent(ctx context.Context, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error)
	GetBuildLogs(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.LogsResponse, error)
}

type DeploymentToolsetHandler interface {
	GetAgentDeployments(ctx context.Context, orgName string, projectName string, agentName string) ([]*models.DeploymentResponse, error)
	DeployAgent(ctx context.Context, orgName string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error)
	UpdateDeploymentState(ctx context.Context, orgName string, projectName string, agentName string, environment string, state string) error
}
