package configuration

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetAgentConfigurations retrieves the runtime configurations (env vars) for an agent
// in a specific environment.
func GetAgentConfigurations(g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) framework.ConfigurationResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/configurations?environment=%s",
		orgName, projName, agentName, environment)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get agent configurations request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.ConfigurationResponse](g, resp)
}
