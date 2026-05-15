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

// Validates past monitor with a custom Python code evaluator: evaluator
// creation, monitor creation, run completion, and score verification.

package monitors

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/evaluator"
	"github.com/wso2/agent-manager/test/e2e/operations/monitor"
)

var _ = Describe("Past Monitor - Custom Evaluator", Ordered, Label("monitors", "custom-evaluator"), func() {
	var (
		suffix                string
		traceStartTime        time.Time
		traceEndTime          time.Time
		customEvalIdentifier  string
		customPastMonitorName string
	)

	BeforeAll(func() {
		Expect(Shared).NotTo(BeNil(), "shared agent must be available")

		suffix = uuid.New().String()[:8]
		customEvalIdentifier = "e2e-test-mon-evaluator-" + suffix
		customPastMonitorName = "e2e-test-mon-monitor-" + suffix

		By("Invoking shared agent to generate traces")
		traceStartTime = time.Now().Add(-10 * time.Minute)
		endpointURL := Shared.EndpointURL + "/chat"
		agentops.InvokeAgentEndpoint(endpointURL, Shared.InvokeReq, Shared.APIKey)
		traceEndTime = time.Now()
		GinkgoWriter.Printf("Invocation completed, trace window: %s to %s\n",
			traceStartTime.Format(time.RFC3339), traceEndTime.Format(time.RFC3339))
	})

	It("should create a custom evaluator", func() {
		customEval := evaluator.CreateCustomEvaluator(Default, Client, &evaluator.CreateCustomEvaluatorParams{
			OrgName:     Cfg.DefaultOrg,
			Identifier:  customEvalIdentifier,
			DisplayName: "E2E Custom Evaluator",
			Description: "Custom code evaluator for e2e test",
			Type:        "code",
			Level:       "trace",
			Source: `from amp_evaluation import EvalResult
from amp_evaluation.trace.models import Trace

def evaluate(trace: Trace) -> EvalResult:
    return EvalResult(
        score=1.0,
        passed=True,
        explanation="e2e test evaluator always passes",
    )
`,
		})
		Expect(customEval.Identifier).To(Equal(customEvalIdentifier))
		GinkgoWriter.Printf("Custom evaluator created: %s\n", customEval.Identifier)
	})

	It("should create a past monitor with custom evaluator", func() {
		samplingRate := 1.0
		mon := monitor.CreateMonitor(Default, Client, &monitor.CreateMonitorParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			Request: framework.CreateMonitorRequest{
				Name:            customPastMonitorName,
				DisplayName:     "E2E Custom Past Monitor",
				Description:     "Historical monitor with custom evaluator for e2e test",
				EnvironmentName: Cfg.DefaultEnv,
				Type:            "past",
				SamplingRate:    &samplingRate,
				TraceStart:      &traceStartTime,
				TraceEnd:        &traceEndTime,
				Evaluators: []framework.MonitorEvaluator{
					{
						Identifier:  customEvalIdentifier,
						DisplayName: "E2E Custom Evaluator",
					},
				},
			},
		})
		Expect(mon.Name).To(Equal(customPastMonitorName))
		GinkgoWriter.Printf("Custom past monitor created: %s\n", mon.Name)
	})

	It("should have a completed run", func() {
		run := monitor.WaitForMonitorRun(Client, &monitor.WaitForMonitorRunParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			MonitorName: customPastMonitorName,
			Timeout:     10 * time.Minute,
		})
		Expect(run.Status).To(Equal("success"))
		GinkgoWriter.Printf("Custom past monitor run completed: %s\n", run.ID)
	})

	It("should have scores from the custom evaluator", func() {
		runs := monitor.ListMonitorRuns(Default, Client, &monitor.ListMonitorRunsParams{
			OrgName:       Cfg.DefaultOrg,
			ProjectName:   Shared.ProjectName,
			AgentName:     Shared.AgentName,
			MonitorName:   customPastMonitorName,
			IncludeScores: true,
		})
		Expect(runs.Runs).NotTo(BeEmpty())

		var completedRun framework.MonitorRunResponse
		for _, r := range runs.Runs {
			if r.Status == "success" {
				completedRun = r
				break
			}
		}
		Expect(completedRun.ID).NotTo(BeEmpty())
		Expect(completedRun.Scores).NotTo(BeEmpty(), "expected scores from custom evaluator")
		GinkgoWriter.Printf("Custom evaluator scores: %d summaries\n", len(completedRun.Scores))
	})
})
