// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package requests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
)

// HttpClient interface for making HTTP requests.
// Use RetryableHTTPClient for retry support.
type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Compile-time check that http.Client implements HttpClient
var _ HttpClient = (*http.Client)(nil)

// SendRequest builds and sends an HTTP request, returning a Result for response handling.
func SendRequest(ctx context.Context, client HttpClient, req *HttpRequest) *Result {
	log := logger.GetLogger(ctx).With(slog.String("request", req.Name))

	httpReq, err := req.buildHttpRequest(ctx)
	if err != nil {
		return &Result{err: fmt.Errorf("failed to build http request: %w", err)}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return &Result{err: fmt.Errorf("request failed: %w", err)}
	}

	// Read response body and close immediately to avoid resource leaks
	respBody, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if closeErr != nil {
		log.Warn("failed to close response body", slog.String("error", closeErr.Error()))
	}
	if err != nil {
		return &Result{err: fmt.Errorf("failed to read response body: %w", err)}
	}

	return &Result{response: resp, responseBody: respBody}
}

// Result holds the response from SendRequest.
type Result struct {
	responseBody []byte
	response     *http.Response
	err          error
}

// ScanResponse unmarshals the response body into the provided struct if status matches.
func (r *Result) ScanResponse(body any, successStatus int) error {
	if r.err != nil {
		return r.err
	}
	if r.response == nil {
		return fmt.Errorf("unexpected nil response")
	}
	if body == nil || reflect.ValueOf(body).Kind() != reflect.Ptr {
		return fmt.Errorf("non-nil pointer expected for decoding response body")
	}
	if r.response.StatusCode != successStatus {
		return &HttpError{
			StatusCode: r.response.StatusCode,
			Body:       string(r.responseBody),
		}
	}
	if err := json.Unmarshal(r.responseBody, body); err != nil {
		return fmt.Errorf("failed to decode response body for status %d: %w", r.response.StatusCode, err)
	}
	return nil
}

// GetHeader returns the value of a response header.
func (r *Result) GetHeader(key string) string {
	if r.err != nil || r.response == nil {
		return ""
	}
	return r.response.Header.Get(key)
}
