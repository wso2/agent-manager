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

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MonitorAlertController manages shared monitor alert settings.
type MonitorAlertController interface {
	GetMonitorAlert(w http.ResponseWriter, r *http.Request)
	PutMonitorAlert(w http.ResponseWriter, r *http.Request)
	DeleteMonitorAlert(w http.ResponseWriter, r *http.Request)
}

type monitorAlertController struct {
	svc services.MonitorAlertService
}

// NewMonitorAlertController creates a MonitorAlertController.
func NewMonitorAlertController(svc services.MonitorAlertService) MonitorAlertController {
	return &monitorAlertController{svc: svc}
}

func monitorRef(r *http.Request) services.MonitorRef {
	return services.MonitorRef{
		OUID:        middleware.OUIDFromRequest(r),
		ProjectName: r.PathValue(utils.PathParamProjName),
		AgentName:   r.PathValue(utils.PathParamAgentName),
		MonitorName: r.PathValue(utils.PathParamMonitorName),
	}
}

func (c *monitorAlertController) GetMonitorAlert(w http.ResponseWriter, r *http.Request) {
	alert, found, err := c.svc.GetAlert(r.Context(), monitorRef(r))
	if err != nil {
		writeNotificationError(w, r, err, "Failed to get monitor alert settings")
		return
	}
	if !found {
		// Defaults before alerting is configured.
		utils.WriteSuccessResponse(w, http.StatusOK, spec.MonitorAlertResponse{
			Enabled:         false,
			NotifyOnFailure: true,
			DestinationIds:  []string{},
			Thresholds:      []spec.MonitorScoreThreshold{},
		})
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toMonitorAlertResponse(alert))
}

func (c *monitorAlertController) PutMonitorAlert(w http.ResponseWriter, r *http.Request) {
	var req spec.MonitorAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	settings := services.MonitorAlertSettings{
		DestinationIDs:  req.DestinationIds,
		NotifyOnFailure: req.NotifyOnFailure,
		NotifyOnSuccess: req.NotifyOnSuccess,
		Thresholds:      make([]models.ScoreThreshold, 0, len(req.Thresholds)),
	}
	for _, t := range req.Thresholds {
		settings.Thresholds = append(settings.Thresholds, models.ScoreThreshold{
			Evaluator: t.Evaluator, Aggregation: t.Aggregation, Operator: t.Operator, Value: t.Value,
		})
	}
	alert, err := c.svc.SaveAlert(r.Context(), monitorRef(r), settings)
	if err != nil {
		writeNotificationError(w, r, err, "Failed to save monitor alert settings")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toMonitorAlertResponse(alert))
}

func (c *monitorAlertController) DeleteMonitorAlert(w http.ResponseWriter, r *http.Request) {
	if err := c.svc.DeleteAlert(r.Context(), monitorRef(r)); err != nil {
		writeNotificationError(w, r, err, "Failed to delete monitor alert settings")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeNotificationError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, utils.ErrMonitorNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	default:
		logger.GetLogger(r.Context()).Error(fallback, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, fallback)
	}
}

func toSpecThresholds(in []models.ScoreThreshold) []spec.MonitorScoreThreshold {
	out := make([]spec.MonitorScoreThreshold, 0, len(in))
	for _, t := range in {
		out = append(out, spec.MonitorScoreThreshold{
			Evaluator: t.Evaluator, Aggregation: t.Aggregation, Operator: t.Operator, Value: t.Value,
		})
	}
	return out
}

func toMonitorAlertResponse(alert *models.MonitorAlert) spec.MonitorAlertResponse {
	updated := alert.UpdatedAt
	return spec.MonitorAlertResponse{
		Enabled:         true,
		NotifyOnFailure: alert.NotifyOnFailure,
		DestinationIds:  append([]string{}, alert.DestinationIDs...),
		NotifyOnSuccess: alert.NotifyOnSuccess,
		Thresholds:      toSpecThresholds(alert.Thresholds),
		Breached:        toSpecThresholds(alert.BreachedThresholds()),
		UpdatedAt:       &updated,
	}
}
