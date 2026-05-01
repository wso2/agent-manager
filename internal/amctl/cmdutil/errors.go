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

	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/amctl/clierr"
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
