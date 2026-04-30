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

// Package render handles output dispatch for am commands. When --json is set,
// structured JSON envelopes are written to stdout. Otherwise, text output is
// the default: errors go to stderr with color, and each command writes its
// own success output.
//
// JSON success envelope:
//
//	{ "instance": "...", "org": "...", "project": "...", "data": <any> }
//
// JSON error envelope:
//
//	{ "instance": "...", "org": "...", "project": "...", "error": { ... } }
//
// `instance` is always present; `org` and `project` are omitted when empty.
package render

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wso2/agent-manager/internal/amctl/clierr"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
)

// Scope is the {instance, org, project} triple included on every JSON envelope.
type Scope struct {
	Instance string `json:"instance"`
	Org      string `json:"org,omitempty"`
	Project  string `json:"project,omitempty"`
}

type successEnvelope struct {
	Scope
	Data any `json:"data"`
}

type errorEnvelope struct {
	Scope
	Error clierr.CLIError `json:"error"`
}

// JSONSuccess writes the success JSON envelope to io.Out.
func JSONSuccess(ios *iostreams.IOStreams, scope Scope, data any) error {
	return writeJSON(ios.Out, successEnvelope{Scope: scope, Data: data})
}

// JSONError writes the error JSON envelope to io.Out and returns a sentinel
// that signals the envelope has already been written.
func JSONError(ios *iostreams.IOStreams, scope Scope, err error) error {
	_ = writeJSON(ios.Out, errorEnvelope{Scope: scope, Error: asCLIError(err)})
	return &renderedError{err: err}
}

// Error dispatches to JSONError or writes a text error to stderr. Every
// command calls this; the --json flag determines the output path.
func Error(ios *iostreams.IOStreams, scope Scope, err error) error {
	if ios.JSON {
		return JSONError(ios, scope, err)
	}
	return textError(ios, err)
}

func textError(ios *iostreams.IOStreams, err error) error {
	cs := ios.StderrColorScheme()
	msg := err.Error()
	var cliErr clierr.CLIError
	if errors.As(err, &cliErr) {
		msg = cliErr.Message
	}
	fmt.Fprintf(ios.ErrOut, "%s %s\n", cs.FailureIcon(), msg)
	return &renderedError{err: err}
}

// IsRendered reports whether err is (or wraps) a value returned by Error.
func IsRendered(err error) bool {
	var r *renderedError
	return errors.As(err, &r)
}

type renderedError struct {
	err error
}

func (r *renderedError) Error() string { return r.err.Error() }
func (r *renderedError) Unwrap() error { return r.err }

func asCLIError(err error) clierr.CLIError {
	var cliErr clierr.CLIError
	if errors.As(err, &cliErr) {
		if cliErr.AdditionalData == nil {
			cliErr.AdditionalData = map[string]any{}
		}
		return cliErr
	}
	return clierr.CLIError{
		Code:           clierr.Transport,
		Message:        err.Error(),
		AdditionalData: map[string]any{},
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
