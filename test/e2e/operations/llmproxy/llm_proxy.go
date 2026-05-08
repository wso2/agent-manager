package llmproxy

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateLLMProxy creates a project-level LLM proxy linked to an org-level provider.
func CreateLLMProxy(g Gomega, client *framework.AMPClient, orgName, projName string, req framework.CreateLLMProxyRequest) framework.LLMProxyResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/llm-proxies", orgName, projName)

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "create LLM proxy request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.LLMProxyResponse](g, resp)
}

// GetLLMProxy retrieves an LLM proxy by ID.
func GetLLMProxy(g Gomega, client *framework.AMPClient, orgName, projName, proxyID string) framework.LLMProxyResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/llm-proxies/%s", orgName, projName, proxyID)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get LLM proxy request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LLMProxyResponse](g, resp)
}

// UpdateLLMProxy updates an LLM proxy.
func UpdateLLMProxy(g Gomega, client *framework.AMPClient, orgName, projName, proxyID string, req framework.UpdateLLMProxyRequest) framework.LLMProxyResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/llm-proxies/%s", orgName, projName, proxyID)

	resp, err := client.Put(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "update LLM proxy request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LLMProxyResponse](g, resp)
}

// DeleteLLMProxy deletes an LLM proxy.
func DeleteLLMProxy(g Gomega, client *framework.AMPClient, orgName, projName, proxyID string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/llm-proxies/%s", orgName, projName, proxyID)

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete LLM proxy request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
