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

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const selfHostedTestOU = "ou-1"

type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (c *auditCapture) Record(_ context.Context, e audit.Event) { c.add(e) }
func (c *auditCapture) RecordSync(_ context.Context, e audit.Event) error {
	c.add(e)
	return nil
}
func (c *auditCapture) Close(context.Context) error { return nil }

func (c *auditCapture) add(e audit.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *auditCapture) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = nil
}

// semanticEvent returns the event a handler emitted for action, ignoring the route
// envelope the audit middleware writes for any request.
func (c *auditCapture) semanticEvent(action audit.Action) (audit.Event, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if _, isEnvelope := e.Details["envelope"]; e.Action == action && !isEnvelope {
			return e, true
		}
	}
	return audit.Event{}, false
}

// selfHostedFixture serves the self-hosted routes through the real RouteRegistrar and
// a real service over a fake repository holding one managed and one self-hosted gateway.
type selfHostedFixture struct {
	mux        *http.ServeMux
	audits     *auditCapture
	managed    *models.Gateway
	selfHosted *models.Gateway
	token      *models.GatewayToken
	mutations  []string
}

func newSelfHostedFixture(t *testing.T) *selfHostedFixture {
	t.Helper()
	f := &selfHostedFixture{audits: &auditCapture{}}
	f.managed = &models.Gateway{UUID: uuid.New(), OUID: selfHostedTestOU, Name: "dev-managed", Hosting: models.GatewayHostingManaged}
	f.selfHosted = &models.Gateway{UUID: uuid.New(), OUID: selfHostedTestOU, Name: "dev-edge", Hosting: models.GatewayHostingSelfHosted}
	f.token = &models.GatewayToken{UUID: uuid.New(), GatewayUUID: f.selfHosted.UUID, Status: "active"}
	byUUID := map[string]*models.Gateway{f.managed.UUID.String(): f.managed, f.selfHosted.UUID.String(): f.selfHosted}
	mutated := func(op string) { f.mutations = append(f.mutations, op) }

	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(id string) (*models.Gateway, error) {
			if gw, ok := byUUID[id]; ok {
				return gw, nil
			}
			return nil, utils.ErrGatewayNotFound
		},
		GetByNameAndOrgIDFunc: func(string, string) (*models.Gateway, error) { return nil, utils.ErrGatewayNotFound },
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]models.GatewayEnvironmentMapping, error) {
			return []models.GatewayEnvironmentMapping{}, nil
		},
		GetEnvironmentMappingsByGatewayIDsFunc: func([]string) (map[string][]models.GatewayEnvironmentMapping, error) {
			return map[string][]models.GatewayEnvironmentMapping{}, nil
		},
		CountWithFiltersFunc: func(repositories.GatewayFilterOptions) (int64, error) { return 1, nil },
		ListWithFiltersFunc: func(opts repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			require.NotNil(t, opts.Hosting)
			require.Equal(t, models.GatewayHostingSelfHosted, *opts.Hosting)
			return []*models.Gateway{f.selfHosted}, nil
		},
		GetActiveTokensByGatewayUUIDFunc: func(string) ([]*models.GatewayToken, error) {
			return []*models.GatewayToken{f.token}, nil
		},
		GetTokenByUUIDFunc:        func(string) (*models.GatewayToken, error) { return f.token, nil },
		CountActiveTokensFunc:     func(string) (int, error) { return 0, nil },
		HasGatewayDeploymentsFunc: func(string, string) (bool, error) { return false, nil },
		TransactionFunc: func(fn func(tx *gorm.DB) error) error {
			mutated("transaction")
			return fn(nil)
		},
		CreateTxFunc: func(_ *gorm.DB, gw *models.Gateway) error {
			mutated("create")
			byUUID[gw.UUID.String()] = gw
			return nil
		},
		UpdateGatewayFunc: func(*models.Gateway) error { mutated("update"); return nil },
		DeleteFunc:        func(string, string) error { mutated("delete"); return nil },
		CreateTokenFunc:   func(*models.GatewayToken) error { mutated("create-token"); return nil },
		RevokeTokenFunc:   func(string) error { mutated("revoke-token"); return nil },
	}
	repo.WithTxFunc = func(*gorm.DB) repositories.GatewayRepository { return repo }
	repo.AcquireGatewayLockFunc = func(*gorm.DB, string) error { return nil }
	repo.AcquireEnvironmentLockFunc = func(*gorm.DB, string) error { return nil }
	repo.EnvironmentMappingExistsFunc = func(string, string) (bool, error) { return false, nil }
	repo.CreateEnvironmentMappingTxFunc = func(*gorm.DB, *models.GatewayEnvironmentMapping) error {
		mutated("map-environment")
		return nil
	}

	ocClient := &clientmocks.OpenChoreoClientMock{
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: uuid.NewString(), Name: "dev"}}, nil
		},
	}
	ctrl := controllers.NewGatewayController(services.NewPlatformGatewayService(repo, nil), ocClient, nil)
	f.mux = http.NewServeMux()
	RegisterGatewayRoutes(middleware.NewRouteRegistrar(f.mux, nil, f.audits), ctrl)
	return f
}

