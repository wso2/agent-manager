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
	"fmt"
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// MonitorController defines the interface for monitor HTTP handlers
type MonitorController interface {
	CreateMonitor(w http.ResponseWriter, r *http.Request)
	GetMonitor(w http.ResponseWriter, r *http.Request)
	ListMonitors(w http.ResponseWriter, r *http.Request)
	DeleteMonitor(w http.ResponseWriter, r *http.Request)
	UpdateMonitor(w http.ResponseWriter, r *http.Request)
	StopMonitor(w http.ResponseWriter, r *http.Request)
	StartMonitor(w http.ResponseWriter, r *http.Request)
	ListMonitorRuns(w http.ResponseWriter, r *http.Request)
	RerunMonitor(w http.ResponseWriter, r *http.Request)
	GetMonitorRunLogs(w http.ResponseWriter, r *http.Request)
}

type monitorController struct {
	monitorService services.MonitorManagerService
}

// NewMonitorController creates a new monitor controller instance
func NewMonitorController(monitorService services.MonitorManagerService) MonitorController {
	return &monitorController{
		monitorService: monitorService,
	}
}

// CreateMonitor handles POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors
func (c *monitorController) CreateMonitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	projName := r.PathValue("projName")
	agentName := r.PathValue("agentName")

	var req spec.CreateMonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("Failed to parse request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Monitor name is required")
		return
	}
	if req.EnvironmentName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Environment name is required")
		return
	}
	if len(req.Evaluators) == 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "At least one evaluator is required")
		return
	}
	for i, eval := range req.Evaluators {
		if eval.Identifier == "" {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("evaluators[%d].identifier is required", i))
			return
		}
		if eval.DisplayName == "" {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("evaluators[%d].displayName is required", i))
			return
		}
	}
	if req.Type == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Monitor type is required (future or past)")
		return
	}
	if req.Type != models.MonitorTypeFuture && req.Type != models.MonitorTypePast {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Monitor type must be 'future' or 'past'")
		return
	}

	if !isValidDNSName(req.Name) {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Monitor name must be DNS-compatible (lowercase, alphanumeric, hyphens only)")
		return
	}

	// Convert spec request to models request with path parameters
	modelReq := utils.ConvertToCreateMonitorRequest(&req, projName, agentName)

	monitor, err := c.monitorService.CreateMonitor(ctx, orgName, modelReq)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorAlreadyExists) {
			utils.WriteErrorResponse(w, http.StatusConflict, "Monitor already exists")
			return
		}
		if errors.Is(err, utils.ErrAgentNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		}
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error("Failed to create monitor", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create monitor")
		return
	}

	// Convert to spec response
	response := utils.ConvertToMonitorResponse(monitor)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// GetMonitor handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}
func (c *monitorController) GetMonitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	monitorName := r.PathValue("monitorName")

	monitor, err := c.monitorService.GetMonitor(ctx, orgName, monitorName)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		log.Error("Failed to get monitor", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get monitor")
		return
	}

	// Convert to spec response
	response := utils.ConvertToMonitorResponse(monitor)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// ListMonitors handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors
func (c *monitorController) ListMonitors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	projName := r.PathValue("projName")
	agentName := r.PathValue("agentName")

	result, err := c.monitorService.ListMonitors(ctx, orgName, projName, agentName)
	if err != nil {
		log.Error("Failed to list monitors", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list monitors")
		return
	}

	// Convert to spec response
	response := utils.ConvertToMonitorListResponse(result)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// DeleteMonitor handles DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}
func (c *monitorController) DeleteMonitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	monitorName := r.PathValue("monitorName")

	err := c.monitorService.DeleteMonitor(ctx, orgName, monitorName)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		log.Error("Failed to delete monitor", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete monitor")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateMonitor handles PATCH /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}
func (c *monitorController) UpdateMonitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	monitorName := r.PathValue("monitorName")

	var req spec.UpdateMonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("Failed to parse request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate evaluator fields if provided
	for i, eval := range req.Evaluators {
		if eval.Identifier == "" {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("evaluators[%d].identifier is required", i))
			return
		}
		if eval.DisplayName == "" {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("evaluators[%d].displayName is required", i))
			return
		}
	}

	// Convert spec request to models request
	modelReq := utils.ConvertToUpdateMonitorRequest(&req)

	monitor, err := c.monitorService.UpdateMonitor(ctx, orgName, monitorName, modelReq)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error("Failed to update monitor", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update monitor")
		return
	}

	// Convert to spec response
	response := utils.ConvertToMonitorResponse(monitor)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// ListMonitorRuns handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs
func (c *monitorController) ListMonitorRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	monitorName := r.PathValue("monitorName")

	result, err := c.monitorService.ListMonitorRuns(ctx, orgName, monitorName)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		log.Error("Failed to list monitor runs", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list monitor runs")
		return
	}

	// Convert to spec response
	response := utils.ConvertToMonitorRunListResponse(result)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// RerunMonitor handles POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs/{runId}/rerun
func (c *monitorController) RerunMonitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	monitorName := r.PathValue("monitorName")
	runID := r.PathValue("runId")

	result, err := c.monitorService.RerunMonitor(ctx, orgName, monitorName, runID)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		if errors.Is(err, utils.ErrMonitorRunNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor run not found")
			return
		}
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error("Failed to rerun monitor", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to rerun monitor")
		return
	}

	// Convert to spec response
	response := utils.ConvertToMonitorRunResponse(result)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// GetMonitorRunLogs handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs/{runId}/logs
func (c *monitorController) GetMonitorRunLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	monitorName := r.PathValue("monitorName")
	runID := r.PathValue("runId")

	result, err := c.monitorService.GetMonitorRunLogs(ctx, orgName, monitorName, runID)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		if errors.Is(err, utils.ErrMonitorRunNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor run not found")
			return
		}
		log.Error("Failed to get monitor run logs", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get monitor run logs")
		return
	}

	logsResponse := utils.ConvertToLogsResponse(*result)
	utils.WriteSuccessResponse(w, http.StatusOK, logsResponse)
}

// StopMonitor handles POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/stop
func (c *monitorController) StopMonitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	monitorName := r.PathValue("monitorName")

	result, err := c.monitorService.StopMonitor(ctx, orgName, monitorName)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, utils.ErrMonitorAlreadyStopped) {
			utils.WriteErrorResponse(w, http.StatusConflict, "Monitor is already stopped")
			return
		}
		log.Error("Failed to stop monitor", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to stop monitor")
		return
	}

	// Convert to spec response
	response := utils.ConvertToMonitorResponse(result)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// StartMonitor handles POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/start
func (c *monitorController) StartMonitor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue("orgName")
	monitorName := r.PathValue("monitorName")

	result, err := c.monitorService.StartMonitor(ctx, orgName, monitorName)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, utils.ErrMonitorAlreadyActive) {
			utils.WriteErrorResponse(w, http.StatusConflict, "Monitor is already active")
			return
		}
		log.Error("Failed to start monitor", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to start monitor")
		return
	}

	// Convert to spec response
	response := utils.ConvertToMonitorResponse(result)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// isValidDNSName checks if the name is valid for Kubernetes resources
func isValidDNSName(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	for i, c := range name {
		if c >= 'a' && c <= 'z' {
			continue
		}
		if c >= '0' && c <= '9' {
			continue
		}
		if c == '-' && i > 0 && i < len(name)-1 {
			continue
		}
		return false
	}
	return true
}
