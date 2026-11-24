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

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// llmProviderUpdate is a PUT document that names handle and sets only spec.
func llmProviderUpdate(handle string, spec map[string]any) map[string]any {
	if _, ok := spec["displayName"]; !ok {
		spec["displayName"] = "Provider " + handle
	}
	return map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1",
		"kind":       "LlmProvider",
		"metadata":   map[string]any{"name": handle},
		"spec":       spec,
	}
}

// A provider document is created, read by handle and by uid, listed, updated and deleted.
func TestLLMRoutesProviderLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	handle := unique("prov")

	created := llmCreateProvider(t, a, handle)
	require.Equal(t, "LlmProvider", created.Kind)
	require.Equal(t, handle, created.Metadata.Name)
	require.Equal(t, "Provider "+handle, created.Spec.DisplayName)
	require.Equal(t, "openai", created.Spec.Template)
	require.Equal(t, "v1.0", created.Spec.Version)
	require.NotNil(t, created.Status)
	require.Empty(t, created.Status.Gateways, "a provider created without gateways is placed nowhere")
	uid := *created.Metadata.Uid

	for _, id := range []string{handle, uid} {
		got := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodGet, "/llm-providers/"+id, nil), http.StatusOK)
		require.Equal(t, uid, *got.Metadata.Uid)
	}

	list := expectInto[gwspec.LLMProviderListResponse](t, a.do(http.MethodGet, "/llm-providers?limit=0&offset=-1", nil), http.StatusOK)
	require.EqualValues(t, 1, *list.Count)
	require.EqualValues(t, 20, *list.Pagination.Limit, "a zero limit falls back to the default")
	require.EqualValues(t, 1, *list.Pagination.Total)
	require.Equal(t, handle, list.List[0].Metadata.Name)
	require.Nil(t, list.List[0].Spec.Openapi, "list rows leave out the OpenAPI spec")

	updated := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodPut, "/llm-providers/"+uid,
		llmProviderUpdate(handle, map[string]any{
			"displayName": "Renamed",
			"description": "changed",
			"upstream": map[string]any{"main": map[string]any{
				"url": "https://llm.example.com/v1", "auth": map[string]any{"type": "api-key", "header": "Authorization"},
			}},
		})), http.StatusOK)
	require.Equal(t, "Renamed", updated.Spec.DisplayName)
	require.Equal(t, "changed", *updated.Spec.Description)
	require.Equal(t, "v1.0", updated.Spec.Version, "an omitted version keeps the stored one")
	require.Equal(t, "/"+handle, updated.Spec.Context, "an omitted context keeps the stored one")

	got := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodGet, "/llm-providers/"+uid, nil), http.StatusOK)
	require.Equal(t, "Renamed", got.Spec.DisplayName)
	require.Equal(t, "https://llm.example.com/v1", *got.Spec.Upstream.Main.Url)

	rec := a.do(http.MethodDelete, "/llm-providers/"+handle, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	rec = a.do(http.MethodGet, "/llm-providers/"+handle, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "LLM provider not found", errorMessage(t, rec))
}

// spec.inCatalog is applied on create and on update, and read back.
func TestLLMRoutesProviderCatalogFlag(t *testing.T) {
	a := newRouteAPI(t)
	handle := unique("cat")
	body := llmProviderDoc(handle)
	body["spec"].(map[string]any)["inCatalog"] = false

	created := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodPost, "/llm-providers", body), http.StatusCreated)
	require.False(t, *created.Spec.InCatalog)
	uid := *created.Metadata.Uid

	updated := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodPut, "/llm-providers/"+uid,
		llmProviderUpdate(handle, map[string]any{"inCatalog": true})), http.StatusOK)
	require.True(t, *updated.Spec.InCatalog)

	got := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodGet, "/llm-providers/"+handle, nil), http.StatusOK)
	require.True(t, *got.Spec.InCatalog, "the catalog change is stored")

	// The handle names the provider on update too, as it does on GET and DELETE.
	byHandle := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodPut, "/llm-providers/"+handle,
		llmProviderUpdate(handle, map[string]any{"inCatalog": false, "gateways": []string{}})), http.StatusOK)
	require.False(t, *byHandle.Spec.InCatalog)
	require.Equal(t, uid, *byHandle.Metadata.Uid)
}

