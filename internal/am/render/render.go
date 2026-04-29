package render

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wso2/agent-manager/internal/am/iostreams"
)

const (
	CodeTransport            = "CLI_TRANSPORT"
	CodeAuthTokenExpired     = "AUTH_TOKEN_EXPIRED"
	CodeAuthRefreshFailed    = "AUTH_REFRESH_FAILED"
	CodeConfigNotLoaded      = "CONFIG_NOT_LOADED"
	CodeNoInstance           = "NO_INSTANCE"
	CodeNoOrg                = "NO_ORG"
	CodeNoProject            = "NO_PROJECT"
	CodeConfirmationRequired = "CONFIRMATION_REQUIRED"
	CodeInvalidFlag          = "INVALID_FLAG"
	CodeServerInvalid        = "SERVER_RESPONSE_INVALID"
)

type Scope struct {
	Instance string `json:"instance"`
	Org      string `json:"org,omitempty"`
	Project  string `json:"project,omitempty"`
}

type successEnvelope struct {
	Instance string `json:"instance"`
	Org      string `json:"org,omitempty"`
	Project  string `json:"project,omitempty"`
	Data     any    `json:"data"`
}

type errorEnvelope struct {
	Instance string   `json:"instance"`
	Org      string   `json:"org,omitempty"`
	Project  string   `json:"project,omitempty"`
	Error    CLIError `json:"error"`
}

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

func NewError(code, message string) CLIError {
	return CLIError{
		Code:           code,
		Message:        message,
		AdditionalData: map[string]any{},
	}
}

func NewErrorf(code, format string, args ...any) CLIError {
	return NewError(code, fmt.Sprintf(format, args...))
}

func Success(io *iostreams.IOStreams, scope Scope, data any) error {
	return write(io.Out, successEnvelope{
		Instance: scope.Instance,
		Org:      scope.Org,
		Project:  scope.Project,
		Data:     data,
	})
}

func Error(io *iostreams.IOStreams, scope Scope, err error) error {
	cliErr := asCLIError(err)
	if cliErr.AdditionalData == nil {
		cliErr.AdditionalData = map[string]any{}
	}
	return write(io.Out, errorEnvelope{
		Instance: scope.Instance,
		Org:      scope.Org,
		Project:  scope.Project,
		Error:    cliErr,
	})
}

// Emit writes the error envelope and returns err so callers can propagate a
// non-zero exit through cobra in a single statement.
func Emit(io *iostreams.IOStreams, scope Scope, err error) error {
	_ = Error(io, scope, err)
	return err
}

func asCLIError(err error) CLIError {
	var cliErr CLIError
	if errors.As(err, &cliErr) {
		return cliErr
	}
	return CLIError{
		Code:    CodeTransport,
		Message: err.Error(),
	}
}

func write(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
