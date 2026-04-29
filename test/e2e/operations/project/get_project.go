package project

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetProject retrieves a project by name.
func GetProject(t *testing.T, client *framework.AMPClient, orgName, projName string) framework.ProjectResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", orgName, projName)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "get project request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.ProjectResponse](t, resp)
}

// ListProjects returns all projects in an organization.
func ListProjects(t *testing.T, client *framework.AMPClient, orgName string) framework.ProjectListResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects", orgName)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "list projects request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.ProjectListResponse](t, resp)
}
