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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

func selfHostedRequest(method, body, gatewayID string) *http.Request {
	req := httptest.NewRequest(method, "/orgs/acme/self-hosted-gateways/"+gatewayID, strings.NewReader(body))
	req.SetPathValue("gatewayID", gatewayID)
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
}

func TestUpdateSelfHostedGateway_ChangesOnlyEditableFields(t *testing.T) {
	gateway := &models.Gateway{
		UUID: uuid.New(), OUID: "ou-1", Name: "dev-edge", DisplayName: "Edge", Vhost: "https://edge.example.com",
		Hosting: models.GatewayHostingSelfHosted, GatewayFunctionalityType: models.GatewayRoleEgress,
	}
	var saved *models.Gateway
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*models.Gateway, error) { return gateway, nil },
		UpdateGatewayFunc: func(gw *models.Gateway) error {
			saved = gw
			return nil
		},
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]models.GatewayEnvironmentMapping, error) {
			return []models.GatewayEnvironmentMapping{}, nil
		},
	}
	ctrl := NewGatewayController(services.NewPlatformGatewayService(repo, nil), nil, nil)
	body := `{"displayName":"Edge 2","description":"rack 4","isCritical":true,"properties":{"site":"berlin"}}`
	w := httptest.NewRecorder()

	ctrl.UpdateSelfHostedGateway(w, selfHostedRequest(http.MethodPut, body, gateway.UUID.String()))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, saved)
	assert.Equal(t, "Edge 2", saved.DisplayName)
	assert.Equal(t, "rack 4", saved.Description)
	assert.True(t, saved.IsCritical)
	assert.Equal(t, map[string]interface{}{"site": "berlin"}, saved.Properties)
	assert.Equal(t, "dev-edge", saved.Name)
	assert.Equal(t, "https://edge.example.com", saved.Vhost)
	assert.Equal(t, models.GatewayHostingSelfHosted, saved.Hosting)
}

func TestSelfHostedGatewayRoutes_ANameResolvesToItsUUIDBeforeTheHandlerRuns(t *testing.T) {
	gateway := &models.Gateway{UUID: uuid.New(), OUID: "ou-1", Name: "dev-edge", Hosting: models.GatewayHostingSelfHosted}
	var deleted string
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc:     func(string, string) (*models.Gateway, error) { return gateway, nil },
		GetByUUIDFunc:             func(string) (*models.Gateway, error) { return gateway, nil },
		HasGatewayDeploymentsFunc: func(string, string) (bool, error) { return false, nil },
		DeleteFunc: func(id, _ string) error {
			deleted = id
			return nil
		},
	}
	ctrl := NewGatewayController(services.NewPlatformGatewayService(repo, nil), nil, nil)
	req := selfHostedRequest(http.MethodDelete, "", "dev-edge")
	req = req.WithContext(audit.WithRecorder(req.Context(), audit.NewNoopRecorder()))
	w := httptest.NewRecorder()

	ctrl.DeleteSelfHostedGateway(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Equal(t, gateway.UUID.String(), deleted)
}

func TestSelfHostedGatewayRoutes_ManagedGatewayIsNotFound(t *testing.T) {
	managed := &models.Gateway{UUID: uuid.New(), OUID: "ou-1", Name: "dev-managed", Hosting: models.GatewayHostingManaged}
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*models.Gateway, error) { return managed, nil },
	}
	ctrl := NewGatewayController(services.NewPlatformGatewayService(repo, nil), nil, nil)
	w := httptest.NewRecorder()

	ctrl.GetSelfHostedGateway(w, selfHostedRequest(http.MethodGet, "", managed.UUID.String()))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), managed.Name)
}
