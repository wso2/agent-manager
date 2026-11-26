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
	"errors"
	"io"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// PushPricing handles POST /api/internal/v1/gateways/{gatewayId}/pricing.
// A gateway sends its model price file (model_prices.json) after it connects; the
// models not stored yet join the global catalog, and the stored ones are left as they are.
func (c *gatewayInternalController) PushPricing(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	gatewayID := r.PathValue("gatewayId")
	if gatewayID == "" {
		http.Error(w, "Gateway ID is required", http.StatusBadRequest)
		return
	}

	identity, ok := c.authenticateGateway(w, r, "push-pricing")
	if !ok {
		return
	}
	if identity.ID != gatewayID {
		// A valid key used for another gateway: a stronger signal than an invalid one.
		log.Warn("Gateway pricing token does not match gateway", "gatewayId", gatewayID, "tokenGatewayId", identity.ID)
		recordGatewayAuthFailure(r, "gateway-id-mismatch", identity.ID)
		http.Error(w, "Gateway token does not match gateway ID", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, gwsvc.PricingMaxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "Gateway pricing is too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Invalid gateway pricing", http.StatusBadRequest)
		}
		log.Warn("Refused gateway pricing", "gatewayId", gatewayID)
		return
	}

	// Recorded after the fact, as the manifest push is: the gateway is a machine client
	// that retries, so refusing the push when the trail is unavailable gains nothing.
	saved, err := c.pricingService.Save(r.Context(), body)
	modkit.AuditResource(r.Context(), gwaudit.ResourceGateway, gatewayID, "")
	modkit.AuditDetail(r.Context(), "modelCount", saved.Sent)
	modkit.AuditDetail(r.Context(), "addedCount", saved.Added)
	if err != nil {
		var refused *gwsvc.PricingError
		if errors.As(err, &refused) {
			log.Warn("Refused gateway pricing", "gatewayId", gatewayID, "reason", refused.Message)
			httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest, refused.Message, "", refused.Code())
			return
		}
		log.Error("Failed to store gateway pricing", "gatewayId", gatewayID, "error", err)
		http.Error(w, "Failed to store gateway pricing", http.StatusInternalServerError)
		return
	}
	log.Info("Stored gateway pricing", "gatewayId", gatewayID, "models", saved.Sent, "skipped", saved.Skipped, "added", saved.Added)
	w.WriteHeader(http.StatusNoContent)
}
