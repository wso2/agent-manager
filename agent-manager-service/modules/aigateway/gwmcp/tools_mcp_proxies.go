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

package gwmcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

type listMCPProxiesInput struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}
type getMCPProxyInput struct {
	ProxyID string `json:"proxy_id"`
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
type getMCPProxyOutput struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Version     string                 `json:"version"`
	Endpoints   []mcpProxyEndpointItem `json:"endpoints"`
}

func (t *Tools) registerMCPProxyTools(reg Registrar) {
	addTool(reg, toolDef{
		Name: "list_mcp_proxies",
		Description: "List the MCP proxies in your organization. An MCP proxy publishes an external MCP server through the AI gateway " +
			"so agents can call its tools with a gateway-issued key. Use a proxy's `id` as the `proxy_id` argument of attach_mcp_proxy_to_agent.",
		InputSchema: createSchema(map[string]any{
			"limit":  intProperty(fmt.Sprintf("Optional. Max proxies to return (default %d, min %d, max %d).", gwutil.DefaultLimit, gwutil.MinLimit, gwutil.MaxLimit)),
			"offset": intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", gwutil.DefaultOffset, gwutil.MinOffset)),
		}, nil),
	}, gwaudit.MCPServerRead, listMCPProxies(t.MCPProxies), gwperm.MCPServerRead)

	addTool(reg, toolDef{
		Name: "get_mcp_proxy",
		Description: "Get one MCP proxy's details: the upstream MCP server URL, the tools it exposes, its guardrails, " +
			"and the environments it is deployed to with each deployment's status. " +
			"Use it to see which environments an agent attached with attach_mcp_proxy_to_agent can reach the proxy in. Credentials are never returned.",
		InputSchema: createSchema(map[string]any{
			"proxy_id": stringProperty("Required. MCP proxy id from list_mcp_proxies."),
		}, []string{"proxy_id"}),
	}, gwaudit.MCPServerRead, getMCPProxy(t.MCPProxies, t.Core), gwperm.MCPServerRead)
}

func listMCPProxies(handler MCPProxyToolsetHandler) func(context.Context, listMCPProxiesInput) (any, error) {
	return func(ctx context.Context, input listMCPProxiesInput) (any, error) {
		limit, offset, err := resolvePagination(input.Limit, input.Offset)
		if err != nil {
			return nil, err
		}
		resp, err := handler.ListMCPProxies(ctx, modkit.OrgID(ctx), limit, offset)
		if err != nil {
			return nil, wrapToolError("list_mcp_proxies", err)
		}
		out := listMCPProxiesOutput{Proxies: []mcpProxyItem{}}
		if resp != nil {
			out.Total = resp.Pagination.Count
			for _, p := range resp.List {
				out.Proxies = append(out.Proxies, mcpProxyItem{
					ID:      gwutil.StrPointerAsStr(p.ID, ""),
					Name:    gwutil.StrPointerAsStr(p.Name, ""),
					Version: gwutil.StrPointerAsStr(p.Version, ""),
					Status:  gwutil.StrPointerAsStr(p.Status, ""),
				})
			}
		}
		return out, nil
	}
}

func getMCPProxy(handler MCPProxyToolsetHandler, core CoreReader) func(context.Context, getMCPProxyInput) (any, error) {
	return func(ctx context.Context, input getMCPProxyInput) (any, error) {
		proxyID := strings.TrimSpace(input.ProxyID)
		if proxyID == "" {
			return nil, fmt.Errorf("proxy_id is required. Call list_mcp_proxies to see valid proxies")
		}
		ouID := modkit.OrgID(ctx)
		proxy, err := handler.GetMCPProxy(ctx, ouID, proxyID)
		if err != nil {
			return nil, wrapToolError("get_mcp_proxy", err)
		}
		if proxy == nil {
			return nil, fmt.Errorf("get_mcp_proxy: MCP proxy service returned an empty response")
		}

		envNames := environmentNamesByUUID(ctx, core, ouID)
		out := getMCPProxyOutput{
			ID:          proxy.ID,
			Name:        proxy.Name,
			Description: gwutil.StrPointerAsStr(proxy.Description, ""),
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
		return out, nil
	}
}

// environmentNamesByUUID maps environment UUIDs to names so results speak the
// names other tools take. A failed lookup leaves the map empty and callers
// fall back to the UUID.
func environmentNamesByUUID(ctx context.Context, core CoreReader, ouID string) map[string]string {
	names := map[string]string{}
	if core == nil {
		return names
	}
	envs, err := core.ListEnvironments(ctx, ouID)
	if err != nil {
		return names
	}
	for _, env := range envs {
		names[env.ID] = env.Name
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
