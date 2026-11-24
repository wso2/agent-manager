package llmprovider

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateLLMProvider creates an LLM provider in the token's org.
//
// If the caller leaves Security/AccessControl unset, they default to the same values
// the console sends (API-key security enabled + allow-all access). These are required
// for the provider to deploy with its routes on the gateway — without them the
// provider deploys with zero routes and the proxy that fronts it returns 404.
func CreateLLMProvider(g Gomega, client *framework.AMPClient, name string, spec framework.LLMProviderSpec) framework.LLMProviderDocument {
	if spec.Security == nil {
		spec.Security = &framework.SecurityConfig{
			Enabled: true,
			APIKey:  &framework.SecurityAPIKey{Enabled: true, Key: "API-Key", In: "header"},
		}
	}
	if spec.AccessControl == nil {
		spec.AccessControl = &framework.LLMAccessControl{Mode: "allow_all", Exceptions: []string{}}
	}

	doc := framework.LLMProviderDocument{
		APIVersion: framework.LLMProviderAPIVersion,
		Kind:       framework.LLMProviderKind,
		Metadata:   framework.DocumentMetadata{Name: name},
		Spec:       spec,
	}
	resp, err := client.Post("/m/ai-gateway/llm-providers", doc)
	g.Expect(err).NotTo(HaveOccurred(), "create LLM provider request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.LLMProviderDocument](g, resp)
}

// GetLLMProvider retrieves an LLM provider by handle or UUID.
func GetLLMProvider(g Gomega, client *framework.AMPClient, providerID string) framework.LLMProviderDocument {
	resp, err := client.Get(fmt.Sprintf("/m/ai-gateway/llm-providers/%s", providerID))
	g.Expect(err).NotTo(HaveOccurred(), "get LLM provider request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LLMProviderDocument](g, resp)
}

// DeleteLLMProvider deletes an LLM provider by handle or UUID.
func DeleteLLMProvider(g Gomega, client *framework.AMPClient, providerID string) {
	resp, err := client.Delete(fmt.Sprintf("/m/ai-gateway/llm-providers/%s", providerID))
	g.Expect(err).NotTo(HaveOccurred(), "delete LLM provider request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
