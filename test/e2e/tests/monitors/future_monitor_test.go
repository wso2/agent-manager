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

// Validates future monitor lifecycle: creation before agent invocation,
// automatic run triggered by new traces, score verification, and deletion.

package monitors

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/evaluator"
	"github.com/wso2/agent-manager/test/e2e/operations/monitor"
)

var _ = Describe("Future Monitor", Ordered, Label("monitors", "future-monitor"), func() {
	var (
		suffix                string
		builtinEvalIdentifier string
		futureMonitorName     string
	)

	BeforeAll(func() {
		Expect(Shared).NotTo(BeNil(), "shared agent must be available")

		suffix = uuid.New().String()[:8]
		futureMonitorName = "e2e-test-mon-monitor-" + suffix

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

	It("should create a future monitor", func() {
		samplingRate := 1.0
		mon := monitor.CreateMonitor(Default, Client, &monitor.CreateMonitorParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			Request: framework.CreateMonitorRequest{
				Name:            futureMonitorName,
				DisplayName:     "E2E Future Monitor",
				Description:     "Future monitor for e2e test",
				EnvironmentName: Cfg.DefaultEnv,
				Type:            "future",
				IntervalMinutes: 5,
				SamplingRate:    &samplingRate,
				Evaluators: []framework.MonitorEvaluator{
					{
						Identifier:  builtinEvalIdentifier,
						DisplayName: "Built-in Evaluator",
					},
				},
			},
		})
		Expect(mon.Name).To(Equal(futureMonitorName))
		GinkgoWriter.Printf("Future monitor created: %s (status: %s)\n", mon.Name, mon.Status)
	})

	It("should invoke agent to generate traces", func() {
		endpointURL := Shared.EndpointURL + "/chat"
		agentops.InvokeAgentEndpoint(endpointURL, Shared.InvokeReq, Shared.APIKey)
		GinkgoWriter.Println("Agent invoked to generate traces for future monitor")
	})

	It("should have a completed future monitor run", func() {
		By(fmt.Sprintf("Waiting for future monitor %q to complete a run", futureMonitorName))
		run := monitor.WaitForMonitorRun(Client, &monitor.WaitForMonitorRunParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: Shared.ProjectName,
			AgentName:   Shared.AgentName,
			MonitorName: futureMonitorName,
			Timeout:     8 * time.Minute,
		})
		Expect(run.Status).To(Equal("success"))
		Expect(run.Scores).NotTo(BeEmpty(), "expected scores from future monitor run")
		GinkgoWriter.Printf("Future monitor run completed: %s, scores: %d\n", run.ID, len(run.Scores))
	})

	It("should delete the future monitor", func() {
		monitor.DeleteMonitor(Default, Client, Cfg.DefaultOrg, Shared.ProjectName, Shared.AgentName, futureMonitorName)
		GinkgoWriter.Printf("Future monitor deleted: %s\n", futureMonitorName)
	})
})
