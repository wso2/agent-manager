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

// Validates LLM provider with an external agent: provider creation, proxy
// endpoint invocation, guardrail policy application, and request blocking.

package llmprovider

import (
	"bytes"
	"encoding/json"
	"io"
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
)

var _ = Describe("LLM provider configured for an external agent:", Label("llm-provider", "external-agent"), Ordered, func() {
	var (
		agentName string
		suffix    string

		gatewayUUID   string
		providerID    string
		providerUUID  string
		proxyUUID     string
		proxyAPIKey   string
		proxyAuthName string
		proxyURL      string
		modelConfigID string

		createReq framework.CreateAgentRequest
	)

	BeforeAll(func() {
		suffix = uuid.New().String()[:8]
		agentName = framework.E2EAgentPrefix + suffix
		providerID = framework.E2ELLMProviderPrefix + suffix

		createReq = framework.NewExternalAgentRequest(agentName, "External agent for e2e LLM provider guardrails test")
	})

	// This agent is single-use; delete it so it doesn't accumulate.
	AfterAll(func() {
		agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName)
	})

	It("finds an active AI gateway for the default environment", func() {
		gatewayUUID = gateway.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
	})

	It("creates an OpenAI-backed LLM provider from the built-in template", func() {
		By("Fetching the OpenAI template to get endpoint URL and auth config")
		templates := llmproviderop.ListLLMProviderTemplates(Default, Client, Cfg.DefaultOrg)
		var openaiTpl *framework.LLMProviderTemplateResponse
		for i, t := range templates.Templates {
			if t.ID == "openai" {
				openaiTpl = &templates.Templates[i]
				break
			}
		}
		Expect(openaiTpl).NotTo(BeNil(), "expected built-in 'openai' template to exist")
		Expect(openaiTpl.Metadata).NotTo(BeNil(), "expected template metadata")
		Expect(openaiTpl.Metadata.EndpointURL).NotTo(BeEmpty(), "expected template endpoint URL")
		Expect(openaiTpl.Metadata.Auth).NotTo(BeNil(), "expected template auth config")
		GinkgoWriter.Printf("OpenAI template: url=%s, auth.type=%s, auth.header=%s\n",
			openaiTpl.Metadata.EndpointURL, openaiTpl.Metadata.Auth.Type, openaiTpl.Metadata.Auth.Header)

		By("Creating the LLM provider")
		upstreamURL := openaiTpl.Metadata.EndpointURL
		authHeader := openaiTpl.Metadata.Auth.Header
		authValue := openaiTpl.Metadata.Auth.ValuePrefix + Cfg.OpenAIAPIKey

		prov := llmproviderop.CreateLLMProvider(Default, Client, providerID,
			framework.LLMProviderSpec{
				DisplayName: "E2E OpenAI Provider " + suffix,
				Version:     "v1.0",
				Context:     "/" + providerID,
				Template:    "openai",
				Upstream: framework.UpstreamConfig{
					Main: &framework.UpstreamEndpoint{
						URL: &upstreamURL,
						Auth: &framework.UpstreamAuth{
							Type:   openaiTpl.Metadata.Auth.Type,
							Header: &authHeader,
							Value:  &authValue,
						},
					},
				},
				Security: &framework.SecurityConfig{
					Enabled: true,
					APIKey: &framework.SecurityAPIKey{
						Enabled: true,
						Key:     "X-API-Key",
						In:      "header",
					},
				},
				AccessControl: &framework.LLMAccessControl{
					Mode:       "allow_all",
					Exceptions: []string{},
				},
				Gateways: []string{gatewayUUID},
			})
		providerUUID = prov.Metadata.UID
		Expect(providerUUID).NotTo(BeEmpty())
		GinkgoWriter.Printf("LLM provider created and deployed: %s (UUID: %s, gateway: %s)\n", providerID, providerUUID, gatewayUUID)
	})

	It("registers an external agent", func() {
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     createReq,
		})
		Expect(ag.Metadata.Name).To(Equal(agentName))
		Expect(ag.Spec.Provisioning.Type).To(Equal("external"))
		Expect(ag.Spec.AgentType.Type).To(Equal("external-agent-api"))
		Expect(ag.Spec.AgentType.SubType).To(Equal("custom-api"))
		GinkgoWriter.Printf("External agent: %s\n", agentName)
	})

	It("creates a model config and returns the LLM proxy credentials", func() {
		By("Creating model config — this auto-creates the LLM proxy and returns credentials")
		config := configuration.CreateAgentModelConfig(Default, Client, framework.E2ESharedProjectName, agentName,
			framework.NewModelConfigDocument(framework.E2ESharedProjectName, "e2e-test-llmprov-config-"+suffix, providerID))
		Expect(config.Metadata.UID).NotTo(BeEmpty())
		modelConfigID = config.Metadata.UID

		By("Extracting proxy URL, API key, and auth header from response")
		Expect(config.Status).NotTo(BeNil(), "expected the binding's status")
		status := config.Status

		proxyUUID = status.ProxyUuid
		proxyURL = status.URL
		Expect(proxyUUID).NotTo(BeEmpty(), "proxy UUID should be set")
		Expect(proxyURL).NotTo(BeEmpty(), "proxy URL should be set")

		Expect(status.AuthInfo).NotTo(BeNil(), "expected authInfo with API key")
		Expect(status.AuthInfo.Value).NotTo(BeNil(), "expected API key value")
		proxyAPIKey = *status.AuthInfo.Value
		Expect(status.AuthInfo.In).To(Equal("header"), "expected the proxy API key in a header")
		Expect(status.AuthInfo.Name).NotTo(BeEmpty(), "expected the proxy API key header name")
		proxyAuthName = status.AuthInfo.Name

		GinkgoWriter.Printf("Model config: %s\n  Proxy UUID: %s\n  URL: %s\n  Auth: %s in %s header %s\n",
			config.Metadata.UID, proxyUUID, proxyURL,
			status.AuthInfo.Type,
			status.AuthInfo.In,
			status.AuthInfo.Name)
	})

	It("invokes the LLM proxy endpoint successfully", func() {
		if Cfg.OpenAIAPIKey == "" {
			Skip("OPENAI_API_KEY is not set (pull requests from forks get no secrets)")
		}
		Expect(proxyURL).NotTo(BeEmpty(), "proxy URL must be set")
		Expect(proxyAPIKey).NotTo(BeEmpty(), "proxy API key must be set")

		chatBody := map[string]any{
			"model": "gpt-5-mini",
			"messages": []map[string]string{
				{"role": "user", "content": "Say hello in one word"},
			},
		}
		data, err := json.Marshal(chatBody)
		Expect(err).NotTo(HaveOccurred())

		endpoint := proxyURL + "/chat/completions"
		GinkgoWriter.Printf("Invoking endpoint: %s\n", endpoint)

		httpClient := &http.Client{Timeout: 30 * time.Second}

		// The gateway needs a few seconds to sync the new route after provider deployment.
		Eventually(func(g Gomega) {
			req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
			g.Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(proxyAuthName, proxyAPIKey)

			resp, err := httpClient.Do(req)
			g.Expect(err).NotTo(HaveOccurred(), "proxy invocation request to %s failed: %v", endpoint, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"proxy invocation returned %d: %s", resp.StatusCode, string(body))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	})

	It("adds a content-length guardrail to the model config", func() {
		Expect(modelConfigID).NotTo(BeEmpty(), "model config ID must be set")

		cfg := configuration.GetAgentModelConfig(Default, Client, framework.E2ESharedProjectName, agentName, modelConfigID)
		cfg.Spec.Configuration = map[string]interface{}{
			"policies": []map[string]interface{}{
				{
					"name":    "content-length-guardrail",
					"version": "v1.0.1",
					"paths": []map[string]interface{}{
						{
							"path":    "/*",
							"methods": []string{"POST"},
							"params": map[string]interface{}{
								"request": map[string]interface{}{
									"enabled": true,
									"min":     0,
									"max":     1,
								},
							},
						},
					},
				},
			},
		}
		configuration.UpdateAgentModelConfig(Default, Client, framework.E2ESharedProjectName, agentName, modelConfigID, cfg)
		GinkgoWriter.Printf("Model config updated with content-length-guardrail (maxBytes: 10)\n")
	})

	It("blocks a request that exceeds the content-length guardrail", func() {
		Expect(proxyURL).NotTo(BeEmpty())
		Expect(proxyAPIKey).NotTo(BeEmpty())

		chatBody := map[string]any{
			"model": "gpt-5-mini",
			"messages": []map[string]string{
				{"role": "user", "content": "Say hello in one word"},
			},
		}
		data, err := json.Marshal(chatBody)
		Expect(err).NotTo(HaveOccurred())

		endpoint := proxyURL + "/chat/completions"
		httpClient := &http.Client{Timeout: 30 * time.Second}

		// Wait for the guardrail to propagate to the gateway and block the request.
		// We expect a non-200 AND non-404 response — 404 means the gateway is
		// still resyncing after the config update, so we retry past that.
		Eventually(func(g Gomega) {
			req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(data))
			g.Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(proxyAuthName, proxyAPIKey)

			resp, err := httpClient.Do(req)
			g.Expect(err).NotTo(HaveOccurred(), "proxy invocation request to %s failed: %v", endpoint, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			// Skip 404/200 — gateway still resyncing or guardrail not yet applied
			if resp.StatusCode == http.StatusNotFound {
				g.Expect(resp.StatusCode).NotTo(SatisfyAny(
					Equal(http.StatusNotFound), Equal(http.StatusOK),
				),
					"waiting for guardrail to take effect")
				return
			}

			// Guardrail should return 422 with GUARDRAIL_INTERVENED
			g.Expect(resp.StatusCode).To(Equal(http.StatusUnprocessableEntity),
				"expected 422 from guardrail, got %d: %s", resp.StatusCode, string(body))
			g.Expect(string(body)).To(ContainSubstring("GUARDRAIL_INTERVENED"),
				"expected GUARDRAIL_INTERVENED in response")
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	})
	It("refuses a call to the agent's proxy without its key", func() {
		req, err := http.NewRequest(http.MethodPost, proxyURL+"/chat/completions", bytes.NewBufferString(`{"model":"gpt-5-mini","messages":[]}`))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		Eventually(func(g Gomega) {
			resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req.Clone(req.Context()))
			g.Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			g.Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		}).WithTimeout(time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	})

	It("shows the agent's model config through amctl", func() {
		Expect(cliagent.ListAgentLLM(Default, H, H.Org(), framework.E2ESharedProjectName, agentName).Names()).
			To(ContainElement("e2e-test-llmprov-config-" + suffix))
	})

	It("keeps the agent working after the provider's upstream key is rotated", func() {
		if Cfg.OpenAIAPIKey == "" {
			Skip("OPENAI_API_KEY is not set (pull requests from forks get no secrets)")
		}
		By("Removing the guardrail")
		unguarded := configuration.GetAgentModelConfig(Default, Client, framework.E2ESharedProjectName, agentName, modelConfigID)
		unguarded.Spec.Configuration = map[string]any{}
		configuration.UpdateAgentModelConfig(Default, Client, framework.E2ESharedProjectName, agentName, modelConfigID, unguarded)

		By("Rotating the upstream key on the provider")
		doc := llmproviderop.GetLLMProvider(Default, Client, providerID)
		rotated := "Bearer " + Cfg.OpenAIAPIKey
		doc.Spec.Upstream.Main.Auth.Value = &rotated
		framework.Call(Default, Client, http.MethodPut, "/m/ai-gateway/llm-providers/"+providerUUID, doc, http.StatusOK)

		By("Calling the model through the agent's unchanged proxy key")
		body := `{"model":"gpt-5-mini","messages":[{"role":"user","content":"Say hello in one word"}]}`
		Eventually(func(g Gomega) {
			req, err := http.NewRequest(http.MethodPost, proxyURL+"/chat/completions", bytes.NewBufferString(body))
			g.Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(proxyAuthName, proxyAPIKey)
			resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
			g.Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			out, _ := io.ReadAll(resp.Body)
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK), string(out))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	})
})
