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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	reqlogger "github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// resolveOUID returns the caller's OU ID from the token claims. Org identity
// is never taken from tool input: the token is the single source of truth,
// and services scope all data by ou_id.
func resolveOUID(ctx context.Context) string {
	if claims := jwtassertion.GetTokenClaims(ctx); claims != nil {
		return claims.OuId
	}
	return ""
}

// resolvePagination applies the standard limit/offset defaults and bounds
// shared by every paginated list tool.
func resolvePagination(limitPtr *int, offsetPtr *int) (int, int, error) {
	limit := utils.DefaultLimit
	if limitPtr != nil {
		limit = *limitPtr
	}
	if limit < utils.MinLimit || limit > utils.MaxLimit {
		return 0, 0, fmt.Errorf("limit must be between %d and %d", utils.MinLimit, utils.MaxLimit)
	}
	offset := utils.DefaultOffset
	if offsetPtr != nil {
		offset = *offsetPtr
	}
	if offset < utils.MinOffset {
		return 0, 0, fmt.Errorf("offset must be >= %d", utils.MinOffset)
	}
	return limit, offset, nil
}

// helper functions  that build JSON Schema snippets

func createSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func boolProperty(description string) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
	}
}

func intProperty(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
	}
}

func arrayProperty(description string, itemSchema map[string]any) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       itemSchema,
	}
}

func enumProperty(description string, values []string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
}

// shared hint for org-not-found errors, used by both classification branches below
const orgNotFoundHint = "the organization on your token was not found. Re-authenticate and try again"

// custom error handling to provide more LLM-friendly error messages for common errors
func wrapToolError(toolName string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, utils.ErrOrganizationNotFound):
		return fmt.Errorf("%s: %s", toolName, orgNotFoundHint)
	case errors.Is(err, utils.ErrProjectNotFound):
		return fmt.Errorf("%s: invalid project name. Call list_projects to see valid projects", toolName)
	case errors.Is(err, utils.ErrAgentNotFound):
		return fmt.Errorf("%s: invalid agent name. Call list_agents or list_project_agent_pairs to see valid agents", toolName)
	case errors.Is(err, utils.ErrEnvironmentNotFound):
		return fmt.Errorf("%s: invalid environment name. Call list_environments to see valid environments", toolName)
	case errors.Is(err, utils.ErrEvaluatorNotFound):
		return fmt.Errorf("%s: invalid evaluator id. Call list_evaluators to see valid evaluators", toolName)
	case errors.Is(err, utils.ErrCustomEvaluatorNotFound):
		return fmt.Errorf("%s: custom evaluator not found. Call list_evaluators to see valid evaluators", toolName)
	case errors.Is(err, utils.ErrCustomEvaluatorAlreadyExists):
		return fmt.Errorf("%s: custom evaluator already exists with this identifier or display name", toolName)
	case errors.Is(err, utils.ErrCustomEvaluatorIdentifierTaken):
		return fmt.Errorf("%s: evaluator identifier conflicts with a built-in evaluator", toolName)
	case errors.Is(err, utils.ErrMonitorNotFound):
		return fmt.Errorf("%s: monitor not found. Call list_monitors to see valid monitors", toolName)
	case errors.Is(err, utils.ErrMonitorRunNotFound):
		return fmt.Errorf("%s: monitor run not found. Call list_monitor_runs to see valid runs", toolName)
	case errors.Is(err, utils.ErrMonitorAlreadyStopped):
		return fmt.Errorf("%s: monitor is already stopped", toolName)
	case errors.Is(err, utils.ErrMonitorAlreadyActive):
		return fmt.Errorf("%s: monitor is already active", toolName)
	case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
		return fmt.Errorf("%s: invalid template. Call list_llm_provider_templates to see valid templates", toolName)
	case errors.Is(err, utils.ErrLLMProviderNotFound):
		return fmt.Errorf("%s: LLM provider not found. Call list_llm_providers to see valid providers", toolName)
	case errors.Is(err, utils.ErrLLMProviderExists):
		return fmt.Errorf("%s: an LLM provider with this name already exists. Call list_llm_providers, then reuse it or pick another name", toolName)
	case errors.Is(err, utils.ErrMCPProxyNotFound):
		return fmt.Errorf("%s: MCP proxy not found. Call list_mcp_proxies to see valid proxies", toolName)
	case errors.Is(err, utils.ErrMCPProxyExists), errors.Is(err, utils.ErrArtifactExists):
		return fmt.Errorf("%s: this name is already in use. Call list_mcp_proxies or list_llm_providers, then reuse it or pick another name", toolName)
	case errors.Is(err, utils.ErrAgentConfigAlreadyExists):
		return fmt.Errorf("%s: the agent already has a configuration with this name. Pass a different config_name", toolName)
	case errors.Is(err, utils.ErrURLUnreachable):
		return fmt.Errorf("%s: the MCP server URL is unreachable from Agent Manager. Check the URL", toolName)
	case errors.Is(err, utils.ErrMCPServerUnauthorized):
		return fmt.Errorf("%s: the MCP server returned 401 Unauthorized. Check auth_header and auth_value", toolName)
	case errors.Is(err, utils.ErrNotFound):
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "namespace not found") || strings.Contains(msg, "organization not found"):
			return fmt.Errorf("%s: %s", toolName, orgNotFoundHint)
		case strings.Contains(msg, "project not found"):
			return fmt.Errorf("%s: invalid project name. Call list_projects to see valid projects", toolName)
		case strings.Contains(msg, "agent not found") || strings.Contains(msg, "component not found"):
			return fmt.Errorf("%s: invalid agent name. Call list_agents or list_project_agent_pairs to see valid agents", toolName)
		}
	}
	return fmt.Errorf("%s: %w", toolName, err)
}

// withToolAudit wraps a tool handler with the existing logging and, for tools
// that change state, an audit record.
//
// Read-only tools are skipped, matching the REST policy: recording every list
// call would multiply volume for little forensic gain. The action's class is
// what decides, so the decision is made once where the action is declared
// rather than repeated at each registration.
//
// The record is written after the handler returns so it carries the outcome.
// It is fail-open: MCP tools reach the same services as the REST routes, and
// the operations that must not proceed unrecorded already refuse inside those
// services, so refusing again here would only duplicate the check.
func withToolAudit[T any](
	toolName string,
	action audit.Action,
	handler func(context.Context, *gomcp.CallToolRequest, T) (*gomcp.CallToolResult, any, error),
) func(context.Context, *gomcp.CallToolRequest, T) (*gomcp.CallToolResult, any, error) {
	audited := action.Class() != audit.ClassRead

	return func(ctx context.Context, req *gomcp.CallToolRequest, input T) (*gomcp.CallToolResult, any, error) {
		log := reqlogger.GetLogger(ctx)
		start := time.Now()
		result, meta, err := handler(ctx, req, input)
		duration := time.Since(start).Milliseconds()
		if err != nil {
			log.Error("mcp tool failed", "tool", toolName, "duration_ms", duration, "error", err)
		} else {
			log.Info("mcp tool succeeded", "tool", toolName, "duration_ms", duration)
		}

		if audited {
			// A handler that already emitted its own semantic record marks the
			// request scope, so this does not double-record; see audit.Record.
			audit.Record(
				ctx, action,
				audit.SurfaceOpt(audit.SurfaceMCP),
				audit.Detail("tool", toolName),
				audit.Result(err),
			)
		}
		return result, meta, err
	}
}

func handleToolResult(result any, err error) (*gomcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{
			&gomcp.TextContent{Text: string(jsonData)},
		},
	}, result, nil
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
