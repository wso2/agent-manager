package monitor

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// UpdateMonitor updates a monitor by name.
func UpdateMonitor(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, monitorName string, req framework.UpdateMonitorRequest) framework.MonitorResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s",
		orgName, projName, agentName, monitorName)

	resp, err := client.Patch(path, req)
	if err != nil {
		framework.Fatalf(t, "update monitor request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.MonitorResponse](t, resp)
}
