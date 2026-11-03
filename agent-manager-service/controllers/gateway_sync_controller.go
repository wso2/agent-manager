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
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The gateway runs these routes once per process start (pkg/controlplane/sync.go at gateway/v2026.09.24): it lists
// what it should hold, fetches what it is missing in batches, and asks which of the artifacts it would delete still
// exist.

// maxSyncIDs caps the ids one fetch-batch or artifacts/exists request may name.
const maxSyncIDs = 1000

// maxSyncBodyBytes bounds the request body: maxSyncIDs ids with room for long LLM proxy handles.
const maxSyncBodyBytes = 1 << 20

const tarGzipMediaType = "application/x-tar+gzip"

type controlPlaneDeploymentsResponse struct {
	Deployments []models.ControlPlaneDeployment `json:"deployments"`
}

type artifactsExistResponse struct {
	Artifacts []models.ArtifactExistence `json:"artifacts"`
}

// syncIDsRequest is the body of fetch-batch ({deploymentIds}) and of artifacts/exists ({artifactIds}).
type syncIDsRequest struct {
	DeploymentIDs []string `json:"deploymentIds"`
	ArtifactIDs   []string `json:"artifactIds"`
}

func (r syncIDsRequest) deploymentIDs() []string { return r.DeploymentIDs }
func (r syncIDsRequest) artifactIDs() []string   { return r.ArtifactIDs }

// GetDeployments handles GET /api/internal/v1/deployments: every deployment the calling gateway was sent, in its
// desired state, keyed as the gateway stores the artifact. It only reads.
func (c *gatewayInternalController) GetDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}

	deployments, err := c.gatewayInternalService.ListGatewayDeployments(ctx, identity.OUID, identity.ID)
	if err != nil {
		logger.GetLogger(ctx).Error("Failed to list gateway deployments", "gatewayID", identity.ID, "error", err)
		http.Error(w, "Failed to list deployments", http.StatusInternalServerError)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, controlPlaneDeploymentsResponse{Deployments: deployments})
}

// FetchDeploymentBatch handles POST /api/internal/v1/deployments/fetch-batch: a tar.gz with one directory per
// deployment id holding the artifact's YAML. Ids that aren't the calling gateway's deployments are left out.
func (c *gatewayInternalController) FetchDeploymentBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}
	if !acceptsTarGzip(r.Header.Get("Accept")) {
		http.Error(w, "This endpoint only serves "+tarGzipMediaType, http.StatusNotAcceptable)
		return
	}
	deploymentIDs, ok := decodeSyncIDs(w, r, syncIDsRequest.deploymentIDs)
	if !ok {
		return
	}

	batch, err := c.gatewayInternalService.FetchDeploymentBatch(ctx, identity.OUID, identity.ID, deploymentIDs)
	if err != nil {
		log.Error("Failed to fetch gateway deployments", "gatewayID", identity.ID, "error", err)
		http.Error(w, "Failed to fetch deployments", http.StatusInternalServerError)
		return
	}
	archive, err := utils.CreateBatchDeploymentTarGz(batch)
	if err != nil {
		log.Error("Failed to package gateway deployments", "gatewayID", identity.ID, "error", err)
		http.Error(w, "Failed to package deployments", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", tarGzipMediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"deployments-batch-%s.tar.gz\"", identity.ID))
	w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(archive); err != nil {
		log.Error("Failed to write deployment batch", "gatewayID", identity.ID, "error", err)
	}
}

// CheckArtifactsExist handles POST /api/internal/v1/artifacts/exists. The gateway asks it before deleting artifacts
// missing from GET /deployments, and keeps every one reported as existing.
func (c *gatewayInternalController) CheckArtifactsExist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}
	artifactIDs, ok := decodeSyncIDs(w, r, syncIDsRequest.artifactIDs)
	if !ok {
		return
	}

	artifacts, err := c.gatewayInternalService.CheckArtifactsExist(ctx, identity.OUID, artifactIDs)
	if err != nil {
		logger.GetLogger(ctx).Error("Failed to check artifacts", "gatewayID", identity.ID, "error", err)
		http.Error(w, "Failed to check artifacts", http.StatusInternalServerError)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, artifactsExistResponse{Artifacts: artifacts})
}

// decodeSyncIDs reads the ids a startup-sync request names, answering 400 for a body it can't read and 413 for more
// than maxSyncIDs ids.
func decodeSyncIDs(w http.ResponseWriter, r *http.Request, ids func(syncIDsRequest) []string) ([]string, bool) {
	var req syncIDsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSyncBodyBytes)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return nil, false
	}
	named := ids(req)
	if len(named) > maxSyncIDs {
		http.Error(w, fmt.Sprintf("At most %d ids per request", maxSyncIDs), http.StatusRequestEntityTooLarge)
		return nil, false
	}
	return named, true
}

// acceptsTarGzip reports whether an Accept header admits a tar.gz: no header, or any media range that matches it
// with a non-zero q.
func acceptsTarGzip(accept string) bool {
	if strings.TrimSpace(accept) == "" {
		return true
	}
	for _, mediaRange := range strings.Split(accept, ",") {
		mediaType, params, err := mime.ParseMediaType(mediaRange)
		if err != nil {
			continue
		}
		if q, err := strconv.ParseFloat(params["q"], 64); err == nil && q == 0 {
			continue
		}
		switch mediaType {
		case tarGzipMediaType, "application/*", "*/*":
			return true
		}
	}
	return false
}
