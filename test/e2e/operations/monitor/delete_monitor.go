package monitor

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeleteMonitor deletes a monitor by name.
func DeleteMonitor(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, monitorName string) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s",
		orgName, projName, agentName, monitorName)

	resp, err := client.Delete(path)
	if err != nil {
		t.Fatalf("delete monitor request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 204)
}
