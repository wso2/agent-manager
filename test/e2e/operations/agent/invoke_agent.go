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
// endpoint URL and returns the raw response body as a string.
// It retries on transient errors (503, 502, connection errors) using Eventually.
// The apiKey is sent as an X-API-Key header for authentication.
func InvokeAgentEndpoint(endpointURL string, body any, apiKey string) string {
	data, err := json.Marshal(body)
	Expect(err).NotTo(HaveOccurred(), "marshal agent invocation body")

	httpClient := &http.Client{Timeout: 60 * time.Second}

	var result string
	Eventually(func(g Gomega) {
		req, err := http.NewRequest("POST", endpointURL, bytes.NewBuffer(data))
		g.Expect(err).NotTo(HaveOccurred(), "create agent invocation request")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", apiKey)
		resp, err := httpClient.Do(req)
		g.Expect(err).NotTo(HaveOccurred(), "agent endpoint not reachable")
		defer resp.Body.Close()

		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			StopTrying(fmt.Sprintf("read response body: %v", readErr)).Now()
		}

		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusUnauthorized {
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "agent endpoint returned %d, retrying", resp.StatusCode)
			return
		}

		if resp.StatusCode != http.StatusOK {
			StopTrying(fmt.Sprintf("agent invocation returned status %d: %s", resp.StatusCode, string(respBody))).Now()
		}

		result = string(respBody)
		if result == "" {
			StopTrying("agent invocation returned empty response").Now()
		}

		ginkgo.GinkgoWriter.Printf("Agent invocation response (%d bytes): %.200s\n", len(result), result)
	}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	return result
}
