package project

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateProjectParams holds parameters for creating a project.
type CreateProjectParams struct {
	OrgName string
	Request framework.CreateProjectRequest
}

// CreateProject creates a new project and returns the response.
func CreateProject(t *testing.T, client *framework.AMPClient, params *CreateProjectParams) framework.ProjectResponse {
	t.Helper()
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects", params.OrgName)

	resp, err := client.Post(basePath, params.Request)
	if err != nil {
		framework.Fatalf(t, "create project request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 202)

	return framework.DecodeBody[framework.ProjectResponse](t, resp)
}
