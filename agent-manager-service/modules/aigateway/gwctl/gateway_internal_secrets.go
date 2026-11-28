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

package gwctl

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// gatewaySecretList is the body of GET /api/internal/v1/secrets, in the shape the gateway controller reads.
type gatewaySecretList struct {
	List  []gwsvc.GatewaySecret `json:"list"`
	Count int                   `json:"count"`
}

// gatewaySecretValue is the body of GET /api/internal/v1/secrets/{handle}/value.
type gatewaySecretValue struct {
	Value string `json:"value"`
}

// GetSecrets handles GET /api/internal/v1/secrets: the secrets the artifacts deployed to the calling gateway refer
// to, with their values when includeValues=true. The gateway calls it when it connects, before it syncs deployments.
func (c *gatewayInternalController) GetSecrets(w http.ResponseWriter, r *http.Request) {
	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}
	includeValues := r.URL.Query().Get("includeValues") == "true"
	secrets, err := c.gatewayInternalService.ListGatewaySecrets(r.Context(), identity.OUID, identity.ID, includeValues)
	if err != nil {
		logger.GetLogger(r.Context()).Error("Failed to list gateway secrets", "gatewayID", identity.ID, "error", err)
		http.Error(w, "Failed to list secrets", http.StatusInternalServerError)
		return
	}
	modkit.AuditDetail(r.Context(), "secretCount", len(secrets))
	modkit.AuditDetail(r.Context(), "includeValues", includeValues)
	writeSecretJSON(w, r, gatewaySecretList{List: secrets, Count: len(secrets)})
}

// GetSecretValue handles GET /api/internal/v1/secrets/{handle}/value: the value of a secret an artifact deployed to
// the calling gateway refers to. Any other handle is not found.
func (c *gatewayInternalController) GetSecretValue(w http.ResponseWriter, r *http.Request) {
	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}
	handle := r.PathValue("handle")
	modkit.AuditDetail(r.Context(), "secretHandle", handle)
	value, err := c.gatewayInternalService.GetGatewaySecretValue(r.Context(), identity.OUID, identity.ID, handle)
	if err != nil {
		if errors.Is(err, gwerr.ErrGatewaySecretNotFound) {
			http.Error(w, "Secret not found", http.StatusNotFound)
			return
		}
		logger.GetLogger(r.Context()).Error("Failed to read gateway secret", "gatewayID", identity.ID, "error", err)
		http.Error(w, "Failed to read secret", http.StatusInternalServerError)
		return
	}
	writeSecretJSON(w, r, gatewaySecretValue{Value: value})
}

// writeSecretJSON writes a response that carries secret values: never cached.
func writeSecretJSON(w http.ResponseWriter, r *http.Request, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.GetLogger(r.Context()).Error("Failed to write gateway secret response", "error", err)
	}
}
