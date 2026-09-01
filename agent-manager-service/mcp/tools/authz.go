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

	"github.com/modelcontextprotocol/go-sdk/auth"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// methodCallTool is the MCP method name for tool invocations. The SDK's own
// constant is unexported.
const methodCallTool = "tools/call"

// TokenInfoOUIDKey is the auth.TokenInfo.Extra key under which the MCP token
// verifier (mcp/tokeninfo.go) records the organization of the token that made
// the current request. authzMiddleware compares it against the session's
// organization so a per-request token cannot drive a tool against a different
// org than the one whose identity established the session.
const TokenInfoOUIDKey = "amp:ou-id"

// toolRegistry records the rbac permissions and audit action each registered
// tool declares. authzMiddleware enforces it fail-closed: a tool with no entry —
// one registered by bypassing addTool — is always denied.
type toolRegistry struct {
	permissions map[string][]rbac.Permission
	actions     map[string]audit.Action
}

func newToolRegistry() *toolRegistry {
	return &toolRegistry{
		permissions: make(map[string][]rbac.Permission),
		actions:     make(map[string]audit.Action),
	}
}

// addTool is the only sanctioned way to register an MCP tool. It requires the
// permissions the caller's token must hold (ALL semantics) and the audit action
// the tool performs, records both for authzMiddleware, and wires the logging
// and audit wrapper.
//
// Panics when either is missing: those are registration bugs, caught at startup
// and by every test run, not runtime conditions. Requiring the action is what
// stops a new mutating tool reaching production with no audit record — the same
// guarantee the route registrar gives the REST surface.
//
// The action must be the same constant the REST route for the equivalent
// operation uses, so "who deployed agent X" is one query across both surfaces.
func addTool[T any](reg *toolRegistry, server *gomcp.Server, tool *gomcp.Tool,
	action audit.Action,
	handler func(context.Context, *gomcp.CallToolRequest, T) (*gomcp.CallToolResult, any, error),
	perms ...rbac.Permission,
) {
	if len(perms) == 0 {
		panic(fmt.Sprintf("mcp tool %q registered without permissions", tool.Name))
	}
	if action == "" {
		panic(fmt.Sprintf("mcp tool %q registered without an audit action", tool.Name))
	}
	reg.permissions[tool.Name] = perms
	reg.actions[tool.Name] = action
	gomcp.AddTool(server, tool, withToolAudit(tool.Name, action, handler))
}

