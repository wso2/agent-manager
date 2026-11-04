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

package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// requireSelfHostedThen runs next only when the {gatewayID} path value names a
// self-hosted gateway of the caller's organization. Anything else, a managed gateway
// included, is a 404 before next can read or change it. next sees the gateway's UUID
// in {gatewayID}, so a name that resolved here cannot resolve differently there.
func (c *gatewayController) requireSelfHostedThen(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
		gateway, err := c.gatewayService.RequireSelfHosted(gatewayID, middleware.OUIDFromRequest(r))
		if err != nil {
			logSelfHostedLookupFailure(r, "self-hosted gateway route: gateway not usable", err)
			handleGatewayErrors(w, err, "Failed to get gateway")
			return
		}
		r.SetPathValue("gatewayID", gateway.UUID.String())
		next(w, r)
	}
}

// logSelfHostedLookupFailure logs a failed self-hosted gateway lookup: a 404 is an expected outcome
// of the route (a managed or unknown gateway), anything else is a fault.
func logSelfHostedLookupFailure(r *http.Request, msg string, err error) {
	log := logger.GetLogger(r.Context())
	if errors.Is(err, utils.ErrGatewayNotFound) {
		log.Warn(msg, "error", err)
		return
	}
	log.Error(msg, "error", err)
}

func (c *gatewayController) GetSelfHostedGateway(w http.ResponseWriter, r *http.Request) {
	c.requireSelfHostedThen(c.GetGateway)(w, r)
}

func (c *gatewayController) DeleteSelfHostedGateway(w http.ResponseWriter, r *http.Request) {
	c.requireSelfHostedThen(c.DeleteGateway)(w, r)
}

func (c *gatewayController) ListSelfHostedGatewayTokens(w http.ResponseWriter, r *http.Request) {
	c.requireSelfHostedThen(c.ListGatewayTokens)(w, r)
}

func (c *gatewayController) RotateSelfHostedGatewayToken(w http.ResponseWriter, r *http.Request) {
	c.requireSelfHostedThen(func(w http.ResponseWriter, r *http.Request) {
		c.rotateGatewayToken(w, r, http.StatusCreated)
	})(w, r)
}

func (c *gatewayController) RevokeSelfHostedGatewayToken(w http.ResponseWriter, r *http.Request) {
	c.requireSelfHostedThen(c.RevokeGatewayToken)(w, r)
}

func (c *gatewayController) ListSelfHostedGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	limit, offset := gatewayPagination(r)

	gatewaysResp, err := c.gatewayService.ListSelfHostedGateways(ouID, limit, offset)
	if err != nil {
		log.Error("ListSelfHostedGateways: failed to list gateways", "error", err)
		handleGatewayErrors(w, err, "Failed to list gateways")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, c.toSpecGatewayList(ctx, ouID, gatewaysResp, limit, offset))
}

func (c *gatewayController) RegisterSelfHostedGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	var req spec.SelfHostedGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("RegisterSelfHostedGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	env, err := c.resolveEnvironment(ctx, ouID, req.Environment)
	if err != nil {
		log.Error("RegisterSelfHostedGateway: failed to resolve environment", "environment", req.Environment, "error", err)
		handleGatewayErrors(w, err, "Failed to resolve environment")
		return
	}

	input := services.SelfHostedGatewayInput{
		DisplayName:       req.DisplayName,
		Handle:            req.Handle,
		Description:       derefString(req.Description),
		Environment:       env.UUID,
		FunctionalityType: derefString(req.FunctionalityType),
		Version:           derefString(req.Version),
		Endpoints:         req.Endpoints,
		IsCritical:        req.IsCritical != nil && *req.IsCritical,
		Properties:        req.Properties,
	}
	gateway, err := c.gatewayService.RegisterSelfHostedGateway(ouID, input)
	vhost := ""
	if gateway != nil {
		vhost = gateway.Vhost
	}
	audit.Record(
		ctx, audit.ActionGatewayCreate,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceGateway, gatewayResponseID(gateway), req.DisplayName),
		audit.Environment(env.UUID),
		audit.Detail("gatewayName", req.DisplayName),
		audit.Detail("gatewayType", services.SelfHostedFunctionalityType(input.FunctionalityType)),
		audit.Detail("vhost", vhost),
		audit.Result(err),
	)
	if err != nil {
		log.Error("RegisterSelfHostedGateway: failed to register gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to register gateway")
		return
	}

	// A self-hosted gateway is mapped to exactly the environment it was registered in.
	environments := []models.GatewayEnvironmentResponse{gatewayEnvironmentResponse(env, ouID)}
	utils.WriteSuccessResponse(w, http.StatusCreated, convertGatewayToSpecResponse(gateway, ouID, environments))
}

// UpdateSelfHostedGateway changes only the fields a self-hosted gateway lets its owner
// edit. Unlike UpdateGateway it takes description and properties, and no runtime URL.
func (c *gatewayController) UpdateSelfHostedGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	var req spec.SelfHostedGatewayUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateSelfHostedGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	existing, err := c.gatewayService.RequireSelfHosted(gatewayID, ouID)
	if err != nil {
		logSelfHostedLookupFailure(r, "UpdateSelfHostedGateway: gateway not usable", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}

	var properties *map[string]interface{}
	if req.Properties != nil {
		properties = &req.Properties
	}
	gateway, err := c.gatewayService.UpdateGateway(existing.UUID.String(), ouID, req.Description, req.DisplayName, req.IsCritical, properties, nil)
	audit.Record(
		ctx, audit.ActionGatewayUpdate,
		audit.Org(ouID),
		audit.Resource(audit.ResourceGateway, existing.UUID.String()),
		audit.Result(err),
	)
	if err != nil {
		log.Error("UpdateSelfHostedGateway: failed to update gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}

	environments := c.getGatewayEnvironmentsFromDB(ctx, ouID, gateway.ID)
	utils.WriteSuccessResponse(w, http.StatusOK, convertGatewayToSpecResponse(gateway, ouID, environments))
}
