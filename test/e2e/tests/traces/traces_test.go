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

// Validates trace collection: agent invocation generates traces, traces
// appear in the observer API, and traces can be exported.

package traces

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	traceops "github.com/wso2/agent-manager/test/e2e/operations/trace"
)

var _ = Describe("Traces", Ordered, Label("traces"), func() {
	BeforeAll(func() {
		Expect(Shared).NotTo(BeNil(), "shared agent must be available")
	})

	It("should respond to invocation", func() {
		endpointURL := Shared.EndpointURL + "/chat"
		agentops.InvokeAgentEndpoint(endpointURL, Shared.InvokeReq, Shared.APIKey)
	})

	It("should have traces available", func() {
		traces := traceops.WaitForTraces(Client, &traceops.WaitForTracesParams{
			Organization: Cfg.DefaultOrg,
			Project:      Shared.ProjectName,
			Agent:        Shared.AgentName,
			Environment:  Cfg.DefaultEnv,
			Timeout:      2 * time.Minute,
		})
		Expect(traces.Traces).NotTo(BeEmpty(), "expected at least one trace")
		GinkgoWriter.Printf("Traces found: %d\n", len(traces.Traces))
	})

	It("should export traces successfully", func() {
		var exportedBody []byte
		Eventually(func(g Gomega) {
			exportedBody = traceops.ExportTraces(g, Client, &traceops.ExportTracesParams{
				Organization: Cfg.DefaultOrg,
				Project:      Shared.ProjectName,
				Agent:        Shared.AgentName,
				Environment:  Cfg.DefaultEnv,
				Limit:        10,
				SortOrder:    "desc",
			})
		}).WithTimeout(1 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		GinkgoWriter.Printf("Traces exported: %d bytes\n", len(exportedBody))
	})
})
