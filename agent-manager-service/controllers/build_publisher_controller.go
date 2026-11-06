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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// maxBuildWorkloadBodyBytes bounds the build publisher request. Endpoint schemas
// (inline OpenAPI documents) make up most of it; OpenChoreo and the API server
// enforce the real object size limits.
const maxBuildWorkloadBodyBytes = 4 << 20

// BuildPublisherController handles the build publisher endpoint, called by a source
// build's workload step with its organization-scoped build token.
type BuildPublisherController interface {
	PublishBuildWorkload(w http.ResponseWriter, r *http.Request)
}

type buildPublisherController struct {
	service services.BuildPublisherService
}

// NewBuildPublisherController creates the build publisher controller.
func NewBuildPublisherController(service services.BuildPublisherService) BuildPublisherController {
	return &buildPublisherController{service: service}
}

// PublishBuildWorkload handles POST /build-publisher/builds/{buildName}/workload.
func (c *buildPublisherController) PublishBuildWorkload(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	buildName := r.PathValue(utils.PathParamBuildName)

	var req models.PublishBuildWorkloadRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBuildWorkloadBodyBytes))
	dec.DisallowUnknownFields()
	err := dec.Decode(&req)
	if err == nil && dec.More() {
		err = errors.New("unexpected data after the request body")
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			utils.WriteErrorResponse(w, http.StatusRequestEntityTooLarge, "Request body is too large")
			return
		}
		log.Warn("Failed to parse build workload request", "buildName", buildName, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// The organization comes only from the build token, never from the request:
	// jwtassertion.BuildClientAuthMiddleware admitted the caller because its token
	// is a build token, issued for that organization.
	ouID := jwtassertion.BuildClientOrg(r.Context())
	if err := c.service.PublishWorkload(r.Context(), ouID, buildName, &req); err != nil {
		switch {
		case errors.Is(err, utils.ErrBadRequest):
			log.Warn("Rejected build workload", "buildName", buildName, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid build workload")
		case errors.Is(err, utils.ErrNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Build not found")
		case errors.Is(err, utils.ErrForbidden):
			log.Warn("Refused build workload", "buildName", buildName, "error", err)
			utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
		case errors.Is(err, services.ErrBuildWorkloadAlreadyPublished):
			log.Warn("Refused build workload: the build already published a different one", "buildName", buildName)
			utils.WriteErrorResponse(w, http.StatusConflict, "Build already published a different workload")
		case errors.Is(err, utils.ErrConflict):
			utils.WriteErrorResponse(w, http.StatusConflict, "Build has already finished")
		case errors.Is(err, audit.ErrRecorderUnavailable):
			// The publish is recorded fail-closed; the build step retries a 503.
			log.Error("Refused build workload: audit trail unavailable", "buildName", buildName, "error", err)
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "Audit trail unavailable; retry later")
		default:
			log.Error("Failed to publish build workload", "buildName", buildName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to publish build workload")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"message":"workload published"}`)); err != nil {
		log.Error("Failed to write response", "error", err)
	}
}
