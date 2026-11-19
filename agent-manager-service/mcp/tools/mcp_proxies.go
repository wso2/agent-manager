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
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type listMCPProxiesInput struct {
	ProjectName string `json:"project_name,omitempty"`
	Limit       *int   `json:"limit,omitempty"`
	Offset      *int   `json:"offset,omitempty"`
}
type getMCPProxyInput struct {
	ProxyID string `json:"proxy_id"`
}

type mcpProxyItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status,omitempty"`
	// Project is the project that owns the proxy; absent for an org-level proxy.
	Project string `json:"project,omitempty"`
}
type listMCPProxiesOutput struct {
	Total   int            `json:"total"`
	Proxies []mcpProxyItem `json:"proxies"`
}
type mcpProxyEndpointItem struct {
	ID           string                    `json:"id"`
	UpstreamURL  string                    `json:"upstream_url"`
	UpstreamAuth bool                      `json:"upstream_auth_configured"`
	Tools        []string                  `json:"tools"`
	Guardrails   []string                  `json:"guardrails,omitempty"`
	Environments []mcpProxyEnvironmentItem `json:"environments"`
}
type mcpProxyEnvironmentItem struct {
	Name             string `json:"name"`
	DeploymentStatus string `json:"deployment_status,omitempty"`
}

// mcpProxyBindingItem is a project proxy's binding in one environment: where it is deployed and the tools its MCP
// server exposes there.
type mcpProxyBindingItem struct {
	Name             string   `json:"name"`
	DeploymentStatus string   `json:"deployment_status,omitempty"`
	URL              string   `json:"url,omitempty"`
	Tools            []string `json:"tools"`
	// ToolsSource is "caller" when the publisher gave the tools, "discovered" when the platform listed them from
	// the MCP server.
	ToolsSource string `json:"tools_source,omitempty"`
	// ToolDiscovery is Pending, Discovered or Failed while the platform discovers the tools.
	ToolDiscovery      string `json:"tool_discovery,omitempty"`
	ToolDiscoveryError string `json:"tool_discovery_error,omitempty"`
}
type getMCPProxyOutput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
	// Project is the project that owns the proxy; absent for an org-level proxy.
	Project   string                 `json:"project,omitempty"`
	Endpoints []mcpProxyEndpointItem `json:"endpoints"`
	// Environments are a project proxy's bindings, which carry its tools.
	Environments []mcpProxyBindingItem `json:"environments,omitempty"`
}

