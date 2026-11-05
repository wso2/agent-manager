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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// registerHarness is a gateway controller whose repository accepts one new
// gateway and keeps it in created.
type registerHarness struct {
	ctrl    GatewayController
	created *models.Gateway
}

func newRegisterHarness() *registerHarness {
	h := &registerHarness{}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(_ string, _ string) (*models.Gateway, error) {
			return nil, utils.ErrGatewayNotFound
		},
		TransactionFunc: func(fn func(tx *gorm.DB) error) error {
			return fn(nil)
		},
		CreateTxFunc: func(_ *gorm.DB, gateway *models.Gateway) error {
			h.created = gateway
			return nil
		},
		GetEnvironmentMappingsByGatewayIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
			return []models.GatewayEnvironmentMapping{}, nil
		},
	}
	h.ctrl = NewGatewayController(services.NewPlatformGatewayService(gatewayRepo, nil), nil, nil)
	return h
}

// TestRegisterGateway_DeprecatedAliasesDecodeAndNormalize proves the full path from
// wire body to stored role for the REGULAR/AI deprecated aliases: a gateway chart
// pinned to an older version sends "REGULAR" or "AI" as gatewayType, and the request
// must decode successfully (spec.CreateGatewayRequest.GatewayType now accepts these
// as input-only aliases) and land in the database as the canonical "both"/"egress"
// role. Before the fix, spec.GatewayType's strict UnmarshalJSON rejected these values
// before normalizeGatewayRole ever ran, 400ing every old-chart registration.
func TestRegisterGateway_DeprecatedAliasesDecodeAndNormalize(t *testing.T) {
	tests := []struct {
		name         string
		gatewayType  string
		wantRole     string
		wantHTTPCode int
	}{
		{name: "REGULAR aliases to both", gatewayType: "REGULAR", wantRole: models.GatewayRoleBoth, wantHTTPCode: http.StatusCreated},
		{name: "AI aliases to egress", gatewayType: "AI", wantRole: models.GatewayRoleEgress, wantHTTPCode: http.StatusCreated},
		{name: "canonical INGRESS still works", gatewayType: "INGRESS", wantRole: models.GatewayRoleIngress, wantHTTPCode: http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newRegisterHarness()

			body := `{"name":"gw-` + strings.ToLower(tt.gatewayType) + `","displayName":"Gateway",` +
				`"gatewayType":"` + tt.gatewayType + `","vhost":"gw.example.com"}`
			req := httptest.NewRequest(http.MethodPost, "/orgs/o1/gateways", strings.NewReader(body))
			req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
			w := httptest.NewRecorder()

			h.ctrl.RegisterGateway(w, req)

			require.Equal(t, tt.wantHTTPCode, w.Code, "response body: %s", w.Body.String())
			require.NotNil(t, h.created, "gateway must have been persisted")
			assert.Equal(t, tt.wantRole, h.created.GatewayFunctionalityType)
		})
	}
}

// TestRegisterGateway_UnknownGatewayTypeRejected proves a gatewayType outside the
// accepted canonical values and deprecated aliases (INGRESS/EGRESS/BOTH/REGULAR/AI)
// is still rejected with 400 at decode time — GatewayTypeInput's enum is closed, it
// just now includes the two deprecated aliases alongside the canonical values.

func TestRegisterGateway_UnknownGatewayTypeRejected(t *testing.T) {
	gatewayRepo := &repomocks.GatewayRepositoryMock{} // no calls expected
	svc := services.NewPlatformGatewayService(gatewayRepo, nil)
	ctrl := NewGatewayController(svc, nil, nil)

	body := `{"name":"gw-1","displayName":"Gateway","gatewayType":"SIDEWAYS","vhost":"gw.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/orgs/o1/gateways", strings.NewReader(body))
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
	w := httptest.NewRecorder()

	ctrl.RegisterGateway(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegisterGatewayRejectsCrossOrganization(t *testing.T) {
	ctrl := NewGatewayController(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/orgs/o1/gateways",
		strings.NewReader(`{"name":"gateway","gatewayType":"EGRESS","orgId":"other-org"}`))
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
	w := httptest.NewRecorder()
	ctrl.RegisterGateway(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "orgId must match")
}
