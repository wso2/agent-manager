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

// Package httpx holds the JSON response helpers every HTTP handler uses, in the core and in modules. It imports no
// core package and keeps no state.
package httpx

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the body of every error answer.
type ErrorResponse struct {
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
	Code    string `json:"code"`
}

// WriteSuccessResponse writes a successful API response
func WriteSuccessResponse[T any](w http.ResponseWriter, statusCode int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if statusCode == http.StatusNoContent {
		return
	}
	_ = json.NewEncoder(w).Encode(data) // Ignore encoding errors for response
}

// WriteErrorResponse writes an error API response with auto-derived error code.
// For more control, use WriteErrorResponseWithReason.
func WriteErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	WriteErrorResponseWithReason(w, statusCode, message, "", statusCodeToErrorCode(statusCode))
}

// WriteErrorResponseWithReason writes an error API response with full error details.
// - message: User-friendly message for display in UI
// - reason: Technical details for debugging (can be empty)
// - code: Machine-readable error code for programmatic handling
func WriteErrorResponseWithReason(w http.ResponseWriter, statusCode int, message, reason, code string) {
	if code == "" {
		code = statusCodeToErrorCode(statusCode)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Message: message, Reason: reason, Code: code}) // Ignore encoding errors for response
}

// WriteValidationErrorResponse writes a validation error response.
// If err is a ValidationError, uses its user-friendly Message and technical Reason.
// Otherwise, uses err.Error() for both message and reason.
func WriteValidationErrorResponse(w http.ResponseWriter, err error) {
	if ve := IsValidationError(err); ve != nil {
		WriteErrorResponseWithReason(w, http.StatusBadRequest, ve.Message, ve.Reason, ErrCodeValidation)
		return
	}
	WriteErrorResponseWithReason(w, http.StatusBadRequest, err.Error(), err.Error(), ErrCodeValidation)
}

// statusCodeToErrorCode maps HTTP status codes to default error codes
func statusCodeToErrorCode(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return ErrCodeBadRequest
	case http.StatusUnauthorized:
		return ErrCodeUnauthorized
	case http.StatusForbidden:
		return ErrCodeForbidden
	case http.StatusNotFound:
		return ErrCodeNotFound
	case http.StatusConflict:
		return ErrCodeConflict
	case http.StatusInternalServerError:
		return ErrCodeInternalError
	case http.StatusServiceUnavailable:
		return ErrCodeServiceUnavailable
	case http.StatusNotImplemented:
		return ErrCodeNotImplemented
	default:
		// Classify unmapped status codes by range
		if statusCode >= 400 && statusCode < 500 {
			return ErrCodeBadRequest
		}
		return ErrCodeInternalError
	}
}
