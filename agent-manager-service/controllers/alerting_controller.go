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
	"strconv"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// AlertingController serves the org alert endpoint and monitor alert config.
type AlertingController interface {
	GetAlertEndpoint(w http.ResponseWriter, r *http.Request)
	UpsertAlertEndpoint(w http.ResponseWriter, r *http.Request)
	DeleteAlertEndpoint(w http.ResponseWriter, r *http.Request)
	TestAlertEndpoint(w http.ResponseWriter, r *http.Request)
	ListAlertDeliveries(w http.ResponseWriter, r *http.Request)
	GetMonitorAlertConfig(w http.ResponseWriter, r *http.Request)
	UpdateMonitorAlertConfig(w http.ResponseWriter, r *http.Request)
}

type alertingController struct {
	alertingService services.AlertingService
}

// NewAlertingController creates an AlertingController.
func NewAlertingController(alertingService services.AlertingService) AlertingController {
	return &alertingController{alertingService: alertingService}
}

func (c *alertingController) GetAlertEndpoint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	endpoint, err := c.alertingService.GetEndpoint(ctx, middleware.OUIDFromRequest(r))
	if err != nil {
		writeAlertingError(w, r, err, "Failed to get alert endpoint")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toAlertEndpointResponse(endpoint, ""))
}

func (c *alertingController) UpsertAlertEndpoint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req spec.AlertEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	in := services.UpsertAlertEndpointInput{
		URL:                     req.Url,
		Enabled:                 req.Enabled == nil || *req.Enabled,
		RegenerateSigningSecret: req.RegenerateSigningSecret != nil && *req.RegenerateSigningSecret,
	}
	if req.Headers != nil {
		in.Headers = *req.Headers
		if in.Headers == nil {
			in.Headers = map[string]string{}
		}
	}
	endpoint, secret, err := c.alertingService.UpsertEndpoint(ctx, middleware.OUIDFromRequest(r), in)
	if err != nil {
		writeAlertingError(w, r, err, "Failed to save alert endpoint")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toAlertEndpointResponse(endpoint, secret))
}

func (c *alertingController) DeleteAlertEndpoint(w http.ResponseWriter, r *http.Request) {
	if err := c.alertingService.DeleteEndpoint(r.Context(), middleware.OUIDFromRequest(r)); err != nil {
		writeAlertingError(w, r, err, "Failed to delete alert endpoint")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *alertingController) TestAlertEndpoint(w http.ResponseWriter, r *http.Request) {
	result, err := c.alertingService.TestEndpoint(r.Context(), middleware.OUIDFromRequest(r))
	if err != nil {
		writeAlertingError(w, r, err, "Failed to send test alert")
		return
	}
	resp := spec.AlertTestResponse{Delivered: result.Delivered}
	if result.StatusCode != 0 {
		code := int32(result.StatusCode)
		resp.StatusCode = &code
	}
	if result.Error != "" {
		resp.Error = &result.Error
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *alertingController) ListAlertDeliveries(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsed
	}
	deliveries, err := c.alertingService.ListDeliveries(r.Context(), middleware.OUIDFromRequest(r), limit)
	if err != nil {
		writeAlertingError(w, r, err, "Failed to list alert deliveries")
		return
	}
	resp := spec.AlertDeliveryListResponse{Deliveries: make([]spec.AlertDeliveryResponse, 0, len(deliveries))}
	for i := range deliveries {
		resp.Deliveries = append(resp.Deliveries, toAlertDeliveryResponse(&deliveries[i]))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *alertingController) GetMonitorAlertConfig(w http.ResponseWriter, r *http.Request) {
	cfg, configured, err := c.alertingService.GetMonitorAlertConfig(r.Context(),
		middleware.OUIDFromRequest(r),
		r.PathValue(utils.PathParamProjName),
		r.PathValue(utils.PathParamAgentName),
		r.PathValue(utils.PathParamMonitorName))
	if err != nil {
		writeAlertingError(w, r, err, "Failed to get monitor alert configuration")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toMonitorAlertConfigResponse(cfg, configured))
}

func (c *alertingController) UpdateMonitorAlertConfig(w http.ResponseWriter, r *http.Request) {
	var req spec.MonitorAlertConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	cfg := models.MonitorAlertConfig{
		Enabled:             req.Enabled,
		AlertOnRunFailure:   req.AlertOnRunFailure == nil || *req.AlertOnRunFailure,
		Thresholds:          make([]models.MonitorAlertThreshold, 0, len(req.Thresholds)),
		CooldownMinutes:     models.DefaultAlertCooldownMinutes,
		ConsecutiveBreaches: models.DefaultAlertConsecutiveBreaches,
	}
	if req.CooldownMinutes != nil {
		cfg.CooldownMinutes = int(*req.CooldownMinutes)
	}
	if req.ConsecutiveBreaches != nil {
		cfg.ConsecutiveBreaches = int(*req.ConsecutiveBreaches)
	}
	for _, t := range req.Thresholds {
		op := ""
		if t.Operator != nil {
			op = *t.Operator
		}
		cfg.Thresholds = append(cfg.Thresholds, models.MonitorAlertThreshold{
			Evaluator:   t.Evaluator,
			Aggregation: t.Aggregation,
			Operator:    op,
			Value:       t.Value,
		})
	}

	saved, configured, err := c.alertingService.UpdateMonitorAlertConfig(r.Context(),
		middleware.OUIDFromRequest(r),
		r.PathValue(utils.PathParamProjName),
		r.PathValue(utils.PathParamAgentName),
		r.PathValue(utils.PathParamMonitorName),
		cfg)
	if err != nil {
		writeAlertingError(w, r, err, "Failed to save monitor alert configuration")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toMonitorAlertConfigResponse(saved, configured))
}

func writeAlertingError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, utils.ErrAlertEndpointNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "No alert endpoint is configured for this organization")
	case errors.Is(err, utils.ErrMonitorNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, audit.ErrRecorderUnavailable):
		logger.GetLogger(r.Context()).Error(fallback, "error", err)
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "Audit trail unavailable; the change was not applied")
	default:
		logger.GetLogger(r.Context()).Error(fallback, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, fallback)
	}
}

