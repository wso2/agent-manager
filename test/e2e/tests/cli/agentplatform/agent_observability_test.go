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

package cliagentplatformtests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	cliagent "github.com/wso2/agent-manager/test/e2e/operations/cli/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
)

var _ = Describe("amctl agent (CLI-owned lifecycle)", Label("cli", "agent"), Ordered, Serial, func() {
	// The CLI-owned agent is provisioned once by the suite's
	// SynchronizedBeforeSuite (see suite_test.go), so cfg/apiClient/owned are
	// ready before these specs run — no per-suite BeforeAll setup needed.

	// Deploy/mutation commands. These run first (Ordered) so the observability
	// specs below assert against a freshly redeployed, ready instance.
	Context("deploy", func() {
		It("redeploys the agent via amctl agent deploy", func() {
			res := cliagent.DeployAgentEventually(H, H.Org(), owned.ProjectName, owned.AgentName, 3*time.Minute)
			Expect(res.Agent).To(Equal(owned.AgentName))
			Expect(res.Build).NotTo(BeEmpty())
			Expect(res.ImageId).NotTo(BeEmpty())
			Expect(res.TargetEnvironment).To(Equal(cfg.DefaultEnv))
		})

		It("becomes ready again after redeploy", func() {
			deployment.WaitForReadiness(apiClient, cfg.DefaultOrg, owned.ProjectName, owned.AgentName, cfg.DefaultEnv, 10*time.Minute)
		})
	})

	// Observability commands. The invoke generates the telemetry that the
	// metrics/logs/traces specs then read back through the CLI.
	Context("observability", func() {
		It("invokes the agent to generate a fresh trace", func() {
			_ = agentops.InvokeAgentEndpoint(owned.EndpointURL+"/chat", owned.InvokeReq, owned.APIKey)
		})

		It("exposes CPU metrics via amctl agent metrics", func() {
			m := cliagent.AgentMetrics(Default, H, H.Org(), owned.ProjectName, owned.AgentName, cfg.DefaultEnv)
			Expect(m.CPUUsage).NotTo(BeEmpty(), "expected CPU usage metrics")
		})

		It("exposes runtime logs via amctl agent logs", func() {
			Eventually(func(g Gomega) {
				l := cliagent.AgentLogs(g, H, H.Org(), owned.ProjectName, owned.AgentName, cfg.DefaultEnv)
				g.Expect(l.Logs).NotTo(BeEmpty(), "expected runtime logs")
			}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		})

		It("captures traces via amctl agent traces", func() {
			Eventually(func(g Gomega) {
				t := cliagent.AgentTraces(g, H, H.Org(), owned.ProjectName, owned.AgentName, cfg.DefaultEnv)
				g.Expect(t.Traces).NotTo(BeEmpty(), "expected at least one trace")
			}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		})
	})
})
