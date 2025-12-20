// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type ObservabilityController interface {
	ListTraces(w http.ResponseWriter, r *http.Request)
	GetTrace(w http.ResponseWriter, r *http.Request)
}

type observabilityController struct {
	observabilityService services.ObservabilityManagerService
}

// NewObservabilityController returns a new ObservabilityController instance.
func NewObservabilityController(observabilityService services.ObservabilityManagerService) ObservabilityController {
	return &observabilityController{
		observabilityService: observabilityService,
	}
}

func (c *observabilityController) ListTraces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = "10"
	}
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		offsetStr = "0"
	}

	// Parse and validate pagination parameters
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		log.Error("ListTraces: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid limit parameter: must be between 1 and 100")
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		log.Error("ListTraces: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid offset parameter: must be 0 or greater")
		return
	}

	// Optional query parameters
	environment := r.URL.Query().Get("environment")
	if environment == "" {
		log.Error("ListTraces: environment is required")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing parameter: environment is required")
		return
	}

	startTime := r.URL.Query().Get("startTime")
	endTime := r.URL.Query().Get("endTime")

	// Validate time range parameters if provided
	if startTime != "" || endTime != "" {
		if startTime == "" {
			log.Error("ListTraces: startTime is required")
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing parameter: startTime is required")
			return
		}
		if endTime == "" {
			log.Error("ListTraces: endTime is required")
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing parameter: endTime is required")
			return
		}

		// Validate RFC3339 format for startTime
		if _, err := time.Parse(time.RFC3339, startTime); err != nil {
			log.Error("ListTraces: invalid startTime format", "startTime", startTime, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid startTime format: must be RFC3339 (e.g., 2025-12-20T10:00:00Z)")
			return
		}

		// Validate RFC3339 format for endTime
		if _, err := time.Parse(time.RFC3339, endTime); err != nil {
			log.Error("ListTraces: invalid endTime format", "endTime", endTime, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid endTime format: must be RFC3339 (e.g., 2025-12-20T10:00:00Z)")
			return
		}
	}

	sortOrder := r.URL.Query().Get("sortOrder")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		log.Error("ListTraces: invalid sortOrder parameter", "sortOrder", sortOrder)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid sortOrder parameter: must be 'asc' or 'desc'")
		return
	}

	// Build parameters for the service
	params := services.ListTracesRequest{
		OrgName:     orgName,
		ProjectName: projName,
		AgentName:   agentName,
		Environment: environment,
		StartTime:   startTime,
		EndTime:     endTime,
		Limit:       limit,
		Offset:      offset,
		SortOrder:   sortOrder,
	}

	// Call the service
	response, err := c.observabilityService.ListTraces(ctx, params)
	if err != nil {
		log.Error("ListTraces: failed to list traces", "serviceName", agentName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve traces")
		return
	}

	log.Info("ListTraces: successfully retrieved traces", "serviceName", agentName, "totalCount", response.TotalCount)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *observabilityController) GetTrace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	traceID := r.PathValue(utils.PathParamTraceId)

	// Validate traceID
	if traceID == "" {
		log.Error("GetTrace: traceId is required")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing parameter: traceId is required")
		return
	}

	// Optional query parameters
	environment := r.URL.Query().Get("environment")
	if environment == "" {
		log.Error("ListTraces: environment is required")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing parameter: environment is required")
		return
	}

	// Build parameters for the service
	params := services.TraceDetailsRequest{
		TraceID:     traceID,
		OrgName:     orgName,
		ProjectName: projName,
		AgentName:   agentName,
		Environment: environment,
	}

	// Call the service
	response, err := c.observabilityService.GetTraceDetails(ctx, params)
	if err != nil {
		// Check if it's a "not found" error
		if errors.Is(err, services.ErrTraceNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Trace not found")
			return
		}
		// Other errors are internal server errors
		log.Error("GetTrace: failed to get trace details", "traceId", traceID, "agentName", agentName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve trace details")
		return
	}

	log.Info("GetTrace: successfully retrieved trace details", "traceId", traceID, "agentName", agentName, "spanCount", response.TotalCount)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}
