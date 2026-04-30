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

// Package clierr defines the CLI error wire type and stable error codes used
// in the JSON envelope written by package render. CLIError doubles as a Go
// error (so producers `return clierr.New(...)` and consumers extract via
// errors.As) and as the error body in the wire format documented below.
//
// JSON wire contract — downstream tools depend on this:
//
//	{
//	  "status":         <int>,         // HTTP status when sourced from server, 0 otherwise
//	  "code":           <string>,      // stable identifier; see constants below
//	  "message":        <string>,      // human-readable; may change across versions
//	  "reason":         <string|null>, // optional; serialized as null when absent
//	  "additionalData": <object>       // free-form; always present (possibly {})
//	}
//
// Field names, JSON shapes, and code values are stable. Adding a new code is
// non-breaking; renaming or removing one is breaking.
package clierr

import "fmt"

// Stable error codes. The string values are part of the wire contract.
const (
	Transport            = "CLI_TRANSPORT"
	AuthTokenExpired     = "AUTH_TOKEN_EXPIRED"
	AuthRefreshFailed    = "AUTH_REFRESH_FAILED"
	ConfigNotLoaded      = "CONFIG_NOT_LOADED"
	ConfigSaveFailed     = "CONFIG_SAVE_FAILED"
	NoInstance           = "NO_INSTANCE"
	NoOrg                = "NO_ORG"
	NoProject            = "NO_PROJECT"
	ConfirmationRequired = "CONFIRMATION_REQUIRED"
	InvalidFlag          = "INVALID_FLAG"
	Unauthorized         = "UNAUTHORIZED"
	ServerInvalid        = "SERVER_RESPONSE_INVALID"
)

// CLIError is both the wire body of an error envelope and a Go error value.
type CLIError struct {
	Status         int            `json:"status"`
	Code           string         `json:"code"`
	Message        string         `json:"message"`
	Reason         *string        `json:"reason"`
	AdditionalData map[string]any `json:"additionalData"`
}

func (e CLIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// New builds a CLIError with the given code and message and a non-nil
// AdditionalData map (so JSON marshal emits {} not null).
func New(code, message string) CLIError {
	return CLIError{
		Code:           code,
		Message:        message,
		AdditionalData: map[string]any{},
	}
}

// Newf is New with fmt.Sprintf-style formatting on the message.
func Newf(code, format string, args ...any) CLIError {
	return New(code, fmt.Sprintf(format, args...))
}
