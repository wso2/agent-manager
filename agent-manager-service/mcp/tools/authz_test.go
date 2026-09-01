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
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// setRBACEnabled flips the process-global RBAC switch for one test and
// restores it on cleanup. Tests using it must not run in parallel.
func setRBACEnabled(t *testing.T, enabled bool) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.RBACEnabled
	cfg.RBACEnabled = enabled
	t.Cleanup(func() { cfg.RBACEnabled = orig })
}

// callToolViaMiddleware runs a synthetic tools/call for toolName through the
// registry's middleware with a next handler that records whether it ran.
func callToolViaMiddleware(t *testing.T, reg *toolRegistry, ctx context.Context, toolName string) (result gomcp.Result, nextCalled bool) {
	t.Helper()
	next := func(_ context.Context, _ string, _ gomcp.Request) (gomcp.Result, error) {
		nextCalled = true
		return &gomcp.CallToolResult{}, nil
	}
	req := &gomcp.CallToolRequest{Params: &gomcp.CallToolParamsRaw{Name: toolName}}
	result, err := reg.authzMiddleware()(next)(ctx, "tools/call", req)
	if err != nil {
		t.Fatalf("middleware returned unexpected error: %v", err)
	}
	return result, nextCalled
}

// callToolViaMiddlewareWithExtra is callToolViaMiddleware but lets the caller
// attach a RequestExtra (e.g. carrying per-request auth.TokenInfo), mirroring
// what the SDK's streamable transport stamps onto every JSON-RPC request when
// auth.RequireBearerToken is wired in front of it (see mcp/setup.go).
func callToolViaMiddlewareWithExtra(t *testing.T, reg *toolRegistry, ctx context.Context, toolName string, extra *gomcp.RequestExtra) (result gomcp.Result, nextCalled bool) {
	t.Helper()
	next := func(_ context.Context, _ string, _ gomcp.Request) (gomcp.Result, error) {
		nextCalled = true
		return &gomcp.CallToolResult{}, nil
	}
	req := &gomcp.CallToolRequest{Params: &gomcp.CallToolParamsRaw{Name: toolName}, Extra: extra}
	result, err := reg.authzMiddleware()(next)(ctx, "tools/call", req)
	if err != nil {
		t.Fatalf("middleware returned unexpected error: %v", err)
	}
	return result, nextCalled
}

func denialText(t *testing.T, result gomcp.Result) string {
	t.Helper()
	callResult, ok := result.(*gomcp.CallToolResult)
	if !ok {
		t.Fatalf("result is %T, want *gomcp.CallToolResult", result)
	}
	if !callResult.IsError {
		t.Fatal("expected IsError=true denial result")
	}
	if len(callResult.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(callResult.Content))
	}
	text, ok := callResult.Content[0].(*gomcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *gomcp.TextContent", callResult.Content[0])
	}
	return text.Text
}

func TestAddToolPanicsWithoutPermissions(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when registering a tool without permissions")
		}
		if !strings.Contains(r.(string), "no_perm_tool") {
			t.Fatalf("panic message %q does not name the tool", r)
		}
	}()
	server := gomcp.NewServer(&gomcp.Implementation{Name: "t", Version: "0"}, nil)
	addTool(newToolRegistry(), server, &gomcp.Tool{
		Name:        "no_perm_tool",
		Description: "a tool registered without permissions",
		InputSchema: createSchema(map[string]any{}, nil),
	}, audit.ActionAgentRead, func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{}, nil, nil
	})
}

func TestAddToolRecordsPermissions(t *testing.T) {
	server := gomcp.NewServer(&gomcp.Implementation{Name: "t", Version: "0"}, nil)
	reg := newToolRegistry()
	addTool(reg, server, &gomcp.Tool{
		Name:        "two_perm_tool",
		Description: "a tool with two permissions",
		InputSchema: createSchema(map[string]any{}, nil),
	}, audit.ActionAgentCreate, func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{}, nil, nil
	}, rbac.AgentCreate, rbac.AgentTokenManage)

	got := reg.permissions["two_perm_tool"]
	if len(got) != 2 || got[0] != rbac.AgentCreate || got[1] != rbac.AgentTokenManage {
		t.Fatalf("registry permissions = %v, want [AgentCreate AgentTokenManage]", got)
	}
	if reg.actions["two_perm_tool"] != audit.ActionAgentCreate {
		t.Fatalf("registry action = %q, want %q", reg.actions["two_perm_tool"], audit.ActionAgentCreate)
	}
}

