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
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
)

type BuildCallbackPayload struct {
	AgentName   string `json:"agentName"`
	ProjectName string `json:"projectName"`
	OrgId       uuid.UUID `json:"orgId"`
}

type BuildCIController interface {
	HandleBuildCallback(w http.ResponseWriter, r *http.Request)
}

type buildCIController struct {
	buildCIManagerService services.BuildCIManagerService
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

func NewBuildCIController(buildCIManagerService services.BuildCIManagerService) BuildCIController {
	return &buildCIController{
		buildCIManagerService: buildCIManagerService,
	}
}

func (b *buildCIController) HandleBuildCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	var payload BuildCallbackPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("HandleBuildCallback: failed to decode request body", "error", err)
		writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	log.Info("Build callback received", "payload", payload)

	workloadCR, err := b.buildCIManagerService.HandleBuildCallback(ctx, payload.OrgId, payload.ProjectName, payload.AgentName)
	if err != nil {
		log.Error("HandleBuildCallback: failed to process callback", "error", err)
		writeJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to process build callback"})
		return
	}
	writeJSONResponse(w, http.StatusOK, workloadCR)
}
