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
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// noopScopeRedeployer stands in for the gateway re-emission the scope service
// performs after every write; these tests only assert the HTTP contract.
type noopScopeRedeployer struct{}

func (noopScopeRedeployer) RedeployMCPProxy(context.Context, *models.MCPProxy, string) error {
	return nil
}

func (noopScopeRedeployer) EnsureResourceServersForProxy(context.Context, string, *models.MCPProxy, []string) {
}

// scopeUpdateTestController wires the real scope service behind the controller so
// the tests below exercise the whole decode -> validate -> persist path, which is
// where the nil-vs-empty distinction for "tools" lives.
func scopeUpdateTestController(scopeRepo *repomocks.MCPProxyScopeRepositoryMock) MCPProxyScopeController {
	proxy := &models.MCPProxy{
		UUID:     uuid.New(),
		Artifact: &models.Artifact{Handle: "t4-deepwiki"},
		Endpoints: []models.MCPProxyEndpoint{{
			UUID:   uuid.New(),
			Handle: "primary",
		}},
	}
	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(_ context.Context, _, _ string) (*models.MCPProxy, error) {
			return proxy, nil
		},
	}
	svc := services.NewMCPProxyScopeService(scopeRepo, proxyRepo, nil, nil, noopScopeRedeployer{}, slog.Default())
	return NewMCPProxyScopeController(svc)
}

func updateScopeRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut,
		"/orgs/default/mcp-proxies/t4-deepwiki/scopes/write-only", strings.NewReader(body))
	req.SetPathValue(utils.PathParamOrgName, "default")
	req.SetPathValue(utils.PathParamProxyId, "t4-deepwiki")
	req.SetPathValue(utils.PathParamScopeAction, "write-only")
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-org"}))
}

// Regression for the reported 400 on unbinding a scope's last tool: PUT
// {"tools":[]} must succeed and persist an empty list.
func TestUpdateMCPProxyScope_EmptyToolsClearsBindings(t *testing.T) {
	var persisted *models.MCPProxyScope
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, proxyUUID uuid.UUID, action string) (*models.MCPProxyScope, error) {
			return &models.MCPProxyScope{
				MCPProxyUUID: proxyUUID, Action: action, Tools: []string{"ask_question"},
			}, nil
		},
		UpdateFunc: func(_ context.Context, s *models.MCPProxyScope) error { persisted = s; return nil },
	}
	ctrl := scopeUpdateTestController(scopeRepo)

	w := httptest.NewRecorder()
	ctrl.UpdateMCPProxyScope(w, updateScopeRequest(t, `{"tools":[]}`))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"tools":[]`)
	require.NotNil(t, persisted)
	assert.Empty(t, persisted.Tools)
}

// A description-only PUT omits "tools" entirely, which must leave the existing
// bindings alone rather than clearing them now that empty is a legal value.
func TestUpdateMCPProxyScope_OmittedToolsKeepsBindings(t *testing.T) {
	var persisted *models.MCPProxyScope
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, proxyUUID uuid.UUID, action string) (*models.MCPProxyScope, error) {
			return &models.MCPProxyScope{
				MCPProxyUUID: proxyUUID, Action: action, Tools: []string{"ask_question"},
			}, nil
		},
		UpdateFunc: func(_ context.Context, s *models.MCPProxyScope) error { persisted = s; return nil },
	}
	ctrl := scopeUpdateTestController(scopeRepo)

	w := httptest.NewRecorder()
	ctrl.UpdateMCPProxyScope(w, updateScopeRequest(t, `{"description":"write access"}`))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, persisted)
	assert.Equal(t, []string{"ask_question"}, persisted.Tools)
	assert.Equal(t, "write access", persisted.Description)
}
