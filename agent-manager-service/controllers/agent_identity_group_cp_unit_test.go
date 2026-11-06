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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// fakeGroupProvider is a deployment's group provider that records its calls.
type fakeGroupProvider struct {
	mu      sync.Mutex
	calls   []string
	group   *thundersvc.ThunderGroup
	groups  services.IdentityGroupList
	members services.IdentityGroupMembers
	roles   []thundersvc.ThunderRole
	err     error

	listReq      services.IdentityGroupListRequest
	ref          services.IdentityGroupRef
	writes       []services.IdentityGroupWrite
	memberWrites []services.IdentityGroupMembersWrite
}

func (p *fakeGroupProvider) note(call string) { p.calls = append(p.calls, call) }

func (p *fakeGroupProvider) ListGroups(_ context.Context, r services.IdentityGroupListRequest) (services.IdentityGroupList, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.note("list")
	p.listReq = r
	return p.groups, p.err
}

func (p *fakeGroupProvider) GetGroup(_ context.Context, ref services.IdentityGroupRef) (*thundersvc.ThunderGroup, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.note("get")
	p.ref = ref
	return p.group, p.err
}

func (p *fakeGroupProvider) CreateGroup(_ context.Context, w services.IdentityGroupWrite) (*thundersvc.ThunderGroup, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.note("create")
	p.writes = append(p.writes, w)
	return &thundersvc.ThunderGroup{ID: "g-new", Name: *w.Name}, p.err
}

func (p *fakeGroupProvider) UpdateGroup(_ context.Context, w services.IdentityGroupWrite) (*thundersvc.ThunderGroup, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.note("update")
	p.writes = append(p.writes, w)
	return &thundersvc.ThunderGroup{ID: w.GroupID, Name: "updated"}, p.err
}

func (p *fakeGroupProvider) DeleteGroup(_ context.Context, ref services.IdentityGroupRef) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.note("delete")
	p.ref = ref
	return p.err
}

func (p *fakeGroupProvider) ListGroupMembers(_ context.Context, r services.IdentityGroupMembersRequest) (services.IdentityGroupMembers, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.note("members")
	p.ref = r.IdentityGroupRef
	return p.members, p.err
}

func (p *fakeGroupProvider) AddGroupMembers(_ context.Context, w services.IdentityGroupMembersWrite) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.note("add-members")
	p.memberWrites = append(p.memberWrites, w)
	return p.err
}

func (p *fakeGroupProvider) RemoveGroupMembers(_ context.Context, w services.IdentityGroupMembersWrite) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.note("remove-members")
	p.memberWrites = append(p.memberWrites, w)
	return p.err
}

func (p *fakeGroupProvider) GetGroupRoles(_ context.Context, ref services.IdentityGroupRef) ([]thundersvc.ThunderRole, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.note("group-roles")
	p.ref = ref
	return p.roles, p.err
}

func useGroupMode(t *testing.T, mode thundersvc.IdentityManagementMode, provider services.IdentityGroupProvider) {
	t.Helper()
	services.SetIdentityGroupProvider(mode, provider)
	t.Cleanup(func() { services.SetIdentityGroupProvider(thundersvc.IdentityManagementLegacyDirect, nil) })
}

// agentController is cpController with the agents provisioned in the "dev" environment.
func envAgentController(t *testing.T, agentIDs ...string) AgentIdentityController {
	t.Helper()
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
			t.Error("the environment's own Thunder must not be used in CP_MANAGED mode")
			return nil, errors.New("not expected")
		},
	}
	bindings := &repomocks.AgentThunderClientRepositoryMock{
		FindByOuAndEnvironmentFunc: func(_ context.Context, ouID, env string) ([]models.AgentThunderClient, error) {
			assert.Equal(t, "ou-org", ouID)
			assert.Equal(t, "dev", env)
			rows := []models.AgentThunderClient{{AgentName: "no-identity-yet"}} // a binding without an AgentID is not offered
			for _, id := range agentIDs {
				rows = append(rows, models.AgentThunderClient{AgentName: "agent-" + id, ThunderAgentID: id})
			}
			return rows, nil
		},
	}
	return NewAgentIdentityController(resolver, bindings, &repomocks.MCPProxyRepositoryMock{}, &repomocks.MCPProxyScopeRepositoryMock{}, stubRSIdentifierResolver{})
}

func groupReq(method, body string) *http.Request {
	req := cpRoleRequest(method, "/orgs/o1/environments/dev/agent-identities/groups/grp-1", body)
	req.SetPathValue("groupID", "grp-1")
	return req
}

const assigneesBody = `{"assignments":[{"id":"a1","type":"agent"}]}`

