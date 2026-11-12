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

// Validates trace collection for an external agent: spans posted to the AI
// Gateway's OTLP endpoint with the agent's token appear in the observer API,
// scoped to the agent, and can be exported.

package traces

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	traceops "github.com/wso2/agent-manager/test/e2e/operations/trace"
)

// randomHex returns n random bytes as lowercase hex (OTLP trace and span IDs).
func randomHex(n int) string {
	b := make([]byte, n)
	_, err := rand.Read(b)
	Expect(err).NotTo(HaveOccurred())
	return hex.EncodeToString(b)
}

// postSpan sends one OTLP/JSON span to the gateway and returns the status code.
func postSpan(token, traceID, spanName string) int {
	now := time.Now()
	body := fmt.Sprintf(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"e2e-traces"}}]},`+
		`"scopeSpans":[{"scope":{"name":"e2e"},"spans":[{"traceId":%q,"spanId":%q,"name":%q,"kind":1,`+
		`"startTimeUnixNano":"%d","endTimeUnixNano":"%d"}]}]}]}`,
		traceID, randomHex(8), spanName, now.UnixNano(), now.Add(time.Second).UnixNano())
	req, err := http.NewRequest(http.MethodPost, Cfg.GatewayURL+"/otel/v1/traces", bytes.NewBufferString(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("x-amp-api-key", token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	Expect(err).NotTo(HaveOccurred(), "OTLP request to the gateway failed")
	defer resp.Body.Close()
	return resp.StatusCode
}

var _ = Describe("Basic tracing functionality:", Ordered, Label("traces"), func() {
	var (
		agentName string
		token     string
		traceID   string
	)

	BeforeAll(func() {
		agentName = "e2e-test-agent-" + uuid.New().String()[:8]
		traceID = randomHex(16)
		agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     framework.NewExternalAgentRequest(agentName, "External agent for the traces e2e tests"),
		})
		token = agentops.GenerateAgentToken(Default, Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, "1h").Token
		Expect(token).NotTo(BeEmpty(), "expected an agent token")
	})

	AfterAll(func() {
		agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName)
	})

	It("rejects spans without the agent token", func() {
		Expect(postSpan("", randomHex(16), "unauthenticated")).To(Equal(http.StatusUnauthorized))
	})

	It("accepts spans sent with the agent token", func() {
		// The gateway can take a moment to load a new agent's token issuer.
		Eventually(func() int {
			return postSpan(token, traceID, "e2e-traces-root")
		}).WithTimeout(1 * time.Minute).WithPolling(5 * time.Second).Should(Equal(http.StatusOK))
	})

	It("makes the trace available via the API", func() {
		traces := traceops.WaitForTraces(Client, &traceops.WaitForTracesParams{
			Organization: Cfg.DefaultOrg,
			Project:      framework.E2ESharedProjectName,
			Agent:        agentName,
			Environment:  Cfg.DefaultEnv,
			Timeout:      2 * time.Minute,
		})
		Expect(traces.Traces).NotTo(BeEmpty(), "expected at least one trace")
		ids := make([]string, 0, len(traces.Traces))
		for _, tr := range traces.Traces {
			ids = append(ids, tr.TraceID)
		}
		Expect(ids).To(ContainElement(traceID))
	})

	It("exports the captured traces successfully", func() {
		var exportedBody []byte
		Eventually(func(g Gomega) {
			exportedBody = traceops.ExportTraces(g, Client, &traceops.ExportTracesParams{
				Organization: Cfg.DefaultOrg,
				Project:      framework.E2ESharedProjectName,
				Agent:        agentName,
				Environment:  Cfg.DefaultEnv,
				Limit:        10,
				SortOrder:    "desc",
			})
		}).WithTimeout(1 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		Expect(string(exportedBody)).To(ContainSubstring(traceID))
	})
})
