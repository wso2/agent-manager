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
package httpx

import (
	"errors"
	"fmt"
)

// Classification errors a handler maps to a status. Domain errors wrap them or are mapped next to them.
var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrBadRequest          = errors.New("bad request")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("conflict")
	ErrInternalServerError = errors.New("internal server error")
	ErrServiceUnavailable  = errors.New("service unavailable")
	ErrNotImplemented      = errors.New("not implemented")
)

// Error codes for programmatic error handling by frontend applications.
// These codes are stable and should not change once defined.
const (
	ErrCodeValidation         = "VALIDATION_ERROR"
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeConflict           = "CONFLICT"
	ErrCodeBadRequest         = "BAD_REQUEST"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeForbidden          = "FORBIDDEN"
	ErrCodeInvalidInput       = "INVALID_INPUT"
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrCodeNotImplemented     = "NOT_IMPLEMENTED"
)

// ValidationError represents a validation error with both user-friendly message and technical details.
// Use this when you need to communicate errors to both end-users (via Message) and developers (via Reason).
type ValidationError struct {
	// Message is a user-friendly error message suitable for display in UI.
	// Should be clear, non-technical, and actionable.
	// Example: "Please provide a valid schema path starting with /"
	Message string

	// Reason contains technical details for debugging.
	// Can include field names, specific validation rules, etc.
	// Example: "inputInterface.schema.path is required and must start with /"
	Reason string

	// sentinel, when set, is the classification error this failure also matches
	// under errors.Is — so a check like errors.Is(err, ErrInvalidInput) still
	// recognises a ValidationError raised in place of a wrapped sentinel.
	// Unexported so a constructor is the only way to set it.
	sentinel error
}

// Error implements the error interface, returning the technical reason for logging.
func (e *ValidationError) Error() string {
	return e.Reason
}

// Unwrap exposes the classification sentinel to errors.Is/errors.As.
func (e *ValidationError) Unwrap() error {
	return e.sentinel
}

// NewValidationError creates a new ValidationError with user-friendly message and technical reason.
func NewValidationError(message, reason string) *ValidationError {
	return &ValidationError{Message: message, Reason: reason, sentinel: nil}
}

// NewInvalidInputError is NewValidationError for a failure that must also keep
// matching ErrInvalidInput under errors.Is, so callers that route on the
// sentinel keep working while the UI gets the short Message instead of the
// whole technical string.
//
// The sentinel is fixed rather than a parameter because every ValidationError
// is rendered as 400 by WriteValidationErrorResponse — pairing one with a
// not-found or conflict sentinel would still answer 400.
func NewInvalidInputError(message, reason string) *ValidationError {
	return &ValidationError{Message: message, Reason: reason, sentinel: ErrInvalidInput}
}

// NewValidationErrorf creates a new ValidationError with formatted reason string.
// The message should be user-friendly, while reasonFmt is for technical details.
func NewValidationErrorf(message, reasonFmt string, args ...interface{}) *ValidationError {
	return NewValidationError(message, fmt.Sprintf(reasonFmt, args...))
}

// IsValidationError checks if an error is a ValidationError and returns it.
// Returns nil if the error is not a ValidationError.
func IsValidationError(err error) *ValidationError {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve
	}
	return nil
}
