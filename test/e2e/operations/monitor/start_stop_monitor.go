package monitor

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// StartMonitor starts a monitor.
func StartMonitor(g Gomega, client *framework.AMPClient, orgName, projName, agentName, monitorName string) framework.MonitorResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s/start",
		orgName, projName, agentName, monitorName)

	resp, err := client.Post(path, nil)
	g.Expect(err).NotTo(HaveOccurred(), "start monitor request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MonitorResponse](g, resp)
}

// StopMonitor stops a monitor.
func StopMonitor(g Gomega, client *framework.AMPClient, orgName, projName, agentName, monitorName string) framework.MonitorResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s/stop",
		orgName, projName, agentName, monitorName)

	resp, err := client.Post(path, nil)
	g.Expect(err).NotTo(HaveOccurred(), "stop monitor request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MonitorResponse](g, resp)
}
