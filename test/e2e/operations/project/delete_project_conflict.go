package project

import (
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeleteProjectExpectConflict attempts to delete a project and expects a 409 Conflict response.
func DeleteProjectExpectConflict(g Gomega, client *framework.AMPClient, orgName, projName string) framework.ErrorResponse {
	path := "/api/v1/projects/" + projName

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete project request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 409)

	return framework.DecodeBody[framework.ErrorResponse](g, resp)
}
