package agent

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateAgentParams holds parameters for creating any type of agent.
type CreateAgentParams struct {
	OrgName     string
	ProjectName string
	Request     framework.CreateAgentRequest
}

// CreateAgent creates an agent and returns the response.
// It registers a cleanup function to delete the agent when the test finishes.
func CreateAgent(t *testing.T, client *framework.AMPClient, params *CreateAgentParams) framework.AgentResponse {
	t.Helper()
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", params.OrgName, params.ProjectName)

	resp, err := client.Post(basePath, params.Request)
	if err != nil {
		framework.Fatalf(t, "create agent request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 202)

	return framework.DecodeBody[framework.AgentResponse](t, resp)
}
