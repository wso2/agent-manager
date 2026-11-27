package project

import (
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeleteProject deletes a project by name.
func DeleteProject(g Gomega, client *framework.AMPClient, orgName, projName string) {
	path := "/api/v1/projects/" + projName

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete project request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
