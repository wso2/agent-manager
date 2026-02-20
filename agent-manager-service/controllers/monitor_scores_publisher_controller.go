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
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// MonitorScoresPublisherController defines the interface for monitor scores publishing HTTP handlers
type MonitorScoresPublisherController interface {
	PublishScores(w http.ResponseWriter, r *http.Request)
}

type monitorScoresPublisherController struct {
	scoresService *services.MonitorScoresService
	apiKey        string
}

// NewMonitorScoresPublisherController creates a new monitor scores publisher controller
func NewMonitorScoresPublisherController(
	scoresService *services.MonitorScoresService,
	cfg config.Config,
) MonitorScoresPublisherController {
	return &monitorScoresPublisherController{
		scoresService: scoresService,
		apiKey:        cfg.InternalServer.APIKey,
	}
}

// PublishScores handles POST /monitors/{monitorId}/runs/{runId}/scores
// Accepts evaluation scores from the Python runner and stores them in the database
func (c *monitorScoresPublisherController) PublishScores(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	// Validate API key (using x-api-key header to match Python client)
	providedKey := r.Header.Get("x-api-key")
	if c.apiKey == "" || subtle.ConstantTimeCompare([]byte(providedKey), []byte(c.apiKey)) != 1 {
		log.Warn("Unauthorized access attempt to publish scores", "ip", getClientIP(r))
		http.Error(w, "Invalid or missing API key", http.StatusUnauthorized)
		return
	}

	// Parse path parameters
	monitorID, err := uuid.Parse(r.PathValue("monitorId"))
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid monitor ID")
		return
	}

	runID, err := uuid.Parse(r.PathValue("runId"))
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid run ID")
		return
	}

	// Parse request body
	var req models.PublishScoresRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("Failed to parse publish scores request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request has data
	if len(req.IndividualScores) == 0 && len(req.AggregatedScores) == 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "At least one individual score or aggregated score is required")
		return
	}

	// Publish scores via service
	if err := c.scoresService.PublishScores(monitorID, runID, &req); err != nil {
		log.Error("Failed to publish scores", "monitorId", monitorID, "runId", runID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to publish scores")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write([]byte(`{"message":"scores published successfully"}`)); err != nil {
		log.Error("Failed to write response", "error", err)
	}
}
