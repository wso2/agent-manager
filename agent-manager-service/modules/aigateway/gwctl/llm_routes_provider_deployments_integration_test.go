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

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// llmDeploymentQuery is the query an undeploy or restore names its deployment with.
func llmDeploymentQuery(deploymentID, gatewayID string) string {
	return "?deploymentId=" + deploymentID + "&gatewayId=" + gatewayID
}

// A provider is deployed to a gateway, read back, undeployed, restored, and its superseded deployment deleted.
func TestLLMRoutesProviderDeploymentLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	gwID := gw.UUID.String()
	handle := unique("dep")
	uid := *llmCreateProvider(t, a, handle).Metadata.Uid
	base := "/orgs/{org}/llm-providers/" + handle + "/deployments"

	first := expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodPost, base, map[string]any{"gatewayId": gwID}), http.StatusCreated)
	require.NotEmpty(t, first.DeploymentId)
	require.Equal(t, gwID, first.GatewayId)
	require.Equal(t, uid, first.ArtifactId)

	got := expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base+"/"+first.DeploymentId, nil), http.StatusOK)
	require.Equal(t, first.DeploymentId, got.DeploymentId)
	require.Equal(t, first.Status, got.Status)

	list := expectInto[[]gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base+"?gatewayId="+gwID, nil), http.StatusOK)
	require.Len(t, list, 1)

	rec := a.do(http.MethodDelete, base+"/"+first.DeploymentId, nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, "Cannot delete the gateway's current deployment - undeploy it and wait until it is UNDEPLOYED", errorMessage(t, rec))

	rec = a.do(http.MethodPost, base+"/restore"+llmDeploymentQuery(first.DeploymentId, gwID), nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, "Cannot restore the deployment the gateway is already deploying or serving", errorMessage(t, rec))

	undeployed := expect(t, a.do(http.MethodPost, base+"/undeploy"+llmDeploymentQuery(first.DeploymentId, gwID), nil), http.StatusOK)
	require.Equal(t, "LLM provider undeployed successfully", undeployed["message"])

	got = expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base+"/"+first.DeploymentId, nil), http.StatusOK)
	require.NotEqual(t, first.Status, got.Status, "the undeploy is recorded")

	restored := expectInto[gwspec.LLMDeploymentResponse](t,
		a.do(http.MethodPost, base+"/restore"+llmDeploymentQuery(first.DeploymentId, gwID), nil), http.StatusOK)
	require.Equal(t, first.DeploymentId, restored.DeploymentId)

	// A provider change and a redeploy supersede the first deployment, which can then be deleted.
	expect(t, a.do(http.MethodPut, "/llm-providers/"+uid, llmProviderUpdate(handle, map[string]any{"description": "v2"})), http.StatusOK)
	second := expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodPost, base, map[string]any{"gatewayId": gwID, "base": "current"}), http.StatusCreated)
	require.NotEqual(t, first.DeploymentId, second.DeploymentId)

	rec = a.do(http.MethodDelete, base+"/"+first.DeploymentId, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	remaining := expectInto[[]gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.Len(t, remaining, 1, "the deleted deployment is gone")
	require.Equal(t, second.DeploymentId, remaining[0].DeploymentId)
}

// Provider deployment routes map bad input, unknown resources and state conflicts to their statuses.
func TestLLMRoutesProviderDeploymentErrors(t *testing.T) {
	a := newRouteAPI(t)
	egress := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	ingress := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleIngress)
	handle := unique("deperr")
	llmCreateProvider(t, a, handle)
	base := "/orgs/{org}/llm-providers/" + handle + "/deployments"
	dep := expectInto[gwspec.LLMDeploymentResponse](t, a.do(http.MethodPost, base, map[string]any{"gatewayId": egress.UUID.String()}), http.StatusCreated)
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
			map[string]any{"gatewayId": egress.UUID.String(), "base": dep.DeploymentId},
			http.StatusBadRequest, gwerr.ErrDeploymentBaseNotCurrent.Error(),
		},
		{"deploy to an ingress gateway", http.MethodPost, base, map[string]any{"gatewayId": ingress.UUID.String()}, http.StatusBadRequest, "Invalid input"},
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

// Reading a provider's deployments needs read access; changing them needs deploy access.
func TestLLMRoutesProviderDeploymentNeedsScope(t *testing.T) {
	a := newRouteAPI(t)
	base := "/orgs/{org}/llm-providers/x/deployments"
	read := withScopes(scope(gwperm.LLMProviderRead))
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, base},
		{http.MethodPost, base + "/undeploy"},
		{http.MethodPost, base + "/restore"},
		{http.MethodDelete, base + "/d"},
	} {
		rec := a.do(tc.method, tc.path, nil, read)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
	deploy := withScopes(scope(gwperm.LLMProviderDeploy))
	for _, p := range []string{base, base + "/d"} {
		rec := a.do(http.MethodGet, p, nil, deploy)
		require.Equal(t, http.StatusForbidden, rec.Code, "GET %s: %s", p, rec.Body.String())
	}
}