// TestAddToolPanicsWithoutAuditAction pins the second half of the registration
// guarantee: a tool that cannot be attributed in the trail must fail at startup,
// exactly as one registered without permissions does.
func TestAddToolPanicsWithoutAuditAction(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when registering a tool without an audit action")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "no_action_tool") {
			t.Fatalf("panic message %v does not name the tool", r)
		}
		if !strings.Contains(msg, "audit action") {
			t.Fatalf("panic message %q does not explain what is missing", msg)
		}
	}()
	server := gomcp.NewServer(&gomcp.Implementation{Name: "t", Version: "0"}, nil)
	addTool(newToolRegistry(), server, &gomcp.Tool{
		Name:        "no_action_tool",
		Description: "a tool registered without an audit action",
		InputSchema: createSchema(map[string]any{}, nil),
	}, "", func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{}, nil, nil
	}, rbac.AgentCreate)
}

func TestAuthzMiddlewareDeniesUnregisteredTool(t *testing.T) {
	setRBACEnabled(t, false) // fail-closed applies even with RBAC disabled
	reg := newToolRegistry()
	result, nextCalled := callToolViaMiddleware(t, reg, context.Background(), "rogue_tool")
	if nextCalled {
		t.Fatal("next handler ran for an unregistered tool")
	}
	if got, want := denialText(t, result), `tool "rogue_tool" has no registered permissions`; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

func TestAuthzMiddlewareSkipsCheckWhenRBACDisabled(t *testing.T) {
	setRBACEnabled(t, false)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	// No claims on context at all — must still pass with RBAC disabled.
	_, nextCalled := callToolViaMiddleware(t, reg, context.Background(), "some_tool")
	if !nextCalled {
		t.Fatal("next handler did not run with RBAC disabled")
	}
}

func TestAuthzMiddlewareDeniesMissingScope(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentRead.Scope(), // has read, not build
	})
	result, nextCalled := callToolViaMiddleware(t, reg, ctx, "some_tool")
	if nextCalled {
		t.Fatal("next handler ran despite missing scope")
	}
	if got, want := denialText(t, result), "insufficient permissions: this tool requires the amp:agent:build scope"; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

// TestAuthzMiddlewareDeniesWhenNoClaimsOnContext pins that RBAC enabled with
// no claims/scopes at all on the context (not even an empty scope string)
// fails closed rather than open.
func TestAuthzMiddlewareDeniesWhenNoClaimsOnContext(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	result, nextCalled := callToolViaMiddleware(t, reg, context.Background(), "some_tool")
	if nextCalled {
		t.Fatal("next handler ran despite no claims on context")
	}
	if got, want := denialText(t, result), "insufficient permissions: this tool requires the amp:agent:build scope"; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

func TestAuthzMiddlewareAllowsMatchingScope(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentBuild.Scope(),
	})
	_, nextCalled := callToolViaMiddleware(t, reg, ctx, "some_tool")
	if !nextCalled {
		t.Fatal("next handler did not run despite matching scope")
	}
}

// TestAuthzMiddlewarePerRequestScopeAllowsWithEmptySession proves the
// per-request auth.TokenInfo source is consulted (and is sufficient on its
// own): the session context carries no scopes at all, yet the call is allowed
// because call.Extra.TokenInfo has the required scope.
func TestAuthzMiddlewarePerRequestScopeAllowsWithEmptySession(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	extra := &gomcp.RequestExtra{TokenInfo: &auth.TokenInfo{Scopes: []string{rbac.AgentBuild.Scope()}}}
	_, nextCalled := callToolViaMiddlewareWithExtra(t, reg, context.Background(), "some_tool", extra)
	if !nextCalled {
		t.Fatal("next handler did not run despite matching per-request scope")
	}
}

