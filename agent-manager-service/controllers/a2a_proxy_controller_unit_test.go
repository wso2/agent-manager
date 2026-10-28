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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// proxyServiceStub stands in for the proxy service; the hooks are nil, so calling one would panic.
type proxyServiceStub struct {
	services.A2AProxyHooks
	GetFunc func(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AProxyStatus, error)
	SetFunc func(ctx context.Context, ouID, projectName, agentName, envName string, auth *models.A2AProxyAuthUpdate) error
}

func (s *proxyServiceStub) GetA2AProxy(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AProxyStatus, error) {
	return s.GetFunc(ctx, ouID, projectName, agentName, envName)
}

func (s *proxyServiceStub) SetA2AProxy(ctx context.Context, ouID, projectName, agentName, envName string, auth *models.A2AProxyAuthUpdate) error {
	return s.SetFunc(ctx, ouID, projectName, agentName, envName, auth)
}

func (s *proxyServiceStub) DeleteA2AProxy(context.Context, string, string, string, string) error {
	panic("not used")
}

func (s *proxyServiceStub) RefreshA2AProxy(context.Context, string, string, string, string) error {
	panic("not used")
}

// agentEnvRequest targets resource under the checkout/weather agent's dev environment in a resolved org.
func agentEnvRequest(method, resource, body string) *http.Request {
	req := httptest.NewRequest(method, "/orgs/default/projects/checkout/agents/weather/environments/dev/"+resource,
		strings.NewReader(body))
	req.SetPathValue(utils.PathParamProjName, "checkout")
	req.SetPathValue(utils.PathParamAgentName, "weather")
	req.SetPathValue(utils.PathParamEnvID, "dev")
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-org"}))
}

// The response has no field the secret could leak through.
func TestGetA2AProxyReturnsTheHeaderButNoSecret(t *testing.T) {
	stub := &proxyServiceStub{GetFunc: func(context.Context, string, string, string, string) (*models.A2AProxyStatus, error) {
		return &models.A2AProxyStatus{
			Status: models.A2APublicationStatusPublished, UpstreamAuthHeader: "X-Key", GatewayURL: "https://agents.example.com/weather",
		}, nil
	}}
	rec := httptest.NewRecorder()

	NewA2AProxyController(stub).GetA2AProxy(rec, agentEnvRequest(http.MethodGet, "a2a-proxy", ""))

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, map[string]interface{}{
		"status": "published", "lastError": "", "upstreamAuthHeader": "X-Key", "gatewayUrl": "https://agents.example.com/weather",
	}, body)
}

func TestSetA2AProxyKeepsTheThreeAuthCasesApart(t *testing.T) {
	v := "v"
	cases := map[string]struct {
		body string
		want *models.A2AProxyAuthUpdate
	}{
		"no upstreamAuth clears it":       {`{}`, nil},
		"a header alone keeps the secret": {`{"upstreamAuth":{"header":"X-Key"}}`, &models.A2AProxyAuthUpdate{Header: "X-Key"}},
		"a value replaces it":             {`{"upstreamAuth":{"header":"X-Key","value":"v"}}`, &models.A2AProxyAuthUpdate{Header: "X-Key", Value: &v}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var got *models.A2AProxyAuthUpdate
			stub := &proxyServiceStub{SetFunc: func(_ context.Context, _, _, _, _ string, auth *models.A2AProxyAuthUpdate) error {
				got = auth
				return nil
			}}
			rec := httptest.NewRecorder()
			NewA2AProxyController(stub).SetA2AProxy(rec, agentEnvRequest(http.MethodPut, "a2a-proxy", tc.body))
			require.Equal(t, http.StatusAccepted, rec.Code)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestA2AProxyErrorsMapToStatusCodes(t *testing.T) {
	for sentinel, code := range map[error]int{
		utils.ErrEnvironmentNotProxied:          http.StatusNotFound,
		utils.ErrA2AProxyNeedsCardSource:        http.StatusConflict,
		utils.ErrA2ACardURLNotProxyable:         http.StatusBadRequest,
		utils.ErrA2AProxyUpstreamAuthNeedsHTTPS: http.StatusBadRequest,
		utils.ErrInvalidUpstreamAuth:            http.StatusBadRequest,
		utils.ErrA2AProxyAgentNotExternal:       http.StatusBadRequest,
		utils.ErrAgentNotA2A:                    http.StatusBadRequest,
		utils.ErrForbidden:                      http.StatusForbidden,
		audit.ErrRecorderUnavailable:            http.StatusServiceUnavailable,
	} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			stub := &proxyServiceStub{SetFunc: func(context.Context, string, string, string, string, *models.A2AProxyAuthUpdate) error {
				return fmt.Errorf("wrapped: %w", sentinel)
			}}
			rec := httptest.NewRecorder()
			NewA2AProxyController(stub).SetA2AProxy(rec, agentEnvRequest(http.MethodPut, "a2a-proxy", `{}`))
			assert.Equal(t, code, rec.Code)
		})
	}
}
