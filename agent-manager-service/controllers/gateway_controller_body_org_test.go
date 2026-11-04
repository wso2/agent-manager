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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

const (
	bodyOrgTestCallerOUID = "11111111-1111-1111-1111-111111111111"
	bodyOrgTestOtherOUID  = "22222222-2222-2222-2222-222222222222"
)

var bodyOrgCases = []struct {
	name          string
	orgID         string
	wantForbidden bool
}{
	{name: "absent"},
	{name: "the caller's own org", orgID: bodyOrgTestCallerOUID},
	{name: "the caller's own org in upper case", orgID: strings.ToUpper(bodyOrgTestCallerOUID)},
	{name: "another org", orgID: bodyOrgTestOtherOUID, wantForbidden: true},
}

// withBodyOrg closes a JSON object body, adding orgId when one is given.
func withBodyOrg(fields, orgID string) string {
	if orgID == "" {
		return "{" + fields + "}"
	}
	if fields != "" {
		fields += ","
	}
	return "{" + fields + `"orgId":"` + orgID + `"}`
}

func bodyOrgTestRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: bodyOrgTestCallerOUID})
	return req.WithContext(audit.WithRecorder(ctx, audit.NewNoopRecorder()))
}

func TestRegisterGateway_BodyOrgID(t *testing.T) {
	for _, tt := range bodyOrgCases {
		t.Run(tt.name, func(t *testing.T) {
			h := newRegisterHarness()
			body := withBodyOrg(`"name":"gw-1","displayName":"Gateway","gatewayType":"BOTH","vhost":"gw.example.com"`, tt.orgID)
			w := httptest.NewRecorder()
			h.ctrl.RegisterGateway(w, bodyOrgTestRequest(http.MethodPost, "/orgs/o1/gateways", body))

			if tt.wantForbidden {
				assert.Equal(t, http.StatusForbidden, w.Code, "response body: %s", w.Body.String())
				assert.Nil(t, h.created, "no gateway may be stored")
				return
			}
			require.Equal(t, http.StatusCreated, w.Code, "response body: %s", w.Body.String())
			require.NotNil(t, h.created)
			assert.Equal(t, bodyOrgTestCallerOUID, h.created.OUID)
		})
	}
}

func TestRotateGatewayToken_BodyOrgID(t *testing.T) {
	for _, tt := range bodyOrgCases {
		t.Run(tt.name, func(t *testing.T) {
			// At the two-token limit the rotation stops with a conflict right
			// after finding the caller's gateway.
			gatewayRepo := &repomocks.GatewayRepositoryMock{
				GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
					return &models.Gateway{OUID: bodyOrgTestCallerOUID}, nil
				},
				CountActiveTokensFunc: func(_ string) (int, error) { return 2, nil },
			}
			ctrl := NewGatewayController(services.NewPlatformGatewayService(gatewayRepo, nil), nil, nil)

			req := bodyOrgTestRequest(http.MethodPost, "/orgs/o1/gateways/gw-uuid/tokens", withBodyOrg("", tt.orgID))
			req.SetPathValue("gatewayID", "gw-uuid")
			w := httptest.NewRecorder()
			ctrl.RotateGatewayToken(w, req)

			if tt.wantForbidden {
				assert.Equal(t, http.StatusForbidden, w.Code, "response body: %s", w.Body.String())
				assert.Empty(t, gatewayRepo.GetByUUIDCalls(), "no gateway may be looked up")
				return
			}
			assert.Equal(t, http.StatusConflict, w.Code, "response body: %s", w.Body.String())
		})
	}
}
