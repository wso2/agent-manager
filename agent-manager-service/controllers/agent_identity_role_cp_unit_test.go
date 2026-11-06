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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// fakeRoleProvider is a deployment's role provider. It records the order of its calls together with
// the resource server ensures, so a test can prove the ensure comes first.
type fakeRoleProvider struct {
	mu      sync.Mutex
	calls   *[]string
	list    services.IdentityRoleList
	role    *thundersvc.ThunderRole
	err     error
	listReq services.IdentityRoleListRequest
	getRef  services.IdentityRoleRef
	written []services.IdentityRoleWrite

	deleted        []services.IdentityRoleRef
	assignments    *thundersvc.AgentRoleAssignments
	assigneeWrites []string // "add" or "remove", then the write
	assigned       []services.IdentityRoleAssignmentWrite
}

func (p *fakeRoleProvider) record(call string) {
	if p.calls != nil {
		*p.calls = append(*p.calls, call)
	}
}

func (p *fakeRoleProvider) ListRoles(_ context.Context, request services.IdentityRoleListRequest) (services.IdentityRoleList, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.record("list")
	p.listReq = request
	return p.list, p.err
}

func (p *fakeRoleProvider) GetRole(_ context.Context, ref services.IdentityRoleRef) (*thundersvc.ThunderRole, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.record("get")
	p.getRef = ref
	return p.role, p.err
}

func (p *fakeRoleProvider) CreateRole(_ context.Context, write services.IdentityRoleWrite) (*thundersvc.ThunderRole, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.record("create")
	p.written = append(p.written, write)
	return &thundersvc.ThunderRole{ID: "role-new", Name: *write.Name}, p.err
}

func (p *fakeRoleProvider) UpdateRole(_ context.Context, write services.IdentityRoleWrite) (*thundersvc.ThunderRole, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.record("update")
	p.written = append(p.written, write)
	return &thundersvc.ThunderRole{ID: write.RoleID, Name: "updated"}, p.err
}

func (p *fakeRoleProvider) DeleteRole(_ context.Context, ref services.IdentityRoleRef) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.record("delete")
	p.deleted = append(p.deleted, ref)
	return p.err
}

func (p *fakeRoleProvider) GetRoleAssignments(_ context.Context, ref services.IdentityRoleRef) (*thundersvc.AgentRoleAssignments, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.record("assignments")
	p.getRef = ref
	return p.assignments, p.err
}

func (p *fakeRoleProvider) AddRoleAssignees(_ context.Context, write services.IdentityRoleAssignmentWrite) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.record("add-assignees")
	p.assigned = append(p.assigned, write)
	return p.err
}

func (p *fakeRoleProvider) RemoveRoleAssignees(_ context.Context, write services.IdentityRoleAssignmentWrite) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.record("remove-assignees")
	p.assigned = append(p.assigned, write)
	return p.err
}

func useRoleMode(t *testing.T, mode thundersvc.IdentityManagementMode, provider services.IdentityRoleProvider) {
	t.Helper()
	services.SetIdentityRoleProvider(mode, provider)
	t.Cleanup(func() { services.SetIdentityRoleProvider(thundersvc.IdentityManagementLegacyDirect, nil) })
}

// cpController builds a controller whose environment Thunder resolver fails the test when reached.
func cpController(t *testing.T, rs stubRSIdentifierResolver) AgentIdentityController {
	t.Helper()
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
			t.Error("the environment's own Thunder must not be used in CP_MANAGED mode")
			return nil, errors.New("not expected")
		},
	}
	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) {
			if handle == "gh-proxy" || handle == "jira-proxy" {
				return &models.MCPProxy{UUID: uuid.New(), Handle: handle}, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, proxyUUID uuid.UUID, action string) (*models.MCPProxyScope, error) {
			return &models.MCPProxyScope{MCPProxyUUID: proxyUUID, Action: action}, nil
		},
	}
	return NewAgentIdentityController(resolver, &repomocks.AgentThunderClientRepositoryMock{}, proxyRepo, scopeRepo, rs)
}

func cpRoleRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.SetPathValue("orgName", "o1")
	req.SetPathValue("envName", "dev")
	req.SetPathValue("roleID", "role-1")
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-org"}))
}

const rolesPath = "/orgs/o1/environments/dev/agent-identities/roles"

func TestRolesViaControlPlane_ListAndGet(t *testing.T) {
	provider := &fakeRoleProvider{
		list: services.IdentityRoleList{Roles: []thundersvc.ThunderRole{{ID: "r1", Name: "readers"}}, Total: 7},
		role: &thundersvc.ThunderRole{ID: "role-1", Name: "readers"},
	}
	useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
	ctrl := cpController(t, stubRSIdentifierResolver{})

	w := httptest.NewRecorder()
	ctrl.ListRoles(w, cpRoleRequest(http.MethodGet, rolesPath+"?offset=5&limit=10", ""))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, services.IdentityRoleListRequest{OrganizationID: "ou-org", Environment: "dev", Offset: 5, Limit: 10}, provider.listReq,
		"the organization unit ID and the environment reach the provider")
	assert.Contains(t, w.Body.String(), `"total":7`)
	assert.Contains(t, w.Body.String(), "readers")

	w = httptest.NewRecorder()
	ctrl.GetRole(w, cpRoleRequest(http.MethodGet, rolesPath+"/role-1", ""))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, services.IdentityRoleRef{OrganizationID: "ou-org", Environment: "dev", RoleID: "role-1"}, provider.getRef)
}

