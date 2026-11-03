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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const orgPickerUpstream = "https://internal.example.com/mcp"

func orgPickerOrgProxy() *models.MCPProxy {
	return &models.MCPProxy{UUID: uuid.New(), Artifact: &models.Artifact{Handle: "org-mcp", Name: "Org MCP", Version: "v1"}}
}

func orgPickerProjectProxy() *models.MCPProxy {
	projUUID, projName := "proj-uuid", "p1"
	return &models.MCPProxy{
		UUID:        uuid.New(),
		ProjectUUID: &projUUID,
		ProjectName: &projName,
		Artifact:    &models.Artifact{Handle: "p1-mcp", Name: "P1 MCP", Version: "v1"},
		Endpoints: []models.MCPProxyEndpoint{{
			UUID:   uuid.New(),
			Handle: "primary",
			Configuration: models.MCPEndpointConfig{
				Upstream: &models.UpstreamEndpoint{URL: orgPickerUpstream, Auth: &models.UpstreamAuth{}},
			},
		}},
	}
}

// orgPickerSvc serves the proxies by handle and records the filter List was called with.
func orgPickerSvc(filter *repositories.MCPProxyListFilter, proxies ...*models.MCPProxy) *MCPProxyService {
	return &MCPProxyService{
		logger: slog.Default(),
		repo: &repomocks.MCPProxyRepositoryMock{
			GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) {
				for _, p := range proxies {
					if p.Artifact.Handle == handle {
						return p, nil
					}
				}
				return nil, utils.ErrMCPProxyNotFound
			},
			ListFunc: func(_ context.Context, _ string, f repositories.MCPProxyListFilter) ([]*models.MCPProxy, int64, error) {
				if filter != nil {
					*filter = f
				}
				var out []*models.MCPProxy
				for _, p := range proxies {
					switch {
					case p.ProjectUUID == nil:
						out = append(out, p)
					case f.ProjectUUID != nil && *f.ProjectUUID == *p.ProjectUUID:
						out = append(out, p)
					}
				}
				return out, int64(len(out)), nil
			},
			CountFunc: func(context.Context, string) (int, error) { return 1, nil },
		},
	}
}

func TestOrgListWithoutProjectExcludesProjectProxies(t *testing.T) {
	var f repositories.MCPProxyListFilter
	svc := orgPickerSvc(&f, orgPickerOrgProxy(), orgPickerProjectProxy())

	resp, err := svc.List(context.Background(), "org", nil, 20, 0)
	require.NoError(t, err)
	assert.Nil(t, f.ProjectUUID)
	assert.False(t, f.AllProjects)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "org-mcp", *resp.List[0].ID)
	assert.Nil(t, resp.List[0].ProjectName)
}

func TestOrgListWithProjectIncludesBoth(t *testing.T) {
	var f repositories.MCPProxyListFilter
	svc := orgPickerSvc(&f, orgPickerOrgProxy(), orgPickerProjectProxy())

	proj := "proj-uuid"
	resp, err := svc.List(context.Background(), "org", &proj, 20, 0)
	require.NoError(t, err)
	require.NotNil(t, f.ProjectUUID)
	assert.Equal(t, "proj-uuid", *f.ProjectUUID)
	assert.True(t, f.IncludeOrgOwned)
	assert.False(t, f.AllProjects)
	require.Len(t, resp.List, 2)
	assert.Equal(t, 2, resp.Count)
	assert.EqualValues(t, 2, resp.Pagination.Count)
	byID := map[string]models.MCPProxyListItem{}
	for _, it := range resp.List {
		byID[*it.ID] = it
	}
	assert.Nil(t, byID["org-mcp"].ProjectName)
	require.NotNil(t, byID["p1-mcp"].ProjectName)
	assert.Equal(t, "p1", *byID["p1-mcp"].ProjectName)
}

