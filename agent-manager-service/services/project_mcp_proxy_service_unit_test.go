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
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	testOrg      = "org-uuid"
	testProjA    = "project-a-uuid"
	testProjB    = "project-b-uuid"
	testProjName = "proj-a"
)

// projectProxyFixture is a map-backed MCP proxy repository behind the real service.
type projectProxyFixture struct {
	svc          *ProjectMCPProxyService
	proxies      map[string]*models.MCPProxy // handle -> proxy
	endpoints    []*models.MCPProxyEndpoint
	updateCalls  int
	createCalls  int
	txCalls      int
	mappings     []models.EnvAgentMCPMapping
	endpointEnvs []models.MCPProxyEndpointEnvironment
	deletedProxy []string
	reassigned   []string // "handle:from->to" per ReassignProject

	componentTypes  map[string]string // component name -> OpenChoreo component type (default agent-api)
	getComponentErr error
	// componentProjects overrides the project a component belongs to (default: the project it is looked up in).
	componentProjects map[string]string
}

func newProjectProxyFixture(t *testing.T) *projectProxyFixture {
	t.Helper()
	f := &projectProxyFixture{proxies: map[string]*models.MCPProxy{}}

	repo := &repomocks.MCPProxyRepositoryMock{
		GetInProjectFunc: func(_ context.Context, handle, org, project string) (*models.MCPProxy, error) {
			p, ok := f.proxies[handle]
			if !ok || p.ProjectUUID == nil || *p.ProjectUUID != project {
				return nil, utils.ErrMCPProxyNotFound
			}
			cp := *p
			return &cp, nil
		},
		GetInProjectByNameFunc: func(_ context.Context, handle, _, projectName string) (*models.MCPProxy, error) {
			p, ok := f.proxies[handle]
			if !ok || p.ProjectUUID == nil || p.ProjectName == nil || *p.ProjectName != projectName {
				return nil, utils.ErrMCPProxyNotFound
			}
			cp := *p
			return &cp, nil
		},
		ReassignProjectFunc: func(_ context.Context, _ *gorm.DB, proxyUUID uuid.UUID, _, projectName, from, to string) error {
			for handle, p := range f.proxies {
				if p.UUID == proxyUUID && p.ProjectUUID != nil && *p.ProjectUUID == from && p.ProjectName != nil && *p.ProjectName == projectName {
					f.reassigned = append(f.reassigned, handle+":"+from+"->"+to)
					cp := *p
					cp.ProjectUUID = &to
					f.proxies[handle] = &cp
					return nil
				}
			}
			return utils.ErrMCPProxyNotFound
		},
		GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) {
			p, ok := f.proxies[handle]
			if !ok {
				return nil, gorm.ErrRecordNotFound
			}
			cp := *p
			return &cp, nil
		},
		ExistsFunc: func(_ context.Context, handle, _ string) (bool, error) {
			_, ok := f.proxies[handle]
			return ok, nil
		},
		CreateFunc: func(_ context.Context, _ *gorm.DB, p *models.MCPProxy, handle, name, version, _ string, projectUUID, projectName *string) error {
			f.createCalls++
			p.UUID = uuid.New()
			p.ProjectUUID, p.ProjectName = projectUUID, projectName
			p.Artifact = &models.Artifact{UUID: p.UUID, Handle: handle, Name: name, Version: version}
			f.proxies[handle] = p
			return nil
		},
		UpdateFunc: func(_ context.Context, _ *gorm.DB, p *models.MCPProxy, _ string) error {
			f.updateCalls++
			f.proxies[p.Artifact.Handle] = p
			return nil
		},
		ListFunc: func(_ context.Context, _ string, flt repositories.MCPProxyListFilter) ([]*models.MCPProxy, int64, error) {
			var out []*models.MCPProxy
			for _, p := range f.proxies {
				switch {
				case flt.ProjectUUID != nil && p.ProjectUUID != nil && *p.ProjectUUID == *flt.ProjectUUID:
					out = append(out, p)
				case flt.ProjectUUID == nil && flt.ProjectName != nil && p.ProjectUUID != nil &&
					p.ProjectName != nil && *p.ProjectName == *flt.ProjectName:
					out = append(out, p)
				}
			}
			return out, int64(len(out)), nil
		},
		DeleteFunc: func(_ context.Context, handle, _ string) error {
			f.deletedProxy = append(f.deletedProxy, handle)
			delete(f.proxies, handle)
			return nil
		},
	}
	endpointRepo := &repomocks.MCPProxyEndpointRepositoryMock{
		CreateEndpointFunc: func(_ context.Context, _ *gorm.DB, e *models.MCPProxyEndpoint) error {
			f.endpoints = append(f.endpoints, e)
			return nil
		},
		ListEndpointEnvironmentsByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error) {
			return f.endpointEnvs, nil
		},
	}
	mappingRepo := &repomocks.EnvAgentMCPMappingRepositoryMock{
		ListByMCPProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
			return f.mappings, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyScope, error) { return nil, nil },
	}
	base := &MCPProxyService{
		repo:              repo,
		endpointRepo:      endpointRepo,
		envMCPMappingRepo: mappingRepo,
		mcpProxyScopeRepo: scopeRepo,
		infraManager: stubInfraManager{listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return nil, nil
		}},
		logger: discardLogger(),
		tx:     func(ctx context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) },
	}
	f.componentTypes = map[string]string{}
	f.componentProjects = map[string]string{}
	f.getComponentErr = nil
	oc := &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(_ context.Context, _, projectName, name string) (*models.AgentResponse, error) {
			if f.getComponentErr != nil {
				return nil, f.getComponentErr
			}
			typ := f.componentTypes[name]
			if typ == "" {
				typ = "agent-api"
			}
			if project, ok := f.componentProjects[name]; ok {
				projectName = project
			}
			return &models.AgentResponse{Name: name, ProjectName: projectName, Type: models.AgentType{Type: typ}}, nil
		},
	}
	f.svc = NewProjectMCPProxyService(base, oc, nil)
	return f
}

