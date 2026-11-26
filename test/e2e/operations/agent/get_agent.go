package agent

import (
	"fmt"
	"net/url"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetAgent retrieves an agent by name.
func GetAgent(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) framework.AgentDocument {
	path := AgentPath(projName, agentName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get agent request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentDocument](g, resp)
}

// ListAgents returns all agents in a project.
func ListAgents(g Gomega, client *framework.AMPClient, orgName, projName string) framework.AgentListResponse {
	path := "/api/v1/agents?" + url.Values{"project": {projName}, "limit": {"500"}}.Encode()

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list agents request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentListResponse](g, resp)
}

// AgentPath is the API path of one agent, its project in the query.
func AgentPath(projName, agentName string) string {
	return fmt.Sprintf("/api/v1/agents/%s?%s", url.PathEscape(agentName), url.Values{"project": {projName}}.Encode())
}
