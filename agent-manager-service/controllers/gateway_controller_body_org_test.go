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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	callerOUID = "11111111-1111-1111-1111-111111111111"
	otherOUID  = "22222222-2222-2222-2222-222222222222"
)

func requestAsCaller(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: callerOUID})
	return req.WithContext(audit.WithRecorder(ctx, audit.NewNoopRecorder()))
}

func TestRegisterGateway_BodyOrgID(t *testing.T) {
	tests := []struct {
		name     string
		orgField string
		wantCode int
		wantOUID string
	}{
		{name: "absent uses the caller's org", orgField: "", wantCode: http.StatusCreated, wantOUID: callerOUID},
		{name: "caller's own org is accepted", orgField: `,"orgId":"` + callerOUID + `"`, wantCode: http.StatusCreated, wantOUID: callerOUID},
		{name: "caller's own org in upper case is accepted", orgField: `,"orgId":"` + strings.ToUpper(callerOUID) + `"`, wantCode: http.StatusCreated, wantOUID: callerOUID},
		{name: "another org is refused", orgField: `,"orgId":"` + otherOUID + `"`, wantCode: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created *models.Gateway
			gatewayRepo := &repomocks.GatewayRepositoryMock{
				GetByNameAndOrgIDFunc: func(_ string, _ string) (*models.Gateway, error) {
					return nil, utils.ErrGatewayNotFound
				},
				TransactionFunc: func(fn func(tx *gorm.DB) error) error { return fn(nil) },
				CreateTxFunc: func(_ *gorm.DB, gateway *models.Gateway) error {
					created = gateway
					return nil
				},
				GetEnvironmentMappingsByGatewayIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
					return []models.GatewayEnvironmentMapping{}, nil
				},
			}
			ctrl := NewGatewayController(services.NewPlatformGatewayService(gatewayRepo, nil), nil, nil)

			body := `{"name":"gw-1","displayName":"Gateway","gatewayType":"BOTH","vhost":"gw.example.com"` + tt.orgField + `}`
			w := httptest.NewRecorder()
			ctrl.RegisterGateway(w, requestAsCaller(http.MethodPost, "/orgs/o1/gateways", body))

			require.Equal(t, tt.wantCode, w.Code, "response body: %s", w.Body.String())
			if tt.wantCode != http.StatusCreated {
				assert.Nil(t, created, "no gateway may be stored")
				return
			}
			require.NotNil(t, created)
			assert.Equal(t, tt.wantOUID, created.OUID)
		})
	}
}

func TestRotateGatewayToken_AnotherOrgInBodyRefused(t *testing.T) {
	gatewayRepo := &repomocks.GatewayRepositoryMock{} // any repository call panics
	ctrl := NewGatewayController(services.NewPlatformGatewayService(gatewayRepo, nil), nil, nil)

	req := requestAsCaller(http.MethodPost, "/orgs/o1/gateways/gw-uuid/tokens", `{"orgId":"`+otherOUID+`"}`)
	req.SetPathValue("gatewayID", "gw-uuid")
	w := httptest.NewRecorder()
	ctrl.RotateGatewayToken(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, "response body: %s", w.Body.String())
}

func TestRotateGatewayToken_CallerOrgInBodyLooksUpCallerOrg(t *testing.T) {
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			return &models.Gateway{OUID: callerOUID}, nil
		},
		CountActiveTokensFunc: func(_ string) (int, error) { return 2, nil },
	}
	ctrl := NewGatewayController(services.NewPlatformGatewayService(gatewayRepo, nil), nil, nil)

	req := requestAsCaller(http.MethodPost, "/orgs/o1/gateways/gw-uuid/tokens", `{"orgId":"`+callerOUID+`"}`)
	req.SetPathValue("gatewayID", "gw-uuid")
	w := httptest.NewRecorder()
	ctrl.RotateGatewayToken(w, req)

	assert.NotEqual(t, http.StatusForbidden, w.Code, "response body: %s", w.Body.String())
	assert.Len(t, gatewayRepo.CountActiveTokensCalls(), 1, "the caller's gateway must be found")
}
