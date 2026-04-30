package agent

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeleteAgent deletes an agent by name.
func DeleteAgent(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", orgName, projName, agentName)

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete agent request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
