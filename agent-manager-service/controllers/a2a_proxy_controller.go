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

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A2AProxyController routes an external A2A agent through an environment's gateway.
type A2AProxyController interface {
	GetA2AProxy(w http.ResponseWriter, r *http.Request)
	SetA2AProxy(w http.ResponseWriter, r *http.Request)
	DeleteA2AProxy(w http.ResponseWriter, r *http.Request)
	RefreshA2AProxy(w http.ResponseWriter, r *http.Request)
}

type a2aProxyController struct {
	proxyService services.A2AProxyService
}

// NewA2AProxyController creates an A2AProxyController.
func NewA2AProxyController(proxyService services.A2AProxyService) A2AProxyController {
	return &a2aProxyController{proxyService: proxyService}
}

func (c *a2aProxyController) GetA2AProxy(w http.ResponseWriter, r *http.Request) {
	ouID, projName, agentName, envName := agentCardPath(r)
	status, err := c.proxyService.GetA2AProxy(r.Context(), ouID, projName, agentName, envName)
	if err != nil {
		writeA2AProxyError(w, r, err, "Failed to get the A2A proxy")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK,
		spec.NewA2AProxyResponse(string(status.Status), status.LastError, status.UpstreamAuthHeader, status.GatewayURL))
}

func (c *a2aProxyController) SetA2AProxy(w http.ResponseWriter, r *http.Request) {
	ouID, projName, agentName, envName := agentCardPath(r)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req spec.SetA2AProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	err := c.proxyService.SetA2AProxy(r.Context(), ouID, projName, agentName, envName, toA2AProxyAuthUpdate(req.UpstreamAuth))
	if err != nil {
		writeA2AProxyError(w, r, err, "Failed to set the A2A proxy")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (c *a2aProxyController) DeleteA2AProxy(w http.ResponseWriter, r *http.Request) {
	ouID, projName, agentName, envName := agentCardPath(r)
	if err := c.proxyService.DeleteA2AProxy(r.Context(), ouID, projName, agentName, envName); err != nil {
		writeA2AProxyError(w, r, err, "Failed to remove the A2A proxy")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *a2aProxyController) RefreshA2AProxy(w http.ResponseWriter, r *http.Request) {
	ouID, projName, agentName, envName := agentCardPath(r)
	if err := c.proxyService.RefreshA2AProxy(r.Context(), ouID, projName, agentName, envName); err != nil {
		writeA2AProxyError(w, r, err, "Failed to refresh the A2A proxy")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// toA2AProxyAuthUpdate keeps the request's three cases apart: no auth, a header alone, a header and a value.
func toA2AProxyAuthUpdate(auth *spec.A2AProxyUpstreamAuth) *models.A2AProxyAuthUpdate {
	if auth == nil {
		return nil
	}
	return &models.A2AProxyAuthUpdate{Header: auth.Header, Value: auth.Value}
}

// writeA2AProxyError maps the proxy sentinels, then defers to handleCommonErrors (which owns the
// 503 for an audit refusal, shared with the card-source routes).
func writeA2AProxyError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	logger.GetLogger(r.Context()).Warn("A2A proxy request failed", "path", r.URL.Path, "error", err)
	switch {
	case errors.Is(err, utils.ErrEnvironmentNotProxied):
		utils.WriteErrorResponse(w, http.StatusNotFound, err.Error())
	case errors.Is(err, utils.ErrA2AProxyNeedsCardSource):
		utils.WriteErrorResponse(w, http.StatusConflict, err.Error())
	case errors.Is(err, utils.ErrAgentNotA2A),
		errors.Is(err, utils.ErrA2AProxyAgentNotExternal),
		errors.Is(err, utils.ErrInvalidUpstreamAuth),
		errors.Is(err, utils.ErrA2AProxyUpstreamAuthNeedsHTTPS),
		errors.Is(err, utils.ErrA2ACardURLNotProxyable):
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	default:
		handleCommonErrors(w, err, fallback)
	}
}
