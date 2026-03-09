//
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
//

package client

import (
	"fmt"
	"log/slog"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// ErrorResponses holds typed error responses from API calls.
type ErrorResponses struct {
	JSON400 *gen.BadRequest
	JSON401 *gen.Unauthorized
	JSON403 *gen.Forbidden
	JSON404 *gen.NotFound
	JSON409 *gen.Conflict
	JSON500 *gen.InternalError
}

// handleErrorResponse converts typed error responses to domain errors.
func handleErrorResponse(statusCode int, errs ErrorResponses) error {
	switch {
	case errs.JSON400 != nil:
		logErrorDetails(errs.JSON400)
		return fmt.Errorf("%w: %s", utils.ErrBadRequest, errs.JSON400.Error)
	case errs.JSON401 != nil:
		logErrorDetails(errs.JSON401)
		return fmt.Errorf("%w: %s", utils.ErrUnauthorized, errs.JSON401.Error)
	case errs.JSON403 != nil:
		logErrorDetails(errs.JSON403)
		return fmt.Errorf("%w: %s", utils.ErrForbidden, errs.JSON403.Error)
	case errs.JSON404 != nil:
		logErrorDetails(errs.JSON404)
		return fmt.Errorf("%w: %s", utils.ErrNotFound, errs.JSON404.Error)
	case errs.JSON409 != nil:
		logErrorDetails(errs.JSON409)
		return fmt.Errorf("%w: %s", utils.ErrConflict, errs.JSON409.Error)
	case errs.JSON500 != nil:
		logErrorDetails(errs.JSON500)
		return fmt.Errorf("%w: %s", utils.ErrInternalServerError, errs.JSON500.Error)
	default:
		return fmt.Errorf("unexpected error: status %d", statusCode)
	}
}

// logErrorDetails logs error details if present.
func logErrorDetails(errResp *gen.ErrorResponse) {
	if errResp == nil || errResp.Details == nil {
		return
	}
	for _, d := range *errResp.Details {
		field := ""
		message := ""
		if d.Field != nil {
			field = *d.Field
		}
		if d.Message != nil {
			message = *d.Message
		}
		slog.Debug("API error detail", "field", field, "message", message)
	}
}