// spec.gateways deploys on create, is reported back, and an empty list on update undeploys.
func TestLLMRoutesProviderGatewaysPlacement(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	handle := unique("placed")

	created := llmCreateProvider(t, a, handle, gw.UUID.String())
	require.Equal(t, []string{gw.UUID.String()}, created.Spec.Gateways)
	require.Len(t, created.Status.Gateways, 1)
	require.Equal(t, gw.UUID.String(), created.Status.Gateways[0].GatewayId)
	uid := *created.Metadata.Uid

	deployments := expectInto[[]gwspec.LLMDeploymentResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/llm-providers/"+handle+"/deployments", nil), http.StatusOK)
	require.Len(t, deployments, 1)
	require.Equal(t, gw.UUID.String(), deployments[0].GatewayId)

	list := expectInto[gwspec.LLMProviderListResponse](t, a.do(http.MethodGet, "/llm-providers", nil), http.StatusOK)
	require.Equal(t, []string{gw.UUID.String()}, list.List[0].Spec.Gateways)

	updated := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodPut, "/llm-providers/"+uid,
		llmProviderUpdate(handle, map[string]any{"gateways": []string{}})), http.StatusOK)
	require.Equal(t, "Provider "+handle, updated.Spec.DisplayName)

	undeploying := expectInto[[]gwspec.LLMDeploymentResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/llm-providers/"+handle+"/deployments?status=UNDEPLOYING", nil), http.StatusOK)
	require.Len(t, undeploying, 1, "the empty gateway list undeploys the provider")
}

// A create that names an ingress-only gateway is refused and stores nothing.
func TestLLMRoutesProviderCreateOnIngressGatewayFails(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleIngress)
	handle := unique("ingress")

	rec := a.do(http.MethodPost, "/llm-providers", llmProviderDoc(handle, gw.UUID.String()))
	require.GreaterOrEqual(t, rec.Code, http.StatusBadRequest, rec.Body.String())

	rec = a.do(http.MethodGet, "/llm-providers/"+handle, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, "the failed create is rolled back: %s", rec.Body.String())
}

// Provider creates refuse a bad document or input with 400 and a reused handle with 409.
func TestLLMRoutesProviderCreateErrors(t *testing.T) {
	a := newRouteAPI(t)
	existing := unique("taken")
	llmCreateProvider(t, a, existing)

	withSpec := func(handle string, change func(spec map[string]any)) map[string]any {
		doc := llmProviderDoc(handle)
		change(doc["spec"].(map[string]any))
		return doc
	}
	wrongVersion := llmProviderDoc(unique("v"))
	wrongVersion["apiVersion"] = "v0"
	wrongKind := llmProviderDoc(unique("k"))
	wrongKind["kind"] = "LlmProxy"
	noName := llmProviderDoc("")

	tests := []struct {
		name    string
		body    any
		status  int
		message string
	}{
		{"malformed body", "{", http.StatusBadRequest, "Invalid request body"},
		{
			"wrong apiVersion", wrongVersion, http.StatusBadRequest,
			`invalid document: apiVersion must be "ai-control-plane.choreolabs.com/v1", got "v0"`,
		},
		{"wrong kind", wrongKind, http.StatusBadRequest, `invalid document: kind must be "LlmProvider" on this route, got "LlmProxy"`},
		{"no metadata.name", noName, http.StatusBadRequest, "invalid document: metadata.name is required - it is the provider's handle"},
		{
			"no displayName", withSpec(unique("d"), func(s map[string]any) { s["displayName"] = " " }), http.StatusBadRequest,
			"invalid document: spec.displayName is required",
		},
		{"no version", withSpec(unique("nv"), func(s map[string]any) { delete(s, "version") }), http.StatusBadRequest, "Invalid input"},
		{"malformed version", withSpec(unique("bv"), func(s map[string]any) { s["version"] = "one" }), http.StatusBadRequest, "Invalid input"},
		{"no template", withSpec(unique("nt"), func(s map[string]any) { s["template"] = "" }), http.StatusBadRequest, "Invalid input"},
		{
			"unknown template", withSpec(unique("ut"), func(s map[string]any) { s["template"] = "no-such-template" }), http.StatusBadRequest,
			"Referenced template not found",
		},
		{"reused handle", llmProviderDoc(existing), http.StatusConflict, "A provider with the ID \"" + existing +
			"\" already exists (auto-generated from the name). Try a different name, or rename/delete the existing provider first."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, "/llm-providers", tt.body)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, tt.message, errorMessage(t, rec))
		})
	}

	list := expectInto[gwspec.LLMProviderListResponse](t, a.do(http.MethodGet, "/llm-providers", nil), http.StatusOK)
	require.EqualValues(t, 1, *list.Count, "no refused create is stored")
}

