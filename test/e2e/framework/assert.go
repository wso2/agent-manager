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

package framework

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// RequireStatus asserts the HTTP response status code matches expected.
// On mismatch it logs the full response body to both the log file and test output.
func RequireStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		Fatalf(t, "expected status %d, got %d\nresponse body: %s", expected, resp.StatusCode, string(body))
	}
}

// DecodeBody reads the response body and JSON-decodes it into type T.
func DecodeBody[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var result T
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")
	require.NoError(t, json.Unmarshal(body, &result), "decode response body: %s", string(body))
	return result
}

// RequireErrorResponse asserts the response has the expected status code
// and that the error message contains the expected substring.
func RequireErrorResponse(t *testing.T, resp *http.Response, expectedStatus int, expectedMsgContains string) {
	t.Helper()
	RequireStatus(t, resp, expectedStatus)
	errResp := DecodeBody[ErrorResponse](t, resp)
	require.Contains(t, errResp.Message, expectedMsgContains,
		"error message should contain %q, got %q", expectedMsgContains, errResp.Message)
}
