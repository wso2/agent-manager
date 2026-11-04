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

// Validates LLM provider usage across a two-environment promotion. It provisions
// its own agent in the two-env infra project (rather than reusing a shared agent)
// and its own OpenAI-backed provider on both environment gateways, creates the agent
// with USE_LLM_PROVIDER=true and a model config for the default env, invokes through
// the provider, extends the model config to the second environment, promotes, and
// verifies traffic there.

package llmprovider

import (
	"encoding/json"
	"fmt"
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
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

var llmProviderEnvVars = []framework.EnvironmentVariableConfig{
	{Key: "url", Name: "LLM_PROVIDER_URL"},
	{Key: "apikey", Name: "LLM_PROVIDER_KEY"},
}

var _ = Describe("LLM provider configured across environments:", Label("llm-provider", "two-env"), Ordered, func() {
	var (
		suffix      string
		agentName   string
		providerID  string
		projectName string
		secondEnv   string

		gatewayUUIDEnv1 string
		gatewayUUIDEnv2 string

		createReq framework.CreateAgentRequest
		invokeReq json.RawMessage
	)

	BeforeAll(func(ctx SpecContext) {
		Expect(Cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set")

		suffix = uuid.New().String()[:8]
		agentName = framework.E2EAgentPrefix + suffix
		providerID = framework.E2ELLMProviderPrefix + suffix

		By("Provisioning two-env infrastructure (environment, pipeline, project)")
		projectName, secondEnv = testsetup.EnsurePromotableInfra(ctx, Client, Cfg)
		GinkgoWriter.Printf("Two-env infra ready: project=%s secondEnv=%s\n", projectName, secondEnv)

		createReq = framework.NewITHelpdeskAgentRequest(
			agentName,
			"IT helpdesk agent for e2e LLM provider promotion test",
			map[string]string{"USE_LLM_PROVIDER": "true"},
		)
		// Wired at creation so the first deploy has LLM_PROVIDER_URL; the agent exits on startup without it.
		createReq.ModelConfig = []framework.ModelConfigRequest{
			{
				ProviderName:         providerID,
				EnvironmentVariables: llmProviderEnvVars,
			},
		}
		invokeReq = framework.DefaultInvokeRequest()
	})

	// This agent is single-use; delete it so its workload pods (default + second
	// environment) are freed.
	AfterAll(func() {
		agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, projectName, agentName)
	})

	It("finds active AI gateways in both environments", func() {
		gatewayUUIDEnv1 = gateway.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		gatewayUUIDEnv2 = gateway.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, secondEnv, 3*time.Minute)
		Expect(gatewayUUIDEnv1).NotTo(BeEmpty())
		Expect(gatewayUUIDEnv2).NotTo(BeEmpty())
	})

	It("creates an LLM provider deployed on both environment gateways", func() {
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
		Expect(openaiTpl.Metadata.Auth).NotTo(BeNil(), "expected template auth config")

		upstreamURL := openaiTpl.Metadata.EndpointURL
		authHeader := openaiTpl.Metadata.Auth.Header
		authValue := openaiTpl.Metadata.Auth.ValuePrefix + Cfg.OpenAIAPIKey

		prov := llmproviderop.CreateLLMProvider(Default, Client, Cfg.DefaultOrg,
			framework.CreateLLMProviderRequest{
				ID:       providerID,
				Name:     "E2E Promotion OpenAI Provider " + suffix,
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
				// Deploy on both gateways so the provider is reachable in both
				// environments after promotion.
				Gateways: []string{gatewayUUIDEnv1, gatewayUUIDEnv2},
			})
		Expect(prov.UUID).NotTo(BeEmpty())
		GinkgoWriter.Printf("LLM provider on both gateways: %s (UUID: %s)\n", providerID, prov.UUID)
	})

	It("creates a IT-helpdesk agent to promote across environments", func() {
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			Request:     createReq,
		})
		Expect(ag.Name).To(Equal(agentName))
		GinkgoWriter.Printf("Agent: %s (project=%s)\n", agentName, projectName)
	})

	It("builds the agent image to completion", func() {
		build.WaitForBuildSuccess(Client, &build.WaitForBuildParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			AgentName:   agentName,
			Timeout:     20 * time.Minute,
		})
	})

	It("deploys the agent to the default environment", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			Timeout:     5 * time.Minute,
		})
	})

	It("becomes ready in the default environment", func() {
		deployment.WaitForReadiness(Client, Cfg.DefaultOrg, projectName, agentName, Cfg.DefaultEnv, 10*time.Minute)
	})

	It("returns a chat response via the LLM provider in the default environment", func() {
		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, projectName, agentName, Cfg.DefaultEnv)
		endpointURL := deployment.FirstEndpointURL(endpoints)
		Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL should not be empty")

		apiKeyResp := agentops.CreateAgentAPIKey(Default, Client,
			Cfg.DefaultOrg, projectName, agentName, Cfg.DefaultEnv,
			framework.CreateAgentAPIKeyRequest{
				DisplayName: "e2e-llm-2env-default-key",
				ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		Expect(apiKeyResp.ApiKey).NotTo(BeEmpty())

		GinkgoWriter.Printf("Invoking via LLM provider in default env: %s\n", endpointURL)
		agentops.InvokeAgentEndpoint(fmt.Sprintf("%s/chat", endpointURL), invokeReq, apiKeyResp.ApiKey)
	})

	It("extends the model config to the second environment", func() {
		configs := configuration.ListAgentModelConfigs(Default, Client,
			Cfg.DefaultOrg, projectName, agentName)
		var modelConfigID string
		for _, c := range configs.Configs {
			if c.Type == "llm" {
				modelConfigID = c.UUID
				break
			}
		}
		Expect(modelConfigID).NotTo(BeEmpty(), "expected the LLM model config created with the agent")

		configuration.UpdateAgentModelConfig(Default, Client,
			Cfg.DefaultOrg, projectName, agentName, modelConfigID,
			framework.UpdateAgentModelConfigRequest{
				EnvMappings: map[string]framework.EnvModelConfigRequest{
					Cfg.DefaultEnv: {ProviderName: providerID},
					secondEnv:      {ProviderName: providerID},
				},
				EnvironmentVariables: llmProviderEnvVars,
			})
		GinkgoWriter.Printf("Model config %s now covers %s and %s\n", modelConfigID, Cfg.DefaultEnv, secondEnv)
	})

	It("promotes the agent with the LLM provider to the second environment", func() {
		useSource := false
		resp := agentops.PromoteAgent(Default, Client, Cfg.DefaultOrg, projectName, agentName,
			framework.PromoteAgentRequest{
				SourceEnvironment:      Cfg.DefaultEnv,
				TargetEnvironment:      secondEnv,
				UseConfigFromSourceEnv: &useSource,
				Env: []framework.EnvironmentVariable{
					{Key: "USE_LLM_PROVIDER", Value: "true"},
				},
			})
		Expect(resp.TargetEnvironment).To(Equal(secondEnv))
		GinkgoWriter.Printf("Agent promoted with LLM provider: %s -> %s\n", Cfg.DefaultEnv, secondEnv)
	})

	It("deploys the promoted agent with the LLM provider in the second environment", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			AgentName:   agentName,
			Environment: secondEnv,
			Timeout:     5 * time.Minute,
		})
		deployment.WaitForReadiness(Client, Cfg.DefaultOrg, projectName, agentName, secondEnv, 10*time.Minute)
		GinkgoWriter.Printf("Agent deployed with LLM provider in %s\n", secondEnv)
	})

	It("returns a chat response via the LLM provider in the second environment", func() {

		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, projectName, agentName, secondEnv)
		endpointURL := deployment.FirstEndpointURL(endpoints)
		Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL in second env should not be empty")

		apiKeyResp := agentops.CreateAgentAPIKey(Default, Client,
			Cfg.DefaultOrg, projectName, agentName, secondEnv,
			framework.CreateAgentAPIKeyRequest{
				DisplayName: "e2e-llm-2env-promo-key",
				ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		Expect(apiKeyResp.ApiKey).NotTo(BeEmpty())

		GinkgoWriter.Printf("Invoking via LLM provider in %s: %s\n", secondEnv, endpointURL)
		agentops.InvokeAgentEndpoint(fmt.Sprintf("%s/chat", endpointURL), invokeReq, apiKeyResp.ApiKey)
	})
})
