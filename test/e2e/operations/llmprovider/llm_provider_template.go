package llmprovider

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateLLMProviderTemplate creates an LLM provider template.
func CreateLLMProviderTemplate(g Gomega, client *framework.AMPClient, orgName string, req framework.CreateLLMProviderTemplateRequest) framework.LLMProviderTemplateResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/llm-provider-templates", orgName)

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "create LLM provider template request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.LLMProviderTemplateResponse](g, resp)
}

// ListLLMProviderTemplates returns all LLM provider templates.
func ListLLMProviderTemplates(g Gomega, client *framework.AMPClient, orgName string) framework.LLMProviderTemplateListResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/llm-provider-templates", orgName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list LLM provider templates request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LLMProviderTemplateListResponse](g, resp)
}

// DeleteLLMProviderTemplate deletes an LLM provider template by ID.
func DeleteLLMProviderTemplate(g Gomega, client *framework.AMPClient, orgName, templateID string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/llm-provider-templates/%s", orgName, templateID)

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete LLM provider template request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