func TestRoleDeleteAndAssignmentsViaControlPlane(t *testing.T) {
	existing := &thundersvc.ThunderRole{ID: "role-1", Name: "readers"}

	t.Run("delete passes the organization and environment", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).DeleteRole(w, cpRoleRequest(http.MethodDelete, rolesPath+"/role-1", ""))
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, []services.IdentityRoleRef{{OrganizationID: "ou-org", Environment: "dev", RoleID: "role-1"}}, provider.deleted)
	})

	t.Run("a hidden native role cannot be deleted", func(t *testing.T) {
		provider := &fakeRoleProvider{role: &thundersvc.ThunderRole{ID: "role-1", Name: thundersvc.NativeAdministratorRoleName}}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).DeleteRole(w, cpRoleRequest(http.MethodDelete, rolesPath+"/role-1", ""))
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, provider.deleted)
	})

	t.Run("assignments are read through the provider", func(t *testing.T) {
		provider := &fakeRoleProvider{assignments: &thundersvc.AgentRoleAssignments{Agents: []thundersvc.AssignmentEntry{{ID: "a1", Type: "agent"}}}}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).GetRoleAssignments(w, cpRoleRequest(http.MethodGet, rolesPath+"/role-1/assignments", ""))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"a1"`)
		assert.Equal(t, "dev", provider.getRef.Environment)
	})

	t.Run("an unknown role is a 404", func(t *testing.T) {
		provider := &fakeRoleProvider{err: services.ErrIdentityRoleNotFound}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).GetRoleAssignments(w, cpRoleRequest(http.MethodGet, rolesPath+"/role-1/assignments", ""))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAddRoleAssigneesViaControlPlane(t *testing.T) {
	existing := &thundersvc.ThunderRole{ID: "role-1", Name: "readers"}
	add := func(t *testing.T, ctrl AgentIdentityController, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		ctrl.AddRoleAssignees(w, cpRoleRequest(http.MethodPost, rolesPath+"/role-1/assignments/add", body))
		return w
	}

	t.Run("an agent of the environment is assigned", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := add(t, envAgentController(t, "a1"), assigneesBody)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Len(t, provider.assigned, 1)
		assert.Equal(t, services.IdentityRoleAssignmentWrite{
			OrganizationID: "ou-org", Environment: "dev", RoleID: "role-1",
			Assignments: []thundersvc.AssignmentEntry{{ID: "a1", Type: "agent"}},
		}, provider.assigned[0])
	})

	t.Run("an agent that is not provisioned in the environment is refused", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := add(t, envAgentController(t, "someone-else"), assigneesBody)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "not provisioned in environment")
		assert.Empty(t, provider.assigned)
	})

	t.Run("a group of the environment is assigned", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, &fakeGroupProvider{group: &thundersvc.ThunderGroup{ID: "g1", Name: "ops"}})
		w := add(t, envAgentController(t), `{"assignments":[{"id":"g1","type":"group"}]}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Len(t, provider.assigned, 1)
	})

	t.Run("a group the environment does not have is refused", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, &fakeGroupProvider{err: services.ErrIdentityGroupNotFound})
		w := add(t, envAgentController(t), `{"assignments":[{"id":"g1","type":"group"}]}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, provider.assigned)
	})

	t.Run("groups the identity platform cannot hold give 501", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, &fakeGroupProvider{err: services.ErrIdentityGroupUnsupported})
		w := add(t, envAgentController(t), `{"assignments":[{"id":"g1","type":"group"}]}`)
		assert.Equal(t, http.StatusNotImplemented, w.Code)
		assert.Empty(t, provider.assigned)
	})

	t.Run("no group provider gives 503 for a group assignee", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, nil)
		w := add(t, envAgentController(t), `{"assignments":[{"id":"g1","type":"group"}]}`)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("bad requests", func(t *testing.T) {
		provider := &fakeRoleProvider{role: existing}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		for name, body := range map[string]string{
			"empty":    `{"assignments":[]}`,
			"bad type": `{"assignments":[{"id":"u1","type":"user"}]}`,
			"no id":    `{"assignments":[{"id":"","type":"agent"}]}`,
			"not json": `{`,
		} {
			w := add(t, envAgentController(t, "a1"), body)
			assert.Equal(t, http.StatusBadRequest, w.Code, name)
		}
		assert.Empty(t, provider.assigned)
	})

	t.Run("a hidden native role takes no assignees", func(t *testing.T) {
		provider := &fakeRoleProvider{role: &thundersvc.ThunderRole{ID: "role-1", Name: thundersvc.NativeAdministratorRoleName}}
		useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := add(t, envAgentController(t, "a1"), assigneesBody)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, provider.assigned)
	})
}

func TestRemoveRoleAssigneesViaControlPlane(t *testing.T) {
	provider := &fakeRoleProvider{}
	useRoleMode(t, thundersvc.IdentityManagementCPManaged, provider)
	w := httptest.NewRecorder()
	// The agent is not in the environment any more: removing it must still work.
	envAgentController(t).RemoveRoleAssignees(w, cpRoleRequest(http.MethodPost, rolesPath+"/role-1/assignments/remove", assigneesBody))
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, provider.assigned, 1)
	assert.Equal(t, "role-1", provider.assigned[0].RoleID)
}

func TestGroupsViaControlPlane(t *testing.T) {
	group := &thundersvc.ThunderGroup{ID: "grp-1", Name: "ops"}

	t.Run("list passes the organization and environment", func(t *testing.T) {
		provider := &fakeGroupProvider{groups: services.IdentityGroupList{Groups: []thundersvc.ThunderGroup{*group}, Total: 4}}
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).ListGroups(w, cpRoleRequest(http.MethodGet, "/orgs/o1/environments/dev/agent-identities/groups?offset=2&limit=5", ""))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, services.IdentityGroupListRequest{OrganizationID: "ou-org", Environment: "dev", Offset: 2, Limit: 5}, provider.listReq)
		assert.Contains(t, w.Body.String(), `"total":4`)
	})

	t.Run("get, with the native group hidden", func(t *testing.T) {
		provider := &fakeGroupProvider{group: group}
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).GetGroup(w, groupReq(http.MethodGet, ""))
		assert.Equal(t, http.StatusOK, w.Code)

		provider.group = &thundersvc.ThunderGroup{ID: "grp-1", Name: thundersvc.NativeAdministratorsGroupName}
		w = httptest.NewRecorder()
		envAgentController(t).GetGroup(w, groupReq(http.MethodGet, ""))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("create", func(t *testing.T) {
		provider := &fakeGroupProvider{}
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).CreateGroup(w, cpRoleRequest(http.MethodPost, "/orgs/o1/environments/dev/agent-identities/groups", `{"name":"ops","description":"d"}`))
		require.Equal(t, http.StatusCreated, w.Code)
		require.Len(t, provider.writes, 1)
		assert.Equal(t, "ou-org", provider.writes[0].OrganizationID)
		assert.Equal(t, "dev", provider.writes[0].Environment)
		assert.Equal(t, "ops", *provider.writes[0].Name)
		assert.Equal(t, "d", *provider.writes[0].Description)
	})

	t.Run("create refuses a missing or reserved name", func(t *testing.T) {
		provider := &fakeGroupProvider{}
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, provider)
		for _, body := range []string{`{}`, `{"name":"` + thundersvc.NativeAdministratorsGroupName + `"}`, `{`} {
			w := httptest.NewRecorder()
			envAgentController(t).CreateGroup(w, cpRoleRequest(http.MethodPost, "/orgs/o1/environments/dev/agent-identities/groups", body))
			assert.Equal(t, http.StatusBadRequest, w.Code, body)
		}
		assert.Empty(t, provider.writes)
	})

	t.Run("update keeps an omitted name", func(t *testing.T) {
		provider := &fakeGroupProvider{group: group}
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).UpdateGroup(w, groupReq(http.MethodPut, `{"description":"new"}`))
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, provider.writes, 1)
		assert.Nil(t, provider.writes[0].Name)
		assert.Equal(t, "grp-1", provider.writes[0].GroupID)
	})

	t.Run("delete", func(t *testing.T) {
		provider := &fakeGroupProvider{group: group}
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).DeleteGroup(w, groupReq(http.MethodDelete, ""))
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "grp-1", provider.ref.GroupID)
	})

	t.Run("members and the roles a group holds", func(t *testing.T) {
		provider := &fakeGroupProvider{
			group:   group,
			members: services.IdentityGroupMembers{Members: []thundersvc.GroupMember{{ID: "a1", Type: "agent"}}, Total: 1},
			roles:   []thundersvc.ThunderRole{{ID: "r1", Name: "readers"}},
		}
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, provider)
		w := httptest.NewRecorder()
		envAgentController(t).GetGroupMembers(w, groupReq(http.MethodGet, ""))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"a1"`)
		w = httptest.NewRecorder()
		envAgentController(t).GetGroupRoles(w, groupReq(http.MethodGet, ""))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "readers")
	})

	t.Run("only agents of the environment can be added, any can be removed", func(t *testing.T) {
		provider := &fakeGroupProvider{group: group}
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, provider)

		w := httptest.NewRecorder()
		envAgentController(t, "a1").AddGroupMembers(w, groupReq(http.MethodPost, `{"agentIds":["a1"]}`))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Len(t, provider.memberWrites, 1)
		assert.Equal(t, []thundersvc.GroupMember{{ID: "a1", Type: "agent"}}, provider.memberWrites[0].Members)

		w = httptest.NewRecorder()
		envAgentController(t, "a1").AddGroupMembers(w, groupReq(http.MethodPost, `{"agentIds":["a1","stranger"]}`))
		assert.Equal(t, http.StatusBadRequest, w.Code, "one agent outside the environment refuses the whole request")
		assert.Len(t, provider.memberWrites, 1)

		w = httptest.NewRecorder()
		envAgentController(t).RemoveGroupMembers(w, groupReq(http.MethodPost, `{"agentIds":["stranger"]}`))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Len(t, provider.memberWrites, 2)

		w = httptest.NewRecorder()
		envAgentController(t, "a1").AddGroupMembers(w, groupReq(http.MethodPost, `{"agentIds":[]}`))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestGroupsViaControlPlaneFailClosed(t *testing.T) {
	operations := map[string]func(AgentIdentityController, http.ResponseWriter){
		"list": func(c AgentIdentityController, w http.ResponseWriter) { c.ListGroups(w, groupReq(http.MethodGet, "")) },
		"get":  func(c AgentIdentityController, w http.ResponseWriter) { c.GetGroup(w, groupReq(http.MethodGet, "")) },
		"create": func(c AgentIdentityController, w http.ResponseWriter) {
			c.CreateGroup(w, groupReq(http.MethodPost, `{"name":"g"}`))
		},
		"update": func(c AgentIdentityController, w http.ResponseWriter) {
			c.UpdateGroup(w, groupReq(http.MethodPut, `{"name":"g"}`))
		},
		"delete": func(c AgentIdentityController, w http.ResponseWriter) {
			c.DeleteGroup(w, groupReq(http.MethodDelete, ""))
		},
		"members": func(c AgentIdentityController, w http.ResponseWriter) {
			c.GetGroupMembers(w, groupReq(http.MethodGet, ""))
		},
		"add": func(c AgentIdentityController, w http.ResponseWriter) {
			c.AddGroupMembers(w, groupReq(http.MethodPost, `{"agentIds":["a"]}`))
		},
		"remove": func(c AgentIdentityController, w http.ResponseWriter) {
			c.RemoveGroupMembers(w, groupReq(http.MethodPost, `{"agentIds":["a"]}`))
		},
		"roles": func(c AgentIdentityController, w http.ResponseWriter) {
			c.GetGroupRoles(w, groupReq(http.MethodGet, ""))
		},
	}

	t.Run("without a provider, 503 and the environment's own Thunder is never used", func(t *testing.T) {
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, nil)
		for name, call := range operations {
			w := httptest.NewRecorder()
			call(envAgentController(t, "a"), w)
			assert.Equal(t, http.StatusServiceUnavailable, w.Code, name)
		}
	})

	t.Run("a provider that cannot hold groups gives 501", func(t *testing.T) {
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, &fakeGroupProvider{err: services.ErrIdentityGroupUnsupported})
		for name, call := range operations {
			w := httptest.NewRecorder()
			call(envAgentController(t, "a"), w)
			assert.Equal(t, http.StatusNotImplemented, w.Code, name)
			assert.NotContains(t, strings.ToLower(w.Body.String()), "panic", name)
		}
	})

	t.Run("an unknown group is a 404", func(t *testing.T) {
		useGroupMode(t, thundersvc.IdentityManagementCPManaged, &fakeGroupProvider{err: services.ErrIdentityGroupNotFound})
		w := httptest.NewRecorder()
		envAgentController(t).GetGroup(w, groupReq(http.MethodGet, ""))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestGroupsLegacyModeIgnoresAProvider(t *testing.T) {
	provider := &fakeGroupProvider{}
	useGroupMode(t, thundersvc.IdentityManagementLegacyDirect, provider)
	listed := false
	envClient := &clientmocks.EnvIdentityClientMock{
		GetDefaultOUIDFunc: func(context.Context) (string, error) { return "ou-env", nil },
		ListGroupsByOUIdFunc: func(context.Context, string, int, int) ([]thundersvc.ThunderGroup, int, error) {
			listed = true
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
	ctrl.ListGroups(w, cpRoleRequest(http.MethodGet, "/orgs/o1/environments/dev/agent-identities/groups", ""))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, listed, "the environment's own Thunder, as before")
	assert.Empty(t, provider.calls)
}
