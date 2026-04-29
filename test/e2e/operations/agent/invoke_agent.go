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
	"testing"
	"time"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// InvokeAgentEndpoint sends a POST request with the given body to an absolute
// endpoint URL and returns the raw response body as a string.
// It retries on transient errors (503, connection errors) with polling until
// the server is ready. No authentication is required for agent endpoints.
func InvokeAgentEndpoint(t *testing.T, endpointURL string, body any) string {
	t.Helper()

	data, err := json.Marshal(body)
	if err != nil {
		framework.Fatalf(t, "marshal agent invocation body: %v", err)
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}

	result := framework.Poll(t, "agent endpoint to respond", framework.PollConfig{
		Timeout:         3 * time.Minute,
		InitialInterval: 5 * time.Second,
		MaxInterval:     15 * time.Second,
	}, func() (string, bool, error) {
		resp, err := httpClient.Post(endpointURL, "application/json", bytes.NewBuffer(data))
		if err != nil {
			framework.Log(t, "agent endpoint not reachable yet: %v", err)
			return "", false, nil // retry on connection errors
		}
		defer resp.Body.Close()

		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", false, fmt.Errorf("read response body: %w", readErr)
		}

		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway {
			framework.Log(t, "agent endpoint returned %d, retrying...", resp.StatusCode)
			return "", false, nil // retry on 503/502
		}

		if resp.StatusCode != http.StatusOK {
			return "", false, fmt.Errorf("agent invocation returned status %d: %s", resp.StatusCode, string(respBody))
		}

		body := string(respBody)
		if body == "" {
			return "", false, fmt.Errorf("agent invocation returned empty response")
		}

		framework.Log(t, "agent invocation response (%d bytes): %.200s", len(body), body)
		return body, true, nil
	})

	return result
}
