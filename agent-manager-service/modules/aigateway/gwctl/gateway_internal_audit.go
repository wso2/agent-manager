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
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// recordGatewayAuthFailure records a rejected gateway api-key.
//
// The internal server has no JWT middleware, so its authentication happens
// inside each handler. Without this a gateway-credential brute force would
// leave no signal anywhere: these keys grant access to every API key the
// gateway is entitled to sync. The presented key is never recorded, only
// whether one was supplied and why it was refused; the core rate-limits the
// records per source and reason.
func recordGatewayAuthFailure(r *http.Request, reason, gatewayID string) {
	modkit.AuditInternalCaller(r.Context(), modkit.InternalCaller{
		Kind: gwaudit.ResourceGateway, ID: gatewayID, AuthMethod: "api-key",
		Refused: reason, CredentialPresent: r.Header.Get("api-key") != "",
	})
}

// authenticateGateway resolves the gateway behind an internal-API request.
//
// Every internal handler repeated this block; centralising it is what gives the
// rejection path a single place to be recorded. It writes the HTTP error itself
// and reports whether the caller may continue.
func (c *gatewayInternalController) authenticateGateway(
	w http.ResponseWriter,
	r *http.Request,
	operation string,
) (gatewayIdentity, bool) {
	log := logger.GetLogger(r.Context())
	clientIP := httpx.ClientIP(r)

	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		log.Warn("Unauthorized access attempt - Missing API key", "ip", clientIP, "operation", operation)
		recordGatewayAuthFailure(r, "missing-api-key", "")
		http.Error(w, "API key is required. Provide 'api-key' header.", http.StatusUnauthorized)
		return gatewayIdentity{}, false
	}

	gateway, err := c.gatewayService.VerifyToken(r.Context(), apiKey)
	if err != nil {
		log.Warn("Authentication failed", "ip", clientIP, "operation", operation, "error", err)
		recordGatewayAuthFailure(r, "invalid-api-key", "")
		http.Error(w, "Invalid or expired API key", http.StatusUnauthorized)
		return gatewayIdentity{}, false
	}

	gatewayID := gateway.UUID.String()
	// Name the caller now that it is known. The coverage envelope is emitted
	// after the handler returns and coalesces per caller; until this point the
	// request carries only an address, which cannot tell two gateways apart.
	modkit.AuditInternalCaller(r.Context(), modkit.InternalCaller{Kind: gwaudit.ResourceGateway, ID: gatewayID, AuthMethod: "api-key"})

	return gatewayIdentity{ID: gatewayID, OUID: gateway.OUID}, true
}

// gatewayIdentity is the subset of an authenticated gateway the internal
// handlers and their audit records need.
type gatewayIdentity struct {
	ID   string
	OUID string
}
