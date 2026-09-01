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

// Package mcp wires the am-obs-mcp streamable-HTTP server onto the
// observer's root mux.
package mcp

import (
	"net/http"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/mcp/tools"
)

// Dependencies holds the controllers backing the seven am-obs-mcp tools.
type Dependencies struct {
	Tracing       *controllers.TracingController
	Observability *controllers.ObservabilityController
	// RBACEnabled applies the REST routes' scope policy per tool call: each
	// tool requires its amp:observability:* scope on the per-call token.
	RBACEnabled bool
}

// RegisterRoute builds the MCP HTTP handler and registers it on mux at
// /mcp and /mcp/, wrapped with authMiddleware. It is mounted on the root
// mux (not under /api/v1/): publisher-audience tokens may call it, confined
// to their implicit trace-read permission by the per-tool guards.
func RegisterRoute(mux *http.ServeMux, deps Dependencies, authMiddleware func(http.Handler) http.Handler) {
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "am-obs-mcp",
		Version: "0.1.0",
	}, nil)

	toolsets := &tools.Toolsets{
		Tracing:       deps.Tracing,
		Observability: deps.Observability,
		RBACEnabled:   deps.RBACEnabled,
	}
	toolsets.Register(server)

	handler := gomcp.NewStreamableHTTPHandler(func(_ *http.Request) *gomcp.Server {
		return server
	}, nil)

	mux.Handle("/mcp", authMiddleware(handler))
	mux.Handle("/mcp/", authMiddleware(handler))
}
