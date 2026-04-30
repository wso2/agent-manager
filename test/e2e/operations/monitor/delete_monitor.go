package monitor

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeleteMonitor deletes a monitor by name.
func DeleteMonitor(g Gomega, client *framework.AMPClient, orgName, projName, agentName, monitorName string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s",
		orgName, projName, agentName, monitorName)

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete monitor request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
