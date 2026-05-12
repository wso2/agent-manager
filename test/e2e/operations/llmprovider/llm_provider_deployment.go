package llmprovider

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeployLLMProviderRequest is the request body for deploying a provider to a gateway.
type DeployLLMProviderRequest struct {
	Name      string `json:"name"`
	GatewayID string `json:"gatewayId"`
}

// LLMDeploymentResponse is the response from creating a deployment.
type LLMDeploymentResponse struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerId"`
	GatewayID  string `json:"gatewayId"`
	Status     string `json:"status"`
}

// DeployLLMProvider deploys an LLM provider to a gateway.
func DeployLLMProvider(g Gomega, client *framework.AMPClient, orgName, providerID string, req DeployLLMProviderRequest) LLMDeploymentResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/llm-providers/%s/deployments", orgName, providerID)

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "deploy LLM provider request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[LLMDeploymentResponse](g, resp)
}
