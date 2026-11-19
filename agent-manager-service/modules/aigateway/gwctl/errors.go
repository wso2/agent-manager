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

package gwctl

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// handleCommonErrors writes the response for an error the gateway's services
// share: a hook refusal, a validation error, a missing core resource or a
// generic httpx class; anything else is a 500 with fallbackMsg.
func handleCommonErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	var rejected *modkit.Error
	switch {
	case errors.Is(err, httpx.ErrNotImplemented):
		httpx.WriteErrorResponse(w, http.StatusNotImplemented, err.Error())
	case errors.As(err, &rejected):
		httpx.WriteErrorResponse(w, rejected.Status, rejected.Message)
	// Keep this before the httpx classes: a ValidationError may also match one,
	// and its own message is the useful one.
	case httpx.IsValidationError(err) != nil:
		httpx.WriteValidationErrorResponse(w, err)
	case errors.Is(err, gwerr.ErrOrganizationNotFound):
		httpx.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Organization not found", err.Error(), gwerr.ErrCodeOrganizationNotFound)
	case errors.Is(err, gwerr.ErrProjectNotFound):
		httpx.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Project not found", err.Error(), gwerr.ErrCodeProjectNotFound)
	case errors.Is(err, gwerr.ErrAgentNotFound):
		httpx.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Agent not found", err.Error(), gwerr.ErrCodeAgentNotFound)
	case errors.Is(err, gwerr.ErrEnvironmentNotFound):
		httpx.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Environment not found", err.Error(), gwerr.ErrCodeEnvironmentNotFound)
	case errors.Is(err, httpx.ErrConflict):
		httpx.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Conflict", err.Error(), httpx.ErrCodeConflict)
	case errors.Is(err, httpx.ErrInvalidInput):
		httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Invalid input provided", err.Error(), httpx.ErrCodeValidation)
	case errors.Is(err, httpx.ErrBadRequest):
		httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Bad request", err.Error(), httpx.ErrCodeBadRequest)
	case errors.Is(err, httpx.ErrUnauthorized):
		httpx.WriteErrorResponseWithReason(w, http.StatusUnauthorized,
			"Unauthorized", err.Error(), httpx.ErrCodeUnauthorized)
	case errors.Is(err, httpx.ErrForbidden):
		httpx.WriteErrorResponseWithReason(w, http.StatusForbidden,
			"Forbidden", err.Error(), httpx.ErrCodeForbidden)
	case errors.Is(err, httpx.ErrServiceUnavailable):
		httpx.WriteErrorResponseWithReason(w, http.StatusServiceUnavailable,
			"Service temporarily unavailable", err.Error(), httpx.ErrCodeServiceUnavailable)
	default:
		httpx.WriteErrorResponseWithReason(w, http.StatusInternalServerError,
			fallbackMsg, "Internal server error", httpx.ErrCodeInternalError)
	}
}

// handleEnvironmentErrors writes the response for an environment lookup's
// error.
func handleEnvironmentErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, gwerr.ErrEnvironmentNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
	case errors.Is(err, httpx.ErrConflict):
		httpx.WriteErrorResponse(w, http.StatusConflict, "Environment already exists")
	case errors.Is(err, httpx.ErrInvalidInput):
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
	default:
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, fallbackMsg)
	}
}

// validateSystemGroup hides the identity provider's administrators group.
func validateSystemGroup(w http.ResponseWriter, groupName string) bool {
	if groupName == identityv1.AdministratorsGroupName {
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
		return false
	}
	return true
}

// validateReservedName refuses a requested name that the identity provider's
// seeded principals hold; an empty name (no rename) passes.
func validateReservedName(w http.ResponseWriter, requested, kind string, reserved ...string) bool {
	for _, r := range reserved {
		if requested == r {
			httpx.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("%q is a reserved %s name", r, kind))
			return false
		}
	}
	return true
}

func validateReservedGroupName(w http.ResponseWriter, name string) bool {
	return validateReservedName(w, name, "group", identityv1.AdministratorsGroupName)
}

func validateReservedRoleName(w http.ResponseWriter, name string) bool {
	return validateReservedName(w, name, "role", identityv1.AdministratorRoleName, identityv1.SystemClientRoleName)
}
