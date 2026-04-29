package agent

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetAgent retrieves an agent by name.
func GetAgent(t *testing.T, client *framework.AMPClient, orgName, projName, agentName string) framework.AgentResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", orgName, projName, agentName)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "get agent request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.AgentResponse](t, resp)
}

// ListAgents returns all agents in a project.
func ListAgents(t *testing.T, client *framework.AMPClient, orgName, projName string) framework.AgentListResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", orgName, projName)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "list agents request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.AgentListResponse](t, resp)
}
