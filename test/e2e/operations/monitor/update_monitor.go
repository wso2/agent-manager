package monitor

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// UpdateMonitor updates a monitor by name.
func UpdateMonitor(g Gomega, client *framework.AMPClient, orgName, projName, agentName, monitorName string, req framework.UpdateMonitorRequest) framework.MonitorResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s",
		orgName, projName, agentName, monitorName)

	resp, err := client.Patch(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "update monitor request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MonitorResponse](g, resp)
}
