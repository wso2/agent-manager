package configuration

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateAgentModelConfig creates a model configuration for an agent.
func CreateAgentModelConfig(g Gomega, client *framework.AMPClient, orgName, projName, agentName string, req framework.CreateAgentModelConfigRequest) framework.AgentModelConfigResponse {
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs",
		orgName, projName, agentName)

	resp, err := client.Post(basePath, req)
	g.Expect(err).NotTo(HaveOccurred(), "create agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.AgentModelConfigResponse](g, resp)
}

// ListAgentModelConfigs returns all model configurations for an agent.
func ListAgentModelConfigs(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) framework.AgentModelConfigListResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list agent model configs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentModelConfigListResponse](g, resp)
}

// GetAgentModelConfig retrieves a specific model configuration by ID.
func GetAgentModelConfig(g Gomega, client *framework.AMPClient, orgName, projName, agentName, configID string) framework.AgentModelConfigResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs/%s",
		orgName, projName, agentName, configID)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentModelConfigResponse](g, resp)
}

// UpdateAgentModelConfig updates a model configuration.
func UpdateAgentModelConfig(g Gomega, client *framework.AMPClient, orgName, projName, agentName, configID string, req framework.UpdateAgentModelConfigRequest) framework.AgentModelConfigResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs/%s",
		orgName, projName, agentName, configID)

	resp, err := client.Put(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "update agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentModelConfigResponse](g, resp)
}

// DeleteAgentModelConfig deletes a model configuration.
func DeleteAgentModelConfig(g Gomega, client *framework.AMPClient, orgName, projName, agentName, configID string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs/%s",
		orgName, projName, agentName, configID)

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
