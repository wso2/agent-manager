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
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// llmDeploymentQuery is the query a restore names its deployment with.
func llmDeploymentQuery(deploymentID, gatewayID string) string {
	return "?deploymentId=" + deploymentID + "&gatewayId=" + gatewayID
}

// llmDeploymentsPath is the deployment history of the provider with the given handle.
func llmDeploymentsPath(handle string) string {
	return "/llm-providers/" + handle + "/deployments"
}

// A provider placed through spec.gateways is listed, undeployed by an empty list, and restored.
func TestLLMRoutesProviderDeploymentLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	gwID := gw.UUID.String()
	handle := unique("dep")
	uid := *llmCreateProvider(t, a, handle, gwID).Metadata.Uid
	base := llmDeploymentsPath(handle)

	list := expectInto[[]gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base+"?gatewayId="+gwID, nil), http.StatusOK)
	require.Len(t, list, 1)
	first := list[0]
	require.NotEmpty(t, first.DeploymentId)
	require.Equal(t, uid, first.ArtifactId)

	rec := a.do(http.MethodPost, base+"/restore"+llmDeploymentQuery(first.DeploymentId, gwID), nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, "Cannot restore the deployment the gateway is already deploying or serving", errorMessage(t, rec))

	expect(t, a.do(http.MethodPut, "/llm-providers/"+uid, llmProviderUpdate(handle, map[string]any{"gateways": []string{}})), http.StatusOK)
	undeploying := expectInto[[]gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base+"?status=UNDEPLOYING", nil), http.StatusOK)
	require.Len(t, undeploying, 1, "the undeploy is recorded")

	restored := expectInto[gwspec.LLMDeploymentResponse](t,
		a.do(http.MethodPost, base+"/restore"+llmDeploymentQuery(first.DeploymentId, gwID), nil), http.StatusOK)
	require.Equal(t, first.DeploymentId, restored.DeploymentId)
}

// Provider deployment routes map bad input, unknown resources and state conflicts to their statuses.
func TestLLMRoutesProviderDeploymentErrors(t *testing.T) {
	a := newRouteAPI(t)
	egress := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	handle := unique("deperr")
	llmCreateProvider(t, a, handle, egress.UUID.String())
	base := llmDeploymentsPath(handle)
	deps := expectInto[[]gwspec.LLMDeploymentResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.Len(t, deps, 1)
	dep := deps[0]
	unknown := uuid.NewString()
	missing := llmDeploymentsPath(unique("missing"))

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}{
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
		{"list an unknown provider", http.MethodGet, missing, nil, http.StatusNotFound, "LLM provider not found"},
		{
			"restore on an unknown provider", http.MethodPost, missing + "/restore" + llmDeploymentQuery(unknown, unknown), nil,
			http.StatusNotFound, "LLM provider not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := a.do(tt.method, tt.path, tt.body)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, tt.message, errorMessage(t, rec))
		})
	}
}

// Reading a provider's deployments needs read access; restoring one needs deploy access.
func TestLLMRoutesProviderDeploymentNeedsScope(t *testing.T) {
	a := newRouteAPI(t)
	base := llmDeploymentsPath("x")
	rec := a.do(http.MethodPost, base+"/restore", nil, withScopes(scope(gwperm.LLMProviderRead)))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, base, nil, withScopes(scope(gwperm.LLMProviderDeploy)))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

// The removed deployment operations are no longer served.
func TestLLMRoutesProviderDeploymentRemovedRoutes(t *testing.T) {
	a := newRouteAPI(t)
	base := llmDeploymentsPath("x")
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, base},
		{http.MethodPost, base + "/undeploy"},
		{http.MethodGet, base + "/d"},
		{http.MethodDelete, base + "/d"},
	} {
		rec := a.do(tc.method, tc.path, nil)
		require.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
}

// A sync marks only the gateways it newly placed the provider on, which are the ones a deployed
// event is raised for; a redeploy to a current gateway is not one.
func TestLLMProviderSyncMarksNewPlacements(t *testing.T) {
	a := newRouteAPI(t)
	gw := aigateway.Current()
	gwID := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress).UUID.String()
	handle := unique("placed")
	llmCreateProvider(t, a, handle)
	ctx := context.Background()
	sync := func(gateways ...string) *gwsvc.UpdateAndSyncResponse {
		t.Helper()
		stored, err := gw.LLMProviderService.Get(handle, a.org)
		require.NoError(t, err)
		resp, err := gw.LLMProviderService.UpdateAndSync(ctx, handle, a.org, stored, gateways, gw.LLMProviderDeploymentService)
		require.NoError(t, err)
		return resp
	}

	first := sync(strings.ToUpper(gwID), gwID)
	require.Len(t, first.Deployments, 1, "a gateway named more than once is placed once")
	require.True(t, first.Deployments[0].Success)
	require.True(t, first.Deployments[0].Placed, "the provider was on no gateway")

	again := sync(gwID)
	require.Len(t, again.Deployments, 1)
	require.True(t, again.Deployments[0].Success)
	require.False(t, again.Deployments[0].Placed, "a redeploy to a current gateway is not a new placement")

	removed := sync()
	require.Empty(t, removed.Deployments)
	require.Len(t, removed.Undeployments, 1)
	require.True(t, removed.Undeployments[0].Success)
	require.Equal(t, gwID, removed.Undeployments[0].GatewayID)
}