func orderReq() models.ProjectMCPProxyRequest {
	return models.ProjectMCPProxyRequest{
		Name:        "Orders MCP",
		Description: "orders",
		Context:     "/orders-mcp-3f2a9c1e",
		Source:      models.MCPProxySource{ComponentName: "orders-mcp", EndpointName: "mcp"},
	}
}

func TestUpsertCreatesThen200OnIdenticalBody(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()

	first, created, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders-mcp-mcp-3f2a9c1e", orderReq())
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "/orders-mcp-3f2a9c1e", first.Context, "context reads back exactly as sent")
	assert.Equal(t, "v1", first.Version)
	assert.Equal(t, testProjName, first.ProjectName)
	assert.Equal(t, "orders-mcp", first.Source.ComponentName)
	assert.Empty(t, first.Environments)
	assert.NotNil(t, first.Environments, "environments serializes as [] not null")

	// No endpoint until the proxy is bound to an environment.
	assert.Empty(t, f.endpoints)

	second, created, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders-mcp-mcp-3f2a9c1e", orderReq())
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first, second, "identical body: nothing changes")
	assert.Zero(t, f.updateCalls, "identical body must not write")
	assert.Equal(t, 1, f.createCalls)
	assert.Empty(t, f.endpoints, "still no endpoint row")
}

func TestUpsertDefaultsContextAndVersion(t *testing.T) {
	f := newProjectProxyFixture(t)
	req := orderReq()
	req.Context = ""
	resp, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, testProjA, "orders", req)
	require.NoError(t, err)
	assert.Equal(t, "/orders", resp.Context)
	assert.Equal(t, "v1", resp.Version)
	// Re-sending the same defaulted body is still a no-op.
	_, created, err := f.svc.Upsert(context.Background(), testOrg, testProjName, testProjA, "orders", req)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Zero(t, f.updateCalls)
}

func TestUpsertChangedNameUpdates(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
	require.NoError(t, err)

	req := orderReq()
	req.Name = "Orders MCP v2"
	resp, created, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", req)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, "Orders MCP v2", resp.Name)
	assert.Equal(t, 1, f.updateCalls)
}

func TestUpsertSourceIsImmutable(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
	require.NoError(t, err)

	req := orderReq()
	req.Source.EndpointName = "other"
	_, _, err = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", req)
	assert.ErrorIs(t, err, utils.ErrImmutableFieldChange)
	assert.Zero(t, f.updateCalls)
}