func TestOrgGetProjectProxyIsReadOnlyNoUpstream(t *testing.T) {
	svc := orgPickerSvc(nil, orgPickerProjectProxy())

	resp, err := svc.Get(context.Background(), "org", "p1-mcp")
	require.NoError(t, err)
	require.NotNil(t, resp.ProjectName)
	assert.Equal(t, "p1", *resp.ProjectName)
	require.Len(t, resp.Endpoints, 1)
	assert.Nil(t, resp.Endpoints[0].Upstream.Main, "upstream must not be exposed")
	assert.Nil(t, resp.Endpoints[0].Upstream.Sandbox)
}

func TestOrgGetOrgProxyKeepsUpstreamAndHasNoProjectName(t *testing.T) {
	p := orgPickerOrgProxy()
	p.Endpoints = []models.MCPProxyEndpoint{{
		UUID: uuid.New(), Handle: "primary",
		Configuration: models.MCPEndpointConfig{Upstream: &models.UpstreamEndpoint{URL: orgPickerUpstream}},
	}}
	svc := orgPickerSvc(nil, p)

	resp, err := svc.Get(context.Background(), "org", "org-mcp")
	require.NoError(t, err)
	assert.Nil(t, resp.ProjectName)
	require.NotNil(t, resp.Endpoints[0].Upstream.Main)
	assert.Equal(t, orgPickerUpstream, resp.Endpoints[0].Upstream.Main.URL)
}

func TestOrgUpdateProjectProxy_409ProjectOwned(t *testing.T) {
	svc := orgPickerSvc(nil, orgPickerProjectProxy())

	_, err := svc.Update(context.Background(), "org", "p1-mcp", &models.MCPProxyDTO{ID: "p1-mcp", Name: "x", Version: "v1"})
	assert.ErrorIs(t, err, utils.ErrMCPProxyProjectOwned)
}

func TestOrgDeleteProjectProxy_409ProjectOwned(t *testing.T) {
	svc := orgPickerSvc(nil, orgPickerProjectProxy())

	err := svc.Delete(context.Background(), "org", "org-name", "p1-mcp")
	assert.ErrorIs(t, err, utils.ErrMCPProxyProjectOwned)
}

func TestScopeMutationsOnProjectProxy_ProjectOwned(t *testing.T) {
	proxy := orgPickerProjectProxy()
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{}
	svc := newScopeSvcForTest(scopeRepo, proxy)
	ctx := context.Background()

	_, err := svc.Create(ctx, "org", "org-name", "p1-mcp", models.MCPProxyScopeInput{Action: "read"})
	assert.ErrorIs(t, err, utils.ErrMCPProxyProjectOwned)
	_, err = svc.Update(ctx, "org", "org-name", "p1-mcp", "read", models.MCPProxyScopeUpdateInput{})
	assert.ErrorIs(t, err, utils.ErrMCPProxyProjectOwned)
	err = svc.Delete(ctx, "org", "org-name", "p1-mcp", "read")
	assert.ErrorIs(t, err, utils.ErrMCPProxyProjectOwned)
}

func TestListEnvironmentScopesExcludesProjectProxies(t *testing.T) {
	var f repositories.MCPProxyListFilter
	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		ListFunc: func(_ context.Context, _ string, got repositories.MCPProxyListFilter) ([]*models.MCPProxy, int64, error) {
			f = got
			return nil, 0, nil
		},
	}
	envUUID := uuid.New()
	infra := stubInfraManager{listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
		return []*models.EnvironmentResponse{{Name: "dev", UUID: envUUID.String()}}, nil
	}}
	svc := NewMCPProxyScopeService(&repomocks.MCPProxyScopeRepositoryMock{}, proxyRepo, infra, nil, nil, slog.Default())

	_, err := svc.ListEnvironmentScopes(context.Background(), "org", "dev")
	require.NoError(t, err)
	assert.False(t, f.AllProjects, "the user-facing scope catalog must not sweep project proxies")
	assert.Nil(t, f.ProjectUUID)
}
