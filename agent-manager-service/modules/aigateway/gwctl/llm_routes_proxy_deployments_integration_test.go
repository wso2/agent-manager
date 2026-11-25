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
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// llmProxyDeploymentsFixture is a proxy in a fresh project with an egress gateway to deploy it to.
type llmProxyDeploymentsFixture struct {
	a       *routeAPI
	gateway string
	proxy   gwspec.LLMProxyResponse
	base    string // the proxy's deployments collection
}

func llmNewProxyDeploymentsFixture(t *testing.T) llmProxyDeploymentsFixture {
	t.Helper()
	a := newRouteAPI(t)
	project := unique("proj")
	dbtest.EnsureProject(t, a.db, a.org, project)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	providerUID := *llmCreateProvider(t, a, unique("prov")).Metadata.Uid
	name := unique("px")
	proxy := llmCreateProxy(t, a, project, providerUID, name)
	return llmProxyDeploymentsFixture{
		a: a, gateway: gw.UUID.String(), proxy: proxy,
		base: llmProxiesPath(project) + "/" + name + "/deployments",
	}
}

// A proxy is deployed to a gateway, read back, undeployed, restored, and its superseded deployment deleted.
func TestLLMRoutesProxyDeploymentLifecycle(t *testing.T) {
	f := llmNewProxyDeploymentsFixture(t)
	a, base, gwID := f.a, f.base, f.gateway

	first := expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodPost, base, map[string]any{"gatewayId": gwID}), http.StatusCreated)
	require.Equal(t, gwID, first.GatewayId)
	require.Equal(t, f.proxy.Uuid, first.ArtifactId)

	got := expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base+"/"+first.DeploymentId, nil), http.StatusOK)
	require.Equal(t, first.DeploymentId, got.DeploymentId)

	list := expectInto[[]gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base+"?gatewayId="+gwID+"&status="+first.Status, nil), http.StatusOK)
	require.Len(t, list, 1)

	rec := a.do(http.MethodDelete, base+"/"+first.DeploymentId, nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, "Cannot delete the gateway's current deployment - undeploy it and wait until it is UNDEPLOYED", errorMessage(t, rec))

	rec = a.do(http.MethodPost, base+"/restore"+llmDeploymentQuery(first.DeploymentId, gwID), nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, "Cannot restore the deployment the gateway is already deploying or serving", errorMessage(t, rec))

	undeployed := expect(t, a.do(http.MethodPost, base+"/undeploy"+llmDeploymentQuery(first.DeploymentId, gwID), nil), http.StatusOK)
	require.Equal(t, "LLM proxy undeployed successfully", undeployed["message"])

	got = expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base+"/"+first.DeploymentId, nil), http.StatusOK)
	require.NotEqual(t, first.Status, got.Status, "the undeploy is recorded")

	rec = a.do(http.MethodPost, base+"/undeploy"+llmDeploymentQuery(first.DeploymentId, gwID), nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())

	restored := expectInto[gwspec.LLMDeploymentResponse](t,
		a.do(http.MethodPost, base+"/restore"+llmDeploymentQuery(first.DeploymentId, gwID), nil), http.StatusOK)
	require.Equal(t, first.DeploymentId, restored.DeploymentId)

	// A proxy change and a redeploy supersede the first deployment, which can then be deleted.
	proxyPath := base[:len(base)-len("/deployments")]
	expect(t, a.do(http.MethodPut, proxyPath, map[string]any{
		"description": "v2", "providerUuid": f.proxy.ProviderUuid,
		"configuration": map[string]any{
			"name": *f.proxy.Configuration.Name, "version": "v1.0", "context": *f.proxy.Configuration.Context, "provider": f.proxy.ProviderUuid,
		},
	}), http.StatusOK)
	second := expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodPost, base, map[string]any{"gatewayId": gwID}), http.StatusCreated)
	require.NotEqual(t, first.DeploymentId, second.DeploymentId)

	rec = a.do(http.MethodDelete, base+"/"+first.DeploymentId, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	remaining := expectInto[[]gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.Len(t, remaining, 1, "the deleted deployment is gone")
	require.Equal(t, second.DeploymentId, remaining[0].DeploymentId)

	// A deployed proxy is undeployed as part of its deletion.
	rec = a.do(http.MethodDelete, proxyPath, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

// Proxy deployment routes map bad input, unknown resources and state conflicts to their statuses.
func TestLLMRoutesProxyDeploymentErrors(t *testing.T) {
	f := llmNewProxyDeploymentsFixture(t)
	a, base := f.a, f.base
	ingress := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleIngress)
	dep := expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodPost, base, map[string]any{"gatewayId": f.gateway}), http.StatusCreated)
	unknown := uuid.NewString()

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}{
		{"deploy malformed body", http.MethodPost, base, "{", http.StatusBadRequest, "Invalid request body"},
		{"deploy without a gateway", http.MethodPost, base, map[string]any{}, http.StatusBadRequest, "gatewayId is required"},
		{
			"deploy an earlier base", http.MethodPost, base,
			map[string]any{"gatewayId": f.gateway, "base": dep.DeploymentId},
			http.StatusBadRequest, gwerr.ErrDeploymentBaseNotCurrent.Error(),
		},
		{"deploy to an ingress gateway", http.MethodPost, base, map[string]any{"gatewayId": ingress.UUID.String()}, http.StatusBadRequest, "Invalid input"},
		{"deploy to an unknown gateway", http.MethodPost, base, map[string]any{"gatewayId": unknown}, http.StatusNotFound, "Gateway not found"},
		{
			"undeploy without a deployment", http.MethodPost, base + "/undeploy?gatewayId=" + unknown, nil, http.StatusBadRequest,
			"deploymentId query parameter is required",
		},
		{
			"undeploy without a gateway", http.MethodPost, base + "/undeploy?deploymentId=" + unknown, nil, http.StatusBadRequest,
			"gatewayId query parameter is required",
		},
		{
			"undeploy on another gateway", http.MethodPost, base + "/undeploy" + llmDeploymentQuery(dep.DeploymentId, unknown), nil, http.StatusBadRequest,
			"Deployment is bound to a different gateway",
		},
		{
			"restore without a deployment", http.MethodPost, base + "/restore?gatewayId=" + unknown, nil, http.StatusBadRequest,
			"deploymentId query parameter is required",
		},
		{
			"restore without a gateway", http.MethodPost, base + "/restore?deploymentId=" + unknown, nil, http.StatusBadRequest,
			"gatewayId query parameter is required",
		},
		{
			"restore an unknown deployment", http.MethodPost, base + "/restore" + llmDeploymentQuery(unknown, unknown), nil, http.StatusNotFound,
			"Deployment not found",
		},
		{"get an unknown deployment", http.MethodGet, base + "/" + unknown, nil, http.StatusNotFound, "Deployment not found"},
		{"delete an unknown deployment", http.MethodDelete, base + "/" + unknown, nil, http.StatusNotFound, "Deployment not found"},
		{
			"undeploy an unknown deployment", http.MethodPost, base + "/undeploy" + llmDeploymentQuery(unknown, unknown), nil, http.StatusNotFound,
			"Deployment not found",
		},
		{
			"restore on another gateway", http.MethodPost, base + "/restore" + llmDeploymentQuery(dep.DeploymentId, unknown), nil, http.StatusBadRequest,
			"Deployment is bound to a different gateway",
		},
		{"list with a bad status", http.MethodGet, base + "?status=BOGUS", nil, http.StatusBadRequest, "Invalid deployment status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := a.do(tt.method, tt.path, tt.body)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, tt.message, errorMessage(t, rec))
		})
	}

	list := expectInto[[]gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.Len(t, list, 1, "no refused deploy is stored")
}

