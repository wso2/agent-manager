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
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	reqlogger "github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// methodListTools is the MCP method name for listing tools.
const methodListTools = "tools/list"

// ModuleTool is a tool an active module adds to the server.
type ModuleTool struct {
	Module      string
	Name        string
	Title       string
	Description string
	InputSchema json.RawMessage
	ReadOnly    bool
	Destructive bool
	// Permission is what the caller's token must hold.
	Permission rbac.Permission
	// Action is the audit action a call is recorded under. A read-only tool
	// is recorded only when AuditRead is set.
	Action    audit.Action
	AuditRead bool
	Handler   modkit.MCPToolHandler
}

// ModuleToolSource lists the tools of the active modules. It is asked on every
// request, so tools come and go with their modules without a restart.
type ModuleToolSource interface {
	ModuleTools() []ModuleTool
}

// moduleToolsMiddleware serves the active modules' tools beside the core's:
// tools/list gets them appended, and a tools/call for one is authorized and
// audited the way addTool does it for a core tool, then run here, before the
// core's authorization middleware (which knows only the core's tools) sees
// it. A module tool named like a core tool is never listed or run.
func (reg *toolRegistry) moduleToolsMiddleware(src ModuleToolSource) gomcp.Middleware {
	return func(next gomcp.MethodHandler) gomcp.MethodHandler {
		return func(ctx context.Context, method string, req gomcp.Request) (gomcp.Result, error) {
			switch method {
			case methodListTools:
				res, err := next(ctx, method, req)
				if err != nil {
					return res, err
				}
				list, ok := res.(*gomcp.ListToolsResult)
				if !ok || list.NextCursor != "" {
					// Module tools follow the core's last page.
					return res, nil
				}
				for _, t := range src.ModuleTools() {
					if _, core := reg.permissions[t.Name]; !core {
						list.Tools = append(list.Tools, mcpTool(t))
					}
				}
				return list, nil
			case methodCallTool:
				call, ok := req.(*gomcp.CallToolRequest)
				if !ok {
					return next(ctx, method, req)
				}
				if _, core := reg.permissions[call.Params.Name]; core {
					return next(ctx, method, req)
				}
				for _, t := range src.ModuleTools() {
					if t.Name == call.Params.Name {
						return callModuleTool(ctx, call, t), nil
					}
				}
			}
			return next(ctx, method, req)
		}
	}
}

func mcpTool(t ModuleTool) *gomcp.Tool {
	schema := any(map[string]any{"type": "object"})
	if len(t.InputSchema) > 0 {
		schema = t.InputSchema
	}
	readOnly := t.ReadOnly
	tool := &gomcp.Tool{
		Name: t.Name, Title: t.Title, Description: t.Description, InputSchema: schema,
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: readOnly},
	}
	if !readOnly {
		destructive := t.Destructive
		tool.Annotations.DestructiveHint = &destructive
	}
	return tool
}

// callModuleTool authorizes and runs a module tool call, with the same
// organization and scope checks as authzMiddleware and the same record as
// withToolAudit.
func callModuleTool(ctx context.Context, call *gomcp.CallToolRequest, t ModuleTool) *gomcp.CallToolResult {
	if call.Extra != nil && call.Extra.TokenInfo != nil && !sessionOrgMatchesRequest(ctx, call.Extra.TokenInfo) {
		recordToolDeny(ctx, t.Name, "organization-mismatch")
		return denyResult("organization mismatch: request token is not scoped to the session organization")
	}
	ctx = withEffectiveScopes(ctx, call)
	if missing, short := jwtassertion.FirstMissingScope(ctx, t.Permission); short {
		recordToolDeny(ctx, t.Name, "missing-scope",
			audit.RequiredPermissions(missing),
			audit.Detail("missingScope", missing.Scope()))
		return denyResult(fmt.Sprintf("insufficient permissions: this tool requires the %s scope", missing.Scope()))
	}

	log := reqlogger.GetLogger(ctx)
	start := time.Now()
	args := call.Params.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	result, err := runModuleTool(modkit.WithOrgID(ctx, resolveOUID(ctx)), t, args)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		log.Error("mcp tool failed", "tool", t.Name, "module", t.Module, "duration_ms", duration, "error", err)
	} else {
		log.Info("mcp tool succeeded", "tool", t.Name, "module", t.Module, "duration_ms", duration)
	}
	if !t.ReadOnly || t.AuditRead {
		audit.Record(
			ctx, t.Action,
			audit.SurfaceOpt(audit.SurfaceMCP),
			audit.Detail("tool", t.Name),
			audit.Result(err),
		)
	}
	if err != nil {
		var shown *modkit.Error
		if errors.As(err, &shown) {
			return denyResult(shown.Message)
		}
		return denyResult("the tool failed")
	}
	data, err := json.Marshal(result)
	if err != nil {
		log.Error("mcp tool result is not JSON", "tool", t.Name, "module", t.Module, "error", err)
		return denyResult("the tool failed")
	}
	return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: string(data)}}}
}

func runModuleTool(ctx context.Context, t ModuleTool, args json.RawMessage) (result any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return t.Handler(ctx, args)
}