func toAlertEndpointResponse(e *models.AlertEndpoint, secret string) spec.AlertEndpointResponse {
	names := e.HeaderNames
	if names == nil {
		names = []string{}
	}
	resp := spec.AlertEndpointResponse{
		Url:                 e.URL,
		Enabled:             e.Enabled,
		HeaderNames:         names,
		ConsecutiveFailures: int32(e.ConsecutiveFailures),
		LastSuccessAt:       e.LastSuccessAt,
		LastFailureAt:       e.LastFailureAt,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
	}
	if secret != "" {
		resp.SigningSecret = &secret
	}
	return resp
}

func toAlertDeliveryResponse(d *models.AlertDelivery) spec.AlertDeliveryResponse {
	resp := spec.AlertDeliveryResponse{
		Id:            d.ID.String(),
		EventId:       d.EventID,
		EventType:     d.EventType,
		Status:        d.Status,
		Attempts:      int32(d.Attempts),
		NextAttemptAt: d.NextAttemptAt,
		DeliveredAt:   d.DeliveredAt,
		CreatedAt:     d.CreatedAt,
	}
	if d.MonitorName != "" {
		name := d.MonitorName
		resp.MonitorName = &name
	}
	if d.LastResponseCode != nil {
		code := int32(*d.LastResponseCode)
		resp.LastResponseCode = &code
	}
	if d.LastError != "" {
		msg := d.LastError
		resp.LastError = &msg
	}
	return resp
}

func toMonitorAlertConfigResponse(cfg *models.MonitorAlertConfig, endpointConfigured bool) spec.MonitorAlertConfigResponse {
	thresholds := make([]spec.MonitorAlertThreshold, 0, len(cfg.Thresholds))
	for _, t := range cfg.Thresholds {
		op := t.Operator
		thresholds = append(thresholds, spec.MonitorAlertThreshold{
			Evaluator:   t.Evaluator,
			Aggregation: t.Aggregation,
			Operator:    &op,
			Value:       t.Value,
		})
	}
	var lastAlertedAt *time.Time
	if cfg.LastAlertedAt != nil {
		at := *cfg.LastAlertedAt
		lastAlertedAt = &at
	}
	return spec.MonitorAlertConfigResponse{
		Enabled:               cfg.Enabled,
		AlertOnRunFailure:     cfg.AlertOnRunFailure,
		Thresholds:            thresholds,
		CooldownMinutes:       int32(cfg.CooldownMinutes),
		ConsecutiveBreaches:   int32(cfg.ConsecutiveBreaches),
		OrgEndpointConfigured: endpointConfigured,
		LastAlertedAt:         lastAlertedAt,
	}
}