func TestRolesViaControlPlane_GetErrors(t *testing.T) {
	cases := map[string]struct {
		provider *fakeRoleProvider
		want     int
	}{
		"not found":                 {&fakeRoleProvider{err: fmt.Errorf("get: %w", services.ErrIdentityRoleNotFound)}, http.StatusNotFound},
		"native administrator":      {&fakeRoleProvider{role: &thundersvc.ThunderRole{ID: "role-1", Name: thundersvc.NativeAdministratorRoleName}}, http.StatusNotFound},
		"AMP system client":         {&fakeRoleProvider{role: &thundersvc.ThunderRole{ID: "role-1", Name: thundersvc.AMPSystemClientRoleName}}, http.StatusNotFound},
		"unavailable":               {&fakeRoleProvider{err: services.ErrIdentityRoleUnavailable}, http.StatusServiceUnavailable},
		"the control plane failing": {&fakeRoleProvider{err: errors.New("boom")}, http.StatusBadGateway},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			useRoleMode(t, thundersvc.IdentityManagementCPManaged, test.provider)
			w := httptest.NewRecorder()
			cpController(t, stubRSIdentifierResolver{}).GetRole(w, cpRoleRequest(http.MethodGet, rolesPath+"/role-1", ""))
			assert.Equal(t, test.want, w.Code)
			assert.NotContains(t, w.Body.String(), "boom", "a provider's error text is not shown")
		})
	}
}

func TestRolesViaControlPlane_CreateEnsuresResourceServersFirst(t *testing.T) {
	var calls []string
	provider := &fakeRoleProvider{calls: &calls}
	useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
	var ensured []ensureCall
	ctrl := cpController(t, stubRSIdentifierResolver{ensured: &ensured})

	w := httptest.NewRecorder()
	ctrl.CreateRole(w, cpRoleRequest(http.MethodPost, rolesPath,
		`{"name":"readers","description":"d","scopes":["jira-proxy:write","gh-proxy:read","gh-proxy:list"]}`))

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Equal(t, []ensureCall{
		{ouID: "ou-org", env: "dev", handle: "gh-proxy", actions: []string{"list", "read"}},
		{ouID: "ou-org", env: "dev", handle: "jira-proxy", actions: []string{"write"}},
	}, ensured, "each referenced proxy's resource server is ensured for the environment, in handle order")
	require.Len(t, provider.written, 1)
	write := provider.written[0]
	assert.Equal(t, "ou-org", write.OrganizationID)
	assert.Equal(t, "dev", write.Environment)
	assert.Equal(t, "readers", *write.Name)
	assert.Equal(t, "d", *write.Description)
	require.NotNil(t, write.Permissions)
	assert.Equal(t, []thundersvc.RolePermissionRequest{
		{ResourceServerID: "rs-gh-proxy", Permissions: []string{"gh-proxy:list", "gh-proxy:read"}},
		{ResourceServerID: "rs-jira-proxy", Permissions: []string{"jira-proxy:write"}},
	}, *write.Permissions, "scopes are grouped by the resource server the ensure returned")
}

func TestRolesViaControlPlane_CreateRejections(t *testing.T) {
	cases := map[string]struct {
		body string
		rs   stubRSIdentifierResolver
		want int
	}{
		"no name":              {`{"scopes":[]}`, stubRSIdentifierResolver{}, http.StatusBadRequest},
		"reserved name":        {`{"name":"` + thundersvc.NativeAdministratorRoleName + `"}`, stubRSIdentifierResolver{}, http.StatusBadRequest},
		"malformed scope":      {`{"name":"r","scopes":["noaction"]}`, stubRSIdentifierResolver{}, http.StatusBadRequest},
		"unknown proxy":        {`{"name":"r","scopes":["nope:read"]}`, stubRSIdentifierResolver{}, http.StatusBadRequest},
		"proxy not deployed":   {`{"name":"r","scopes":["gh-proxy:read"]}`, stubRSIdentifierResolver{err: fmt.Errorf("%w: gh-proxy", services.ErrMCPProxyNotDeployedToEnvironment)}, http.StatusBadRequest},
		"no MCP provider":      {`{"name":"r","scopes":["gh-proxy:read"]}`, stubRSIdentifierResolver{err: services.ErrMCPResourceServerUnavailable}, http.StatusServiceUnavailable},
		"the ensure failing":   {`{"name":"r","scopes":["gh-proxy:read"]}`, stubRSIdentifierResolver{err: errors.New("down")}, http.StatusBadGateway},
		"invalid request body": {`{`, stubRSIdentifierResolver{}, http.StatusBadRequest},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			provider := &fakeRoleProvider{}
			useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
			w := httptest.NewRecorder()
			cpController(t, test.rs).CreateRole(w, cpRoleRequest(http.MethodPost, rolesPath, test.body))
			assert.Equal(t, test.want, w.Code, w.Body.String())
			assert.Empty(t, provider.written, "no role is written when the request is refused")
		})
	}
}

