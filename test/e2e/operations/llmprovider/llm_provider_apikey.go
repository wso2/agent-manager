package llmprovider

import (
	"fmt"
	"net/url"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateLLMProviderAPIKey creates an API key for an LLM provider.
func CreateLLMProviderAPIKey(g Gomega, client *framework.AMPClient, providerID string, req framework.CreateLLMAPIKeyRequest) framework.CreateLLMAPIKeyResponse {
	path := fmt.Sprintf("/m/ai-gateway/llm-providers/%s/api-keys", url.PathEscape(providerID))

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "create LLM provider API key request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.CreateLLMAPIKeyResponse](g, resp)
}