// Provider updates refuse a bad document, a mismatched name, bad input and an unknown provider.
func TestLLMRoutesProviderUpdateErrors(t *testing.T) {
	a := newRouteAPI(t)
	handle := unique("upd")
	path := "/llm-providers/" + *llmCreateProvider(t, a, handle).Metadata.Uid

	noSpec := llmProviderUpdate(handle, map[string]any{})
	delete(noSpec, "spec")

	tests := []struct {
		name    string
		path    string
		body    any
		status  int
		message string
	}{
		{"malformed body", path, "{", http.StatusBadRequest, "invalid document: unexpected end of JSON input"},
		{"no spec", path, noSpec, http.StatusBadRequest, "invalid document: spec.displayName is required"},
		{
			"unknown provider", "/llm-providers/" + unique("missing"), llmProviderUpdate("x", map[string]any{}), http.StatusNotFound,
			"LLM provider not found",
		},
		{
			"another provider's name", path, llmProviderUpdate("someone-else", map[string]any{}), http.StatusBadRequest,
			"metadata.name does not match the LLM provider in the path",
		},
		{
			"unknown template", path, llmProviderUpdate(handle, map[string]any{"template": "no-such-template"}), http.StatusBadRequest,
			"Referenced template not found",
		},
		{
			"unknown template with gateways", path, llmProviderUpdate(handle, map[string]any{"template": "no-such-template", "gateways": []string{}}),
			http.StatusBadRequest, "Referenced template not found",
		},
		{"malformed version", path, llmProviderUpdate(handle, map[string]any{"version": "one"}), http.StatusBadRequest, "Invalid input"},
		{
			"malformed version with gateways", path, llmProviderUpdate(handle, map[string]any{"version": "one", "gateways": []string{}}),
			http.StatusBadRequest, "Invalid input",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := a.do(http.MethodPut, tt.path, tt.body)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, tt.message, errorMessage(t, rec))
		})
	}

	got := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodGet, path, nil), http.StatusOK)
	require.Equal(t, "v1.0", got.Spec.Version, "no refused update is stored")
	require.Equal(t, "openai", got.Spec.Template)
}

