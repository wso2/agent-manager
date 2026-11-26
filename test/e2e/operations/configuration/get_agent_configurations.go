package configuration

import (
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetAgentConfigurations reads an agent's AI Gateway settings and the state of its LLM and MCP bindings.
func GetAgentConfigurations(g Gomega, client *framework.AMPClient, projName, agentName string) framework.AgentGatewayConfiguration {
	resp, err := client.Get(framework.GWAgent(projName, agentName, "/configurations"))
	g.Expect(err).NotTo(HaveOccurred(), "get agent configurations request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentGatewayConfiguration](g, resp)
}
