package configuration

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetAgentConfigurations reads an agent's AI Gateway settings and the state of
// its LLM and MCP bindings in a specific environment.
func GetAgentConfigurations(g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) framework.AgentGatewayConfiguration {
	path := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/configurations?environment=%s",
		orgName, projName, agentName, environment)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get agent configurations request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentGatewayConfiguration](g, resp)
}