// Deleting an unknown provider is a 404, and one a proxy still uses is a 409.
func TestLLMRoutesProviderDeleteErrors(t *testing.T) {
	a := newRouteAPI(t)
	rec := a.do(http.MethodDelete, "/llm-providers/"+unique("missing"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "LLM provider not found", errorMessage(t, rec))

	handle := unique("used")
	created := llmCreateProvider(t, a, handle)
	a.agent("llm-del", unique("agent"))
	llmCreateProxy(t, a, "llm-del", *created.Metadata.Uid, unique("px"))

	rec = a.do(http.MethodDelete, "/llm-providers/"+handle, nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.NotEmpty(t, errorMessage(t, rec))
	expect(t, a.do(http.MethodGet, "/llm-providers/"+handle, nil), http.StatusOK)
}

// The available policies are those the organization's active gateways report, scoped to a provider's gateways
// when one is named.
func TestLLMRoutesProviderPolicies(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	require.NoError(t, a.db.Model(gw).Select("manifest").Updates(&gwmodels.Gateway{Manifest: map[string]any{
		"policies": []any{map[string]any{"name": "word-count-guardrail", "version": "v1.0.0", "displayName": "Word count"}},
	}}).Error)

	all := expect(t, a.do(http.MethodGet, "/orgs/{org}/llm-providers/policies", nil), http.StatusOK)
	require.EqualValues(t, 1, all["count"])
	require.Equal(t, "word-count-guardrail", all["list"].([]any)[0].(map[string]any)["name"])

	handle := unique("pol")
	llmCreateProvider(t, a, handle)
	none := expect(t, a.do(http.MethodGet, "/orgs/{org}/llm-providers/policies?providerId="+handle, nil), http.StatusOK)
	require.EqualValues(t, 0, none["count"], "a provider placed nowhere has no policies")

	rec := a.do(http.MethodGet, "/orgs/{org}/llm-providers/policies?providerId="+unique("missing"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "LLM provider not found", errorMessage(t, rec))
}

// The proxies on a provider and the agents that consume them are listed per provider.
func TestLLMRoutesProviderProxiesAndConsumers(t *testing.T) {
	a := newRouteAPI(t)
	handle := unique("cons")
	created := llmCreateProvider(t, a, handle)
	uid := *created.Metadata.Uid

	empty := expectInto[gwspec.LLMProviderConsumerListResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/llm-providers/"+handle+"/consumers", nil), http.StatusOK)
	require.Empty(t, empty.Consumers)

	agentID := a.agent("llm-cons", "cons-agent")
	proxy := llmCreateProxy(t, a, "llm-cons", uid, unique("px"))
	llmCreateProxy(t, a, "llm-cons", uid, unique("px"))
	gwtest.LLMConfig(t, a.db, a.org, agentID, a.defaultEnv(), llmProxyUUID(t, proxy))

	byProvider := expectInto[gwspec.LLMProxyListResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/llm-providers/"+uid+"/llm-proxies?limit=1&offset=-2", nil), http.StatusOK)
	require.EqualValues(t, 2, byProvider.Total)
	require.Len(t, byProvider.Proxies, 1)
	require.EqualValues(t, 1, byProvider.Limit)
	require.Equal(t, uid, byProvider.Proxies[0].ProviderUuid)

	consumers := expectInto[gwspec.LLMProviderConsumerListResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/llm-providers/"+uid+"/consumers", nil), http.StatusOK)
	require.EqualValues(t, 1, consumers.Total)
	require.Equal(t, "agent", consumers.Consumers[0].ConsumerType)
	require.Equal(t, "llm-cons", consumers.Consumers[0].ProjectName)

	byHandle := expectInto[gwspec.LLMProxyListResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/llm-providers/"+handle+"/llm-proxies", nil), http.StatusOK)
	require.EqualValues(t, 2, byHandle.Total)

	// An unknown provider is a 404 whether it is named by UUID or by handle.
	for _, id := range []string{uuid.NewString(), unique("missing")} {
		for _, p := range []string{"/llm-proxies", "/consumers"} {
			rec := a.do(http.MethodGet, "/orgs/{org}/llm-providers/"+id+p, nil)
			require.Equal(t, http.StatusNotFound, rec.Code, "%s%s: %s", id, p, rec.Body.String())
			require.Equal(t, "LLM provider not found", errorMessage(t, rec))
		}
	}
}

// Each provider route needs its own permission.
func TestLLMRoutesProviderNeedsScope(t *testing.T) {
	a := newRouteAPI(t)
	other := withScopes(scope(gwperm.LLMProxyRead))
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/llm-providers"},
		{http.MethodGet, "/llm-providers"},
		{http.MethodGet, "/llm-providers/x"},
		{http.MethodPut, "/llm-providers/x"},
		{http.MethodDelete, "/llm-providers/x"},
		{http.MethodGet, "/orgs/{org}/llm-providers/policies"},
		{http.MethodGet, "/orgs/{org}/llm-providers/x/consumers"},
	} {
		rec := a.do(tc.method, tc.path, nil, other)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
	rec := a.do(http.MethodGet, "/orgs/{org}/llm-providers/x/llm-proxies", nil, withScopes(scope(gwperm.LLMProviderRead)))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

// A PUT that declares gateways deploys the provider to them, and deleting the provider takes it off them.
func TestLLMRoutesProviderUpdateDeploysAndDeleteUndeploys(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	handle := unique("sync")
	uid := *llmCreateProvider(t, a, handle).Metadata.Uid

	updated := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodPut, "/llm-providers/"+uid,
		llmProviderUpdate(handle, map[string]any{"gateways": []string{gw.UUID.String()}, "description": "placed"})), http.StatusOK)
	require.Equal(t, []string{gw.UUID.String()}, updated.Spec.Gateways)
	require.Len(t, updated.Status.Gateways, 1)
	require.Equal(t, "placed", *updated.Spec.Description)

	got := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodGet, "/llm-providers/"+handle, nil), http.StatusOK)
	require.Equal(t, []string{gw.UUID.String()}, got.Spec.Gateways, "the placement is stored")

	rec := a.do(http.MethodDelete, "/llm-providers/"+uid, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, "/llm-providers/"+uid, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}
