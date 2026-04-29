package configuration

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetAgentConfigurations retrieves the runtime configurations (env vars) for an agent
// in a specific environment.
func GetAgentConfigurations(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, environment string) framework.ConfigurationResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/configurations?environment=%s",
		orgName, projName, agentName, environment)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "get agent configurations request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.ConfigurationResponse](t, resp)
}
