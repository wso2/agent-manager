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

package agent

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
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/configuration"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
)

var _ = Describe("Agent Configuration Lifecycle", Label("agent", "config-lifecycle"), Ordered, func() {
	var (
		agentName          string
		endpointURL        string
		invokeReq          json.RawMessage
		sensitiveSecretRef string
		lastDeployedBefore time.Time
	)

	BeforeAll(func() {
		suffix := uuid.New().String()[:8]
		agentName = "e2e-config-" + suffix

		envVars := map[string]string{
			"TAVILY_API_KEY": Cfg.TavilyAPIKey,
			"OPENAI_API_KEY": Cfg.OpenAIAPIKey,
			"DATABASE_URL":   "http://localhost:5000",
		}

		Expect(Cfg.TavilyAPIKey).NotTo(BeEmpty(), "TAVILY_API_KEY must be set")
		Expect(Cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set")

		By("Creating agent for config lifecycle tests")
		var createReq framework.CreateAgentRequest
		framework.LoadTestData(TestDataDir, "internal-chat-agent/create_agent.json", &createReq)
		createReq.Name = agentName
		framework.InjectEnvVars(createReq.Configurations, envVars)

		agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     createReq,
		})
		GinkgoWriter.Printf("Config lifecycle agent created: %s in project %s\n", agentName, framework.E2ESharedProjectName)

		By("Waiting for build to complete")
		build.WaitForBuildSuccess(Client, &build.WaitForBuildParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Timeout:     20 * time.Minute,
		})

		By("Waiting for deployment")
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			Timeout:     5 * time.Minute,
		})

		By("Waiting for agent readiness")
		agentops.WaitForRuntimeLog(Client, &agentops.WaitForRuntimeLogParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			SearchText:  "Uvicorn running on",
			Timeout:     10 * time.Minute,
		})

		By("Getting agent endpoint")
		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv)
		for _, ep := range endpoints {
			if ep.URL != "" {
				endpointURL = ep.URL
				break
			}
		}
		Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL should not be empty")

		framework.LoadTestData(TestDataDir, "internal-chat-agent/invoke_request.json", &invokeReq)
		GinkgoWriter.Printf("Config lifecycle agent ready: %s endpoint=%s\n", agentName, endpointURL)
	})

	It("should have correct initial configurations", func() {
		By("Verifying initial configurations")
		configs := configuration.GetAgentConfigurations(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv)

		for _, c := range configs.Configurations {
			if c.Key == "DATABASE_URL" {
				Expect(c.Value).To(Equal("http://localhost:5000"), "DATABASE_URL should have initial value")
				Expect(c.IsSensitive).To(BeFalse(), "DATABASE_URL should not be sensitive")
			}
			if c.Key == "OPENAI_API_KEY" {
				Expect(c.IsSensitive).To(BeTrue(), "OPENAI_API_KEY should be sensitive")
				Expect(c.Value).To(BeEmpty(), "sensitive config value should be masked")
				Expect(c.SecretRef).NotTo(BeEmpty(), "OPENAI_API_KEY should have a secretRef")
			}
			if c.Key == "TAVILY_API_KEY" {
				Expect(c.IsSensitive).To(BeTrue(), "TAVILY_API_KEY should be sensitive")
				Expect(c.Value).To(BeEmpty(), "sensitive config value should be masked")
				Expect(c.SecretRef).NotTo(BeEmpty(), "TAVILY_API_KEY should have a secretRef")
				sensitiveSecretRef = c.SecretRef
			}
		}
		GinkgoWriter.Printf("Configurations verified: %d items, secretRef=%s\n",
			len(configs.Configurations), sensitiveSecretRef)
	})

	It("should respond to invocation", func() {
		chatEndpoint := endpointURL + "/chat"
		GinkgoWriter.Printf("Endpoint: %s\n", chatEndpoint)
		agentops.InvokeAgentEndpoint(chatEndpoint, invokeReq)
	})

	It("should redeploy with updated configurations", func() {
		By("Getting current deployment to capture imageId and lastDeployed")
		deps := deployment.GetDeploymentDetails(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName)
		dep, exists := deps[Cfg.DefaultEnv]
		Expect(exists).To(BeTrue(), "deployment should exist for default environment")
		imageID := dep.ImageID
		Expect(imageID).NotTo(BeEmpty(), "imageId should not be empty")
		lastDeployedBefore = dep.LastDeployed

		By("Redeploying with updated environment variables")
		autoInstr := true
		deployment.DeployAgent(Default, Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName,
			framework.DeployAgentRequest{
				ImageID: imageID,
				Env: []framework.EnvironmentVariable{
					{Key: "TAVILY_API_KEY", IsSensitive: true, SecretRef: sensitiveSecretRef, Value: ""},
					{Key: "OPENAI_API_KEY", Value: "sk-invalid-key-for-e2e-test", IsSensitive: true},
					{Key: "DATABASE_URL", Value: "http://localhost:6000", IsSensitive: false},
				},
				EnableAutoInstrumentation: &autoInstr,
			})
	})

	It("should become active after redeployment", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:       Cfg.DefaultOrg,
			ProjectName:   framework.E2ESharedProjectName,
			AgentName:     agentName,
			Environment:   Cfg.DefaultEnv,
			Timeout:       5 * time.Minute,
			DeployedAfter: lastDeployedBefore,
		})
	})

	It("should become ready after redeployment", func() {
		agentops.WaitForRuntimeLog(Client, &agentops.WaitForRuntimeLogParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			SearchText:  "Uvicorn running on",
			Timeout:     10 * time.Minute,
		})
	})

	It("should have updated non-secret configurations", func() {
		configs := configuration.GetAgentConfigurations(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv)

		for _, c := range configs.Configurations {
			if c.Key == "DATABASE_URL" {
				Expect(c.Value).To(Equal("http://localhost:6000"),
					"DATABASE_URL should have the updated value")
			}
		}
	})

	It("should fail invocation with invalid API key", func() {
		chatEndpoint := endpointURL + "/chat"

		data, err := json.Marshal(invokeReq)
		Expect(err).NotTo(HaveOccurred())

		httpClient := &http.Client{Timeout: 60 * time.Second}
		Eventually(func(g Gomega) {
			resp, err := httpClient.Post(chatEndpoint, "application/json", bytes.NewBuffer(data))
			g.Expect(err).NotTo(HaveOccurred(), "agent endpoint not reachable")
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusOK {
				g.Expect(string(body)).To(
					SatisfyAny(
						ContainSubstring("error"),
						ContainSubstring("Error"),
						ContainSubstring("invalid"),
						ContainSubstring("AuthenticationError"),
					),
					"expected error indicator in response body")
			} else {
				GinkgoWriter.Printf("Invocation failed with status %d as expected\n", resp.StatusCode)
			}
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	})

	It("should have error in runtime logs", func() {
		agentops.WaitForRuntimeLog(Client, &agentops.WaitForRuntimeLogParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			SearchText:  "Incorrect API key provided",
			Timeout:     5 * time.Minute,
		})
	})
})