// authzMiddleware returns a server middleware that authorizes every tools/call
// against the registry, mirroring middleware.RequirePermission semantics:
// RBAC_ENABLED=false skips the scope check (zero-downtime rollout switch),
// while the unknown-tool denial applies regardless. Denials are returned as
// IsError tool results so MCP clients surface an actionable message instead
// of a protocol error.
func (reg *toolRegistry) authzMiddleware() gomcp.Middleware {
	return func(next gomcp.MethodHandler) gomcp.MethodHandler {
		return func(ctx context.Context, method string, req gomcp.Request) (gomcp.Result, error) {
			if method != methodCallTool {
				return next(ctx, method, req)
			}
			call, ok := req.(*gomcp.CallToolRequest)
			if !ok {
				return denyResult(fmt.Sprintf("unexpected %s request type", methodCallTool)), nil
			}
			perms, registered := reg.permissions[call.Params.Name]
			if !registered {
				// A call for a tool that was never registered. Recorded because
				// it is a probe, not an ordinary authorization failure.
				recordToolDeny(ctx, call.Params.Name, "unknown-tool")
				return denyResult(fmt.Sprintf("tool %q has no registered permissions", call.Params.Name)), nil
			}
			// Organization-consistency guard: tool handlers resolve the org from
			// the session/initialize context (resolveOUID) while scopes are taken
			// from the per-request token. Reject when the per-request token targets
			// a different org than the session, so scopes granted in one org cannot
			// authorize actions against another. This is an identity-integrity check
			// like the SDK's sub-based session-hijack guard, so it applies
			// regardless of RBAC_ENABLED. Skipped when there is no per-request
			// TokenInfo (in-memory transports have no HTTP layer).
			if call.Extra != nil && call.Extra.TokenInfo != nil {
				if !sessionOrgMatchesRequest(ctx, call.Extra.TokenInfo) {
					// An attempt to drive a tool against a different org than
					// the session's — an identity-integrity failure, and the
					// clearest attack signal this surface produces.
					recordToolDeny(ctx, call.Params.Name, "organization-mismatch")
					return denyResult("organization mismatch: request token is not scoped to the session organization"), nil
				}
			}
			// Both this gate and everything downstream of it — notably the
			// service layer's environment-tier check — must be decided by the
			// same scope set. Installing it on ctx here is what makes that true;
			// reading two sources would let one request be gated on the request
			// token and tiered on the session token.
			ctx = withEffectiveScopes(ctx, call)
			if !config.GetConfig().RBACEnabled {
				return next(ctx, method, req)
			}
			if missing, short := jwtassertion.FirstMissingScope(ctx, perms...); short {
				recordToolDeny(ctx, call.Params.Name, "missing-scope",
					audit.RequiredPermissions(missing),
					audit.Detail("missingScope", missing.Scope()))
				return denyResult(fmt.Sprintf("insufficient permissions: this tool requires the %s scope", missing.Scope())), nil
			}
			return next(ctx, method, req)
		}
	}
}

// withEffectiveScopes returns ctx carrying the scope set that decides this
// request. The per-request scopes the SDK attaches via call.Extra.TokenInfo
// (populated by claimsTokenVerifier through auth.RequireBearerToken — see
// mcp/tokeninfo.go and mcp/setup.go) win over the session's, deliberately: they
// reflect the token that made this specific HTTP request, and a per-request
// token must not borrow session privileges.
//
// When Extra.TokenInfo is absent — in-memory transports have no HTTP layer, so
// it is always nil there — the session scopes jwtassertion already put on ctx
// ARE the effective set, and ctx is returned untouched.
func withEffectiveScopes(ctx context.Context, call *gomcp.CallToolRequest) context.Context {
	if call.Extra == nil || call.Extra.TokenInfo == nil {
		return ctx
	}
	return jwtassertion.ContextWithScopeList(ctx, call.Extra.TokenInfo.Scopes)
}

// sessionOrgMatchesRequest reports whether the organization on the per-request
// token (recorded under TokenInfoOUIDKey by claimsTokenVerifier) equals the
// organization of the session's claims on ctx. A session with no claims yields
// an empty org, which only matches a request that likewise carries no org.
func sessionOrgMatchesRequest(ctx context.Context, info *auth.TokenInfo) bool {
	var sessionOUID string
	if claims := jwtassertion.GetTokenClaims(ctx); claims != nil {
		sessionOUID = claims.OuId
	}
	requestOUID, _ := info.Extra[TokenInfoOUIDKey].(string)
	return requestOUID == sessionOUID
}

// recordToolDeny records a refused MCP tool call. The tool name is recorded as
// the request path so that MCP denials and REST denials answer the same query.
func recordToolDeny(ctx context.Context, toolName, reason string, opts ...audit.Option) {
	all := make([]audit.Option, 0, len(opts)+4)
	all = append(
		all,
		audit.SurfaceOpt(audit.SurfaceMCP),
		audit.OutcomeOpt(audit.OutcomeDeny),
		audit.Detail("reason", reason),
		audit.Detail("tool", toolName),
	)
	all = append(all, opts...)
	audit.RecordAncillary(ctx, audit.ActionAuthzDeny, all...)
}

func denyResult(message string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		IsError: true,
		Content: []gomcp.Content{&gomcp.TextContent{Text: message}},
	}
}
