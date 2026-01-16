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

package api

import (
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware"
)

// registerAgentTokenRoutes registers the agent token API routes
func registerAgentTokenRoutes(mux *http.ServeMux, ctrl controllers.AgentTokenController) {
	// Token generation endpoint
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/token", ctrl.GenerateToken)
}

// registerJWKSRoute registers the JWKS endpoint on the provided mux
func registerJWKSRoute(mux *http.ServeMux, ctrl controllers.AgentTokenController) {
	// JWKS endpoint - no authentication required for public key retrieval
	mux.HandleFunc("GET /auth/external/jwks.json", ctrl.GetJWKS)
}