// TestAuthzMiddlewarePerRequestScopeTakesPrecedenceOverSession proves that
// when call.Extra.TokenInfo is present, it is authoritative even if the
// SESSION context separately carries the required scope: a request whose
// per-request TokenInfo lacks the scope is denied.
func TestAuthzMiddlewarePerRequestScopeTakesPrecedenceOverSession(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	// Session context has the required scope...
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentBuild.Scope(),
	})
	// ...but the per-request TokenInfo does not (org matches so the scope is
	// the only failing dimension).
	extra := &gomcp.RequestExtra{TokenInfo: &auth.TokenInfo{
		Scopes: []string{rbac.AgentRead.Scope()},
		Extra:  map[string]any{"amp:ou-id": testOrgName},
	}}
	result, nextCalled := callToolViaMiddlewareWithExtra(t, reg, ctx, "some_tool", extra)
	if nextCalled {
		t.Fatal("next handler ran despite per-request TokenInfo lacking the required scope")
	}
	if got, want := denialText(t, result), "insufficient permissions: this tool requires the amp:agent:build scope"; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

// TestAuthzMiddlewareDeniesOrgMismatch proves the per-request token must target
// the same organization as the MCP session. Tool handlers resolve the org from
// the session/initialize context (resolveOUID) while scopes are taken from the
// per-request token, so a token for org A that carries the required scope must
// not be allowed to drive a tool against org B's data. The request here has the
// required scope but a different ouId than the session — it must be denied.
func TestAuthzMiddlewareDeniesOrgMismatch(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	// Session established for org B, and it even carries the required scope.
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  "org-b",
		Scope: rbac.AgentBuild.Scope(),
	})
	// Per-request token is for org A but carries the required scope.
	extra := &gomcp.RequestExtra{TokenInfo: &auth.TokenInfo{
		Scopes: []string{rbac.AgentBuild.Scope()},
		Extra:  map[string]any{"amp:ou-id": "org-a"},
	}}
	result, nextCalled := callToolViaMiddlewareWithExtra(t, reg, ctx, "some_tool", extra)
	if nextCalled {
		t.Fatal("next handler ran despite request/session organization mismatch")
	}
	if got, want := denialText(t, result), "organization mismatch: request token is not scoped to the session organization"; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

// TestAuthzMiddlewareAllowsMatchingOrg is the happy path: a single-token client
// whose per-request org equals the session org passes the consistency check.
func TestAuthzMiddlewareAllowsMatchingOrg(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  "org-a",
		Scope: rbac.AgentBuild.Scope(),
	})
	extra := &gomcp.RequestExtra{TokenInfo: &auth.TokenInfo{
		Scopes: []string{rbac.AgentBuild.Scope()},
		Extra:  map[string]any{"amp:ou-id": "org-a"},
	}}
	_, nextCalled := callToolViaMiddlewareWithExtra(t, reg, ctx, "some_tool", extra)
	if !nextCalled {
		t.Fatal("next handler did not run despite matching organization")
	}
}

// TestAuthzMiddlewareDeniesOrgMismatchEvenWhenRBACDisabled proves the org
// consistency check is an identity-integrity guard, not scope authorization:
// it holds even when RBAC_ENABLED is false (mirroring the SDK's sub-based
// session-hijack guard, which is always active).
func TestAuthzMiddlewareDeniesOrgMismatchEvenWhenRBACDisabled(t *testing.T) {
	setRBACEnabled(t, false)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId: "org-b",
	})
	extra := &gomcp.RequestExtra{TokenInfo: &auth.TokenInfo{
		Extra: map[string]any{"amp:ou-id": "org-a"},
	}}
	result, nextCalled := callToolViaMiddlewareWithExtra(t, reg, ctx, "some_tool", extra)
	if nextCalled {
		t.Fatal("next handler ran despite org mismatch with RBAC disabled")
	}
	if got, want := denialText(t, result), "organization mismatch: request token is not scoped to the session organization"; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

