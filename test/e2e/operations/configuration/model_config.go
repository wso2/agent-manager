package configuration

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateAgentModelConfig creates a model configuration for an agent.
func CreateAgentModelConfig(g Gomega, client *framework.AMPClient, projName, agentName string, doc framework.AgentConfigDocument) framework.AgentConfigDocument {
	resp, err := client.Post(framework.GWAgent(projName, agentName, "/model-configs"), doc)
	g.Expect(err).NotTo(HaveOccurred(), "create agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.AgentConfigDocument](g, resp)
}

// CreateAgentMCPConfig attaches an MCP proxy to an agent.
func CreateAgentMCPConfig(g Gomega, client *framework.AMPClient, projName, agentName string, doc framework.AgentConfigDocument) framework.AgentConfigDocument {
	resp, err := client.Post(framework.GWAgent(projName, agentName, "/mcp-configs"), doc)
	g.Expect(err).NotTo(HaveOccurred(), "create agent MCP config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.AgentConfigDocument](g, resp)
}

// DeleteAgentMCPConfigBestEffort removes a disposable MCP binding before its
// agent and proxy are deleted. Removing the binding first avoids a proxy-delete
// conflict during suite teardown, while never hiding the original spec result
// behind a cleanup failure.
func DeleteAgentMCPConfigBestEffort(ctx context.Context, client *framework.AMPClient, projName, agentName, configID string) {
	if configID == "" {
		return
	}
	response, err := client.DeleteWithContext(ctx, framework.GWAgent(projName, agentName, "/mcp-configs/%s", configID))
	if err != nil {
		ginkgo.GinkgoWriter.Printf("teardown: delete agent MCP config %q failed: %v\n", configID, err)
		return
	}
	defer response.Body.Close()
	ginkgo.GinkgoWriter.Printf("teardown: deleted agent MCP config %q (status %d)\n", configID, response.StatusCode)
}

// ListAgentModelConfigs returns all model configurations for an agent.
func ListAgentModelConfigs(g Gomega, client *framework.AMPClient, projName, agentName string) framework.AgentConfigListResponse {
	resp, err := client.Get(framework.GWAgent(projName, agentName, "/model-configs"))
	g.Expect(err).NotTo(HaveOccurred(), "list agent model configs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentConfigListResponse](g, resp)
}

// GetAgentModelConfig retrieves a specific model configuration by ID.
func GetAgentModelConfig(g Gomega, client *framework.AMPClient, projName, agentName, configID string) framework.AgentConfigDocument {
	resp, err := client.Get(framework.GWAgent(projName, agentName, "/model-configs/%s", configID))
	g.Expect(err).NotTo(HaveOccurred(), "get agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentConfigDocument](g, resp)
}

// UpdateAgentModelConfig replaces a model configuration with doc.
func UpdateAgentModelConfig(g Gomega, client *framework.AMPClient, projName, agentName, configID string, doc framework.AgentConfigDocument) framework.AgentConfigDocument {
	doc.Status = nil
	resp, err := client.Put(framework.GWAgent(projName, agentName, "/model-configs/%s", configID), doc)
	g.Expect(err).NotTo(HaveOccurred(), "update agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.AgentConfigDocument](g, resp)
}

// DeleteAgentModelConfig deletes a model configuration.
func DeleteAgentModelConfig(g Gomega, client *framework.AMPClient, projName, agentName, configID string) {
	resp, err := client.Delete(framework.GWAgent(projName, agentName, "/model-configs/%s", configID))
	g.Expect(err).NotTo(HaveOccurred(), "delete agent model config request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