func TestUpsertHandleOwnedByOtherProject_409HandleInUse(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.Upsert(ctx, testOrg, "proj-b", testProjB, "orders", orderReq())
	require.NoError(t, err)

	_, _, err = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
	assert.ErrorIs(t, err, utils.ErrMCPProxyHandleInUse)
	assert.Equal(t, 1, f.createCalls)
}

func TestUpsertHandleOwnedByOrgProxy_409HandleInUse(t *testing.T) {
	f := newProjectProxyFixture(t)
	f.proxies["orders"] = &models.MCPProxy{UUID: uuid.New(), Artifact: &models.Artifact{Handle: "orders"}} // no project
	_, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, testProjA, "orders", orderReq())
	assert.ErrorIs(t, err, utils.ErrMCPProxyHandleInUse)
	assert.Zero(t, f.createCalls)
}

func TestUpsertCreateNotPermitted(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.UpsertWithCreateCheck(ctx, testOrg, testProjName, testProjA, "orders", orderReq(), false)
	assert.ErrorIs(t, err, utils.ErrMCPProxyCreateNotPermitted)
	assert.Zero(t, f.createCalls)

	_, _, err = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
	require.NoError(t, err)
	_, created, err := f.svc.UpsertWithCreateCheck(ctx, testOrg, testProjName, testProjA, "orders", orderReq(), false)
	require.NoError(t, err, "an existing proxy needs no create permission")
	assert.False(t, created)
}

func TestUpsertValidation(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	bad := orderReq()
	bad.Name = ""
	_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", bad)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)

	bad = orderReq()
	bad.Source.ComponentName = ""
	_, _, err = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", bad)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)

	_, _, err = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "Bad_Handle", orderReq())
	assert.ErrorIs(t, err, utils.ErrInvalidInput)

	bad = orderReq()
	bad.Context = "no-slash"
	_, _, err = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", bad)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestGetFromOtherProject_404(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.Upsert(ctx, testOrg, "proj-b", testProjB, "orders", orderReq())
	require.NoError(t, err)

	_, err = f.svc.Get(ctx, testOrg, testProjName, testProjA, "orders")
	assert.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	got, err := f.svc.Get(ctx, testOrg, "proj-b", testProjB, "orders")
	require.NoError(t, err)
	assert.Equal(t, "orders", got.ID)
}

func TestListScopedToProject(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, _ = f.svc.Upsert(ctx, testOrg, "proj-b", testProjB, "b-one", orderReq())
	_, _, _ = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "a-one", orderReq())

	resp, err := f.svc.List(ctx, testOrg, testProjA, "", "", 50, 0)
	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "a-one", resp.List[0].ID)
	assert.Equal(t, 1, resp.Count)
	assert.Equal(t, 1, resp.Pagination.Count)
	assert.NotNil(t, resp.List[0].Environments)
}

func TestDeleteFromOtherProject_404(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.Upsert(ctx, testOrg, "proj-b", testProjB, "orders", orderReq())
	require.NoError(t, err)

	err = f.svc.Delete(ctx, testOrg, testProjName, testProjA, "orders")
	assert.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	assert.Empty(t, f.deletedProxy)
}

func TestDeleteWithAgentMapping_409McpProxyInUse(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
	require.NoError(t, err)
	f.mappings = []models.EnvAgentMCPMapping{{}}

	err = f.svc.Delete(ctx, testOrg, testProjName, testProjA, "orders")
	assert.ErrorIs(t, err, utils.ErrMCPProxyHasMappings)
	assert.Empty(t, f.deletedProxy, "nothing is deleted while agents use the proxy")
}

func TestDeleteUndeploysAllEnvironments(t *testing.T) {
	f := newProjectProxyFixture(t)
	ctx := context.Background()
	_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
	require.NoError(t, err)
	f.endpointEnvs = []models.MCPProxyEndpointEnvironment{
		{ArtifactUUID: uuid.New()}, {ArtifactUUID: uuid.New()},
	}
	// From here the runner only counts: each environment artifact is torn down in its own transaction.
	f.svc.base.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		f.txCalls++
		return nil
	}

	require.NoError(t, f.svc.Delete(ctx, testOrg, testProjName, testProjA, "orders"))
	assert.Equal(t, 2, f.txCalls, "one teardown per environment artifact")
	assert.Equal(t, []string{"orders"}, f.deletedProxy)
}

