package project

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateProjectParams holds parameters for creating a project.
type CreateProjectParams struct {
	OrgName string
	Request framework.CreateProjectRequest
}

// CreateProject creates a new project and returns the response.
func CreateProject(g Gomega, client *framework.AMPClient, params *CreateProjectParams) framework.ProjectResponse {
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects", params.OrgName)

	resp, err := client.Post(basePath, params.Request)
	g.Expect(err).NotTo(HaveOccurred(), "create project request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 202)

	return framework.DecodeBody[framework.ProjectResponse](g, resp)
}
