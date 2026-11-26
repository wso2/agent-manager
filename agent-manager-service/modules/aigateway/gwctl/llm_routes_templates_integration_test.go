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

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

const llmTemplatesPath = "/llm-provider-templates"

// llmTemplateDoc is a template document with the given handle and spec.
func llmTemplateDoc(handle string, spec map[string]any) map[string]any {
	return map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1",
		"kind":       "LlmProviderTemplate",
		"metadata":   map[string]any{"name": handle},
		"spec":       spec,
	}
}

// A custom template is created, listed after the built-ins, read, replaced and deleted.
func TestLLMRoutesTemplateLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	handle := unique("custom-tpl")

	before := expectInto[gwspec.LLMProviderTemplateListResponse](t, a.do(http.MethodGet, llmTemplatesPath, nil), http.StatusOK)
	require.NotEmpty(t, before.List, "the built-in templates are listed")
	require.EqualValues(t, len(before.List), *before.Count)
	require.EqualValues(t, len(before.List), *before.Pagination.Total)

	created := expectInto[gwspec.LlmProviderTemplateDocument](t, a.do(http.MethodPost, llmTemplatesPath, llmTemplateDoc(handle, map[string]any{
		"displayName": "Custom", "description": "a custom template",
		"metadata": map[string]any{"endpointUrl": "https://llm.example.com/v1"},
	})), http.StatusCreated)
	require.Equal(t, "LlmProviderTemplate", created.Kind)
	require.Equal(t, handle, created.Metadata.Name)
	require.Equal(t, "Custom", created.Spec.DisplayName)
	require.NotNil(t, created.Metadata.Uid)
	require.NotNil(t, created.Spec.Metadata)
	require.Equal(t, "https://llm.example.com/v1", *created.Spec.Metadata.EndpointUrl)

	after := expectInto[gwspec.LLMProviderTemplateListResponse](t,
		a.do(http.MethodGet, llmTemplatesPath+"?limit=500&offset=-3", nil), http.StatusOK)
	require.EqualValues(t, *before.Pagination.Total+1, *after.Pagination.Total)
	require.EqualValues(t, 100, *after.Pagination.Limit, "the limit is capped")
	require.EqualValues(t, 0, *after.Pagination.Offset, "a negative offset reads from the start")
	names := make([]string, 0, len(after.List))
	for _, tpl := range after.List {
		names = append(names, tpl.Metadata.Name)
	}
	require.Contains(t, names, handle)

	got := expectInto[gwspec.LlmProviderTemplateDocument](t, a.do(http.MethodGet, llmTemplatesPath+"/"+handle, nil), http.StatusOK)
	require.Equal(t, *created.Metadata.Uid, *got.Metadata.Uid)

	// A PUT replaces the spec: the description it leaves out is cleared.
	updated := expectInto[gwspec.LlmProviderTemplateDocument](t, a.do(http.MethodPut, llmTemplatesPath+"/"+handle,
		llmTemplateDoc(handle, map[string]any{"displayName": "Renamed"})), http.StatusOK)
	require.Equal(t, "Renamed", updated.Spec.DisplayName)
	require.Nil(t, updated.Spec.Description)
	require.Nil(t, updated.Spec.Metadata)

	rec := a.do(http.MethodDelete, llmTemplatesPath+"/"+handle, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	rec = a.do(http.MethodGet, llmTemplatesPath+"/"+handle, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "LLM provider template not found", errorMessage(t, rec))
}

// A template an LLM provider uses is not deleted until the provider is gone.
func TestLLMRoutesTemplateInUseIsNotDeleted(t *testing.T) {
	a := newRouteAPI(t)
	handle := unique("used-tpl")
	expect(t, a.do(http.MethodPost, llmTemplatesPath, llmTemplateDoc(handle, map[string]any{
		"displayName": "Used", "metadata": map[string]any{"endpointUrl": "https://llm.example.com/v1"},
	})), http.StatusCreated)
	provider := unique("on-custom")
	doc := llmProviderDoc(provider)
	doc["spec"].(map[string]any)["template"] = handle
	expect(t, a.do(http.MethodPost, "/llm-providers", doc), http.StatusCreated)

	rec := a.do(http.MethodDelete, llmTemplatesPath+"/"+handle, nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, gwerr.ErrLLMProviderTemplateInUse.Error(), errorMessage(t, rec))
	expect(t, a.do(http.MethodGet, llmTemplatesPath+"/"+handle, nil), http.StatusOK)

	expect(t, a.do(http.MethodDelete, "/llm-providers/"+provider, nil), http.StatusNoContent)
	expect(t, a.do(http.MethodDelete, llmTemplatesPath+"/"+handle, nil), http.StatusNoContent)
}

