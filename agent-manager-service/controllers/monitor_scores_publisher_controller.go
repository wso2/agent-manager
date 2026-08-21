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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MonitorScoresPublisherController defines the interface for monitor scores publishing HTTP handlers
type MonitorScoresPublisherController interface {
	PublishScores(w http.ResponseWriter, r *http.Request)
}

type monitorScoresPublisherController struct {
	scoresService *services.MonitorScoresService
}

// NewMonitorScoresPublisherController creates a new monitor scores publisher controller
func NewMonitorScoresPublisherController(
	scoresService *services.MonitorScoresService,
) MonitorScoresPublisherController {
	return &monitorScoresPublisherController{
		scoresService: scoresService,
	}
}

// PublishScores handles POST /monitors/{monitorId}/runs/{runId}/scores
// Accepts evaluation scores from the Python runner and stores them in the database
func (c *monitorScoresPublisherController) PublishScores(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	// Parse path parameters
	monitorID, err := uuid.Parse(r.PathValue(utils.PathParamMonitorId))
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid monitor ID")
		return
	}

	runID, err := uuid.Parse(r.PathValue(utils.PathParamRunId))
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

	// This route carries no rbac.Permission, so this record is the sole account of
	// who wrote scores for a run. The actor is a machine client, not a user.
	publishErr := c.scoresService.PublishScores(monitorID, runID, publisherOUID(r.Context()), &req)
	audit.Record(
		r.Context(), audit.ActionMonitorScorePublish,
		audit.ResourceNamed("monitor-run", runID.String(), runID.String()),
		audit.SurfaceOpt(audit.SurfacePublisher),
		audit.Actor(audit.ActorService, publisherAudience(r.Context()), ""),
		audit.AuthMethod("publisher-client"),
		audit.Detail("monitorId", monitorID.String()),
		audit.Detail("runId", runID.String()),
		audit.Detail("scoreCount", len(req.IndividualScores)+len(req.AggregatedScores)),
		audit.Result(publishErr),
	)
	if publishErr != nil {
		log.Warn("Failed to publish scores", "monitor_id", monitorID, "run_id", runID, "error", publishErr)
		switch {
		case errors.Is(publishErr, utils.ErrForbidden):
			utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
		case errors.Is(publishErr, utils.ErrNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor run not found")
		default:
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to publish scores")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write([]byte(`{"message":"scores published successfully"}`)); err != nil {
		log.Error("Failed to write response", "error", err)
	}
}

// publisherOUID returns the token's organization; empty is rejected downstream.
func publisherOUID(ctx context.Context) string {
	claims := jwtassertion.GetTokenClaims(ctx)
	if claims == nil {
		return ""
	}
	return claims.OuId
}

// publisherAudience returns the publisher client identity behind a score
// publish. The route is authorized by matching an "amp-publisher-*" audience
// rather than a subject, so the audience is the only actor identity available.
func publisherAudience(ctx context.Context) string {
	claims := jwtassertion.GetTokenClaims(ctx)
	if claims == nil {
		return ""
	}
	for _, aud := range claims.Audience {
		if strings.HasPrefix(aud, "amp-publisher-") {
			return aud
		}
	}
	if claims.Sub != "" {
		return claims.Sub
	}
	return ""
}
