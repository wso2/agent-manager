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

package tools

import gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

// Register registers every configured toolset on the server and installs the
// authorization middleware that enforces each tool's declared permissions.
func (tools *Toolsets) Register(server *gomcp.Server) {
	tools.register(server)
}

// register is Register minus the exported surface: it returns the registry so
// same-package tests can assert the tool→permission wiring directly.
// The fail-closed authorization middleware is installed on the server before
// the nil-receiver check, so even a server built with nil toolsets (no tools
// registered at all) denies every tools/call rather than silently having no
// authz middleware installed. Installing it before any tools are added is
// safe: sessions only exist after Connect, which happens after Register
// returns in both production and tests.
func (tools *Toolsets) register(server *gomcp.Server) *toolRegistry {
	reg := newToolRegistry()
	server.AddReceivingMiddleware(reg.authzMiddleware())
	if tools == nil {
		return reg
	}
	if tools.ProjectToolset != nil {
		tools.registerProjectTools(server, reg)
	}
	if tools.AgentToolset != nil {
		tools.registerAgentTools(server, reg)
	}
	if tools.DeploymentToolset != nil {
		tools.registerDeploymentTools(server, reg)
	}
	if tools.EnvironmentToolset != nil {
		tools.registerEnvironmentTools(server, reg)
	}
	if tools.LLMProviderToolset != nil {
		tools.registerLLMProviderTools(server, reg)
	}
	if tools.MCPProxyToolset != nil {
		tools.registerMCPProxyTools(server, reg)
	}
	if tools.AgentConfigToolset != nil {
		tools.registerAgentConfigTools(server, reg)
	}
	return reg
}
