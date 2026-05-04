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

	reqlogger "github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// helper function for resolving the org of the agent
const defaultOrgName = "default"

func resolveOrgName(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return defaultOrgName
}

// helper function for resolving the environment of the agent
const defaultEnvName = "default"

func resolveEnv(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return defaultEnvName
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

// custom error handling to provide more LLM-friendly error messages for common errors
func wrapToolError(toolName string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, utils.ErrOrganizationNotFound):
		return fmt.Errorf("%s: invalid org name. Use a valid org name or omit it to use the default value", toolName)
	case errors.Is(err, utils.ErrProjectNotFound):
		return fmt.Errorf("%s: invalid project name. Call list_projects to see valid projects", toolName)
	case errors.Is(err, utils.ErrAgentNotFound):
		return fmt.Errorf("%s: invalid agent name. Call list_agents or list_project_agent_pairs to see valid agents", toolName)
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
	case errors.Is(err, utils.ErrNotFound):
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "namespace not found") || strings.Contains(msg, "organization not found"):
			return fmt.Errorf("%s: invalid org name. Use a valid org name or omit it to use the default value", toolName)
		case strings.Contains(msg, "project not found"):
			return fmt.Errorf("%s: invalid project name. Call list_projects to see valid projects", toolName)
		case strings.Contains(msg, "agent not found") || strings.Contains(msg, "component not found"):
			return fmt.Errorf("%s: invalid agent name. Call list_agents or list_project_agent_pairs to see valid agents", toolName)
		}
	}
	return fmt.Errorf("%s: %w", toolName, err)
}

// custom logging layer for mcp tools
func withToolLogging[T any](toolName string, handler func(context.Context, *gomcp.CallToolRequest, T) (*gomcp.CallToolResult, any, error)) func(context.Context, *gomcp.CallToolRequest, T) (*gomcp.CallToolResult, any, error) {
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

// format logs (both runtime and build logs) response to a more LLM-friendly format
func reduceLogsResponse(resp *models.LogsResponse) map[string]any {
	if resp == nil {
		return map[string]any{
			"logs":       []map[string]any{},
			"totalCount": 0,
			"tookMs":     0,
		}
	}

	logs := make([]map[string]any, 0, len(resp.Logs))
	for _, entry := range resp.Logs {
		logs = append(logs, map[string]any{
			"timestamp": entry.Timestamp,
			"logLevel":  entry.LogLevel,
			"log":       entry.Log,
		})
	}
	return map[string]any{
		"logs":       logs,
		"totalCount": resp.TotalCount,
		"tookMs":     resp.TookMs,
	}
}
