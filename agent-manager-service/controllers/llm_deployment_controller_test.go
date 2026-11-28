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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestLLMProviderDeploymentHistoryIsOptIn(t *testing.T) {
	providerID := uuid.New()
	deployment := &models.Deployment{
		DeploymentID: uuid.New(),
		GatewayUUID:  uuid.New(),
		CreatedAt:    time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC),
	}
	deployments := make([]*models.Deployment, 1001)
	for i := range deployments {
		deployments[i] = deployment
	}
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(gotID, ouID string) (*models.LLMProvider, error) {
			require.Equal(t, providerID.String(), gotID)
			require.Equal(t, "org-test", ouID)
			return &models.LLMProvider{UUID: providerID}, nil
		},
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeploymentsWithStateFunc: func(string, string, *string, *string, int) ([]*models.Deployment, error) {
			return deployments, nil
		},
		GetDeploymentsWithStatePageFunc: func(_ context.Context, _, _ string, _, _ *string, _ int, cursor *models.DeploymentPageCursor, limit int) ([]*models.Deployment, error) {
			require.Nil(t, cursor)
			require.Equal(t, 1001, limit)
			return deployments, nil
		},
	}
	controller := NewLLMDeploymentController(services.NewLLMProviderDeploymentService(
		nil, deploymentRepo, providerRepo, nil, nil, nil, nil,
	))
	request := httptest.NewRequestWithContext(
		middleware.WithResolvedOrg(t.Context(), middleware.ResolvedOrg{OUID: "org-test"}),
		http.MethodGet, "/", nil,
	)
	request.SetPathValue(utils.PathParamProviderId, providerID.String())

	legacy := httptest.NewRecorder()
	controller.GetLLMProviderDeployments(legacy, request)
	require.Equal(t, http.StatusOK, legacy.Code)
	require.Empty(t, legacy.Header().Get("X-Next-Cursor"))
	var all []spec.LLMDeploymentResponse
	require.NoError(t, json.Unmarshal(legacy.Body.Bytes(), &all))
	require.Len(t, all, 1001)

	paged := httptest.NewRecorder()
	controller.ListLLMProviderDeploymentHistory(paged, request)
	require.Equal(t, http.StatusOK, paged.Code)
	cursor, ok := decodeDeploymentCursor(paged.Header().Get("X-Next-Cursor"))
	require.True(t, ok)
	require.Equal(t, deployment.DeploymentID, cursor.DeploymentID)
	require.Equal(t, deployment.GatewayUUID, cursor.GatewayUUID)
	require.True(t, deployment.CreatedAt.Equal(cursor.CreatedAt))
	var page []spec.LLMDeploymentResponse
	require.NoError(t, json.Unmarshal(paged.Body.Bytes(), &page))
	require.Len(t, page, 1000)
}
