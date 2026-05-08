package llmproxy

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateLLMProxyAPIKey creates an API key for an LLM proxy.
func CreateLLMProxyAPIKey(g Gomega, client *framework.AMPClient, orgName, projName, proxyID string, req framework.CreateLLMAPIKeyRequest) framework.CreateLLMAPIKeyResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/llm-proxies/%s/api-keys",
		orgName, projName, proxyID)

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "create LLM proxy API key request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.CreateLLMAPIKeyResponse](g, resp)
}
