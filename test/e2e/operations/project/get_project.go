package project

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetProject retrieves a project by name.
func GetProject(g Gomega, client *framework.AMPClient, orgName, projName string) framework.ProjectResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", orgName, projName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get project request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.ProjectResponse](g, resp)
}

// ListProjects returns all projects in an organization.
func ListProjects(g Gomega, client *framework.AMPClient, orgName string) framework.ProjectListResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects", orgName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list projects request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.ProjectListResponse](g, resp)
}
