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

type listGatewaysInput struct {
	Environment string `json:"environment,omitempty"`
	Limit       *int   `json:"limit,omitempty"`
	Offset      *int   `json:"offset,omitempty"`
}

type gatewayItem struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	DisplayName       string `json:"display_name"`
	FunctionalityType string `json:"functionality_type"`
	IsActive          bool   `json:"is_active"`
}
type listGatewaysOutput struct {
	Total    int           `json:"total"`
	Gateways []gatewayItem `json:"gateways"`
}

func (t *Toolsets) registerGatewayTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name: "list_gateways",
		Description: "List the AI gateways registered in your organization, optionally only those serving one environment. " +
			"LLM providers and MCP proxies are deployed to gateways; use a gateway's `id` in create_llm_provider's gateway_ids " +
			"or create_mcp_proxy's gateway_id. Only active gateways accept deployments.",
		InputSchema: createSchema(map[string]any{
			"environment": stringProperty("Optional. Environment name from list_environments; returns only gateways serving it."),
			"limit":       intProperty(fmt.Sprintf("Optional. Max gateways to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset":      intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, nil),
	}, audit.ActionGatewayRead, listGateways(t.GatewayToolset, t.EnvironmentToolset), rbac.GatewayRead)
}

func listGateways(handler GatewayToolsetHandler, envHandler EnvironmentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listGatewaysInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listGatewaysInput) (*gomcp.CallToolResult, any, error) {
		limit, offset, err := resolvePagination(input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}
		ouID := resolveOUID(ctx)

		var envID string
		if name := strings.TrimSpace(input.Environment); name != "" {
			if envID, err = resolveEnvironmentUUID(ctx, "list_gateways", envHandler, ouID, name); err != nil {
				return nil, nil, err
			}
		}

		resp, err := handler.ListGateways(ctx, ouID, envID, limit, offset)
		if err != nil {
			return nil, nil, wrapToolError("list_gateways", err)
		}
		out := listGatewaysOutput{Gateways: []gatewayItem{}}
		if resp != nil {
			out.Total = resp.Pagination.Total
			for _, g := range resp.List {
				out.Gateways = append(out.Gateways, gatewayItem{
					ID:                g.ID,
					Name:              g.Name,
					DisplayName:       g.DisplayName,
					FunctionalityType: g.FunctionalityType,
					IsActive:          g.IsActive,
				})
			}
		}
		return handleToolResult(out, nil)
	}
}

// resolveEnvironmentUUID maps an environment name to the UUID the connection
// services key environments by.
func resolveEnvironmentUUID(ctx context.Context, toolName string, handler EnvironmentToolsetHandler, ouID, name string) (string, error) {
	if handler == nil {
		return "", fmt.Errorf("%s: environment lookup is not available", toolName)
	}
	env, err := handler.GetEnvironment(ctx, ouID, name)
	if err != nil {
		return "", wrapToolError(toolName, err)
	}
	if env == nil || env.UUID == "" {
		return "", fmt.Errorf("%s: invalid environment name. Call list_environments to see valid environments", toolName)
	}
	return env.UUID, nil
}

// resolveEnvironmentNames returns the given environment names, validated, or
// every environment in the org when none are given.
func resolveEnvironmentNames(ctx context.Context, toolName string, handler EnvironmentToolsetHandler, ouID string, names []string) ([]string, error) {
	if handler == nil {
		return nil, fmt.Errorf("%s: environment lookup is not available", toolName)
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, err := handler.GetEnvironment(ctx, ouID, n); err != nil {
			return nil, wrapToolError(toolName, err)
		}
		out = append(out, n)
	}
	if len(out) > 0 {
		return out, nil
	}
	result, err := handler.ListEnvironments(ctx, ouID, int32(utils.MaxLimit), int32(utils.DefaultOffset))
	if err != nil {
		return nil, wrapToolError(toolName, err)
	}
	if result == nil || len(result.Environments) == 0 {
		return nil, fmt.Errorf("%s: no environments are configured for your organization", toolName)
	}
	for _, env := range result.Environments {
		out = append(out, env.Name)
	}
	return out, nil
}
