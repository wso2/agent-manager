package project

import (
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetProject retrieves a project by name.
func GetProject(g Gomega, client *framework.AMPClient, orgName, projName string) framework.ProjectDocument {
	path := "/api/v1/projects/" + projName

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get project request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.ProjectDocument](g, resp)
}

// ListProjects returns all projects in an organization.
func ListProjects(g Gomega, client *framework.AMPClient, orgName string) framework.ProjectListResponse {
	path := "/api/v1/projects"

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list projects request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.ProjectListResponse](g, resp)
}
