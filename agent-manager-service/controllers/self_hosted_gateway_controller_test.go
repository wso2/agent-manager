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
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
	var logs bytes.Buffer
	req := selfHostedRequest(http.MethodGet, "", managed.UUID.String())
	req = req.WithContext(logger.WithLogger(req.Context(), slog.New(slog.NewTextHandler(&logs, nil))))

	ctrl.GetSelfHostedGateway(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), managed.Name)
	assert.Contains(t, logs.String(), "level=WARN", "an expected 404 is not an error")
	assert.NotContains(t, logs.String(), "level=ERROR")
}

// capturingRecorder keeps every audit event recorded through it.
type capturingRecorder struct{ events []audit.Event }

func (r *capturingRecorder) Record(_ context.Context, e audit.Event) { r.events = append(r.events, e) }

func (r *capturingRecorder) RecordSync(_ context.Context, e audit.Event) error {
	r.events = append(r.events, e)
	return nil
}

func (r *capturingRecorder) Close(context.Context) error { return nil }

// The create event records what was registered: the normalised kind and the stored (trimmed) vhost, not the
// request's spelling of them.
func TestRegisterSelfHostedGateway_AuditsTheNormalisedKindAndVhost(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc:                 func(string, string) (*models.Gateway, error) { return nil, utils.ErrGatewayNotFound },
		TransactionFunc:                       func(fn func(tx *gorm.DB) error) error { return fn(nil) },
		CreateTxFunc:                          func(*gorm.DB, *models.Gateway) error { return nil },
		AcquireGatewayLockFunc:                func(*gorm.DB, string) error { return nil },
		EnvironmentMappingExistsFunc:          func(string, string) (bool, error) { return false, nil },
		AcquireEnvironmentLockFunc:            func(*gorm.DB, string) error { return nil },
		CountIngressCapableInEnvironmentFunc:  func(*gorm.DB, string) (int64, error) { return 0, nil },
		CreateEnvironmentMappingTxFunc:        func(*gorm.DB, *models.GatewayEnvironmentMapping) error { return nil },
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]models.GatewayEnvironmentMapping, error) { return nil, nil },
	}
	envs := &clientmocks.OpenChoreoClientMock{
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: uuid.NewString(), Name: "dev"}}, nil
		},
	}
	ctrl := NewGatewayController(services.NewPlatformGatewayService(repo, nil), envs, nil)
	body := `{"displayName":"Edge","environment":"dev","functionalityType":"AI","endpoints":["  https://edge.example.com "]}`
	req := httptest.NewRequest(http.MethodPost, "/orgs/acme/self-hosted-gateways", strings.NewReader(body))
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
	recorder := &capturingRecorder{}
	req = req.WithContext(audit.WithRecorder(req.Context(), recorder))
	w := httptest.NewRecorder()

	ctrl.RegisterSelfHostedGateway(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Len(t, recorder.events, 1)
	assert.Equal(t, "ai", recorder.events[0].Details["gatewayType"])
	assert.Equal(t, "https://edge.example.com", recorder.events[0].Details["vhost"])
}
