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
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/ssrf"
)

// A2AAgentCardController serves an A2A agent's stored card per environment.
type A2AAgentCardController interface {
	GetAgentCard(w http.ResponseWriter, r *http.Request)
	RefreshAgentCard(w http.ResponseWriter, r *http.Request)
	SetAgentCardSource(w http.ResponseWriter, r *http.Request)
	DeleteAgentCardSource(w http.ResponseWriter, r *http.Request)
}

type a2aAgentCardController struct {
	cardService gwsvc.A2AAgentCardService
}

// NewA2AAgentCardController creates an A2AAgentCardController.
func NewA2AAgentCardController(cardService gwsvc.A2AAgentCardService) A2AAgentCardController {
	return &a2aAgentCardController{cardService: cardService}
}

func (c *a2aAgentCardController) GetAgentCard(w http.ResponseWriter, r *http.Request) {
	ouID, projName, agentName, envName := agentCardPath(r)
	card, err := c.cardService.GetA2AAgentCard(r.Context(), ouID, projName, agentName, envName)
	if err != nil {
		writeAgentCardError(w, r, err, "Failed to get agent card")
		return
	}
	resp, err := toAgentCardResponse(card)
	if err != nil {
		writeAgentCardError(w, r, err, "Failed to get agent card")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *a2aAgentCardController) RefreshAgentCard(w http.ResponseWriter, r *http.Request) {
	ouID, projName, agentName, envName := agentCardPath(r)
	if err := c.cardService.RefreshA2AAgentCard(r.Context(), ouID, projName, agentName, envName); err != nil {
		writeAgentCardError(w, r, err, "Failed to refresh agent card")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (c *a2aAgentCardController) SetAgentCardSource(w http.ResponseWriter, r *http.Request) {
	ouID, projName, agentName, envName := agentCardPath(r)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req gwspec.SetAgentCardSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := c.cardService.SetA2ACardSource(r.Context(), ouID, projName, agentName, envName, req.Url); err != nil {
		writeAgentCardError(w, r, err, "Failed to set agent card source")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (c *a2aAgentCardController) DeleteAgentCardSource(w http.ResponseWriter, r *http.Request) {
	ouID, projName, agentName, envName := agentCardPath(r)
	if err := c.cardService.DeleteA2ACardSource(r.Context(), ouID, projName, agentName, envName); err != nil {
		writeAgentCardError(w, r, err, "Failed to delete agent card source")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func agentCardPath(r *http.Request) (ouID, projName, agentName, envName string) {
	return middleware.OUIDFromRequest(r),
		r.PathValue(gwutil.PathParamProjName),
		r.PathValue(gwutil.PathParamAgentName),
		r.PathValue(gwutil.PathParamEnvID)
}

// writeAgentCardError maps the card sentinels, then defers to handleCommonErrors.
func writeAgentCardError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	logger.GetLogger(r.Context()).Warn("Agent card request failed", "path", r.URL.Path, "error", err)
	switch {
	case errors.Is(err, gwerr.ErrAgentCardNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Agent card not found")
	case errors.Is(err, gwerr.ErrAgentNotDeployedInEnvironment):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Agent is not deployed in this environment")
	case errors.Is(err, gwerr.ErrAgentNotA2A),
		errors.Is(err, gwerr.ErrAgentCardSourceNotExternal),
		errors.Is(err, ssrf.ErrInvalidURL),
		errors.Is(err, gwerr.ErrA2ACardURLNotProxyable):
		httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, gwerr.ErrA2AProxyOriginChangeNeedsAuthReset):
		httpx.WriteErrorResponse(w, http.StatusConflict, gwerr.ErrA2AProxyOriginChangeNeedsAuthReset.Error())
	default:
		controllers.HandleCommonErrors(w, err, fallback)
	}
}

func toAgentCardResponse(card *gwmodels.A2AAgentCard) (*gwspec.AgentCardResponse, error) {
	resp := gwspec.NewAgentCardResponse(string(card.Source), string(card.Status), card.SourceURL, card.LastError)
	if len(card.Card) > 0 {
		dec := json.NewDecoder(bytes.NewReader(card.Card))
		dec.UseNumber()
		var body map[string]interface{}
		if err := dec.Decode(&body); err != nil {
			return nil, err
		}
		resp.SetCard(body)
	}
	if card.FetchedAt != nil {
		resp.SetFetchedAt(*card.FetchedAt)
	}
	return resp, nil
}
