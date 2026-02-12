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

package client

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// ErrorContext holds context for API error handling.
type ErrorContext struct {
	NotFoundErr error
	ConflictErr error
}

// apiErrorResponse represents the standard API error response structure.
type apiErrorResponse struct {
	Code    *string `json:"code,omitempty"`
	Error   *string `json:"error,omitempty"`
	Message *string `json:"message,omitempty"`
	Success *bool   `json:"success,omitempty"`
}

// handleErrorResponse converts HTTP status codes and response body to domain errors.
func handleErrorResponse(statusCode int, body []byte, ctx ErrorContext) error {
	// Try to parse error message from response body
	errMsg := parseErrorMessage(body)

	switch statusCode {
	case http.StatusBadRequest:
		return fmt.Errorf("%w: %s", utils.ErrBadRequest, errMsg)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", utils.ErrUnauthorized, errMsg)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %s", utils.ErrForbidden, errMsg)
	case http.StatusNotFound:
		if ctx.NotFoundErr != nil {
			return fmt.Errorf("%w: %s", ctx.NotFoundErr, errMsg)
		}
		return fmt.Errorf("not found: %s", errMsg)
	case http.StatusConflict:
		if ctx.ConflictErr != nil {
			return fmt.Errorf("%w: %s", ctx.ConflictErr, errMsg)
		}
		return fmt.Errorf("conflict: %s", errMsg)
	case http.StatusInternalServerError:
		return fmt.Errorf("%w: %s", utils.ErrServiceUnavailable, errMsg)
	default:
		return fmt.Errorf("unexpected status code %d: %s", statusCode, errMsg)
	}
}

// parseErrorMessage extracts error message from API response body.
func parseErrorMessage(body []byte) string {
	if len(body) == 0 {
		return "unknown error"
	}

	var errResp apiErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		// If we can't parse JSON, return the raw body (truncated)
		if len(body) > 200 {
			return string(body[:200]) + "..."
		}
		return string(body)
	}

	// Try different fields that might contain the error message
	if errResp.Error != nil && *errResp.Error != "" {
		return *errResp.Error
	}
	if errResp.Message != nil && *errResp.Message != "" {
		return *errResp.Message
	}

	return "unknown error"
}