// Reading a proxy's deployments needs read access; changing them needs deploy access.
func TestLLMRoutesProxyDeploymentNeedsScope(t *testing.T) {
	a := newRouteAPI(t)
	base := llmProxiesPath("p") + "/x/deployments"
	read := withScopes(scope(gwperm.LLMProxyRead))
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, base},
		{http.MethodPost, base + "/undeploy"},
		{http.MethodPost, base + "/restore"},
		{http.MethodDelete, base + "/d"},
	} {
		rec := a.do(tc.method, tc.path, nil, read)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
	deploy := withScopes(scope(gwperm.LLMProxyDeploy))
	for _, p := range []string{base, base + "/d"} {
		rec := a.do(http.MethodGet, p, nil, deploy)
		require.Equal(t, http.StatusForbidden, rec.Code, "GET %s: %s", p, rec.Body.String())
	}
}

// Proxy deployment routes answer an unknown proxy with 404.
func TestLLMRoutesProxyDeploymentNotFound(t *testing.T) {
	f := llmNewProxyDeploymentsFixture(t)
	a, base := f.a, f.base
	expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodPost, base, map[string]any{"gatewayId": f.gateway}), http.StatusCreated)
	missingBase := base[:len(base)-len("/deployments")] + "-missing/deployments"
	unknown := uuid.NewString()

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		message string
	}{
		{"deploy an unknown proxy", http.MethodPost, missingBase, map[string]any{"gatewayId": f.gateway}, "LLM proxy not found"},
		{"undeploy an unknown proxy", http.MethodPost, missingBase + "/undeploy" + llmDeploymentQuery(unknown, unknown), nil, "LLM proxy not found"},
		{"restore an unknown proxy", http.MethodPost, missingBase + "/restore" + llmDeploymentQuery(unknown, unknown), nil, "LLM proxy not found"},
		{"list an unknown proxy", http.MethodGet, missingBase, nil, "LLM proxy not found"},
		{"get on an unknown proxy", http.MethodGet, missingBase + "/" + unknown, nil, "LLM proxy not found"},
		{"delete on an unknown proxy", http.MethodDelete, missingBase + "/" + unknown, nil, "LLM proxy not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := a.do(tt.method, tt.path, tt.body)
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			require.Equal(t, tt.message, errorMessage(t, rec))
		})
	}
}

