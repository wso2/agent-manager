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
	"github.com/wso2/agent-manager/test/e2e/operations/configuration"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	llmproviderop "github.com/wso2/agent-manager/test/e2e/operations/llmprovider"
)

var _ = Describe("LLM Provider with External Agent and Guardrails", Label("llm-provider", "external-agent"), Ordered, func() {
	var (
		agentName string
		suffix    string

		gatewayUUID   string
		providerID    string
		providerUUID  string
		proxyUUID     string
		proxyAPIKey   string
		proxyURL      string
		modelConfigID string

		createReq framework.CreateAgentRequest
	)

	BeforeAll(func() {
		suffix = uuid.New().String()[:8]
		agentName = "e2e-llm-ext-" + suffix
		providerID = "e2e-openai-" + suffix

		framework.LoadTestData(TestDataDir, "llm-provider-external-agent/create_agent.json", &createReq)
		createReq.Name = agentName
	})

	It("should have a running AI gateway", func() {
		gatewayUUID = gateway.WaitForActiveAIGateway(Client, Cfg.DefaultOrg, "default", 3*time.Minute)
	})

	It("should create an LLM provider using the OpenAI template", func() {
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

		prov := llmproviderop.CreateLLMProvider(Default, Client, Cfg.DefaultOrg,
			framework.CreateLLMProviderRequest{
				ID:       providerID,
				Name:     "E2E OpenAI Provider " + suffix,
				Version:  "v1.0",
				Context:  "/" + providerID,
				Template: "openai",
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
		providerUUID = prov.UUID
		Expect(providerUUID).NotTo(BeEmpty())
		GinkgoWriter.Printf("LLM provider created and deployed: %s (UUID: %s, gateway: %s)\n", providerID, providerUUID, gatewayUUID)
	})

	It("should create an external agent", func() {
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     createReq,
		})
		Expect(ag.Name).To(Equal(agentName))
		framework.ExpectJSONMatch(Default, "llm-provider-external-agent/expected_create_agent.json", ag)
		GinkgoWriter.Printf("External agent: %s\n", agentName)
	})

	It("should create a model config and extract proxy credentials", func() {
		By("Creating model config — this auto-creates the LLM proxy and returns credentials")
		config := configuration.CreateAgentModelConfig(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName,
			framework.CreateAgentModelConfigRequest{
				Name: "e2e-llm-config-" + suffix,
				Type: "llm",
				EnvMappings: map[string]framework.EnvModelConfigRequest{
					Cfg.DefaultEnv: {
						ProviderName: providerID,
					},
				},
			})
		Expect(config.UUID).NotTo(BeEmpty())
		modelConfigID = config.UUID

		By("Extracting proxy URL, API key, and auth header from response")
		envMapping, exists := config.EnvMappings[Cfg.DefaultEnv]
		Expect(exists).To(BeTrue(), "expected env mapping for %s", Cfg.DefaultEnv)
		Expect(envMapping.Configuration).NotTo(BeNil(), "expected configuration in env mapping")

		proxyUUID = envMapping.Configuration.ProxyUuid
		proxyURL = envMapping.Configuration.URL
		Expect(proxyUUID).NotTo(BeEmpty(), "proxy UUID should be set")
		Expect(proxyURL).NotTo(BeEmpty(), "proxy URL should be set")

		Expect(envMapping.Configuration.AuthInfo).NotTo(BeNil(), "expected authInfo with API key")
		Expect(envMapping.Configuration.AuthInfo.Value).NotTo(BeNil(), "expected API key value")
		proxyAPIKey = *envMapping.Configuration.AuthInfo.Value

		GinkgoWriter.Printf("Model config: %s\n  Proxy UUID: %s\n  URL: %s\n  Auth: %s in %s header %s\n",
			config.UUID, proxyUUID, proxyURL,
			envMapping.Configuration.AuthInfo.Type,
			envMapping.Configuration.AuthInfo.In,
			envMapping.Configuration.AuthInfo.Name)
	})

	It("should invoke the proxy endpoint successfully", func() {
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
			req.Header["api-key"] = []string{proxyAPIKey}

			resp, err := httpClient.Do(req)
			g.Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"proxy invocation returned %d: %s", resp.StatusCode, string(body))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	})

	It("should update model config with content-length guardrail", func() {
		Expect(modelConfigID).NotTo(BeEmpty(), "model config ID must be set")

		configuration.UpdateAgentModelConfig(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, modelConfigID,
			framework.UpdateAgentModelConfigRequest{
				EnvMappings: map[string]framework.EnvModelConfigRequest{
					Cfg.DefaultEnv: {
						ProviderName: providerID,
						Configuration: map[string]interface{}{
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
						},
					},
				},
			})
		GinkgoWriter.Printf("Model config updated with content-length-guardrail (maxBytes: 10)\n")
	})

	It("should block request exceeding content length guardrail", func() {
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
			req.Header["api-key"] = []string{proxyAPIKey}

			resp, err := httpClient.Do(req)
			g.Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			// Skip 404/200 — gateway still resyncing or guardrail not yet applied
			if resp.StatusCode == http.StatusNotFound {
				g.Expect(resp.StatusCode).NotTo(SatisfyAny(
					Equal(http.StatusNotFound), Equal(http.StatusOK)),
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
})
