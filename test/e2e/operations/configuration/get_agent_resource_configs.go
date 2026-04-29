package configuration

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetAgentResourceConfigs retrieves the resource configurations (replicas, CPU, memory,
// autoscaling) for an agent.
func GetAgentResourceConfigs(t *testing.T, client *framework.AMPClient, orgName, projName, agentName string) framework.AgentResourceConfigsResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/resource-configs",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "get agent resource configs request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.AgentResourceConfigsResponse](t, resp)
}

// UpdateAgentResourceConfigs updates the resource configurations for an agent.
func UpdateAgentResourceConfigs(t *testing.T, client *framework.AMPClient, orgName, projName, agentName string, req framework.UpdateAgentResourceConfigsRequest) framework.AgentResourceConfigsResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/resource-configs",
		orgName, projName, agentName)

	resp, err := client.Put(path, req)
	if err != nil {
		framework.Fatalf(t, "update agent resource configs request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.AgentResourceConfigsResponse](t, resp)
}