// Every public change to a proxy an agent's LLM configuration uses is refused, and the proxy is kept.
func TestLLMRoutesProxyUsedByAnAgentIsReadOnly(t *testing.T) {
	f := llmNewProxyDeploymentsFixture(t)
	a, base := f.a, f.base
	dep := expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodPost, base, map[string]any{"gatewayId": f.gateway}), http.StatusCreated)
	agentID := a.agent(unique("proj"), unique("agent"))
	gwtest.LLMConfig(t, a.db, a.org, agentID, a.defaultEnv(), llmProxyUUID(t, f.proxy))
	proxyPath := base[:len(base)-len("/deployments")]
	query := llmDeploymentQuery(dep.DeploymentId, f.gateway)

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, proxyPath, map[string]any{"description": "changed"}},
		{http.MethodDelete, proxyPath, nil},
		{http.MethodPost, base, map[string]any{"gatewayId": f.gateway}},
		{http.MethodPost, base + "/undeploy" + query, nil},
		{http.MethodPost, base + "/restore" + query, nil},
		{http.MethodDelete, base + "/" + dep.DeploymentId, nil},
	} {
		rec := a.do(tc.method, tc.path, tc.body)
		require.Equal(t, http.StatusConflict, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
		require.Equal(t, gwerr.ErrLLMProxyManagedByAMS.Error(), errorMessage(t, rec))
	}

	got := expectInto[gwspec.LLMProxyResponse](t, a.do(http.MethodGet, proxyPath, nil), http.StatusOK)
	require.Equal(t, *f.proxy.Description, *got.Description, "the proxy is unchanged")
	list := expectInto[[]gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.Len(t, list, 1)
	require.Equal(t, dep.Status, list[0].Status, "the deployment is unchanged")
}
