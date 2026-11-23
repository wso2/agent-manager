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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

// llmProxiesPath is the proxies collection of project.
func llmProxiesPath(project string) string { return "/orgs/{org}/projects/" + project + "/llm-proxies" }

// A proxy is created in a project, listed, read, updated and deleted.
func TestLLMRoutesProxyLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	project := unique("proj")
	projectID := dbtest.EnsureProject(t, a.db, a.org, project)
	providerUID := *llmCreateProvider(t, a, unique("prov")).Metadata.Uid
	name := unique("px")

	created := llmCreateProxy(t, a, project, providerUID, name)
	require.Equal(t, projectID.String(), created.ProjectId)
	require.Equal(t, providerUID, created.ProviderUuid)
	require.Equal(t, name, *created.Configuration.Name)
	require.Equal(t, "proxy "+name, *created.Description)

	list := expectInto[gwspec.LLMProxyListResponse](t, a.do(http.MethodGet, llmProxiesPath(project)+"?limit=999&offset=-1", nil), http.StatusOK)
	require.EqualValues(t, 1, list.Total)
	require.EqualValues(t, 100, list.Limit, "the limit is capped")
	require.EqualValues(t, 0, list.Offset)
	require.Equal(t, created.Uuid, list.Proxies[0].Uuid)

	got := expectInto[gwspec.LLMProxyResponse](t, a.do(http.MethodGet, llmProxiesPath(project)+"/"+name, nil), http.StatusOK)
	require.Equal(t, created.Uuid, got.Uuid)

	updated := expectInto[gwspec.LLMProxyResponse](t, a.do(http.MethodPut, llmProxiesPath(project)+"/"+name, map[string]any{
		"description":   "changed",
		"providerUuid":  providerUID,
		"configuration": map[string]any{"name": name, "version": "v1.0", "context": "/" + name, "provider": providerUID},
	}), http.StatusOK)
	require.Equal(t, "changed", *updated.Description)

	got = expectInto[gwspec.LLMProxyResponse](t, a.do(http.MethodGet, llmProxiesPath(project)+"/"+name, nil), http.StatusOK)
	require.Equal(t, "changed", *got.Description, "the update is stored")

	rec := a.do(http.MethodDelete, llmProxiesPath(project)+"/"+name, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	rec = a.do(http.MethodGet, llmProxiesPath(project)+"/"+name, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "LLM proxy not found", errorMessage(t, rec))
	list = expectInto[gwspec.LLMProxyListResponse](t, a.do(http.MethodGet, llmProxiesPath(project), nil), http.StatusOK)
	require.EqualValues(t, 0, list.Total)
}

// Proxy routes map an unknown project, bad input, unknown references and duplicates to their statuses.
func TestLLMRoutesProxyErrors(t *testing.T) {
	a := newRouteAPI(t)
	project := unique("proj")
	dbtest.EnsureProject(t, a.db, a.org, project)
	providerUID := *llmCreateProvider(t, a, unique("prov")).Metadata.Uid
	existing := unique("px")
	llmCreateProxy(t, a, project, providerUID, existing)
	base := llmProxiesPath(project)
	noProject := llmProxiesPath(unique("noproj"))
	unknownProvider := uuid.NewString()
	missing := unique("missing")

	noName := llmProxyBody(providerUID, "")
	unknownRef := llmProxyBody(unknownProvider, unique("px"))

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}{
		{"create in an unknown project", http.MethodPost, noProject, llmProxyBody(providerUID, unique("px")), http.StatusNotFound, "Project not found"},
		{"create malformed body", http.MethodPost, base, "{", http.StatusBadRequest, "Invalid request body"},
		{
			"create with a non-UUID provider", http.MethodPost, base, llmProxyBody("not-a-uuid", unique("px")), http.StatusBadRequest,
			"Invalid UUID in request",
		},
		{"create without a name", http.MethodPost, base, noName, http.StatusBadRequest, "Invalid input"},
		{"create on an unknown provider", http.MethodPost, base, unknownRef, http.StatusBadRequest, "Referenced provider not found"},
		{"create a duplicate", http.MethodPost, base, llmProxyBody(providerUID, existing), http.StatusConflict, "LLM proxy already exists"},
		{"list an unknown project", http.MethodGet, noProject, nil, http.StatusNotFound, "Project not found"},
		{"get in an unknown project", http.MethodGet, noProject + "/" + existing, nil, http.StatusNotFound, "Project not found"},
		{"get an unknown proxy", http.MethodGet, base + "/" + missing, nil, http.StatusNotFound, "LLM proxy not found"},
		{"update in an unknown project", http.MethodPut, noProject + "/" + existing, map[string]any{}, http.StatusNotFound, "Project not found"},
		{"update malformed body", http.MethodPut, base + "/" + existing, "{", http.StatusBadRequest, "Invalid request body"},
		{
			"update with a non-UUID provider", http.MethodPut, base + "/" + existing,
			map[string]any{"providerUuid": "nope"},
			http.StatusBadRequest,
			"Invalid UUID in request",
		},
		{
			"update an unknown proxy", http.MethodPut, base + "/" + missing,
			map[string]any{"providerUuid": providerUID},
			http.StatusNotFound,
			"LLM proxy not found",
		},
		{"delete in an unknown project", http.MethodDelete, noProject + "/" + existing, nil, http.StatusNotFound, "Project not found"},
		{"delete an unknown proxy", http.MethodDelete, base + "/" + missing, nil, http.StatusNotFound, "LLM proxy not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := a.do(tt.method, tt.path, tt.body)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, tt.message, errorMessage(t, rec))
		})
	}

	list := expectInto[gwspec.LLMProxyListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.EqualValues(t, 1, list.Total, "no refused create is stored")
}

// Each proxy route needs its own permission.
func TestLLMRoutesProxyNeedsScope(t *testing.T) {
	a := newRouteAPI(t)
	base := llmProxiesPath("p")
	read := withScopes(scope(gwperm.LLMProxyRead))
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, base},
		{http.MethodPut, base + "/x"},
		{http.MethodDelete, base + "/x"},
	} {
		rec := a.do(tc.method, tc.path, nil, read)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
	write := withScopes(scope(gwperm.LLMProxyCreate), scope(gwperm.LLMProxyUpdate), scope(gwperm.LLMProxyDelete))
	for _, p := range []string{base, base + "/x"} {
		rec := a.do(http.MethodGet, p, nil, write)
		require.Equal(t, http.StatusForbidden, rec.Code, "GET %s: %s", p, rec.Body.String())
	}
}
