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
	"sync"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// fakeModules is a module tool source whose tools tests change between calls.
type fakeModules struct {
	mu    sync.Mutex
	tools []ModuleTool
}

func (f *fakeModules) ModuleTools() []ModuleTool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ModuleTool(nil), f.tools...)
}

func (f *fakeModules) set(tools ...ModuleTool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tools = tools
}

func moduleTool(name string, perm rbac.Permission, h modkit.MCPToolHandler) ModuleTool {
	return ModuleTool{
		Module: "widgets", Name: name, Description: "A module tool", ReadOnly: true,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}}}`),
		Permission:  perm, Action: "widgets:read", Handler: h,
	}
}

func listed(t *testing.T, s *gomcp.ClientSession) map[string]*gomcp.Tool {
	t.Helper()
	res, err := s.ListTools(context.Background(), nil)
	require.NoError(t, err)
	out := map[string]*gomcp.Tool{}
	for _, tool := range res.Tools {
		out[tool.Name] = tool
	}
	return out
}

func call(t *testing.T, s *gomcp.ClientSession, name string, args map[string]any) (bool, string) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &gomcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.NotEmpty(t, res.Content)
	text, _ := res.Content[0].(*gomcp.TextContent)
	require.NotNil(t, text)
	return res.IsError, text.Text
}

func TestModuleToolsAreListedAndCalledPerRequest(t *testing.T) {
	mods := &fakeModules{}
	mock := NewMockToolsetHandler()
	session := setupTestServerWithClaims(t, &Toolsets{ProjectToolset: mock, Modules: mods}, &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: "amp:widgets:read " + rbac.ProjectRead.Scope(),
	})

	tools := listed(t, session)
	assert.Contains(t, tools, "list_projects")
	assert.NotContains(t, tools, "widgets_count", "no module, no module tools")

	var gotOrg string
	var gotArgs string
	mods.set(
		moduleTool("widgets_count", "widgets:read", func(ctx context.Context, args json.RawMessage) (any, error) {
			gotOrg, gotArgs = modkit.OrgID(ctx), string(args)
			return map[string]int{"count": 3}, nil
		}),
		moduleTool("widgets_secret", "widgets:admin", func(context.Context, json.RawMessage) (any, error) { return "x", nil }),
		moduleTool("widgets_refuse", "widgets:read", func(context.Context, json.RawMessage) (any, error) {
			return nil, modkit.Reject("not today")
		}),
		moduleTool("widgets_crash", "widgets:read", func(context.Context, json.RawMessage) (any, error) {
			return nil, errors.New("internal detail that must not leak")
		}),
		moduleTool("list_projects", "widgets:read", func(context.Context, json.RawMessage) (any, error) {
			return "a module cannot take over a core tool", nil
		}),
	)
	tools = listed(t, session)
	require.Contains(t, tools, "widgets_count", "a module's tools appear as soon as it is active")
	assert.True(t, tools["widgets_count"].Annotations.ReadOnlyHint)
	schema, _ := json.Marshal(tools["widgets_count"].InputSchema)
	assert.JSONEq(t, `{"type":"object","properties":{"n":{"type":"integer"}}}`, string(schema))

	isErr, text := call(t, session, "widgets_count", map[string]any{"n": 2})
	assert.False(t, isErr, text)
	assert.JSONEq(t, `{"count":3}`, text)
	assert.Equal(t, testOrgName, gotOrg, "the handler's ctx carries the caller's organization")
	assert.JSONEq(t, `{"n":2}`, gotArgs)

	isErr, text = call(t, session, "widgets_secret", nil)
	assert.True(t, isErr)
	assert.Contains(t, text, "amp:widgets:admin", "a tool needs its permission")

	isErr, text = call(t, session, "widgets_refuse", nil)
	assert.True(t, isErr)
	assert.Equal(t, "not today", text, "a modkit.Error reaches the caller")

	isErr, text = call(t, session, "widgets_crash", nil)
	assert.True(t, isErr)
	assert.NotContains(t, text, "internal detail", "other errors do not")

	isErr, text = call(t, session, "list_projects", nil)
	assert.False(t, isErr, text)
	assert.NotContains(t, text, "take over", "the core's tool answers")

	mods.set()
	assert.NotContains(t, listed(t, session), "widgets_count", "and they go with the module")
	isErr, _ = call(t, session, "widgets_count", nil)
	assert.True(t, isErr, "a tool of an inactive module is refused as unknown")
}