func TestRolesViaControlPlane_UpdateScopesAreOptional(t *testing.T) {
	existing := &thundersvc.ThunderRole{ID: "role-1", Name: "readers"}

	t.Run("omitted scopes leave the permissions alone", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		var ensured []ensureCall
		w := httptest.NewRecorder()
		cpController(t, stubRSIdentifierResolver{ensured: &ensured}).UpdateRole(w, cpRoleRequest(http.MethodPut, rolesPath+"/role-1", `{"description":"new"}`))
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, provider.written, 1)
		assert.Nil(t, provider.written[0].Permissions, "nil means: leave the role's permissions as they are")
		assert.Nil(t, provider.written[0].Name, "an omitted name is preserved")
		assert.Equal(t, "role-1", provider.written[0].RoleID)
		assert.Empty(t, ensured, "no resource server is touched for a metadata-only update")
	})

	t.Run("an explicit empty list clears them", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		cpController(t, stubRSIdentifierResolver{}).UpdateRole(w, cpRoleRequest(http.MethodPut, rolesPath+"/role-1", `{"name":"readers","scopes":[]}`))
		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, provider.written[0].Permissions)
		assert.Empty(t, *provider.written[0].Permissions)
		assert.Equal(t, "readers", *provider.written[0].Name)
	})

	t.Run("scopes replace the permissions after the ensure", func(t *testing.T) {
		var calls []string
		provider := &fakeRoleProvider{role: existing, calls: &calls}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		var ensured []ensureCall
		w := httptest.NewRecorder()
		cpController(t, stubRSIdentifierResolver{ensured: &ensured}).UpdateRole(w, cpRoleRequest(http.MethodPut, rolesPath+"/role-1", `{"scopes":["gh-proxy:read"]}`))
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, ensured, 1)
		assert.Equal(t, []thundersvc.RolePermissionRequest{{ResourceServerID: "rs-gh-proxy", Permissions: []string{"gh-proxy:read"}}}, *provider.written[0].Permissions)
	})

	t.Run("a hidden native role cannot be edited", func(t *testing.T) {
		provider := &fakeRoleProvider{role: &thundersvc.ThunderRole{ID: "role-1", Name: thundersvc.NativeAdministratorRoleName}}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		cpController(t, stubRSIdentifierResolver{}).UpdateRole(w, cpRoleRequest(http.MethodPut, rolesPath+"/role-1", `{"description":"x"}`))
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, provider.written)
	})

	t.Run("a missing role is a 404", func(t *testing.T) {
		provider := &fakeRoleProvider{err: services.ErrIdentityRoleNotFound}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		cpController(t, stubRSIdentifierResolver{}).UpdateRole(w, cpRoleRequest(http.MethodPut, rolesPath+"/role-1", `{"description":"x"}`))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("a reserved name is refused", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		cpController(t, stubRSIdentifierResolver{}).UpdateRole(w, cpRoleRequest(http.MethodPut, rolesPath+"/role-1", `{"name":"`+thundersvc.NativeAdministratorRoleName+`"}`))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, provider.written)
	})
}

func TestRolesViaControlPlane_WithoutAProviderFailsClosed(t *testing.T) {
	useRoleMode(t, thundersvc.IdentityManagementCPManaged, nil)
	ctrl := cpController(t, stubRSIdentifierResolver{}) // its resolver fails the test if reached
	for name, call := range map[string]func(w http.ResponseWriter, r *http.Request){
		"list": ctrl.ListRoles, "get": ctrl.GetRole, "create": ctrl.CreateRole, "update": ctrl.UpdateRole,
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			call(w, cpRoleRequest(http.MethodPost, rolesPath+"/role-1", `{"name":"r"}`))
			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	}
}

func TestRolesLegacyModeIgnoresAProvider(t *testing.T) {
	provider := &fakeRoleProvider{}
	useRoleMode(t, thundersvc.IdentityManagementLegacyDirect, provider)
	listed := false
	envClient := &clientmocks.EnvIdentityClientMock{
		GetDefaultOUIDFunc: func(context.Context) (string, error) { return "ou-env", nil },
		ListRolesFunc: func(_ context.Context, ouID string, _, _ int) ([]thundersvc.ThunderRole, int, error) {
			listed = true
			assert.Equal(t, "ou-env", ouID)
			return nil, 0, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
			return envClient, nil
		},
	}
	ctrl := NewAgentIdentityController(resolver, &repomocks.AgentThunderClientRepositoryMock{}, &repomocks.MCPProxyRepositoryMock{}, &repomocks.MCPProxyScopeRepositoryMock{}, nil)

	w := httptest.NewRecorder()
	ctrl.ListRoles(w, cpRoleRequest(http.MethodGet, rolesPath, ""))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, listed, "the environment's own Thunder, as before")
	assert.Empty(t, provider.calls, "the provider is not used outside CP_MANAGED mode")
}