func TestAuthzMiddlewareRequiresAllPermissions(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["multi_tool"] = []rbac.Permission{rbac.AgentCreate, rbac.AgentTokenManage}
	// Only one of the two scopes present.
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentCreate.Scope(),
	})
	result, nextCalled := callToolViaMiddleware(t, reg, ctx, "multi_tool")
	if nextCalled {
		t.Fatal("next handler ran with only one of two required scopes")
	}
	if got, want := denialText(t, result), "insufficient permissions: this tool requires the amp:agent:token-manage scope"; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

func TestAuthzMiddlewarePassesThroughOtherMethods(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	next := func(_ context.Context, _ string, _ gomcp.Request) (gomcp.Result, error) {
		return &gomcp.ListToolsResult{}, nil
	}
	req := &gomcp.ListToolsRequest{Params: &gomcp.ListToolsParams{}}
	result, err := reg.authzMiddleware()(next)(context.Background(), "tools/list", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.(*gomcp.ListToolsResult); !ok {
		t.Fatalf("tools/list result was intercepted: got %T", result)
	}
}

// TestAuthzMiddlewarePutsPerRequestScopesOnContext is the regression for a split
// decision. The tool gate reads the per-request token's scopes; anything deeper
// — the service layer's environment-tier check — reads ctx. Before the fix those
// were two different scope sets, so one tools/call could be gated on the request
// token and tiered on the session token.
//
// The session here grants the production tier and the per-request token does
// not, which is exactly the direction that matters: the narrower token must win.
func TestAuthzMiddlewarePutsPerRequestScopesOnContext(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentBuild.Scope() + " " + rbac.AgentEnvProduction.Scope(),
	})
	extra := &gomcp.RequestExtra{TokenInfo: &auth.TokenInfo{
		Scopes: []string{rbac.AgentBuild.Scope(), rbac.AgentEnvNonProduction.Scope()},
		Extra:  map[string]any{TokenInfoOUIDKey: testOrgName},
	}}

	var sawProduction, sawNonProduction bool
	next := func(handlerCtx context.Context, _ string, _ gomcp.Request) (gomcp.Result, error) {
		sawProduction = jwtassertion.HasAllScopes(handlerCtx, []string{rbac.AgentEnvProduction.Scope()})
		sawNonProduction = jwtassertion.HasAllScopes(handlerCtx, []string{rbac.AgentEnvNonProduction.Scope()})
		return &gomcp.CallToolResult{}, nil
	}
	req := &gomcp.CallToolRequest{Params: &gomcp.CallToolParamsRaw{Name: "some_tool"}, Extra: extra}
	if _, err := reg.authzMiddleware()(next)(ctx, methodCallTool, req); err != nil {
		t.Fatalf("middleware returned unexpected error: %v", err)
	}
	if sawProduction {
		t.Error("handler context carries the session's production tier, which the per-request token did not grant")
	}
	if !sawNonProduction {
		t.Error("handler context is missing the per-request token's non-production tier")
	}
}

// TestAuthzMiddlewareKeepsSessionScopesWithoutTokenInfo covers the other branch:
// in-memory transports have no HTTP layer, so Extra.TokenInfo is always nil and
// the session scopes already on ctx are the effective set. Rewriting ctx from an
// absent TokenInfo would blank them.
func TestAuthzMiddlewareKeepsSessionScopesWithoutTokenInfo(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentBuild.Scope() + " " + rbac.AgentEnvNonProduction.Scope(),
	})

	var sawNonProduction bool
	next := func(handlerCtx context.Context, _ string, _ gomcp.Request) (gomcp.Result, error) {
		sawNonProduction = jwtassertion.HasAllScopes(handlerCtx, []string{rbac.AgentEnvNonProduction.Scope()})
		return &gomcp.CallToolResult{}, nil
	}
	req := &gomcp.CallToolRequest{Params: &gomcp.CallToolParamsRaw{Name: "some_tool"}}
	if _, err := reg.authzMiddleware()(next)(ctx, methodCallTool, req); err != nil {
		t.Fatalf("middleware returned unexpected error: %v", err)
	}
	if !sawNonProduction {
		t.Error("the session scopes were lost from the handler context")
	}
}
