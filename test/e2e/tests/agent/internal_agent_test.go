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

// Validates the full lifecycle of an internal chat agent: creation, build,
// deployment, invocation, metrics collection, and trace generation.

package agent

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	traceops "github.com/wso2/agent-manager/test/e2e/operations/trace"
)

var _ = Describe("Internal Chat Agent Lifecycle", Label("agent", "internal-agent"), Ordered, func() {
	var (
		agentName   string
		endpointURL string
		apiKey      string
		invokeReq   json.RawMessage
	)

	BeforeAll(func() {
		Expect(Cfg.TavilyAPIKey).NotTo(BeEmpty(), "TAVILY_API_KEY must be set")
		Expect(Cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set")

		suffix := uuid.New().String()[:8]
		agentName = "e2e-test-agent-" + suffix
	})

	It("should create an internal chat agent", func() {
		envVars := map[string]string{
			"TAVILY_API_KEY": Cfg.TavilyAPIKey,
			"OPENAI_API_KEY": Cfg.OpenAIAPIKey,
			"DATABASE_URL":   "http://localhost:5000",
		}

		createReq := framework.NewInternalChatAgentRequest(agentName, "Internal chat agent for e2e agent lifecycle test", envVars)

		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     createReq,
		})
		Expect(ag.Name).To(Equal(agentName))
		GinkgoWriter.Printf("Agent created: %s\n", agentName)
	})

	It("should complete the build", func() {
		buildName := build.WaitForBuildSuccess(Client, &build.WaitForBuildParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Timeout:     20 * time.Minute,
		})
		GinkgoWriter.Printf("Build completed: %s\n", buildName)
	})

	It("should deploy successfully", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			Timeout:     5 * time.Minute,
		})
		GinkgoWriter.Printf("Agent deployed: %s\n", agentName)
	})

	It("should become ready", func() {
		agentops.WaitForRuntimeLog(Client, &agentops.WaitForRuntimeLogParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			SearchText:  "Uvicorn running on",
			Timeout:     10 * time.Minute,
		})

		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv)
		for _, ep := range endpoints {
			if ep.URL != "" {
				endpointURL = ep.URL
				break
			}
		}
		Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL should not be empty")

		apiKeyResp := agentops.CreateAgentAPIKey(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv,
			framework.CreateAgentAPIKeyRequest{
				DisplayName: "e2e-test-key",
				ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		apiKey = apiKeyResp.ApiKey
		Expect(apiKey).NotTo(BeEmpty(), "agent API key should not be empty")

		invokeReq = framework.DefaultInvokeRequest()
		GinkgoWriter.Printf("Agent ready: endpoint=%s\n", endpointURL)
	})

	It("should respond to invocation", func() {
		endpoint := endpointURL + "/chat"
		GinkgoWriter.Printf("Endpoint: %s\n", endpoint)
		agentops.InvokeAgentEndpoint(endpoint, invokeReq, apiKey)
	})

	It("should have metrics available", func() {
		metrics := agentops.GetMetrics(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv)
		Expect(metrics.CPUUsage).NotTo(BeEmpty(), "expected CPU usage metrics")
		Expect(metrics.Memory).NotTo(BeEmpty(), "expected memory metrics")
		GinkgoWriter.Printf("CPU points: %d, Memory points: %d\n", len(metrics.CPUUsage), len(metrics.Memory))
	})

	It("should have traces available", func() {
		traces := traceops.WaitForTraces(Client, &traceops.WaitForTracesParams{
			Organization: Cfg.DefaultOrg,
			Project:      framework.E2ESharedProjectName,
			Agent:        agentName,
			Environment:  Cfg.DefaultEnv,
			Timeout:      2 * time.Minute,
		})
		Expect(traces.Traces).NotTo(BeEmpty(), "expected at least one trace after agent invocation")
		GinkgoWriter.Printf("Traces: %d found\n", len(traces.Traces))
	})
})
