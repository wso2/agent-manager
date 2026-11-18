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

package mcp

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/wso2/agent-manager/agent-manager-service/mcp/handlers"
	"github.com/wso2/agent-manager/agent-manager-service/mcp/tools"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// Dependencies holds the core services the MCP toolsets use.
type Dependencies struct {
	InfraResourceManager     services.InfraResourceManager
	AgentManagerService      services.AgentManagerService
	AgentTokenManagerService services.AgentTokenManagerService
	EnvironmentService       services.EnvironmentService
	// ModuleTools, if set, lists the active modules' tools.
	ModuleTools tools.ModuleToolSource
}

// RegisterRoute builds the MCP HTTP handler, wraps it with the standard middleware chain,
// and registers it on the given mux at /mcp. The core's toolsets (projects, agents,
// environments) are always registered; each extend adds the toolsets of a built-in
// feature, such as the AI Gateway's LLM provider, MCP proxy, agent configuration and
// deployment tools.
func RegisterRoute(mux *http.ServeMux, deps Dependencies, authMiddleware func(http.Handler) http.Handler,
	extend ...func(*tools.Toolsets),
) {
	toolsets := &tools.Toolsets{
		ProjectToolset:     handlers.NewProjectHandler(deps.InfraResourceManager),
		AgentToolset:       handlers.NewAgentHandler(deps.AgentManagerService, deps.AgentTokenManagerService),
		EnvironmentToolset: handlers.NewEnvironmentHandler(deps.EnvironmentService),
		Modules:            deps.ModuleTools,
	}
	for _, e := range extend {
		e(toolsets)
	}

	handler := NewHTTPServer(toolsets)
	// The SDK's bearer-token middleware runs INSIDE our auth middleware: our
	// middleware validates the JWT and puts claims on the request context
	// first, then claimsTokenVerifier (mcp/tokeninfo.go) reads those claims
	// to populate auth.TokenInfo for the SDK's per-request scope plumbing and
	// session-hijack guard.
	wrapped := authMiddleware(auth.RequireBearerToken(claimsTokenVerifier, nil)(handler))
	mux.Handle("/mcp", wrapped)
	mux.Handle("/mcp/", wrapped)
}
