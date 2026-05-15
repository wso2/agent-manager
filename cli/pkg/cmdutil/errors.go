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

package cmdutil

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

// FlagError marks an error as a user-facing flag/argument problem. Wrapped via
// errors.As, it lets amcmd.Main distinguish "user typed bad flags" (exit 2)
// from runtime/server failures (exit 1). The inner clierr.CLIError keeps the
// JSON envelope contract — code stays clierr.InvalidFlag.
type FlagError struct {
	inner clierr.CLIError
}

func (e *FlagError) Error() string { return e.inner.Error() }
func (e *FlagError) Unwrap() error { return e.inner }

// FlagErrorf builds a *FlagError with code = clierr.InvalidFlag.
func FlagErrorf(format string, args ...any) error {
	return &FlagError{inner: clierr.Newf(clierr.InvalidFlag, format, args...)}
}

// FlagErrorWrap promotes an arbitrary error (typically from cobra's
// SetFlagErrorFunc) into a *FlagError so amcmd.Main can detect it.
func FlagErrorWrap(err error) error {
	var cli clierr.CLIError
	if errors.As(err, &cli) {
		return &FlagError{inner: cli}
	}
	return &FlagError{inner: clierr.Newf(clierr.InvalidFlag, "%v", err)}
}

// FlagErrors builds a single *FlagError from multiple validation violations.
// The message is newline-delimited for text rendering; AdditionalData["details"]
// carries the structured list for JSON consumers.
func FlagErrors(violations []string) error {
	var buf strings.Builder
	buf.WriteString("invalid flags")
	for _, v := range violations {
		buf.WriteString("\n    ")
		buf.WriteString(v)
	}
	inner := clierr.New(clierr.InvalidFlag, buf.String())
	inner.AdditionalData["details"] = violations
	return &FlagError{inner: inner}
}

// ErrorFromServer converts an oapi-codegen response and decoded ErrorResponse
// into a clierr.CLIError. body may be nil when the server returned a non-JSON
// error body.
func ErrorFromServer(httpResp *http.Response, body *amsvc.ErrorResponse) clierr.CLIError {
	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}
	if body == nil {
		if status == http.StatusUnauthorized {
			return clierr.CLIError{
				Status:         status,
				Code:           clierr.Unauthorized,
				Message:        "authentication required, try: amctl login",
				AdditionalData: map[string]any{},
			}
		}
		return clierr.CLIError{
			Status:         status,
			Code:           clierr.ServerInvalid,
			Message:        fmt.Sprintf("server returned %d with no JSON body", status),
			AdditionalData: map[string]any{},
		}
	}
	additional := map[string]any{}
	if body.AdditionalData != nil {
		additional = *body.AdditionalData
	}
	return clierr.CLIError{
		Status:         status,
		Code:           body.Code,
		Message:        body.Message,
		Reason:         body.Reason,
		AdditionalData: additional,
	}
}

// FirstNonNil returns the first non-nil ErrorResponse, used to pick whichever
// of the typed error variants oapi-codegen populated for a given response.
func FirstNonNil(errs ...*amsvc.ErrorResponse) *amsvc.ErrorResponse {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// TraceObserverErrorFromResponse converts a traceobssvc error into a CLIError.
// Non-HTTP errors map to clierr.Transport.
func TraceObserverErrorFromResponse(err error) clierr.CLIError {
	var herr *traceobssvc.HTTPError
	if !errors.As(err, &herr) {
		return clierr.CLIError{
			Code:           clierr.Transport,
			Message:        err.Error(),
			AdditionalData: map[string]any{},
		}
	}
	code := clierr.ServerError
	switch herr.StatusCode {
	case http.StatusUnauthorized:
		code = clierr.Unauthorized
	case http.StatusForbidden:
		code = clierr.Forbidden
	case http.StatusNotFound:
		code = clierr.NotFound
	case http.StatusBadRequest:
		code = clierr.Validation
	}
	msg := fmt.Sprintf("trace observer returned %d", herr.StatusCode)
	if herr.Body != nil && herr.Body.Message != "" {
		msg = herr.Body.Message
	}
	return clierr.CLIError{
		Status:         herr.StatusCode,
		Code:           code,
		Message:        msg,
		AdditionalData: map[string]any{},
	}
}
