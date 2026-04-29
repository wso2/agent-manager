// Package render writes the JSON envelopes that wrap every am command's
// stdout. Two shapes are emitted: a success envelope carrying a `data` field
// and an error envelope whose `error` body matches clierr.CLIError.
// Downstream tools depend on the field set and tag choices below being stable.
//
// Success envelope:
//
//	{ "instance": "...", "org": "...", "project": "...", "data": <any> }
//
// Error envelope:
//
//	{ "instance": "...", "org": "...", "project": "...", "error": { ... } }
//
// `instance` is always present; `org` and `project` are omitted when empty.
package render

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/iostreams"
)

// Scope is the {instance, org, project} triple included on every envelope.
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

// Success writes the success envelope to io.Out.
func Success(io *iostreams.IOStreams, scope Scope, data any) error {
	return write(io.Out, successEnvelope{Scope: scope, Data: data})
}

// Error writes the error envelope to io.Out and returns a sentinel that
// signals the envelope has already been written. Use IsRendered upstream to
// avoid double-rendering. errors.As/Is walk through the sentinel, so type
// assertions like errors.As(err, &cmdutil.FlagError{}) keep working.
func Error(io *iostreams.IOStreams, scope Scope, err error) error {
	_ = write(io.Out, errorEnvelope{Scope: scope, Error: asCLIError(err)})
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

func write(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
