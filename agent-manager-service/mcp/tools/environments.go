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

type listEnvironmentsInput struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}

type listEnvironmentItem struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	IsProduction bool   `json:"is_production"`
}
type listEnvironmentsOutput struct {
	Total        int32                 `json:"total"`
	Environments []listEnvironmentItem `json:"environments"`
}

func (t *Toolsets) registerEnvironmentTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name: "list_environments",
		Description: "List the environments configured for your organization (resolved from the caller's token). " +
			"Use this to discover valid environment names for tools that take an `environment` argument, " +
			"such as update_deployment_state and create_external_agent.",
		InputSchema: createSchema(map[string]any{
			"limit":  intProperty(fmt.Sprintf("Optional. Max environments to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset": intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, nil),
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, audit.ActionEnvironmentRead, listEnvironments(t.EnvironmentToolset), rbac.EnvironmentRead)
}

func listEnvironments(handler EnvironmentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listEnvironmentsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listEnvironmentsInput) (*gomcp.CallToolResult, any, error) {
		ouID := resolveOUID(ctx)

		limit, offset, err := resolvePagination(input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}

		result, err := handler.ListEnvironments(ctx, ouID, int32(limit), int32(offset))
		if err != nil {
			return nil, nil, wrapToolError("list_environments", err)
		}
		if result == nil {
			return nil, nil, fmt.Errorf("list_environments: environment service returned an empty response")
		}

		formatted := make([]listEnvironmentItem, 0, len(result.Environments))
		for _, env := range result.Environments {
			formatted = append(formatted, listEnvironmentItem{
				Name:         env.Name,
				DisplayName:  env.DisplayName,
				IsProduction: env.IsProduction,
			})
		}
		response := listEnvironmentsOutput{
			Total:        result.Total,
			Environments: formatted,
		}
		return handleToolResult(response, nil)
	}
}

// resolveTokenEnvironment validates an explicit environment name, or resolves
// the org's only environment when the name is empty.
func resolveTokenEnvironment(ctx context.Context, toolName string, handler EnvironmentToolsetHandler, ouID string, name string) (string, error) {
	if name = strings.TrimSpace(name); name != "" {
		if _, err := handler.GetEnvironment(ctx, ouID, name); err != nil {
			return "", wrapToolError(toolName, err)
		}
		return name, nil
	}
	result, err := handler.ListEnvironments(ctx, ouID, int32(utils.MaxLimit), int32(utils.DefaultOffset))
	if err != nil {
		return "", wrapToolError(toolName, err)
	}
	if result == nil || len(result.Environments) == 0 {
		return "", fmt.Errorf("%s: no environments are configured for your organization", toolName)
	}
	if len(result.Environments) > 1 {
		return "", fmt.Errorf("%s: environment is required when the organization has more than one environment. Call list_environments to see valid names", toolName)
	}
	return result.Environments[0].Name, nil
}
