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

	if toolsets != nil {
		toolsets.Register(server)
	}

	return gomcp.NewStreamableHTTPHandler(func(r *http.Request) *gomcp.Server {
		return server
	}, nil)
}