// A built-in template is readable but cannot be shadowed, changed or deleted.
func TestLLMRoutesTemplateBuiltInIsReadOnly(t *testing.T) {
	a := newRouteAPI(t)

	got := expectInto[gwspec.LlmProviderTemplateDocument](t, a.do(http.MethodGet, llmTemplatesPath+"/openai", nil), http.StatusOK)
	require.Equal(t, "openai", got.Metadata.Name)

	mine := llmTemplateDoc("openai", map[string]any{"displayName": "Mine"})
	rec := a.do(http.MethodPost, llmTemplatesPath, mine)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, "Cannot use handle of built-in template", errorMessage(t, rec))

	rec = a.do(http.MethodPut, llmTemplatesPath+"/openai", mine)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Equal(t, "System templates cannot be modified", errorMessage(t, rec))

	rec = a.do(http.MethodDelete, llmTemplatesPath+"/openai", nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Equal(t, "System templates cannot be deleted", errorMessage(t, rec))
}

// Template writes refuse bad documents, duplicates and unknown handles with the matching status.
func TestLLMRoutesTemplateErrors(t *testing.T) {
	a := newRouteAPI(t)
	existing := unique("dup-tpl")
	expect(t, a.do(http.MethodPost, llmTemplatesPath, llmTemplateDoc(existing, map[string]any{"displayName": "Dup"})), http.StatusCreated)
	missing := unique("missing-tpl")
	wrongKind := llmTemplateDoc(unique("x"), map[string]any{"displayName": "x"})
	wrongKind["kind"] = "LlmProvider"

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}{
		{"create malformed body", http.MethodPost, llmTemplatesPath, "{", http.StatusBadRequest, "Invalid request body"},
		{"create without a displayName", http.MethodPost, llmTemplatesPath, llmTemplateDoc(unique("x"), map[string]any{}),
			http.StatusBadRequest, "invalid document: spec.displayName is required"},
		{"create with the wrong kind", http.MethodPost, llmTemplatesPath, wrongKind,
			http.StatusBadRequest, `invalid document: kind must be "LlmProviderTemplate" on this route, got "LlmProvider"`},
		{"create with a bad handle", http.MethodPost, llmTemplatesPath, llmTemplateDoc("bad handle!", map[string]any{"displayName": "x"}),
			http.StatusBadRequest, "Invalid input"},
		{"create a duplicate", http.MethodPost, llmTemplatesPath, llmTemplateDoc(existing, map[string]any{"displayName": "Again"}),
			http.StatusConflict, "LLM provider template already exists"},
		{"get a bad handle", http.MethodGet, llmTemplatesPath + "/bad%20handle", nil, http.StatusBadRequest, "Invalid template id"},
		{"get a missing template", http.MethodGet, llmTemplatesPath + "/" + missing, nil, http.StatusNotFound, "LLM provider template not found"},
		{"update malformed body", http.MethodPut, llmTemplatesPath + "/" + existing, "{", http.StatusBadRequest, "Invalid request body"},
		{"update without a displayName", http.MethodPut, llmTemplatesPath + "/" + existing, llmTemplateDoc(existing, map[string]any{"description": "d"}),
			http.StatusBadRequest, "invalid document: spec.displayName is required"},
		{"update another template's name", http.MethodPut, llmTemplatesPath + "/" + existing, llmTemplateDoc(missing, map[string]any{"displayName": "x"}),
			http.StatusBadRequest, "metadata.name does not match the LLM provider template in the path"},
		{"update a missing template", http.MethodPut, llmTemplatesPath + "/" + missing, llmTemplateDoc(missing, map[string]any{"displayName": "x"}),
			http.StatusNotFound, "LLM provider template not found"},
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
		rec := a.do(tc.method, tc.path, llmTemplateDoc("x", map[string]any{"displayName": "x"}), read)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
}
