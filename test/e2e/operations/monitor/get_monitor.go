package monitor

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetMonitor retrieves a monitor by name.
func GetMonitor(g Gomega, client *framework.AMPClient, orgName, projName, agentName, monitorName string) framework.MonitorResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s",
		orgName, projName, agentName, monitorName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get monitor request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MonitorResponse](g, resp)
}

// ListMonitors returns all monitors for an agent.
func ListMonitors(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) framework.MonitorListResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list monitors request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MonitorListResponse](g, resp)
}
