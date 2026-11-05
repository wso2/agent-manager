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

package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Every public route that changes an LLM proxy refuses one AMS created for an agent or monitor. The mutating
// repository functions are left unset, so reaching one panics and fails the test.
func TestLLMProxyMutatingRoutes_RefuseAgentOrMonitorProxy(t *testing.T) {
	const (
		ouID        = "ou-acme"
		proxyHandle = "agent-llm-proxy"
	)
	deploymentQuery := "?deploymentId=00000000-0000-0000-0000-000000000002&gatewayId=00000000-0000-0000-0000-000000000001"
	tests := []struct {
		name   string
		method string
		query  string
		body   string
		call   func(deployments LLMProxyDeploymentController, llm LLMController, w http.ResponseWriter, r *http.Request)
	}{
		{
			name: "deploy", method: http.MethodPost,
			body: `{"name":"d1","base":"current","gatewayId":"00000000-0000-0000-0000-000000000001"}`,
			call: func(d LLMProxyDeploymentController, _ LLMController, w http.ResponseWriter, r *http.Request) {
				d.DeployLLMProxy(w, r)
			},
		},
		{
			name: "undeploy", method: http.MethodPost, query: deploymentQuery,
			call: func(d LLMProxyDeploymentController, _ LLMController, w http.ResponseWriter, r *http.Request) {
				d.UndeployLLMProxyDeployment(w, r)
			},
		},
		{
			name: "restore", method: http.MethodPost, query: deploymentQuery,
			call: func(d LLMProxyDeploymentController, _ LLMController, w http.ResponseWriter, r *http.Request) {
				d.RestoreLLMProxyDeployment(w, r)
			},
		},
		{
			name: "delete deployment", method: http.MethodDelete,
			call: func(d LLMProxyDeploymentController, _ LLMController, w http.ResponseWriter, r *http.Request) {
				d.DeleteLLMProxyDeployment(w, r)
			},
		},
		{
			name: "update", method: http.MethodPut, body: `{"description":"changed"}`,
			call: func(_ LLMProxyDeploymentController, l LLMController, w http.ResponseWriter, r *http.Request) {
				l.UpdateLLMProxy(w, r)
			},
		},
		{
			name: "delete", method: http.MethodDelete,
			call: func(_ LLMProxyDeploymentController, l LLMController, w http.ResponseWriter, r *http.Request) {
				l.DeleteLLMProxy(w, r)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxyRepo := &repomocks.LLMProxyRepositoryMock{
				IsAgentOrMonitorProxyFunc: func(_ context.Context, handle, org string) (bool, error) {
					require.Equal(t, proxyHandle, handle)
					require.Equal(t, ouID, org)
					return true, nil
				},
			}
			ocClient := &clientmocks.OpenChoreoClientMock{
				GetProjectFunc: func(_ context.Context, _, projectName string) (*models.ProjectResponse, error) {
					return &models.ProjectResponse{UUID: "00000000-0000-0000-0000-000000000003", Name: projectName}, nil
				},
			}
			proxyDeployments := services.NewLLMProxyDeploymentService(nil, nil, proxyRepo, nil, nil, nil, nil, nil)
			deployments := NewLLMProxyDeploymentController(proxyDeployments)
			llm := NewLLMController(nil, nil, services.NewLLMProxyService(proxyRepo, nil, nil), nil, proxyDeployments, nil, ocClient)

			req := httptest.NewRequest(tt.method, "/llm-proxies/"+proxyHandle+tt.query, strings.NewReader(tt.body))
			req.SetPathValue("id", proxyHandle)
			req.SetPathValue("deploymentId", "00000000-0000-0000-0000-000000000002")
			req.SetPathValue(utils.PathParamProxyId, proxyHandle)
			req.SetPathValue(utils.PathParamProjName, "checkout")
			req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: ouID}))
			recorder := httptest.NewRecorder()

			tt.call(deployments, llm, recorder, req)

			require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
			var response struct {
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, utils.ErrLLMProxyManagedByAMS.Error(), response.Message)
			require.Len(t, proxyRepo.IsAgentOrMonitorProxyCalls(), 1)
		})
	}
}
