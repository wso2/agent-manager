package project

import (
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateProjectParams holds parameters for creating a project.
type CreateProjectParams struct {
	OrgName string
	Request framework.ProjectDocument
}

// CreateProject creates a new project and returns the response.
func CreateProject(g Gomega, client *framework.AMPClient, params *CreateProjectParams) framework.ProjectDocument {
	basePath := "/api/v1/projects"

	resp, err := client.Post(basePath, params.Request)
	g.Expect(err).NotTo(HaveOccurred(), "create project request failed")
	defer resp.Body.Close()
	framework.ExpectStatusIn(g, resp, 202, 409)
	if resp.StatusCode == http.StatusConflict {
		ginkgo.GinkgoWriter.Printf("Project %q already exists, reusing\n", params.Request.Metadata.Name)
	}

	return framework.DecodeBody[framework.ProjectDocument](g, resp)
}
