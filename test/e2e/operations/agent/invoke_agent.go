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
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// InvokeAgentEndpoint sends a POST request with the given body to an absolute
// endpoint URL and returns the raw response body as a string. The apiKey is sent
// as an X-API-Key header. It always retries past transient gateway warmup
// (502/503/504 and 401/403) using Eventually.
//
// The optional requireOK controls what counts as success once the request
// reaches the runtime; it defaults to true when omitted:
//   - true (default): the response must be 200 with a non-empty body (a real chat
//     result); any other status fails the spec. Use for normal invocations.
//   - false: any non-transient status is accepted and its body returned. Use to
//     provoke runtime behaviour (e.g. a failing LLM call) where the response is
//     expected to be an error rather than a chat completion.
func InvokeAgentEndpoint(endpointURL string, body any, apiKey string, requireOK ...bool) string {
	required := true
	if len(requireOK) > 0 {
		required = requireOK[0]
	}
	return InvokeEndpointWithHeaders(endpointURL, body, map[string]string{"X-API-Key": apiKey}, required)
}

// InvokeEndpointWithHeaders is InvokeAgentEndpoint with caller-chosen request
// headers instead of a fixed X-API-Key. Content-Type is always application/json.
func InvokeEndpointWithHeaders(endpointURL string, body any, headers map[string]string, requireOK bool) string {
	required := requireOK

	data, err := json.Marshal(body)
	Expect(err).NotTo(HaveOccurred(), "marshal agent invocation body")

	httpClient := &http.Client{Timeout: 60 * time.Second}

	var result string
	Eventually(func(g Gomega) {
		req, err := http.NewRequest("POST", endpointURL, bytes.NewBuffer(data))
		g.Expect(err).NotTo(HaveOccurred(), "create agent invocation request")
		req.Header.Set("Content-Type", "application/json")
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		resp, err := httpClient.Do(req)
		g.Expect(err).NotTo(HaveOccurred(), "agent endpoint not reachable")
		defer resp.Body.Close()

		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			StopTrying(fmt.Sprintf("read response body: %v", readErr)).Now()
		}

		// Transient while a gateway warms up: 502/503 (no healthy upstream yet) and
		// 504 (upstream timeout during cold start). Retry rather than fail hard.
		// 401/403 are also transient here:
		if resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusGatewayTimeout ||
			resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusForbidden {
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "agent endpoint returned %d, retrying", resp.StatusCode)
			return
		}

		result = string(respBody)

		// Any other status means the request reached the runtime. When required is
		// false that's all we need; otherwise it must be a 200 with a non-empty body.
		if required {
			if resp.StatusCode != http.StatusOK {
				StopTrying(fmt.Sprintf("agent invocation returned status %d: %s", resp.StatusCode, string(respBody))).Now()
			}
			if result == "" {
				StopTrying("agent invocation returned empty response").Now()
			}
		}

		ginkgo.GinkgoWriter.Printf("Agent invocation response (status %d, %d bytes): %.200s\n", resp.StatusCode, len(result), result)
	}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

	return result
}
