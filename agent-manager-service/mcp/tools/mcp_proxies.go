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

import (
	"context"
	"fmt"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	defaultMCPProxyVersion = "1.0.0"
	// mcpSpecVersion is the MCP specification version the console records for
	// proxies it creates.
	mcpSpecVersion = "2025-06-18"
	// mcpProxyEndpointID names the single endpoint this tool creates.
	mcpProxyEndpointID = "default"
	// mcpProxyAPIKeyHeader is the header agents send their proxy API key in,
	// the console's default endpoint security.
	mcpProxyAPIKeyHeader = "X-API-Key"
)

type listMCPProxiesInput struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}
type createMCPProxyInput struct {
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	Environments []string `json:"environments"`
	AuthHeader   string   `json:"auth_header,omitempty"`
	AuthValue    string   `json:"auth_value,omitempty"`
	GatewayID    string   `json:"gateway_id,omitempty"`
	Description  string   `json:"description,omitempty"`
	Version      string   `json:"version,omitempty"`
}

type mcpProxyItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status,omitempty"`
}
type listMCPProxiesOutput struct {
	Total   int            `json:"total"`
	Proxies []mcpProxyItem `json:"proxies"`
}
type createMCPProxyOutput struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Environments []string `json:"environments"`
	Tools        []string `json:"tools"`
	Note         string   `json:"note"`
}

func (t *Toolsets) registerMCPProxyTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name: "list_mcp_proxies",
		Description: "List the MCP proxies in your organization. An MCP proxy publishes an external MCP server through the AI gateway " +
			"so agents can call its tools with a gateway-issued key. Use a proxy's `id` as the `proxy` argument of attach_mcp_proxy_to_agent.",
		InputSchema: createSchema(map[string]any{
			"limit":  intProperty(fmt.Sprintf("Optional. Max proxies to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset": intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, nil),
	}, audit.ActionMCPServerRead, listMCPProxies(t.MCPProxyToolset), rbac.MCPServerRead)

	addTool(reg, server, &gomcp.Tool{
		Name: "create_mcp_proxy",
		Description: "Put an external MCP server behind the AI gateway as an MCP proxy. " +
			"The tool first connects to the server to discover its tools, then creates the proxy and deploys it to the given environments. " +
			"Call list_environments for `environments`. If an environment has more than one gateway, pass gateway_id from list_gateways.",
		InputSchema: createSchema(map[string]any{
			"name":         stringProperty("Required. Display name of the proxy. Its id is derived from it (lowercase, dashes)."),
			"url":          stringProperty("Required. URL of the upstream MCP server, e.g. https://mcp.example.com/mcp."),
			"environments": arrayProperty("Required. Environment names to deploy the proxy to.", map[string]any{"type": "string"}),
			"auth_header":  stringProperty("Optional. Header the upstream MCP server expects credentials in, e.g. Authorization. Requires auth_value."),
			"auth_value":   stringProperty("Optional. Credential value for auth_header, e.g. Bearer <token>. Stored by the gateway and never returned."),
			"gateway_id":   stringProperty("Optional. Gateway id from list_gateways; needed only when an environment has more than one gateway."),
			"description":  stringProperty("Optional. Description of the proxy."),
			"version":      stringProperty(fmt.Sprintf("Optional. Semantic version of the proxy (default %s).", defaultMCPProxyVersion)),
		}, []string{"name", "url", "environments"}),
	}, audit.ActionMCPServerCreate, createMCPProxy(t.MCPProxyToolset, t.EnvironmentToolset), rbac.MCPServerCreate, rbac.MCPServerConnect)
}

func listMCPProxies(handler MCPProxyToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listMCPProxiesInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listMCPProxiesInput) (*gomcp.CallToolResult, any, error) {
		limit, offset, err := resolvePagination(input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}
		resp, err := handler.ListMCPProxies(ctx, resolveOUID(ctx), limit, offset)
		if err != nil {
			return nil, nil, wrapToolError("list_mcp_proxies", err)
		}
		out := listMCPProxiesOutput{Proxies: []mcpProxyItem{}}
		if resp != nil {
			out.Total = resp.Pagination.Count
			for _, p := range resp.List {
				out.Proxies = append(out.Proxies, mcpProxyItem{
					ID:      deref(p.ID),
					Name:    deref(p.Name),
					Version: deref(p.Version),
					Status:  deref(p.Status),
				})
			}
		}
		return handleToolResult(out, nil)
	}
}

