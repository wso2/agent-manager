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

package gwapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
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

// named returns the record that names the resource the handler acted on.
func (c *auditCapture) named() (audit.Event, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e.ResourceID != "" {
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
	managed    *gwmodels.Gateway
	selfHosted *gwmodels.Gateway
	token      *gwmodels.GatewayToken
	mutations  []string
}

func newSelfHostedFixture(t *testing.T) *selfHostedFixture {
	t.Helper()
	f := &selfHostedFixture{audits: &auditCapture{}}
	f.managed = &gwmodels.Gateway{UUID: uuid.New(), OUID: selfHostedTestOU, Name: "dev-managed", Hosting: gwmodels.GatewayHostingManaged}
	f.selfHosted = &gwmodels.Gateway{UUID: uuid.New(), OUID: selfHostedTestOU, Name: "dev-edge", Hosting: gwmodels.GatewayHostingSelfHosted}
	f.token = &gwmodels.GatewayToken{UUID: uuid.New(), GatewayUUID: f.selfHosted.UUID, Status: "active"}
	byUUID := map[string]*gwmodels.Gateway{f.managed.UUID.String(): f.managed, f.selfHosted.UUID.String(): f.selfHosted}
	mutated := func(op string) { f.mutations = append(f.mutations, op) }

	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(id string) (*gwmodels.Gateway, error) {
			if gw, ok := byUUID[id]; ok {
				return gw, nil
			}
			return nil, gwerr.ErrGatewayNotFound
		},
		GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return nil, gwerr.ErrGatewayNotFound },
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]gwmodels.GatewayEnvironmentMapping, error) {
			return []gwmodels.GatewayEnvironmentMapping{}, nil
		},
		GetEnvironmentMappingsByGatewayIDsFunc: func([]string) (map[string][]gwmodels.GatewayEnvironmentMapping, error) {
			return map[string][]gwmodels.GatewayEnvironmentMapping{}, nil
		},
		CountWithFiltersFunc: func(gwrepo.GatewayFilterOptions) (int64, error) { return 1, nil },
		ListWithFiltersFunc: func(opts gwrepo.GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
			require.Equal(t, gwmodels.GatewayHostingSelfHosted, opts.Hosting)
			return []*gwmodels.Gateway{f.selfHosted}, nil
		},
		GetActiveTokensByGatewayUUIDFunc: func(string) ([]*gwmodels.GatewayToken, error) {
			return []*gwmodels.GatewayToken{f.token}, nil
		},
		GetTokenByUUIDFunc:        func(string) (*gwmodels.GatewayToken, error) { return f.token, nil },
		CountActiveTokensFunc:     func(string) (int, error) { return 0, nil },
		HasGatewayDeploymentsFunc: func(string, string) (bool, error) { return false, nil },
		TransactionFunc: func(fn func(tx *gorm.DB) error) error {
			mutated("transaction")
			return fn(nil)
		},
		CreateTxFunc: func(_ *gorm.DB, gw *gwmodels.Gateway) error {
			mutated("create")
			byUUID[gw.UUID.String()] = gw
			return nil
		},
		UpdateGatewayFunc: func(*gwmodels.Gateway) error { mutated("update"); return nil },
		DeleteFunc:        func(string, string) error { mutated("delete"); return nil },
		CreateTokenFunc:   func(*gwmodels.GatewayToken) error { mutated("create-token"); return nil },
		RevokeTokenFunc:   func(string) error { mutated("revoke-token"); return nil },
	}
	repo.WithTxFunc = func(*gorm.DB) gwrepo.GatewayRepository { return repo }
	repo.AcquireGatewayLockFunc = func(*gorm.DB, string) error { return nil }
	repo.AcquireEnvironmentLockFunc = func(*gorm.DB, string) error { return nil }
	repo.EnvironmentMappingExistsFunc = func(string, string) (bool, error) { return false, nil }
	repo.CreateEnvironmentMappingTxFunc = func(*gorm.DB, *gwmodels.GatewayEnvironmentMapping) error {
		mutated("map-environment")
		return nil
	}

	ocClient := &gwtest.Core{
		ListEnvironmentsFunc: func(context.Context, string) ([]*modkit.Environment, error) {
			return []*modkit.Environment{{ID: uuid.NewString(), Name: "dev"}}, nil
		},
	}
	ctrl := gwctl.NewGatewayController(gwsvc.NewPlatformGatewayService(repo, nil), ocClient, nil)
	f.mux = http.NewServeMux()
	RegisterGatewayRoutes(coreRouter{middleware.NewRouteRegistrar(f.mux, nil, f.audits)}, ctrl)
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
		named  bool
	}{
		{"list", http.MethodGet, "", "", gwperm.Read, http.StatusOK, false},
		{"create", http.MethodPost, "", `{"displayName":"Edge","handle":"edge","environment":"dev","endpoints":["https://edge.example.com"]}`, gwperm.Create, http.StatusCreated, true},
		{"get", http.MethodGet, "/" + id, "", gwperm.Read, http.StatusOK, false},
		{"update", http.MethodPut, "/" + id, `{"description":"edge site"}`, gwperm.Update, http.StatusOK, true},
		{"list tokens", http.MethodGet, tokenPath, "", gwperm.TokenManage, http.StatusOK, false},
		{"rotate token", http.MethodPost, tokenPath, "", gwperm.TokenManage, http.StatusCreated, true},
		{"revoke token", http.MethodDelete, tokenPath + "/" + f.token.UUID.String(), "", gwperm.TokenManage, http.StatusNoContent, true},
		{"delete", http.MethodDelete, "/" + id, "", gwperm.Delete, http.StatusNoContent, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.audits.reset()

			rec := f.call(tt.method, tt.path, tt.body, tt.perm)

			require.Equal(t, tt.want, rec.Code, "body: %s", rec.Body.String())
			if !tt.named {
				return
			}
			event, ok := f.audits.named()
			require.True(t, ok, "the route's record must name the gateway")
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

	rec := f.call(http.MethodPost, "", `{"displayName":"Edge","handle":"edge","environment":"dev","endpoints":["https://edge.example.com"]}`, gwperm.Create)

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
		{"get", http.MethodGet, "/" + id, "", gwperm.Read},
		{"update", http.MethodPut, "/" + id, `{"displayName":"Hijacked"}`, gwperm.Update},
		{"delete", http.MethodDelete, "/" + id, "", gwperm.Delete},
		{"list tokens", http.MethodGet, tokenPath, "", gwperm.TokenManage},
		{"rotate token", http.MethodPost, tokenPath, "", gwperm.TokenManage},
		{"revoke token", http.MethodDelete, tokenPath + "/" + f.token.UUID.String(), "", gwperm.TokenManage},
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
		{"placeholder endpoint", `{"displayName":"Edge","handle":"edge","environment":"dev","endpoints":["pending://awaiting-connection"]}`, http.StatusBadRequest},
		{"unknown environment", `{"displayName":"Edge","handle":"edge","environment":"nowhere","endpoints":["https://edge.example.com"]}`, http.StatusNotFound},
		{"missing handle", `{"displayName":"Edge","environment":"dev","endpoints":["https://edge.example.com"]}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSelfHostedFixture(t)

			rec := f.call(http.MethodPost, "", tt.body, gwperm.Create)

			assert.Equal(t, tt.want, rec.Code, "body: %s", rec.Body.String())
			assert.Empty(t, f.mutations)
		})
	}
}
