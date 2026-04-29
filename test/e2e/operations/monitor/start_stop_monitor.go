package monitor

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// StartMonitor starts a monitor.
func StartMonitor(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, monitorName string) framework.MonitorResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s/start",
		orgName, projName, agentName, monitorName)

	resp, err := client.Post(path, nil)
	if err != nil {
		framework.Fatalf(t, "start monitor request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.MonitorResponse](t, resp)
}

// StopMonitor stops a monitor.
func StopMonitor(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, monitorName string) framework.MonitorResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s/stop",
		orgName, projName, agentName, monitorName)

	resp, err := client.Post(path, nil)
	if err != nil {
		framework.Fatalf(t, "stop monitor request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.MonitorResponse](t, resp)
}
