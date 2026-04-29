package project

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateProjectParams holds parameters for creating a project.
type CreateProjectParams struct {
	OrgName            string
	Name               string
	DisplayName        string
	Description        string
	DeploymentPipeline string
}

// CreateProject creates a new project and returns the response.
// It registers a cleanup function to delete the project when the test finishes.
func CreateProject(t *testing.T, client *framework.AMPClient, params *CreateProjectParams) framework.ProjectResponse {
	t.Helper()
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects", params.OrgName)

	req := framework.CreateProjectRequest{
		Name:               params.Name,
		DisplayName:        params.DisplayName,
		DeploymentPipeline: params.DeploymentPipeline,
	}
	if params.Description != "" {
		req.Description = &params.Description
	}

	resp, err := client.Post(basePath, req)
	if err != nil {
		t.Fatalf("create project request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 202)

	return framework.DecodeBody[framework.ProjectResponse](t, resp)
}
