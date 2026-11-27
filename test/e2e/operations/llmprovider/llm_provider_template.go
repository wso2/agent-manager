package llmprovider

import (
	"net/url"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// templatesPath is the LLM provider template collection.
const templatesPath = "/m/ai-gateway/llm-provider-templates"

// CreateLLMProviderTemplate creates an LLM provider template from its document.
func CreateLLMProviderTemplate(g Gomega, client *framework.AMPClient, doc framework.LLMProviderTemplateDocument) framework.LLMProviderTemplateDocument {
	resp, err := client.Post(templatesPath, doc)
	g.Expect(err).NotTo(HaveOccurred(), "create LLM provider template request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.LLMProviderTemplateDocument](g, resp)
}

// ListLLMProviderTemplates returns all LLM provider templates.
func ListLLMProviderTemplates(g Gomega, client *framework.AMPClient) framework.LLMProviderTemplateList {
	resp, err := client.Get(templatesPath)
	g.Expect(err).NotTo(HaveOccurred(), "list LLM provider templates request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LLMProviderTemplateList](g, resp)
}

// DeleteLLMProviderTemplate deletes an LLM provider template by handle.
func DeleteLLMProviderTemplate(g Gomega, client *framework.AMPClient, templateID string) {
	resp, err := client.Delete(templatesPath + "/" + url.PathEscape(templateID))
	g.Expect(err).NotTo(HaveOccurred(), "delete LLM provider template request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
