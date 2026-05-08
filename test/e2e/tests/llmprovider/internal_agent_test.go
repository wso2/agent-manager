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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/configuration"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	llmproviderop "github.com/wso2/agent-manager/test/e2e/operations/llmprovider"
)

var _ = Describe("Internal Agent with LLM Provider Config", Label("llm-provider", "internal-agent"), Ordered, func() {
	var (
		agentName string
		suffix    string
		envVars   map[string]string

		providerID  string
		gatewayUUID string

		createReq framework.CreateAgentRequest
	)

	BeforeAll(func() {
		suffix = uuid.New().String()[:8]
		agentName = "e2e-llm-int-" + suffix
		providerID = "e2e-int-openai-" + suffix

		envVars = map[string]string{
			"TAVILY_API_KEY": Cfg.TavilyAPIKey,
			"OPENAI_API_KEY": Cfg.OpenAIAPIKey,
			"DATABASE_URL":   "http://localhost:5000",
		}

		framework.LoadTestData(TestDataDir, "internal-agent-llm-provider/create_agent.json", &createReq)
		createReq.Name = agentName
		framework.InjectEnvVars(createReq.Configurations, envVars)
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
				Name:     "E2E Internal OpenAI Provider " + suffix,
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
				Gateways: []string{gatewayUUID},
			})
		Expect(prov.UUID).NotTo(BeEmpty())
		GinkgoWriter.Printf("LLM provider: %s (UUID: %s)\n", providerID, prov.UUID)
	})

	It("should create an internal agent with model config", func() {
		// Add model config referencing the LLM provider with custom env var names
		createReq.ModelConfig = []framework.ModelConfigRequest{
			{
				EnvMappings: map[string]framework.EnvModelConfigRequest{
					Cfg.DefaultEnv: {
						ProviderName: providerID,
					},
				},
				EnvironmentVariables: []framework.EnvironmentVariableConfig{
					{Key: "apikey", Name: "LLM_API_KEY"},
					{Key: "url", Name: "LLM_BASE_URL"},
				},
			},
		}

		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     createReq,
		})
		Expect(ag.Name).To(Equal(agentName))
		GinkgoWriter.Printf("Agent: %s (with modelConfig)\n", agentName)
	})

	It("should complete the build", func() {
		build.WaitForBuildSuccess(Client, &build.WaitForBuildParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Timeout:     20 * time.Minute,
		})
	})

	It("should deploy the agent", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			Timeout:     5 * time.Minute,
		})
	})

	It("should have LLM provider env vars in configurations", func() {
		By("Verifying configurations include LLM provider variables")
		configs := configuration.GetAgentConfigurations(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv)

		GinkgoWriter.Printf("Agent configurations (%d items):\n", len(configs.Configurations))
		for _, c := range configs.Configurations {
			GinkgoWriter.Printf("  %s (sensitive: %v)\n", c.Key, c.IsSensitive)
		}

		// Verify the model config env vars are injected
		configKeys := make(map[string]bool)
		for _, c := range configs.Configurations {
			configKeys[c.Key] = true
		}

		// The LLM provider env vars (LLM_API_KEY, LLM_BASE_URL) should be present
		Expect(configKeys).To(HaveKey("LLM_API_KEY"),
			"LLM_API_KEY from model config should be in configurations")
		Expect(configKeys).To(HaveKey("LLM_BASE_URL"),
			"LLM_BASE_URL from model config should be in configurations")
	})
})
