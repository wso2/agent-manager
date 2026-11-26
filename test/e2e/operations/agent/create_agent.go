package agent

import (
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
func CreateAgent(g Gomega, client *framework.AMPClient, params *CreateAgentParams) framework.AgentDocument {
	resp, err := client.Post("/api/v1/agents", params.Request.Document(params.ProjectName))
	g.Expect(err).NotTo(HaveOccurred(), "create agent request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 202)

	return framework.DecodeBody[framework.AgentDocument](g, resp)
}
