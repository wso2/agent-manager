package tools

import (
	"context"
	// "time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)
type Toolsets struct {
		ProjectToolset       ProjectToolsetHandler
}

type ProjectToolsetHandler interface {
	ListProjects(ctx context.Context, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error)
	CreateProject(ctx context.Context, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error)
    GenerateName(ctx context.Context, orgName string, req spec.ResourceNameRequest) (string, error)
	// 	ListOrganizations(ctx context.Context, limit int, offset int) ([]*models.OrganizationResponse, int32, error)
	// 	ListEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error)
}