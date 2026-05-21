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

// Validates past monitor lifecycle with a built-in rule-based evaluator
// (length_compliance): creation, run completion, log retrieval, score
// verification, and monitor rerun.

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

var _ = Describe("Past Monitor - Built-in Evaluator", Ordered, Label("monitors", "past-monitor"), func() {
	var (
		suffix                string
		traceStartTime        time.Time
		traceEndTime          time.Time
		builtinEvalIdentifier string
		pastMonitorName       string
		pastMonitorRunID      string
	)

	BeforeAll(func() {
		Expect(Shared).NotTo(BeNil(), "shared agent must be available")

		suffix = uuid.New().String()[:8]
		pastMonitorName = "e2e-test-mon-monitor-" + suffix

		By("Invoking shared agent to generate traces")
		traceStartTime = time.Now().Add(-10 * time.Minute)
		endpointURL := Shared.EndpointURL + "/chat"
		agentops.InvokeAgentEndpoint(endpointURL, Shared.InvokeReq, Shared.APIKey)
		traceEndTime = time.Now()
		GinkgoWriter.Printf("Invocation completed, trace window: %s to %s\n",
			traceStartTime.Format(time.RFC3339), traceEndTime.Format(time.RFC3339))

		By("Finding built-in length_compliance evaluator")
		evals := evaluator.ListEvaluators(Default, Client, Cfg.DefaultOrg)
		Expect(evals.Evaluators).NotTo(BeEmpty(), "expected at least one evaluator")
		for _, ev := range evals.Evaluators {
			if ev.Identifier == "length_compliance" {
				builtinEvalIdentifier = ev.Identifier
				break
			}
		}
		Expect(builtinEvalIdentifier).NotTo(BeEmpty(), "expected 'length_compliance' evaluator")
		GinkgoWriter.Printf("Using built-in evaluator: %s\n", builtinEvalIdentifier)
	})

	It("should create a past monitor with built-in evaluator", func() {
		samplingRate := 1.0
		mon := monitor.CreateMonitor(Default, Client, &monitor.CreateMonitorParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			Request: framework.CreateMonitorRequest{
				Name:            pastMonitorName,
				DisplayName:     "E2E Past Monitor",
				Description:     "Historical monitor for e2e test",
				EnvironmentName: Cfg.DefaultEnv,
				Type:            "past",
				SamplingRate:    &samplingRate,
				TraceStart:      &traceStartTime,
				TraceEnd:        &traceEndTime,
				Evaluators: []framework.MonitorEvaluator{
					{
						Identifier:  builtinEvalIdentifier,
						DisplayName: "Built-in Evaluator",
					},
				},
			},
		})
		Expect(mon.Name).To(Equal(pastMonitorName))
		GinkgoWriter.Printf("Past monitor created: %s\n", mon.Name)
	})

	It("should have a completed run for the past monitor", func() {
		run := monitor.WaitForMonitorRun(Client, &monitor.WaitForMonitorRunParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			MonitorName: pastMonitorName,
			Timeout:     10 * time.Minute,
		})
		Expect(run.Status).To(Equal("success"))
		pastMonitorRunID = run.ID
		GinkgoWriter.Printf("Past monitor run completed: %s\n", run.ID)
	})

	It("should have logs for the past monitor run", func() {
		runs := monitor.ListMonitorRuns(Default, Client, &monitor.ListMonitorRunsParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			MonitorName: pastMonitorName,
		})
		Expect(runs.Runs).NotTo(BeEmpty())

		var completedRunID string
		for _, r := range runs.Runs {
			if r.Status == "success" {
				completedRunID = r.ID
				break
			}
		}
		Expect(completedRunID).NotTo(BeEmpty(), "expected a completed run")

		logs := monitor.GetMonitorRunLogs(Default, Client, &monitor.GetMonitorRunLogsParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			MonitorName: pastMonitorName,
			RunID:       completedRunID,
		})
		Expect(logs.Logs).NotTo(BeEmpty(), "expected logs for the monitor run")
		GinkgoWriter.Printf("Past monitor run logs: %d entries\n", len(logs.Logs))
	})

	It("should have scores for the past monitor run", func() {
		runs := monitor.ListMonitorRuns(Default, Client, &monitor.ListMonitorRunsParams{
			OrgName:       Cfg.DefaultOrg,
			ProjectName:   Shared.ProjectName,
			AgentName:     Shared.AgentName,
			MonitorName:   pastMonitorName,
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
		Expect(completedRun.ID).NotTo(BeEmpty(), "expected a completed run")
		Expect(completedRun.Scores).NotTo(BeEmpty(), "expected scores in completed run")
		GinkgoWriter.Printf("Past monitor scores: %d evaluator summaries\n", len(completedRun.Scores))
	})

	It("should rerun the past monitor and succeed", func() {
		Expect(pastMonitorRunID).NotTo(BeEmpty(), "past monitor run ID not captured")

		rerun := monitor.RerunMonitor(Default, Client, &monitor.RerunMonitorParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			MonitorName: pastMonitorName,
			RunID:       pastMonitorRunID,
		})
		GinkgoWriter.Printf("Rerun triggered: %s (status: %s)\n", rerun.ID, rerun.Status)

		runs := monitor.WaitForMonitorRunCount(Client, &monitor.WaitForMonitorRunParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			MonitorName: pastMonitorName,
			Timeout:     10 * time.Minute,
		}, 2)
		Expect(len(runs)).To(BeNumerically(">=", 2), "expected at least 2 successful runs after rerun")
		GinkgoWriter.Printf("Past monitor now has %d successful runs\n", len(runs))
	})
})
