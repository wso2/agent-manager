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
	"errors"
	"net/http"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// GetSecrets handles GET /secrets, the gateway's secret sync: the secrets the deployments assigned to the calling
// gateway name. ?includeValues=true adds their values, for the gateway's startup sync; ?updatedAfter=<RFC3339> lists
// only those changed since.
func (c *gatewayInternalController) GetSecrets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity, ok := c.authenticateGateway(w, r, "secret-sync")
	if !ok {
		return
	}
	var updatedAfter *time.Time
	if raw := r.URL.Query().Get("updatedAfter"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "Invalid 'updatedAfter' parameter. Expected RFC3339 format.", http.StatusBadRequest)
			return
		}
		utc := t.UTC()
		updatedAfter = &utc
	}
	includeValues := r.URL.Query().Get("includeValues") == "true"

	list, err := c.gatewayInternalService.ListGatewaySecrets(ctx, identity.OUID, identity.ID, updatedAfter, includeValues)
	if err != nil {
		logger.GetLogger(ctx).Error("Failed to list gateway secrets", "gatewayID", identity.ID, "error", err)
		http.Error(w, "Failed to retrieve secrets", http.StatusInternalServerError)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, list)
}

// GetSecretValue handles GET /secrets/{handle}/value: the value of a secret a deployment assigned to the calling
// gateway names. Any other handle, another gateway's or another organization's included, is not found.
func (c *gatewayInternalController) GetSecretValue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	identity, ok := c.authenticateGateway(w, r, "secret-value")
	if !ok {
		return
	}
	handle := r.PathValue("handle")
	if handle == "" {
		http.Error(w, "Secret handle is required", http.StatusBadRequest)
		return
	}
	value, err := c.gatewayInternalService.GetGatewaySecretValue(ctx, identity.OUID, identity.ID, handle)
	if err != nil {
		if errors.Is(err, services.ErrGatewaySecretNotFound) {
			http.Error(w, "Secret not found", http.StatusNotFound)
			return
		}
		logger.GetLogger(ctx).Error("Failed to read gateway secret", "gatewayID", identity.ID, "handle", handle, "error", err)
		http.Error(w, "Failed to read secret", http.StatusInternalServerError)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]string{"value": value})
}
