package configuration

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetAgentResourceConfigs retrieves the resource configurations (replicas, CPU, memory,
// autoscaling) for an agent.
func GetAgentResourceConfigs(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) framework.AgentResourceConfigsResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/resource-configs",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get agent resource configs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentResourceConfigsResponse](g, resp)
}

// UpdateAgentResourceConfigs updates the resource configurations for an agent.
func UpdateAgentResourceConfigs(g Gomega, client *framework.AMPClient, orgName, projName, agentName string, req framework.UpdateAgentResourceConfigsRequest) framework.AgentResourceConfigsResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/resource-configs",
		orgName, projName, agentName)

	resp, err := client.Put(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "update agent resource configs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentResourceConfigsResponse](g, resp)
}