func (f *selfHostedFixture) call(method, path, body string, perm rbac.Permission) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/orgs/acme/self-hosted-gateways"+path, strings.NewReader(body))
	req = req.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(req.Context(), &jwtassertion.TokenClaims{
		Sub: "user-a", OuId: selfHostedTestOU, OuHandle: "acme", Scope: perm.Scope(),
	}))
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func TestSelfHostedGatewayRoutes_SelfHostedGatewaySucceeds(t *testing.T) {
	f := newSelfHostedFixture(t)
	id := f.selfHosted.UUID.String()
	tokenPath := "/" + id + "/tokens"

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		perm   rbac.Permission
		want   int
		audit  audit.Action
	}{
		{"list", http.MethodGet, "", "", rbac.GatewayRead, http.StatusOK, ""},
		{"create", http.MethodPost, "", `{"displayName":"Edge","environment":"dev","endpoints":["https://edge.example.com"]}`, rbac.GatewayCreate, http.StatusCreated, audit.ActionGatewayCreate},
		{"get", http.MethodGet, "/" + id, "", rbac.GatewayRead, http.StatusOK, ""},
		{"update", http.MethodPut, "/" + id, `{"description":"edge site"}`, rbac.GatewayUpdate, http.StatusOK, audit.ActionGatewayUpdate},
		{"list tokens", http.MethodGet, tokenPath, "", rbac.GatewayTokenManage, http.StatusOK, ""},
		{"rotate token", http.MethodPost, tokenPath, "", rbac.GatewayTokenManage, http.StatusCreated, audit.ActionGatewayTokenRotate},
		{"revoke token", http.MethodDelete, tokenPath + "/" + f.token.UUID.String(), "", rbac.GatewayTokenManage, http.StatusNoContent, audit.ActionGatewayTokenRevoke},
		{"delete", http.MethodDelete, "/" + id, "", rbac.GatewayDelete, http.StatusNoContent, audit.ActionGatewayDelete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.audits.reset()

			rec := f.call(tt.method, tt.path, tt.body, tt.perm)

			require.Equal(t, tt.want, rec.Code, "body: %s", rec.Body.String())
			if tt.audit == "" {
				return
			}
			event, ok := f.audits.semanticEvent(tt.audit)
			require.True(t, ok, "want a handler-emitted %s record, not just the route envelope", tt.audit)
			assert.Equal(t, selfHostedTestOU, event.OUID)
			if tt.method != http.MethodPost || tt.path != "" {
				assert.Equal(t, id, event.ResourceID)
			} else {
				assert.NotEmpty(t, event.ResourceID)
			}
		})
	}
}

func TestSelfHostedGatewayRoutes_CreateReturnsNoToken(t *testing.T) {
	f := newSelfHostedFixture(t)

	rec := f.call(http.MethodPost, "", `{"displayName":"Edge","environment":"dev","endpoints":["https://edge.example.com"]}`, rbac.GatewayCreate)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), `"token"`)
	assert.Contains(t, rec.Body.String(), `"SELF_HOSTED"`)
}

func TestSelfHostedGatewayRoutes_ManagedGatewayIsNotFoundAndUntouched(t *testing.T) {
	f := newSelfHostedFixture(t)
	id := f.managed.UUID.String()
	tokenPath := "/" + id + "/tokens"

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		perm   rbac.Permission
	}{
		{"get", http.MethodGet, "/" + id, "", rbac.GatewayRead},
		{"update", http.MethodPut, "/" + id, `{"displayName":"Hijacked"}`, rbac.GatewayUpdate},
		{"delete", http.MethodDelete, "/" + id, "", rbac.GatewayDelete},
		{"list tokens", http.MethodGet, tokenPath, "", rbac.GatewayTokenManage},
		{"rotate token", http.MethodPost, tokenPath, "", rbac.GatewayTokenManage},
		{"revoke token", http.MethodDelete, tokenPath + "/" + f.token.UUID.String(), "", rbac.GatewayTokenManage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := f.call(tt.method, tt.path, tt.body, tt.perm)

			assert.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body.String())
			assert.NotContains(t, rec.Body.String(), f.managed.Name)
		})
	}
	assert.Empty(t, f.mutations, "a managed gateway must never be mutated through a self-hosted route")
	assert.Equal(t, "", f.managed.Description)
	assert.Equal(t, "", f.managed.DisplayName)
}

func TestSelfHostedGatewayRoutes_CreateRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"placeholder endpoint", `{"displayName":"Edge","environment":"dev","endpoints":["pending://awaiting-connection"]}`, http.StatusBadRequest},
		{"unknown environment", `{"displayName":"Edge","environment":"nowhere","endpoints":["https://edge.example.com"]}`, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSelfHostedFixture(t)

			rec := f.call(http.MethodPost, "", tt.body, rbac.GatewayCreate)

			assert.Equal(t, tt.want, rec.Code, "body: %s", rec.Body.String())
			assert.Empty(t, f.mutations)
		})
	}
}
