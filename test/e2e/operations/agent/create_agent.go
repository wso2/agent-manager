package agent

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateAgentParams holds parameters for creating any type of agent.
type CreateAgentParams struct {
	OrgName     string
	ProjectName string
	Request     framework.CreateAgentRequest
}

// CreateAgent creates an agent and returns the response.
func CreateAgent(g Gomega, client *framework.AMPClient, params *CreateAgentParams) framework.AgentResponse {
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", params.OrgName, params.ProjectName)

	resp, err := client.Post(basePath, params.Request)
	g.Expect(err).NotTo(HaveOccurred(), "create agent request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 202)

	return framework.DecodeBody[framework.AgentResponse](g, resp)
}
