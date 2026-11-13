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

package services

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const scopeTestProjectUUID = "11111111-2222-3333-4444-555555555555"

// projectScopeSvc is a scope service whose proxy belongs to scopeTestProjectUUID: the project lookup finds it only
// there, and the org lookup sees it as project-owned.
func projectScopeSvc(t *testing.T, scopeRepo *repomocks.MCPProxyScopeRepositoryMock, proxy *models.MCPProxy, redeployer MCPProxyRedeployer) MCPProxyScopeService {
	t.Helper()
	projectUUID := scopeTestProjectUUID
	proxy.ProjectUUID = &projectUUID
	if scopeRepo.GetFunc == nil {
		scopeRepo.GetFunc = func(context.Context, uuid.UUID, string) (*models.MCPProxyScope, error) {
			return nil, gorm.ErrRecordNotFound
		}
	}
	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		GetInProjectFunc: func(_ context.Context, handle, _, projectUUID string) (*models.MCPProxy, error) {
			if handle == proxy.Artifact.Handle && projectUUID == scopeTestProjectUUID {
				return proxy, nil
			}
			return nil, utils.ErrMCPProxyNotFound
		},
		GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) {
			if handle == proxy.Artifact.Handle {
				return proxy, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	// No environments: a removed scope has nothing to clean up in an identity service.
	infra := stubInfraManager{listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
		return nil, nil
	}}
	return NewMCPProxyScopeService(scopeRepo, proxyRepo, infra, nil, redeployer, slog.Default())
}

// A project proxy's scopes are managed on the project routes with the same validation and the same redeploy of its
// bindings, so its identity bindings enforce them per tool.
func TestProjectScopes_CreateUpdateListDelete(t *testing.T) {
	proxy := scopeTestProxy("weather", "get_forecast", "get_alerts")
	var stored []models.MCPProxyScope
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		CreateFunc: func(_ context.Context, s *models.MCPProxyScope) error {
			stored = append(stored, *s)
			return nil
		},
		UpdateFunc: func(_ context.Context, s *models.MCPProxyScope) error {
			stored[0] = *s
			return nil
		},
		ListByProxyFunc: func(context.Context, uuid.UUID) ([]models.MCPProxyScope, error) { return stored, nil },
		DeleteFunc: func(context.Context, uuid.UUID, string) error {
			stored = nil
			return nil
		},
	}
	redeployer := &recordingRedeployer{}
	svc := projectScopeSvc(t, scopeRepo, proxy, redeployer)
	ctx := context.Background()

	created, err := svc.CreateInProject(ctx, "org-uuid", scopeTestProjectUUID, "weather",
		models.MCPProxyScopeInput{Action: "read", Tools: []string{"get_forecast"}})
	require.NoError(t, err)
	assert.Equal(t, "weather", created.ProxyHandle)
	assert.Equal(t, "weather:read", created.Scope.ScopeString(created.ProxyHandle))
	require.Len(t, redeployer.calls, 1, "the bindings are rendered again with the scope")
	assert.Same(t, proxy, redeployer.calls[0].proxy)

	_, err = svc.CreateInProject(ctx, "org-uuid", scopeTestProjectUUID, "weather",
		models.MCPProxyScopeInput{Action: "write", Tools: []string{"not_a_tool"}})
	require.ErrorIs(t, err, utils.ErrInvalidInput, "tools are checked against the proxy's capabilities")

	scopeRepo.GetFunc = func(_ context.Context, _ uuid.UUID, action string) (*models.MCPProxyScope, error) {
		for i := range stored {
			if stored[i].Action == action {
				s := stored[i]
				return &s, nil
			}
		}
		return nil, gorm.ErrRecordNotFound
	}
	tools := []string{"get_forecast", "get_alerts"}
	updated, err := svc.UpdateInProject(ctx, "org-uuid", scopeTestProjectUUID, "weather", "read",
		models.MCPProxyScopeUpdateInput{Tools: tools})
	require.NoError(t, err)
	assert.ElementsMatch(t, tools, updated.Scope.Tools)

	listed, err := svc.ListInProject(ctx, "org-uuid", scopeTestProjectUUID, "weather")
	require.NoError(t, err)
	require.Len(t, listed.Scopes, 1)

	require.NoError(t, svc.DeleteInProject(ctx, "org-uuid", scopeTestProjectUUID, "weather", "read"))
	assert.Len(t, redeployer.calls, 3, "every change renders the bindings again")
}

// The project routes reach only that project's proxy; the org routes still refuse it as project-owned.
func TestProjectScopes_OtherProjectIsNotFound(t *testing.T) {
	proxy := scopeTestProxy("weather")
	svc := projectScopeSvc(t, &repomocks.MCPProxyScopeRepositoryMock{}, proxy, noopRedeployer{})
	ctx := context.Background()
	other := uuid.NewString()

	_, err := svc.ListInProject(ctx, "org-uuid", other, "weather")
	require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	_, err = svc.CreateInProject(ctx, "org-uuid", other, "weather", models.MCPProxyScopeInput{Action: "read"})
	require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	_, err = svc.UpdateInProject(ctx, "org-uuid", other, "weather", "read", models.MCPProxyScopeUpdateInput{})
	require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	require.ErrorIs(t, svc.DeleteInProject(ctx, "org-uuid", other, "weather", "read"), utils.ErrMCPProxyNotFound)

	_, err = svc.Create(ctx, "org-uuid", "org", "weather", models.MCPProxyScopeInput{Action: "read"})
	require.ErrorIs(t, err, utils.ErrMCPProxyProjectOwned)
}

// The scopes a role can grant include project proxies', so an agent can be given access to a tool of a proxy in its
// project.
func TestListEnvironmentScopes_IncludesProjectProxies(t *testing.T) {
	envUUID := uuid.New()
	on := true
	projectUUID := uuid.NewString()
	projectProxy := &models.MCPProxy{
		UUID: uuid.New(), ProjectUUID: &projectUUID, Artifact: &models.Artifact{Handle: "weather", Name: "p1 Weather"},
		Endpoints: []models.MCPProxyEndpoint{{
			UUID: uuid.New(), Name: "dev",
			Environments:  []models.MCPProxyEndpointEnvironment{{EnvironmentUUID: envUUID, ArtifactUUID: uuid.New()}},
			Configuration: models.MCPEndpointConfig{Security: &models.SecurityConfig{Enabled: &on, Identity: &models.IdentitySecurity{Enabled: &on}}},
		}},
	}
	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		ListFunc: func(_ context.Context, _ string, f repositories.MCPProxyListFilter) ([]*models.MCPProxy, int64, error) {
			assert.True(t, f.AllProjects, "project proxies are listed too")
			if f.Offset > 0 {
				return nil, 0, nil
			}
			return []*models.MCPProxy{projectProxy}, 1, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyUUIDsFunc: func(context.Context, []uuid.UUID) ([]models.MCPProxyScope, error) {
			return []models.MCPProxyScope{{MCPProxyUUID: projectProxy.UUID, Action: "read"}}, nil
		},
	}
	infra := stubInfraManager{listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
		return []*models.EnvironmentResponse{{Name: "dev", UUID: envUUID.String()}}, nil
	}}
	svc := NewMCPProxyScopeService(scopeRepo, proxyRepo, infra, nil, nil, slog.Default())
	entries, err := svc.ListEnvironmentScopes(context.Background(), "org-uuid", "dev")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "weather:read", entries[0].Scope)
}
