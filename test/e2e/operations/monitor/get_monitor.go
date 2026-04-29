package monitor

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetMonitor retrieves a monitor by name.
func GetMonitor(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, monitorName string) framework.MonitorResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s",
		orgName, projName, agentName, monitorName)

	resp, err := client.Get(path)
	if err != nil {
		t.Fatalf("get monitor request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.MonitorResponse](t, resp)
}

// ListMonitors returns all monitors for an agent.
func ListMonitors(t *testing.T, client *framework.AMPClient, orgName, projName, agentName string) framework.MonitorListResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	if err != nil {
		t.Fatalf("list monitors request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.MonitorListResponse](t, resp)
}
