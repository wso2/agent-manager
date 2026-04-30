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

	. "github.com/onsi/gomega"
)

// ExpectStatus asserts the HTTP response status code matches expected.
// On mismatch it includes the response body in the failure message.
func ExpectStatus(g Gomega, resp *http.Response, expected int) {
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		g.Expect(resp.StatusCode).To(Equal(expected), "response body: %s", string(body))
	}
}

// DecodeBody reads the response body and JSON-decodes it into type T.
func DecodeBody[T any](g Gomega, resp *http.Response) T {
	var result T
	body, err := io.ReadAll(resp.Body)
	g.Expect(err).NotTo(HaveOccurred(), "read response body")
	err = json.Unmarshal(body, &result)
	g.Expect(err).NotTo(HaveOccurred(), "decode response body: %s", string(body))
	return result
}

// ExpectErrorResponse asserts the response has the expected status code
// and that the error message contains the expected substring.
func ExpectErrorResponse(g Gomega, resp *http.Response, expectedStatus int, expectedMsgContains string) {
	ExpectStatus(g, resp, expectedStatus)
	errResp := DecodeBody[ErrorResponse](g, resp)
	g.Expect(errResp.Message).To(ContainSubstring(expectedMsgContains),
		"error message should contain %q, got %q", expectedMsgContains, errResp.Message)
}
