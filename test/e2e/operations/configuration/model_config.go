package configuration

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateAgentModelConfig creates a model configuration for an agent.
func CreateAgentModelConfig(g Gomega, client *framework.AMPClient, orgName, projName, agentName string, req framework.CreateAgentModelConfigRequest) framework.AgentModelConfigResponse {
	basePath := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/model-configs",
		orgName, projName, agentName)

	resp, err := client.Post(basePath, req)
	g.Expect(err).NotTo(HaveOccurred(), "create agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.AgentModelConfigResponse](g, resp)
}

// CreateAgentMCPConfig attaches an MCP proxy to an agent via the dedicated
// mcp-configs endpoint. The server forces the config type to "mcp", so the
// request's env mappings should reference an MCP proxy via MCPProxyName.
func CreateAgentMCPConfig(g Gomega, client *framework.AMPClient, orgName, projName, agentName string, req framework.CreateAgentModelConfigRequest) framework.AgentModelConfigResponse {
	basePath := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/mcp-configs",
		orgName, projName, agentName)

	resp, err := client.Post(basePath, req)
	g.Expect(err).NotTo(HaveOccurred(), "create agent MCP config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.AgentModelConfigResponse](g, resp)
}

// DeleteAgentMCPConfigBestEffort removes a disposable MCP binding before its
// agent and proxy are deleted. Removing the binding first avoids a proxy-delete
// conflict during suite teardown, while never hiding the original spec result
// behind a cleanup failure.
func DeleteAgentMCPConfigBestEffort(ctx context.Context, client *framework.AMPClient, orgName, projName, agentName, configID string) {
	if configID == "" {
		return
	}
	path := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/mcp-configs/%s",
		orgName, projName, agentName, configID)
	response, err := client.DeleteWithContext(ctx, path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("teardown: delete agent MCP config %q failed: %v\n", configID, err)
		return
	}
	defer response.Body.Close()
	ginkgo.GinkgoWriter.Printf("teardown: deleted agent MCP config %q (status %d)\n", configID, response.StatusCode)
}

// ListAgentModelConfigs returns all model configurations for an agent.
func ListAgentModelConfigs(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) framework.AgentModelConfigListResponse {
	path := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/model-configs",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list agent model configs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentModelConfigListResponse](g, resp)
}

// GetAgentModelConfig retrieves a specific model configuration by ID.
func GetAgentModelConfig(g Gomega, client *framework.AMPClient, orgName, projName, agentName, configID string) framework.AgentModelConfigResponse {
	path := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/model-configs/%s",
		orgName, projName, agentName, configID)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentModelConfigResponse](g, resp)
}

// UpdateAgentModelConfig updates a model configuration.
func UpdateAgentModelConfig(g Gomega, client *framework.AMPClient, orgName, projName, agentName, configID string, req framework.UpdateAgentModelConfigRequest) framework.AgentModelConfigResponse {
	path := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/model-configs/%s",
		orgName, projName, agentName, configID)

	resp, err := client.Put(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "update agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentModelConfigResponse](g, resp)
}

// DeleteAgentModelConfig deletes a model configuration.
func DeleteAgentModelConfig(g Gomega, client *framework.AMPClient, orgName, projName, agentName, configID string) {
	path := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/model-configs/%s",
		orgName, projName, agentName, configID)

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
