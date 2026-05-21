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

// Validates past monitor with an LLM-judge evaluator (accuracy): LLM provider
// setup, monitor creation with provider config, run completion, and scores.

package monitors

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	"github.com/wso2/agent-manager/test/e2e/operations/llmprovider"
	"github.com/wso2/agent-manager/test/e2e/operations/monitor"
)

var _ = Describe("Past Monitor - LLM Judge", Ordered, Label("monitors", "llm-judge"), func() {
	var (
		suffix              string
		traceStartTime      time.Time
		traceEndTime        time.Time
		llmJudgeMonitorName string
		llmProviderID       string
	)

	BeforeAll(func() {
		Expect(Shared).NotTo(BeNil(), "shared agent must be available")
		Expect(Cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set")

		suffix = uuid.New().String()[:8]
		llmJudgeMonitorName = "e2e-test-mon-monitor-" + suffix
		llmProviderID = "e2e-test-mon-provider-" + suffix

		By("Invoking shared agent to generate traces")
		traceStartTime = time.Now().Add(-10 * time.Minute)
		endpointURL := Shared.EndpointURL + "/chat"
		agentops.InvokeAgentEndpoint(endpointURL, Shared.InvokeReq, Shared.APIKey)
		traceEndTime = time.Now()
		GinkgoWriter.Printf("Invocation completed, trace window: %s to %s\n",
			traceStartTime.Format(time.RFC3339), traceEndTime.Format(time.RFC3339))
	})

	It("should create an LLM provider for LLM-judge evaluator", func() {
		By("Waiting for an active AI gateway")
		gatewayUUID := gateway.WaitForActiveAIGateway(Client, Cfg.DefaultOrg, "api-platform-default-default", 3*time.Minute)

		By("Fetching the OpenAI template")
		templates := llmprovider.ListLLMProviderTemplates(Default, Client, Cfg.DefaultOrg)
		var openaiTpl *framework.LLMProviderTemplateResponse
		for i, t := range templates.Templates {
			if t.ID == "openai" {
				openaiTpl = &templates.Templates[i]
				break
			}
		}
		Expect(openaiTpl).NotTo(BeNil(), "expected 'openai' template")

		By("Creating and deploying the LLM provider")
		upstreamURL := openaiTpl.Metadata.EndpointURL
		authHeader := openaiTpl.Metadata.Auth.Header
		authValue := openaiTpl.Metadata.Auth.ValuePrefix + Cfg.OpenAIAPIKey

		prov := llmprovider.CreateLLMProvider(Default, Client, Cfg.DefaultOrg,
			framework.CreateLLMProviderRequest{
				ID:       llmProviderID,
				Name:     "E2E LLM Judge Provider " + suffix,
				Version:  "v1.0",
				Context:  "/" + llmProviderID,
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

		GinkgoWriter.Printf("LLM provider created: %s (UUID: %s)\n", llmProviderID, prov.UUID)
	})

	It("should create a past monitor with LLM-judge evaluator", func() {
		samplingRate := 1.0
		mon := monitor.CreateMonitor(Default, Client, &monitor.CreateMonitorParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			Request: framework.CreateMonitorRequest{
				Name:            llmJudgeMonitorName,
				DisplayName:     "E2E LLM Judge Monitor",
				Description:     "Past monitor with LLM-judge evaluator for e2e test",
				EnvironmentName: Cfg.DefaultEnv,
				Type:            "past",
				SamplingRate:    &samplingRate,
				TraceStart:      &traceStartTime,
				TraceEnd:        &traceEndTime,
				LLMProvider:     &framework.MonitorLLMProviderRef{ProviderName: llmProviderID},
				Evaluators: []framework.MonitorEvaluator{
					{
						Identifier:  "accuracy",
						DisplayName: "Accuracy",
						Config: map[string]any{
							"model":       "gpt-4o-mini",
							"temperature": 0,
							"max_tokens":  1024,
							"max_retries": 2,
						},
					},
				},
			},
		})
		Expect(mon.Name).To(Equal(llmJudgeMonitorName))
		GinkgoWriter.Printf("LLM-judge monitor created: %s\n", mon.Name)
	})

	It("should have a completed run with scores", func() {
		run := monitor.WaitForMonitorRun(Client, &monitor.WaitForMonitorRunParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			MonitorName: llmJudgeMonitorName,
			Timeout:     10 * time.Minute,
		})
		Expect(run.Status).To(Equal("success"))
		Expect(run.Scores).NotTo(BeEmpty(), "expected scores from LLM-judge evaluator")
		GinkgoWriter.Printf("LLM-judge monitor run completed: %s, scores: %d\n", run.ID, len(run.Scores))
	})
})
