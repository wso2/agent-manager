//go:build integration

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

package gwctl_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

const llmTemplatesPath = "/orgs/{org}/llm-provider-templates"

// A custom template is created, listed after the built-ins, read and deleted.
func TestLLMRoutesTemplateLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	handle := unique("custom-tpl")

	before := expectInto[gwspec.LLMProviderTemplateListResponse](t, a.do(http.MethodGet, llmTemplatesPath, nil), http.StatusOK)
	require.NotEmpty(t, before.Templates, "the built-in templates are listed")
	require.EqualValues(t, len(before.Templates), before.Total)

	created := expectInto[gwspec.LLMProviderTemplateResponse](t, a.do(http.MethodPost, llmTemplatesPath, map[string]any{
		"id": handle, "name": "Custom", "description": "a custom template",
		"metadata": map[string]any{"endpointUrl": "https://llm.example.com/v1"},
	}), http.StatusCreated)
	require.Equal(t, handle, created.Id)
	require.Equal(t, "Custom", created.Name)
	require.NotEmpty(t, created.Uuid)
	require.NotNil(t, created.Metadata)
	require.Equal(t, "https://llm.example.com/v1", *created.Metadata.EndpointUrl)

	after := expectInto[gwspec.LLMProviderTemplateListResponse](t,
		a.do(http.MethodGet, llmTemplatesPath+"?limit=500&offset=-3", nil), http.StatusOK)
	require.EqualValues(t, before.Total+1, after.Total)
	require.EqualValues(t, 100, after.Limit, "the limit is capped")
	require.EqualValues(t, 0, after.Offset, "a negative offset reads from the start")
	ids := make([]string, 0, len(after.Templates))
	for _, tpl := range after.Templates {
		ids = append(ids, tpl.Id)
	}
	require.Contains(t, ids, handle)

	got := expectInto[gwspec.LLMProviderTemplateResponse](t, a.do(http.MethodGet, llmTemplatesPath+"/"+handle, nil), http.StatusOK)
	require.Equal(t, created.Uuid, got.Uuid)

	rec := a.do(http.MethodDelete, llmTemplatesPath+"/"+handle, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	rec = a.do(http.MethodGet, llmTemplatesPath+"/"+handle, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "LLM provider template not found", errorMessage(t, rec))
}

// A built-in template is readable but cannot be shadowed, changed or deleted.
func TestLLMRoutesTemplateBuiltInIsReadOnly(t *testing.T) {
	a := newRouteAPI(t)

	got := expectInto[gwspec.LLMProviderTemplateResponse](t, a.do(http.MethodGet, llmTemplatesPath+"/openai", nil), http.StatusOK)
	require.Equal(t, "openai", got.Id)

	rec := a.do(http.MethodPost, llmTemplatesPath, map[string]any{"id": "openai", "name": "Mine"})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, "Cannot use handle of built-in template", errorMessage(t, rec))

	rec = a.do(http.MethodPut, llmTemplatesPath+"/openai", map[string]any{"name": "Mine"})
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Equal(t, "System templates cannot be modified", errorMessage(t, rec))

	rec = a.do(http.MethodDelete, llmTemplatesPath+"/openai", nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Equal(t, "System templates cannot be deleted", errorMessage(t, rec))
}

// Template writes refuse bad input, duplicates and unknown handles with the matching status.
func TestLLMRoutesTemplateErrors(t *testing.T) {
	a := newRouteAPI(t)
	existing := unique("dup-tpl")
	expect(t, a.do(http.MethodPost, llmTemplatesPath, map[string]any{"id": existing, "name": "Dup"}), http.StatusCreated)
	missing := unique("missing-tpl")

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}{
		{"create malformed body", http.MethodPost, llmTemplatesPath, "{", http.StatusBadRequest, "Invalid request body"},
		{"create without a name", http.MethodPost, llmTemplatesPath, map[string]any{"id": unique("x")}, http.StatusBadRequest, "Invalid input"},
		{"create with a bad handle", http.MethodPost, llmTemplatesPath, map[string]any{"id": "bad handle!", "name": "x"}, http.StatusBadRequest, "Invalid input"},
		{"create a duplicate", http.MethodPost, llmTemplatesPath, map[string]any{"id": existing, "name": "Again"}, http.StatusConflict, "LLM provider template already exists"},
		{"get a bad handle", http.MethodGet, llmTemplatesPath + "/bad%20handle", nil, http.StatusBadRequest, "Invalid template id"},
		{"get a missing template", http.MethodGet, llmTemplatesPath + "/" + missing, nil, http.StatusNotFound, "LLM provider template not found"},
		{"update malformed body", http.MethodPut, llmTemplatesPath + "/" + existing, "{", http.StatusBadRequest, "Invalid request body"},
		{"update without a name", http.MethodPut, llmTemplatesPath + "/" + existing, map[string]any{"description": "d"}, http.StatusBadRequest, "Invalid input"},
		{"update a missing template", http.MethodPut, llmTemplatesPath + "/" + missing, map[string]any{"name": "x"}, http.StatusNotFound, "LLM provider template not found"},
		{"delete a bad handle", http.MethodDelete, llmTemplatesPath + "/bad%20handle", nil, http.StatusBadRequest, "Invalid template id"},
		{"delete a missing template", http.MethodDelete, llmTemplatesPath + "/" + missing, nil, http.StatusNotFound, "LLM provider template not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := a.do(tt.method, tt.path, tt.body)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, tt.message, errorMessage(t, rec))
		})
	}
}

// Each template route needs its own permission.
func TestLLMRoutesTemplateNeedsScope(t *testing.T) {
	a := newRouteAPI(t)
	read := withScopes(scope(gwperm.LLMProviderRead))
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, llmTemplatesPath},
		{http.MethodGet, llmTemplatesPath},
		{http.MethodGet, llmTemplatesPath + "/openai"},
		{http.MethodPut, llmTemplatesPath + "/x"},
		{http.MethodDelete, llmTemplatesPath + "/x"},
	} {
		rec := a.do(tc.method, tc.path, map[string]any{"id": "x", "name": "x"}, read)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
}
