package configuration

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateAgentModelConfig creates a model configuration for an agent.
// It registers a cleanup function to delete the config when the test finishes.
func CreateAgentModelConfig(t *testing.T, client *framework.AMPClient, orgName, projName, agentName string, req framework.CreateAgentModelConfigRequest) framework.AgentModelConfigResponse {
	t.Helper()
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs",
		orgName, projName, agentName)

	resp, err := client.Post(basePath, req)
	if err != nil {
		framework.Fatalf(t, "create agent model config request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 201)

	config := framework.DecodeBody[framework.AgentModelConfigResponse](t, resp)

	configPath := fmt.Sprintf("%s/%s", basePath, config.UUID)
	framework.RegisterCleanup(t, client, configPath, "model-config "+config.Name)

	return config
}

// ListAgentModelConfigs returns all model configurations for an agent.
func ListAgentModelConfigs(t *testing.T, client *framework.AMPClient, orgName, projName, agentName string) framework.AgentModelConfigListResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "list agent model configs request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.AgentModelConfigListResponse](t, resp)
}

// GetAgentModelConfig retrieves a specific model configuration by ID.
func GetAgentModelConfig(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, configID string) framework.AgentModelConfigResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs/%s",
		orgName, projName, agentName, configID)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "get agent model config request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.AgentModelConfigResponse](t, resp)
}

// UpdateAgentModelConfig updates a model configuration.
func UpdateAgentModelConfig(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, configID string, req framework.UpdateAgentModelConfigRequest) framework.AgentModelConfigResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs/%s",
		orgName, projName, agentName, configID)

	resp, err := client.Put(path, req)
	if err != nil {
		framework.Fatalf(t, "update agent model config request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.AgentModelConfigResponse](t, resp)
}

// DeleteAgentModelConfig deletes a model configuration.
func DeleteAgentModelConfig(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, configID string) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/model-configs/%s",
		orgName, projName, agentName, configID)

	resp, err := client.Delete(path)
	if err != nil {
		framework.Fatalf(t, "delete agent model config request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 204)
}