func TestUpsert_MIMCPServer_400(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "create"
		if existing {
			name = "update"
		}
		t.Run(name, func(t *testing.T) {
			f := newProjectProxyFixture(t)
			ctx := context.Background()
			if existing {
				_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
				require.NoError(t, err)
			}
			creates, updates := f.createCalls, f.updateCalls
			f.componentTypes["orders-mcp"] = "mi-mcp-server"

			req := orderReq()
			req.Description = "changed"
			_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", req)
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.Contains(t, err.Error(), "legacy HTTP+SSE MCP servers are not supported")
			assert.Equal(t, creates, f.createCalls, "nothing written")
			assert.Equal(t, updates, f.updateCalls, "nothing written")
		})
	}
}

func TestUpsert_SourceComponentLookup(t *testing.T) {
	t.Run("a component in another project is a 400 and nothing is written", func(t *testing.T) {
		for _, existing := range []bool{false, true} {
			f := newProjectProxyFixture(t)
			ctx := context.Background()
			req := orderReq()
			if existing {
				_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", req)
				require.NoError(t, err)
				req.Description = "changed"
			}
			creates, updates := f.createCalls, f.updateCalls
			f.componentProjects[req.Source.ComponentName] = "default"
			_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", req)
			require.ErrorIs(t, err, utils.ErrMCPSourceComponentNotFound)
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.Contains(t, err.Error(), "is not in project")
			assert.Equal(t, creates, f.createCalls, "nothing written")
			assert.Equal(t, updates, f.updateCalls, "nothing written")
		}
	})
	// An ensure-PUT after the component was deleted tells "component gone" apart from a bad body.
	t.Run("a component deleted after the proxy was created is SOURCE_COMPONENT_NOT_FOUND on update", func(t *testing.T) {
		f := newProjectProxyFixture(t)
		ctx := context.Background()
		_, _, err := f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
		require.NoError(t, err)
		updates := f.updateCalls
		f.getComponentErr = fmt.Errorf("get: %w", utils.ErrNotFound)
		_, _, err = f.svc.Upsert(ctx, testOrg, testProjName, testProjA, "orders", orderReq())
		require.ErrorIs(t, err, utils.ErrMCPSourceComponentNotFound)
		assert.Equal(t, updates, f.updateCalls, "nothing written")
	})
	t.Run("missing component is a 400", func(t *testing.T) {
		f := newProjectProxyFixture(t)
		f.getComponentErr = fmt.Errorf("get: %w", utils.ErrNotFound)
		_, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, testProjA, "orders", orderReq())
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Zero(t, f.createCalls)
	})
	t.Run("other lookup failures are not a 400", func(t *testing.T) {
		f := newProjectProxyFixture(t)
		f.getComponentErr = errors.New("openchoreo down")
		_, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, testProjA, "orders", orderReq())
		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrInvalidInput)
		assert.Zero(t, f.createCalls)
	})
}

// A context is a clean absolute path: the gateway would resolve ., .. or an empty segment to a different path than
// the one stored and clash-checked, so those are refused before anything is written.
func TestUpsert_ContextSegments(t *testing.T) {
	for _, bad := range []string{"/a/../b", "/a/./b", "/a//b", "/../a", "/a/", "/"} {
		t.Run(bad, func(t *testing.T) {
			f := newProjectProxyFixture(t)
			req := orderReq()
			req.Context = bad
			_, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, testProjA, "orders", req)
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.Zero(t, f.createCalls, "nothing written")
		})
	}
	for _, good := range []string{"/orders", "/a/b-c/v1.2", "/a/..b"} {
		t.Run(good, func(t *testing.T) {
			f := newProjectProxyFixture(t)
			req := orderReq()
			req.Context = good
			resp, _, err := f.svc.Upsert(context.Background(), testOrg, testProjName, testProjA, "orders", req)
			require.NoError(t, err)
			assert.Equal(t, good, resp.Context, "reads back as sent")
		})
	}
}
