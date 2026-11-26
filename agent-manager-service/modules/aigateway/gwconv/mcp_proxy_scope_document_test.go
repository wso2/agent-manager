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

package gwconv_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

func mcpScope() gwmodels.MCPProxyScope {
	created := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	return gwmodels.MCPProxyScope{
		UUID:        uuid.MustParse("3f6b2c18-9a4d-4f21-8c7e-5d0a1b2c3d4e"),
		Action:      "read",
		Description: "read the forecast",
		Tools:       []string{"get_forecast"},
		CreatedAt:   created,
		UpdatedAt:   created.Add(time.Hour),
	}
}

// scopeDocOf decodes a scope document from its JSON, as the handler does.
func scopeDocOf(t *testing.T, apiVersion, kind, name string, spec map[string]any) gwspec.McpProxyScopeDocument {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]any{"name": name}, "spec": spec,
	})
	require.NoError(t, err)
	var doc gwspec.McpProxyScopeDocument
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

// The action is metadata.name, and spec.scope the token string derived from it and the handle.
func TestMCPProxyScopeDocument(t *testing.T) {
	doc := gwconv.MCPProxyScopeDocument("forecast", mcpScope())

	assert.Equal(t, gwconv.GroupVersion, doc.ApiVersion)
	assert.Equal(t, "McpProxyScope", doc.Kind)
	assert.Equal(t, "read", doc.Metadata.Name)
	require.NotNil(t, doc.Spec.Scope)
	assert.Equal(t, "forecast:read", *doc.Spec.Scope)
	require.NotNil(t, doc.Spec.Description)
	assert.Equal(t, "read the forecast", *doc.Spec.Description)
	assert.Equal(t, []string{"get_forecast"}, doc.Spec.Tools)

	require.NotNil(t, doc.Metadata.Uid)
	assert.Equal(t, "3f6b2c18-9a4d-4f21-8c7e-5d0a1b2c3d4e", *doc.Metadata.Uid)
	require.NotNil(t, doc.Metadata.CreationTimestamp)
	require.NotNil(t, doc.Metadata.UpdateTimestamp)
}

// A scope bound to no tool reads as an empty array, never null.
func TestMCPProxyScopeDocumentToolsAreAlwaysAnArray(t *testing.T) {
	scope := mcpScope()
	scope.Tools = nil

	raw, err := json.Marshal(gwconv.MCPProxyScopeDocument("forecast", scope))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"tools":[]`)
}

func TestMCPProxyScopeListEnvelope(t *testing.T) {
	out := gwconv.MCPProxyScopeList(&gwmodels.MCPProxyScopesResult{
		ProxyHandle: "forecast",
		Scopes:      []gwmodels.MCPProxyScope{mcpScope()},
	})

	require.NotNil(t, out.Count)
	assert.EqualValues(t, 1, *out.Count)
	require.Len(t, out.List, 1)
	assert.Equal(t, "forecast:read", *out.List[0].Spec.Scope)

	raw, err := json.Marshal(gwconv.MCPProxyScopeList(nil))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"list":[]`, "an empty list is an array, never null")
}

func TestMCPProxyScopeFromDocument(t *testing.T) {
	in, err := gwconv.MCPProxyScopeFromDocument(scopeDocOf(t, gwconv.GroupVersion, "McpProxyScope", "read",
		map[string]any{"description": "read the forecast", "tools": []string{"get_forecast"}}))
	require.NoError(t, err)

	assert.Equal(t, "read", in.Action, "metadata.name is the action the service takes")
	assert.Equal(t, "read the forecast", in.Description)
	assert.Equal(t, []string{"get_forecast"}, in.Tools)
}

func TestMCPProxyScopeFromDocumentChecksTheEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		kind       string
		action     string
		contains   string
	}{
		{"wrong apiVersion", "v1", "McpProxyScope", "read", "apiVersion must be"},
		{"wrong kind", gwconv.GroupVersion, "McpProxy", "read", `kind must be "McpProxyScope"`},
		{"no action", gwconv.GroupVersion, "McpProxyScope", " ", "metadata.name is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := scopeDocOf(t, tc.apiVersion, tc.kind, tc.action, map[string]any{})
			_, err := gwconv.MCPProxyScopeFromDocument(doc)
			require.ErrorIs(t, err, gwconv.ErrDocument)
			assert.ErrorContains(t, err, tc.contains)

			_, err = gwconv.MCPProxyScopeUpdateFromDocument(doc)
			require.ErrorIs(t, err, gwconv.ErrDocument, "the update checks the same envelope")
		})
	}
}

// An absent tools key leaves the bindings alone; a declared empty one unbinds every tool.
// Collapsing the two would clear a scope on a description-only edit.
func TestMCPProxyScopeUpdateKeepsToolsOmittedApartFromEmpty(t *testing.T) {
	omitted, err := gwconv.MCPProxyScopeUpdateFromDocument(scopeDocOf(t, gwconv.GroupVersion, "McpProxyScope", "read",
		map[string]any{"description": "write access"}))
	require.NoError(t, err)
	assert.Nil(t, omitted.Tools, "an absent key leaves the tool list unchanged")
	require.NotNil(t, omitted.Description)
	assert.Equal(t, "write access", *omitted.Description)

	declared, err := gwconv.MCPProxyScopeUpdateFromDocument(scopeDocOf(t, gwconv.GroupVersion, "McpProxyScope", "read",
		map[string]any{"tools": []string{}}))
	require.NoError(t, err)
	require.NotNil(t, declared.Tools, "a declared empty list unbinds every tool")
	assert.Empty(t, declared.Tools)
	assert.Nil(t, declared.Description, "an absent description leaves it unchanged")
}