func (t *Toolsets) registerMCPProxyTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name: "list_mcp_proxies",
		Description: "List the MCP proxies in your organization. An MCP proxy publishes an MCP server through the AI gateway " +
			"so agents can call its tools with a gateway-issued key. Use a proxy's `id` as the `proxy_id` argument of attach_mcp_proxy_to_agent. " +
			"Without `project_name` it lists the organization's proxies only. With it, it also lists that project's own proxies, " +
			"such as MCP servers published from the project's integrations (`project` names their owner); " +
			"a project's proxy can be attached only to agents in that project.",
		InputSchema: createSchema(map[string]any{
			"project_name": stringProperty("Optional. Also list this project's own MCP proxies, the ones its agents can use."),
			"limit":        intProperty(fmt.Sprintf("Optional. Max proxies to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset":       intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, nil),
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, audit.ActionMCPServerRead, listMCPProxies(t.MCPProxyToolset), rbac.MCPServerRead)

	addTool(reg, server, &gomcp.Tool{
		Name: "get_mcp_proxy",
		Description: "Get one MCP proxy's details: the upstream MCP server URL, the tools it exposes, its guardrails, " +
			"and the environments it is deployed to with each deployment's status. " +
			"For a project's proxy, `environments` lists each environment's deployment, URL and tools, and whether the tools are still being discovered. " +
			"Use it to see which environments an agent attached with attach_mcp_proxy_to_agent can reach the proxy in. Credentials are never returned.",
		InputSchema: createSchema(map[string]any{
			"proxy_id": stringProperty("Required. MCP proxy id from list_mcp_proxies."),
		}, []string{"proxy_id"}),
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, audit.ActionMCPServerRead, getMCPProxy(t.MCPProxyToolset, t.EnvironmentToolset), rbac.MCPServerRead)
}

func listMCPProxies(handler MCPProxyToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listMCPProxiesInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listMCPProxiesInput) (*gomcp.CallToolResult, any, error) {
		limit, offset, err := resolvePagination(input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}
		resp, err := handler.ListMCPProxies(ctx, resolveOUID(ctx), strings.TrimSpace(input.ProjectName), limit, offset)
		if err != nil {
			return nil, nil, wrapToolError("list_mcp_proxies", err)
		}
		out := listMCPProxiesOutput{Proxies: []mcpProxyItem{}}
		if resp != nil {
			out.Total = resp.Pagination.Count
			for _, p := range resp.List {
				out.Proxies = append(out.Proxies, mcpProxyItem{
					ID:      utils.StrPointerAsStr(p.ID, ""),
					Name:    utils.StrPointerAsStr(p.Name, ""),
					Version: utils.StrPointerAsStr(p.Version, ""),
					Status:  utils.StrPointerAsStr(p.Status, ""),
					Project: utils.StrPointerAsStr(p.ProjectName, ""),
				})
			}
		}
		return handleToolResult(out, nil)
	}
}

func getMCPProxy(handler MCPProxyToolsetHandler, envHandler EnvironmentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getMCPProxyInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getMCPProxyInput) (*gomcp.CallToolResult, any, error) {
		proxyID := strings.TrimSpace(input.ProxyID)
		if proxyID == "" {
			return nil, nil, fmt.Errorf("proxy_id is required. Call list_mcp_proxies to see valid proxies")
		}
		ouID := resolveOUID(ctx)
		proxy, err := handler.GetMCPProxy(ctx, ouID, proxyID)
		if err != nil {
			return nil, nil, wrapToolError("get_mcp_proxy", err)
		}
		if proxy == nil {
			return nil, nil, fmt.Errorf("get_mcp_proxy: MCP proxy service returned an empty response")
		}

		if project := utils.StrPointerAsStr(proxy.ProjectName, ""); project != "" {
			return getProjectMCPProxy(ctx, handler, ouID, project, proxyID)
		}

		envNames := environmentNamesByUUID(ctx, envHandler, ouID)
		out := getMCPProxyOutput{
			ID:          proxy.ID,
			Name:        proxy.Name,
			Description: utils.StrPointerAsStr(proxy.Description, ""),
			Version:     proxy.Version,
			Endpoints:   []mcpProxyEndpointItem{},
		}
		for _, ep := range proxy.Endpoints {
			item := mcpProxyEndpointItem{ID: ep.ID, Tools: []string{}, Environments: []mcpProxyEnvironmentItem{}}
			// Only the redacted URL and whether auth is set: the auth value is the
			// upstream credential, and a URL can carry credentials of its own.
			if ep.Upstream.Main != nil {
				item.UpstreamURL = redactURL(ep.Upstream.Main.URL)
				item.UpstreamAuth = ep.Upstream.Main.Auth != nil
			}
			if ep.Capabilities != nil {
				item.Tools = toolNames(ep.Capabilities.Tools)
			}
			if ep.Policies != nil {
				for _, p := range *ep.Policies {
					item.Guardrails = append(item.Guardrails, p.Name)
				}
			}
			for _, env := range ep.Environments {
				name := envNames[env.EnvironmentUUID]
				if name == "" {
					name = env.EnvironmentUUID
				}
				item.Environments = append(item.Environments, mcpProxyEnvironmentItem{Name: name, DeploymentStatus: env.DeploymentStatus})
			}
			out.Endpoints = append(out.Endpoints, item)
		}
		return handleToolResult(out, nil)
	}
}

// getProjectMCPProxy is get_mcp_proxy for a project's proxy. Its endpoint rows are its environment bindings, so the
// proxy is read through the project's view, where each binding carries its tools and their discovery.
func getProjectMCPProxy(
	ctx context.Context, handler MCPProxyToolsetHandler, ouID, project, proxyID string,
) (*gomcp.CallToolResult, any, error) {
	proxy, err := handler.GetProjectMCPProxy(ctx, ouID, project, proxyID)
	if err != nil {
		return nil, nil, wrapToolError("get_mcp_proxy", err)
	}
	if proxy == nil {
		return nil, nil, fmt.Errorf("get_mcp_proxy: MCP proxy service returned an empty response")
	}
	out := getMCPProxyOutput{
		ID:           proxy.ID,
		Name:         proxy.Name,
		Description:  proxy.Description,
		Version:      proxy.Version,
		Project:      proxy.ProjectName,
		Endpoints:    []mcpProxyEndpointItem{},
		Environments: []mcpProxyBindingItem{},
	}
	for _, env := range proxy.Environments {
		item := mcpProxyBindingItem{
			Name:             env.EnvironmentName,
			DeploymentStatus: env.DeploymentStatus,
			URL:              redactURL(env.PublicURL),
			Tools:            []string{},
			ToolsSource:      env.CapabilitiesSource,
		}
		if env.Capabilities != nil {
			item.Tools = toolNames(env.Capabilities.Tools)
		}
		if d := env.CapabilityDiscovery; d != nil {
			item.ToolDiscovery = d.Status
			item.ToolDiscoveryError = d.LastError
		}
		out.Environments = append(out.Environments, item)
	}
	return handleToolResult(out, nil)
}

// environmentNamesByUUID maps environment UUIDs to names so results speak the
// names other tools take. A failed lookup leaves the map empty and callers
// fall back to the UUID.
func environmentNamesByUUID(ctx context.Context, handler EnvironmentToolsetHandler, ouID string) map[string]string {
	names := map[string]string{}
	if handler == nil {
		return names
	}
	envs, err := listAllEnvironments(ctx, handler, ouID)
	if err != nil {
		return names
	}
	for _, env := range envs {
		names[env.UUID] = env.Name
	}
	return names
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
