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

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wso2/agent-manager/agent-manager-service/mcp/tools"
)

// NewHTTPServer creates a streamable MCP HTTP server wired to the service layer.
func NewHTTPServer(toolsets *tools.Toolsets) http.Handler {
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "agent-manager",
		Version: "0.1.0",
	}, nil)

	// Register's nil-receiver path is safe — it still installs the fail-closed
	// authz middleware even when toolsets is nil — so this always runs
	// unconditionally.
	toolsets.Register(server)

	return newStreamableHandler(server)
}

// newStreamableHandler serves server over streamable HTTP in stateless mode.
//
// The service runs as several replicas behind a plain load balancer. A stateful
// handler keeps each Mcp-Session-Id in this process's memory, so a client that
// initialized on one replica gets 404 "session not found" once a later request
// lands on another. In stateless mode every request gets a short-lived session
// built from that request's own context, so any replica can serve it.
//
// Stateless mode gives up the GET SSE stream (now 405) and server-to-client
// requests such as sampling and elicitation; no tool here uses either. The
// org-consistency guard in tools/authz.go still holds: the org a tool acts on
// and the scopes that authorize it now both come from the same request token.
func newStreamableHandler(server *gomcp.Server) http.Handler {
	return gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server {
		return server
	}, &gomcp.StreamableHTTPOptions{Stateless: true})
}
