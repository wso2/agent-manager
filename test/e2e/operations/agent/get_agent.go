package agent

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetAgent retrieves an agent by name.
func GetAgent(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) framework.AgentResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", orgName, projName, agentName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get agent request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentResponse](g, resp)
}

// ListAgents returns all agents in a project.
func ListAgents(g Gomega, client *framework.AMPClient, orgName, projName string) framework.AgentListResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", orgName, projName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list agents request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentListResponse](g, resp)
}
