// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// J4 (administration): an AI lead adds a custom provider template, builds a
// provider from it, deploys it to the gateway and takes it down again, and
// rotates its upstream key; amctl sees the same providers.

package llmprovider

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	clillmprovider "github.com/wso2/agent-manager/test/e2e/operations/cli/llmprovider"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	llmproviderop "github.com/wso2/agent-manager/test/e2e/operations/llmprovider"
)

var _ = Describe("LLM provider administration:", Label("llm-provider", "admin"), Ordered, func() {
	var suffix, templateID, providerName, providerID, gatewayID string
	tpl := func(f string, a ...any) string { return framework.GW(Cfg, "/llm-provider-templates"+f, a...) }
	dep := func(f string, a ...any) string {
		return framework.GW(Cfg, "/llm-providers/%s/deployments"+f, append([]any{providerID}, a...)...)
	}

	BeforeAll(func() {
		suffix = uuid.NewString()[:8]
		templateID = "e2e-test-tpl-" + suffix
		providerName = framework.E2ELLMProviderPrefix + "adm-" + suffix
		gatewayID = gateway.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		DeferCleanup(func() {
			framework.Send(Default, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil)
			framework.Send(Default, Client, http.MethodDelete, tpl("/%s", templateID), nil)
		})
	})

	It("adds, reads and updates a custom provider template", func() {
		framework.CallObj(Default, Client, http.MethodPost, tpl(""), framework.Obj{
			"id": templateID, "name": "E2E template " + suffix, "description": "custom",
			"metadata": framework.Obj{"endpointUrl": "https://llm.example.test/v1", "auth": framework.Obj{"type": "api-key", "header": "Authorization", "valuePrefix": "Bearer "}},
		}, http.StatusCreated)
		got := framework.CallObj(Default, Client, http.MethodGet, tpl("/%s", templateID), nil, http.StatusOK)
		Expect(got["name"]).To(Equal("E2E template " + suffix))
		Expect(framework.Str(got, "metadata", "endpointUrl")).To(Equal("https://llm.example.test/v1"))

		framework.CallObj(Default, Client, http.MethodPut, tpl("/%s", templateID), framework.Obj{
			"id": templateID, "name": "E2E template " + suffix, "description": "renamed",
			"metadata": framework.Obj{"endpointUrl": "https://llm.example.test/v2"},
		}, http.StatusOK)
		Expect(framework.CallObj(Default, Client, http.MethodGet, tpl("/%s", templateID), nil, http.StatusOK)["description"]).To(Equal("renamed"))
		list := llmproviderop.ListLLMProviderTemplates(Default, Client, Cfg.DefaultOrg)
		ids := []string{}
		for _, t := range list.Templates {
			ids = append(ids, t.ID)
		}
		Expect(ids).To(ContainElements(templateID, "openai"))

		// Built-in templates are read-only.
		status, _ := framework.Send(Default, Client, http.MethodDelete, tpl("/openai"), nil)
		Expect(status).To(BeNumerically(">=", 400))
		framework.CallError(Default, Client, http.MethodPost, tpl(""), framework.Obj{"id": templateID, "name": "dup"}, http.StatusConflict)
	})

	It("builds a provider from the template, which amctl lists", func() {
		url, header, value := "https://llm.example.test/v2", "Authorization", "Bearer sk-e2e-placeholder"
		prov := llmproviderop.CreateLLMProvider(Default, Client, providerName, framework.LLMProviderSpec{
			DisplayName: providerName, Version: "v1.0", Context: "/" + providerName, Template: templateID,
			Upstream: framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{
				URL: &url, Auth: &framework.UpstreamAuth{Type: "api-key", Header: &header, Value: &value},
			}},
		})
		providerID = prov.Metadata.UID
		Expect(clillmprovider.ListLLMProviders(Default, H, H.Org()).Ids()).To(ContainElement(providerName))
		list := framework.CallObj(Default, Client, http.MethodGet, "/m/ai-gateway/llm-providers", nil, http.StatusOK)
		Expect(list["count"]).To(BeNumerically(">=", 1))
		policies := framework.CallObj(Default, Client, http.MethodGet, framework.GW(Cfg, "/llm-providers/policies"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(policies, "list"), "name")).To(ContainElement("content-length-guardrail"))
	})

	It("rotates the provider's upstream key without exposing it", func() {
		doc := llmproviderop.GetLLMProvider(Default, Client, providerName)
		rotated := "Bearer sk-e2e-rotated-" + suffix
		doc.Spec.Upstream.Main.Auth.Value = &rotated
		// Updates take the provider's UUID or its handle, like GET and DELETE.
		framework.Call(Default, Client, http.MethodPut, "/m/ai-gateway/llm-providers/"+providerID, doc, http.StatusOK)
		framework.Call(Default, Client, http.MethodPut, "/m/ai-gateway/llm-providers/"+providerName, doc, http.StatusOK)
		raw := framework.Call(Default, Client, http.MethodGet, "/m/ai-gateway/llm-providers/"+providerName, nil, http.StatusOK)
		Expect(string(raw)).NotTo(ContainSubstring("sk-e2e-rotated-"), "the upstream key must not be returned")
	})

	It("deploys the provider to a gateway, undeploys and restores it", func() {
		d := framework.CallObj(Default, Client, http.MethodPost, dep(""),
			framework.Obj{"name": "e2e-dep-" + suffix, "base": "current", "gatewayId": gatewayID}, http.StatusCreated)
		depID := framework.Str(d, "deploymentId")
		Expect(framework.Field(framework.CallJSON[[]framework.Obj](Default, Client, http.MethodGet, dep(""), nil, http.StatusOK), "deploymentId")).
			To(ContainElement(depID))
		Expect(framework.CallObj(Default, Client, http.MethodGet, dep("/%s", depID), nil, http.StatusOK)["gatewayId"]).To(Equal(gatewayID))

		framework.Call(Default, Client, http.MethodPost, dep("/undeploy?deploymentId=%s&gatewayId=%s", depID, gatewayID), nil, http.StatusOK)
		framework.Call(Default, Client, http.MethodPost, dep("/restore?deploymentId=%s&gatewayId=%s", depID, gatewayID), nil, http.StatusOK)
		framework.Call(Default, Client, http.MethodPost, dep("/undeploy?deploymentId=%s&gatewayId=%s", depID, gatewayID), nil, http.StatusOK)
		Eventually(func(g Gomega) {
			framework.Call(g, Client, http.MethodDelete, dep("/%s", depID), nil, http.StatusNoContent)
		}).WithTimeout(time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		Expect(framework.Field(framework.CallJSON[[]framework.Obj](Default, Client, http.MethodGet, dep(""), nil, http.StatusOK), "deploymentId")).
			NotTo(ContainElement(depID))
	})

	It("deletes the provider and then the template", func() {
		clillmprovider.DeleteLLMProvider(Default, H, H.Org(), providerName)
		framework.CallError(Default, Client, http.MethodGet, "/m/ai-gateway/llm-providers/"+providerName, nil, http.StatusNotFound)
		Eventually(func(g Gomega) {
			framework.Call(g, Client, http.MethodDelete, tpl("/%s", templateID), nil, http.StatusNoContent)
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		framework.CallError(Default, Client, http.MethodGet, tpl("/%s", templateID), nil, http.StatusNotFound)
	})
})