func createMCPProxy(handler MCPProxyToolsetHandler, envHandler EnvironmentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, createMCPProxyInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createMCPProxyInput) (*gomcp.CallToolResult, any, error) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return nil, nil, fmt.Errorf("name is required")
		}
		handle := toHandle(name)
		if handle == "" {
			return nil, nil, fmt.Errorf("name must contain at least one letter or digit")
		}
		url := strings.TrimSpace(input.URL)
		if url == "" {
			return nil, nil, fmt.Errorf("url is required")
		}
		authHeader, authValue := strings.TrimSpace(input.AuthHeader), strings.TrimSpace(input.AuthValue)
		if (authHeader == "") != (authValue == "") {
			return nil, nil, fmt.Errorf("auth_header and auth_value must be given together")
		}
		version := strings.TrimSpace(input.Version)
		if version == "" {
			version = defaultMCPProxyVersion
		}

		ouID := resolveOUID(ctx)
		envNames := make([]string, 0, len(input.Environments))
		bindings := make([]models.MCPEndpointEnvironmentDTO, 0, len(input.Environments))
		for _, envName := range input.Environments {
			envName = strings.TrimSpace(envName)
			if envName == "" {
				continue
			}
			envUUID, err := resolveEnvironmentUUID(ctx, "create_mcp_proxy", envHandler, ouID, envName)
			if err != nil {
				return nil, nil, err
			}
			binding := models.MCPEndpointEnvironmentDTO{EnvironmentUUID: envUUID}
			if gw := strings.TrimSpace(input.GatewayID); gw != "" {
				binding.GatewayID = &gw
			}
			envNames = append(envNames, envName)
			bindings = append(bindings, binding)
		}
		if len(bindings) == 0 {
			return nil, nil, fmt.Errorf("environments is required. Call list_environments to see valid environments")
		}

		var auth *models.UpstreamAuth
		if authHeader != "" {
			authType := "api-key"
			auth = &models.UpstreamAuth{Type: &authType, Header: &authHeader, Value: &authValue}
		}

		// Probe first, as the console does: it proves the URL and credentials
		// work before anything is created, and the discovered capabilities are
		// what the gateway uses to scope the proxy's tools.
		info, err := handler.FetchMCPServerInfo(ctx, &models.MCPServerInfoFetchRequest{URL: &url, Auth: auth})
		if err != nil {
			return nil, nil, wrapToolError("create_mcp_proxy", err)
		}
		capabilities := &models.MCPProxyCapabilities{}
		if info != nil {
			capabilities.Tools, capabilities.Resources, capabilities.Prompts = info.Tools, info.Resources, info.Prompts
		}

		enabled := true
		specVersion := mcpSpecVersion
		dto := &models.MCPProxyDTO{
			ID:             handle,
			Name:           name,
			Version:        version,
			McpSpecVersion: &specVersion,
			Endpoints: []models.MCPProxyEndpointDTO{{
				ID:           mcpProxyEndpointID,
				Upstream:     models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: url, Auth: auth}},
				Capabilities: capabilities,
				Security: &models.SecurityConfig{
					Enabled: &enabled,
					APIKey:  &models.APIKeySecurity{Enabled: &enabled, Key: mcpProxyAPIKeyHeader, In: "header"},
				},
				Environments: bindings,
			}},
		}
		if desc := strings.TrimSpace(input.Description); desc != "" {
			dto.Description = &desc
		}

		created, err := handler.CreateMCPProxy(ctx, ouID, dto)
		if err != nil {
			return nil, nil, wrapToolError("create_mcp_proxy", err)
		}
		if created == nil {
			return nil, nil, fmt.Errorf("create_mcp_proxy: MCP proxy service returned an empty response")
		}

		out := createMCPProxyOutput{
			ID:           created.ID,
			Name:         created.Name,
			Version:      created.Version,
			Environments: envNames,
			Tools:        toolNames(capabilities.Tools),
			Note:         "MCP proxy created. Call attach_mcp_proxy_to_agent to let an agent use it.",
		}
		return handleToolResult(out, nil)
	}
}

func toolNames(tools *[]map[string]interface{}) []string {
	names := []string{}
	if tools == nil {
		return names
	}
	for _, tool := range *tools {
		if n, ok := tool["name"].(string); ok && n != "" {
			names = append(names, n)
		}
	}
	return names
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
