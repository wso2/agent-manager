package cmdutil

import (
	"fmt"
	"net/http"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/render"
)

// ErrorFromServer converts an oapi-codegen response and decoded ErrorResponse
// into a render.CLIError. body may be nil when the server returned a non-JSON
// error body.
func ErrorFromServer(httpResp *http.Response, body *amsvc.ErrorResponse) render.CLIError {
	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}
	if body == nil {
		return render.CLIError{
			Status:         status,
			Code:           render.CodeServerInvalid,
			Message:        fmt.Sprintf("server returned %d with no JSON body", status),
			AdditionalData: map[string]any{},
		}
	}
	additional := map[string]any{}
	if body.AdditionalData != nil {
		additional = *body.AdditionalData
	}
	return render.CLIError{
		Status:         status,
		Code:           body.Code,
		Message:        body.Message,
		Reason:         body.Reason,
		AdditionalData: additional,
	}
}
