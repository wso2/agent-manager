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
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

func policiesRequest(query string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/orgs/default/mcp-proxies/policies?"+query, http.NoBody)
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-org"}))
}

// environmentUuid reaches the service: only the binding in that environment is looked up for placement.
func TestListAvailableMCPPolicies_PassesEnvironmentThrough(t *testing.T) {
	devEnv, prodEnv := uuid.New(), uuid.New()
	devArtifact, prodArtifact := uuid.New(), uuid.New()
	proxy := &models.MCPProxy{
		UUID: uuid.New(),
		Endpoints: []models.MCPProxyEndpoint{{
			Environments: []models.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: devEnv, ArtifactUUID: devArtifact},
				{EnvironmentUUID: prodEnv, ArtifactUUID: prodArtifact},
			},
		}},
	}
	var placedFor []uuid.UUID
	svc := services.NewMCPProxyService(nil,
		&repomocks.MCPProxyRepositoryMock{
			GetByHandleFunc: func(context.Context, string, string) (*models.MCPProxy, error) { return proxy, nil },
		},
		nil,
		&repomocks.DeploymentRepositoryMock{
			PlacedGatewaysFunc: func(_ context.Context, _ string, id uuid.UUID) ([]string, error) {
				placedFor = append(placedFor, id)
				return nil, nil
			},
		},
		&repomocks.GatewayRepositoryMock{
			ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
				return []*models.Gateway{}, nil
			},
		},
		nil, nil, nil, nil, nil, nil, slog.New(slog.DiscardHandler), nil, nil, nil, nil, nil, nil, nil)
	ctrl := NewMCPProxyController(svc, nil, nil)

	rec := httptest.NewRecorder()
	ctrl.ListAvailableMCPPolicies(rec, policiesRequest("proxyId=orders&environmentUuid="+prodEnv.String()))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []uuid.UUID{prodArtifact}, placedFor)
}

func TestListAvailableMCPPolicies_InvalidEnvironmentIs400(t *testing.T) {
	svc := services.NewMCPProxyService(nil, &repomocks.MCPProxyRepositoryMock{}, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, slog.New(slog.DiscardHandler), nil, nil, nil, nil, nil, nil, nil)
	ctrl := NewMCPProxyController(svc, nil, nil)

	rec := httptest.NewRecorder()
	ctrl.ListAvailableMCPPolicies(rec, policiesRequest("proxyId=orders&environmentUuid=not-a-uuid"))

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}
