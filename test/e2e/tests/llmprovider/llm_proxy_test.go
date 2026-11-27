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

// J12: a project gets LLM proxies over an org's provider. A developer creates
// one, deploys it, issues keys for it, and an agent consumes the provider
// through its own proxy; the provider can't be deleted while it has consumers.

package llmprovider

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	cliagent "github.com/wso2/agent-manager/test/e2e/operations/cli/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/configuration"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	llmproviderop "github.com/wso2/agent-manager/test/e2e/operations/llmprovider"
	"github.com/wso2/agent-manager/test/e2e/operations/llmproxy"
)

var _ = Describe("Project LLM proxies:", Label("llm-provider", "llm-proxy"), Ordered, func() {
	var suffix, providerName, providerID, proxyID, agentName, configID, configName, gatewayID string
	proj := framework.E2ESharedProjectName
	px := func(f string, a ...any) string {
		return framework.GW(Cfg, "/projects/%s/llm-proxies"+f, append([]any{proj}, a...)...)
	}
	mc := func(f string, a ...any) string {
		return framework.GWAgent(proj, agentName, "/model-configs"+f, a...)
	}

	BeforeAll(func() {
		suffix = uuid.NewString()[:8]
		providerName = framework.E2ELLMProviderPrefix + "px-" + suffix
		agentName = framework.E2EAgentPrefix + suffix
		configName = "e2e-test-px-config-" + suffix
		gatewayID = gateway.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		DeferCleanup(func() {
			if configID != "" {
				framework.Send(Default, Client, http.MethodDelete, mc("/%s", configID), nil)
			}
			agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, proj, agentName)
			if proxyID != "" {
				framework.Send(Default, Client, http.MethodDelete, px("/%s", proxyID), nil)
			}
			framework.Send(Default, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil)
		})
	})

	It("creates a provider for the project's proxies", func() {
		url, header, value := "https://api.openai.com/v1", "Authorization", "Bearer sk-e2e-placeholder"
		prov := llmproviderop.CreateLLMProvider(Default, Client, providerName, framework.LLMProviderSpec{
			DisplayName: providerName, Version: "v1.0", Context: "/" + providerName, Template: "openai",
			Upstream: framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{
				URL: &url, Auth: &framework.UpstreamAuth{Type: "api-key", Header: &header, Value: &value},
			}},
			Gateways: []string{gatewayID},
		})
		providerID = prov.Metadata.UID
	})

	It("creates, reads, lists and updates a project proxy over the provider", func() {
		name, version, ctx := "e2e-test-proxy-"+suffix, "v1.0", "/e2e-test-proxy-"+suffix
		p := llmproxy.CreateLLMProxy(Default, Client, Cfg.DefaultOrg, proj, framework.CreateLLMProxyRequest{
			ProviderUuid:  providerID,
			Configuration: framework.LLMProxyConfig{Name: &name, Version: &version, Context: &ctx, Provider: &providerID},
		})
		// Proxies are addressed by their name.
		proxyID = name
		Expect(p.UUID).NotTo(BeEmpty())
		Expect(p.ProviderUuid).To(Equal(providerID))

		got := llmproxy.GetLLMProxy(Default, Client, Cfg.DefaultOrg, proj, proxyID)
		Expect(*got.Configuration.Name).To(Equal(name))
		list := framework.CallJSON[framework.LLMProxyListResponse](Default, Client, http.MethodGet, px(""), nil, http.StatusOK)
		ids := []string{}
		for _, l := range list.Proxies {
			ids = append(ids, l.UUID)
		}
		Expect(ids).To(ContainElement(p.UUID))

		desc := "updated"
		updated := llmproxy.UpdateLLMProxy(Default, Client, Cfg.DefaultOrg, proj, proxyID, framework.UpdateLLMProxyRequest{
			Description: &desc, ProviderUuid: &providerID,
			Configuration: &framework.LLMProxyConfig{Name: &name, Version: &version, Context: &ctx, Provider: &providerID},
		})
		Expect(*updated.Description).To(Equal("updated"))

		absent := framework.E2ELLMProviderPrefix + "absent"
		other := name + "-x"
		e := framework.CallError(Default, Client, http.MethodPost, px(""), framework.CreateLLMProxyRequest{
			ProviderUuid: uuid.NewString(), Configuration: framework.LLMProxyConfig{Name: &other, Version: &version, Context: &ctx, Provider: &absent},
		}, http.StatusBadRequest)
		Expect(e.Message).To(ContainSubstring("provider not found"))
		framework.CallError(Default, Client, http.MethodPost, px(""), framework.CreateLLMProxyRequest{
			ProviderUuid: providerID, Configuration: framework.LLMProxyConfig{Name: &other, Version: &version},
		}, http.StatusBadRequest)
	})

	It("deploys the proxy to the gateway, undeploys and restores it", func() {
		dep := framework.CallObj(Default, Client, http.MethodPost, px("/%s/deployments", proxyID),
			framework.Obj{"name": "e2e-dep-" + suffix, "base": "current", "gatewayId": gatewayID}, http.StatusCreated)
		depID := framework.Str(dep, "deploymentId")
		Expect(depID).NotTo(BeEmpty())
		deps := framework.CallJSON[[]framework.Obj](Default, Client, http.MethodGet, px("/%s/deployments", proxyID), nil, http.StatusOK)
		Expect(framework.Field(deps, "deploymentId")).To(ContainElement(depID))
		framework.Call(Default, Client, http.MethodGet, px("/%s/deployments/%s", proxyID, depID), nil, http.StatusOK)

		framework.Call(Default, Client, http.MethodPost, px("/%s/deployments/undeploy?deploymentId=%s&gatewayId=%s", proxyID, depID, gatewayID), nil, http.StatusOK)
		Eventually(func(g Gomega) {
			d := framework.CallObj(g, Client, http.MethodGet, px("/%s/deployments/%s", proxyID, depID), nil, http.StatusOK)
			g.Expect(d["status"]).To(Equal("UNDEPLOYED"))
		}).WithTimeout(time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		framework.Call(Default, Client, http.MethodPost, px("/%s/deployments/restore?deploymentId=%s&gatewayId=%s", proxyID, depID, gatewayID), nil, http.StatusOK)
		framework.Call(Default, Client, http.MethodPost, px("/%s/deployments/undeploy?deploymentId=%s&gatewayId=%s", proxyID, depID, gatewayID), nil, http.StatusOK)
		Eventually(func(g Gomega) {
			framework.Call(g, Client, http.MethodDelete, px("/%s/deployments/%s", proxyID, depID), nil, http.StatusNoContent)
		}).WithTimeout(time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	})

	It("issues, rotates and revokes keys for the proxy", func() {
		key := framework.CallObj(Default, Client, http.MethodPost, px("/%s/api-keys", proxyID),
			framework.Obj{"name": "e2e-px-key", "displayName": "e2e-px-key"}, http.StatusCreated)
		first := framework.Str(key, "apiKey")
		Expect(first).NotTo(BeEmpty())
		keys := framework.CallObj(Default, Client, http.MethodGet, px("/%s/api-keys", proxyID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(keys, "keys"), "name")).To(ContainElement("e2e-px-key"))
		Expect(string(framework.Call(Default, Client, http.MethodGet, px("/%s/api-keys", proxyID), nil, http.StatusOK))).NotTo(ContainSubstring(first))

		rotated := framework.CallObj(Default, Client, http.MethodPut, px("/%s/api-keys/e2e-px-key", proxyID), framework.Obj{}, http.StatusOK)
		Expect(framework.Str(rotated, "apiKey")).NotTo(Equal(first))
		framework.Call(Default, Client, http.MethodDelete, px("/%s/api-keys/e2e-px-key", proxyID), nil, http.StatusNoContent)
		keys = framework.CallObj(Default, Client, http.MethodGet, px("/%s/api-keys", proxyID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(keys, "keys"), "name")).NotTo(ContainElement("e2e-px-key"))

		// Rotating a revoked key does not bring it back.
		framework.CallError(Default, Client, http.MethodPut, px("/%s/api-keys/e2e-px-key", proxyID), framework.Obj{}, http.StatusNotFound)
		keys = framework.CallObj(Default, Client, http.MethodGet, px("/%s/api-keys", proxyID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(keys, "keys"), "name")).NotTo(ContainElement("e2e-px-key"))
	})

	It("lets an agent consume the provider through its own proxy, as amctl shows", func() {
		agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName: Cfg.DefaultOrg, ProjectName: proj, Request: framework.NewExternalAgentRequest(agentName, "llm proxy consumer"),
		})
		cfg := configuration.CreateAgentModelConfig(Default, Client, proj, agentName, framework.NewModelConfigDocument(proj, configName, providerName))
		configID = cfg.Metadata.UID
		Expect(cfg.Status).NotTo(BeNil())
		agentProxy := cfg.Status.ProxyUuid
		Expect(agentProxy).NotTo(BeEmpty())

		consumers := framework.CallObj(Default, Client, http.MethodGet, "/m/ai-gateway/llm-providers/"+providerID+"/consumers", nil, http.StatusOK)
		Expect(consumers["total"]).To(BeNumerically(">=", 1))
		Expect(cliagent.ListAgentLLM(Default, H, H.Org(), proj, agentName).Names()).To(ContainElement(configName))

		got := configuration.GetAgentModelConfig(Default, Client, proj, agentName, configID)
		Expect(got.Metadata.Name).To(Equal(configName))
		list := configuration.ListAgentModelConfigs(Default, Client, proj, agentName)
		Expect(list.List).NotTo(BeEmpty())

		k := framework.CallObj(Default, Client, http.MethodPost, mc("/%s/api-keys", configID), framework.Obj{"name": "e2e-mc-key", "displayName": "e2e-mc-key"}, http.StatusCreated)
		Expect(framework.Str(k, "apiKey")).NotTo(BeEmpty())
		Expect(framework.Field(framework.Items(framework.CallObj(Default, Client, http.MethodGet, mc("/%s/api-keys", configID), nil, http.StatusOK), "keys"), "name")).To(ContainElement("e2e-mc-key"))
		framework.Call(Default, Client, http.MethodPut, mc("/%s/api-keys/e2e-mc-key", configID), framework.Obj{}, http.StatusOK)
		framework.Call(Default, Client, http.MethodDelete, mc("/%s/api-keys/e2e-mc-key", configID), nil, http.StatusNoContent)

		// The agent reaches the provider through its own proxy's gateway URL.
		Expect(cfg.Status.URL).To(HavePrefix("http"))
	})

	It("refuses to delete the provider while it has consumers, and keeps it", func() {
		framework.CallError(Default, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil, http.StatusConflict)
		Expect(llmproviderop.GetLLMProvider(Default, Client, providerName).Metadata.UID).To(Equal(providerID))
	})

	It("deletes the provider once its consumers are gone", func() {
		configuration.DeleteAgentModelConfig(Default, Client, proj, agentName, configID)
		Expect(cliagent.ListAgentLLM(Default, H, H.Org(), proj, agentName).Names()).NotTo(ContainElement(configName))
		configID = ""
		llmproxy.DeleteLLMProxy(Default, Client, Cfg.DefaultOrg, proj, proxyID)
		framework.CallError(Default, Client, http.MethodGet, px("/%s", proxyID), nil, http.StatusNotFound)
		proxyID = ""
		Eventually(func(g Gomega) {
			framework.Call(g, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil, http.StatusNoContent)
		}).WithTimeout(time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	})
})
