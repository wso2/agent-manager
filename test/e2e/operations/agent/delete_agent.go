package agent

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeleteAgent deletes an agent by name.
func DeleteAgent(t *testing.T, client *framework.AMPClient, orgName, projName, agentName string) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", orgName, projName, agentName)

	resp, err := client.Delete(path)
	if err != nil {
		framework.Fatalf(t, "delete agent request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 204)
}
