package llmprovider

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateLLMProvider creates an LLM provider at the org level.
//
// If the caller leaves Security/AccessControl unset, they default to the same values
// the console sends (API-key security enabled + allow-all access). These are required
// for the provider to deploy with its routes on the gateway — without them the
// provider deploys with zero routes and the proxy that fronts it returns 404.
func CreateLLMProvider(g Gomega, client *framework.AMPClient, orgName string, req framework.CreateLLMProviderRequest) framework.LLMProviderResponse {
	if req.Security == nil {
		req.Security = &framework.SecurityConfig{
			Enabled: true,
			APIKey:  &framework.SecurityAPIKey{Enabled: true, Key: "API-Key", In: "header"},
		}
	}
	if req.AccessControl == nil {
		req.AccessControl = &framework.LLMAccessControl{Mode: "allow_all", Exceptions: []string{}}
	}

	path := fmt.Sprintf("/api/v1/orgs/%s/llm-providers", orgName)

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "create LLM provider request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.LLMProviderResponse](g, resp)
}

// GetLLMProvider retrieves an LLM provider by ID.
func GetLLMProvider(g Gomega, client *framework.AMPClient, orgName, providerID string) framework.LLMProviderResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/llm-providers/%s", orgName, providerID)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get LLM provider request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LLMProviderResponse](g, resp)
}

// DeleteLLMProvider deletes an LLM provider by ID.
func DeleteLLMProvider(g Gomega, client *framework.AMPClient, orgName, providerID string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/llm-providers/%s", orgName, providerID)

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete LLM provider request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
