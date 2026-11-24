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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func directoryFor(t *testing.T, c *clientmocks.EnvIdentityClientMock) identityv1.Client {
	t.Helper()
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, ouID, _, env string) (thundersvc.EnvIdentityClient, error) {
			assert.Equal(t, "ou-1", ouID)
			assert.Equal(t, "default", env)
			return c, nil
		},
	}
	dc, err := NewIdentityDirectory(resolver, nil).For(context.Background(), "ou-1", "default")
	require.NoError(t, err)
	return dc
}

func TestIdentityDirectoryForClassifiesResolveErrors(t *testing.T) {
	errDown := errors.New("thunder down")
	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"not provisioned": {err: thundersvc.ErrThunderNotProvisioned, want: identityv1.ErrNotProvisioned},
		"not found":       {err: &thundersvc.NotFoundError{Message: "x"}, want: identityv1.ErrNotFound},
		"other":           {err: errDown, want: errDown},
	} {
		t.Run(name, func(t *testing.T) {
			resolver := &clientmocks.EnvThunderResolverMock{
				ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
					return nil, tc.err
				},
			}
			_, err := NewIdentityDirectory(resolver, nil).For(context.Background(), "ou", "dev")
			require.ErrorIs(t, err, tc.want)
			require.ErrorIs(t, err, tc.err, "the original error stays reachable")
			assert.Equal(t, tc.err.Error(), err.Error(), "the client's message is kept")
			if name == "other" {
				assert.NotErrorIs(t, err, identityv1.ErrNotFound, "an outage is not reported as not-found")
			}
		})
	}
}

func TestIdentityDirectoryListProvisionedAgents(t *testing.T) {
	bindings := &repomocks.AgentThunderClientRepositoryMock{
		FindByOuAndEnvironmentFunc: func(_ context.Context, ouID, env string) ([]models.AgentThunderClient, error) {
			assert.Equal(t, "ou", ouID)
			assert.Equal(t, "dev", env)
			return []models.AgentThunderClient{
				{ProjectName: "p", AgentName: "a", Status: models.AgentThunderStatus("ready"), ThunderAgentID: "tid"},
			}, nil
		},
	}
	got, err := NewIdentityDirectory(nil, bindings).ListProvisionedAgents(context.Background(), "ou", "dev")
	require.NoError(t, err)
	assert.Equal(t, []identityv1.ProvisionedAgent{{ProjectName: "p", AgentName: "a", Status: "ready", AgentID: "tid"}}, got)

	errDB := errors.New("db down")
	bindings.FindByOuAndEnvironmentFunc = func(context.Context, string, string) ([]models.AgentThunderClient, error) {
		return nil, errDB
	}
	_, err = NewIdentityDirectory(nil, bindings).ListProvisionedAgents(context.Background(), "ou", "dev")
	require.ErrorIs(t, err, errDB)
}

func TestIdentityDirectoryGroups(t *testing.T) {
	ctx := context.Background()
	notFound := &thundersvc.NotFoundError{Message: "gone"}
	var gotMembers []thundersvc.GroupMember
	c := &clientmocks.EnvIdentityClientMock{
		GetDefaultOUIDFunc: func(context.Context) (string, error) { return "ou-default", nil },
		ListGroupsByOUIdFunc: func(_ context.Context, ouID string, offset, limit int) ([]thundersvc.ThunderGroup, int, error) {
			assert.Equal(t, []any{"ou", 5, 10}, []any{ouID, offset, limit})
			return []thundersvc.ThunderGroup{{ID: "g1", OuID: "ou", Name: "Devs", Description: "d", CreatedAt: "c", UpdatedAt: "u"}}, 11, nil
		},
		GetGroupFunc: func(_ context.Context, id string) (*thundersvc.ThunderGroup, error) {
			if id == "gone" {
				return nil, notFound
			}
			return &thundersvc.ThunderGroup{ID: id}, nil
		},
		CreateGroupFunc: func(_ context.Context, req thundersvc.CreateGroupRequest) (*thundersvc.ThunderGroup, error) {
			return &thundersvc.ThunderGroup{ID: "new", Name: req.Name, OuID: req.OuID, Description: req.Description}, nil
		},
		UpdateGroupFunc: func(_ context.Context, id string, req thundersvc.UpdateGroupRequest) (*thundersvc.ThunderGroup, error) {
			return nil, &thundersvc.ConflictError{Message: "name taken"}
		},
		DeleteGroupFunc: func(context.Context, string) error { return notFound },
		ListGroupMemberEntriesFunc: func(context.Context, string, int, int) ([]thundersvc.GroupMember, int, error) {
			return []thundersvc.GroupMember{{ID: "a1", Type: "agent"}}, 1, nil
		},
		AddGroupMemberEntriesFunc: func(_ context.Context, _ string, ms []thundersvc.GroupMember) error {
			gotMembers = ms
			return nil
		},
		RemoveGroupMemberEntriesFunc: func(context.Context, string, []thundersvc.GroupMember) error { return nil },
		GetGroupRolesFunc: func(context.Context, string) ([]thundersvc.ThunderRole, error) {
			return []thundersvc.ThunderRole{{ID: "r1"}}, nil
		},
	}
	dc := directoryFor(t, c)

	ou, err := dc.DefaultOUID(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ou-default", ou)

	groups, total, err := dc.ListGroups(ctx, "ou", 5, 10)
	require.NoError(t, err)
	assert.Equal(t, 11, total)
	assert.Equal(t, []identityv1.Group{{ID: "g1", OUID: "ou", Name: "Devs", Description: "d", CreatedAt: "c", UpdatedAt: "u"}}, groups)

	_, err = dc.GetGroup(ctx, "gone")
	require.ErrorIs(t, err, identityv1.ErrNotFound)

	g, err := dc.CreateGroup(ctx, identityv1.GroupInput{Name: "Ops", OUID: "ou", Description: "x"})
	require.NoError(t, err)
	assert.Equal(t, identityv1.Group{ID: "new", Name: "Ops", OUID: "ou", Description: "x"}, g)

	_, err = dc.UpdateGroup(ctx, "g1", identityv1.GroupInput{Name: "dup"})
	require.Error(t, err)
	assert.True(t, thundersvc.IsConflict(err), "a conflict passes through unclassified")
	assert.NotErrorIs(t, err, identityv1.ErrNotFound)

	require.ErrorIs(t, dc.DeleteGroup(ctx, "g1"), identityv1.ErrNotFound)

	members, n, err := dc.ListGroupMembers(ctx, "g1", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, []identityv1.Member{{ID: "a1", Type: "agent"}}, members)

	require.NoError(t, dc.AddGroupMembers(ctx, "g1", []identityv1.Member{{ID: "a2", Type: "agent"}}))
	assert.Equal(t, []thundersvc.GroupMember{{ID: "a2", Type: "agent"}}, gotMembers, "member type is kept, not forced to user")
	require.NoError(t, dc.RemoveGroupMembers(ctx, "g1", nil))

	roles, err := dc.GetGroupRoles(ctx, "g1")
	require.NoError(t, err)
	assert.Equal(t, "r1", roles[0].ID)
}

func TestIdentityDirectoryRoles(t *testing.T) {
	ctx := context.Background()
	errDown := errors.New("down")
	var gotUpdate thundersvc.UpdateRoleRequest
	var gotPerm thundersvc.RolePermissionRequest
	var gotAssign thundersvc.RoleAssignmentsRequest
	role := thundersvc.ThunderRole{
		ID: "r1", OuID: "ou", Name: "dev", IsReadOnly: true,
		Permissions: []thundersvc.RolePermissionRequest{{ResourceServerID: "rs", Permissions: []string{"p1"}}},
	}
	c := &clientmocks.EnvIdentityClientMock{
		ListRolesFunc: func(context.Context, string, int, int) ([]thundersvc.ThunderRole, int, error) {
			return []thundersvc.ThunderRole{role}, 1, nil
		},
		GetRoleFunc: func(context.Context, string) (*thundersvc.ThunderRole, error) { return &role, nil },
		CreateRoleFunc: func(_ context.Context, req thundersvc.CreateRoleRequest) (*thundersvc.ThunderRole, error) {
			return &thundersvc.ThunderRole{ID: "r2", Name: req.Name}, nil
		},
		UpdateRoleFunc: func(_ context.Context, _ string, req thundersvc.UpdateRoleRequest) (*thundersvc.ThunderRole, error) {
			gotUpdate = req
			return &role, nil
		},
		DeleteRoleFunc: func(context.Context, string) error { return errDown },
		AddRolePermissionsFunc: func(_ context.Context, _ string, p thundersvc.RolePermissionRequest) error {
			gotPerm = p
			return nil
		},
		RemoveRolePermissionsFunc: func(context.Context, string, thundersvc.RolePermissionRequest) error { return nil },
		GetAgentRoleAssignmentsFunc: func(context.Context, string) (*thundersvc.AgentRoleAssignments, error) {
			return &thundersvc.AgentRoleAssignments{
				Agents: []thundersvc.AssignmentEntry{{ID: "a1", Type: "agent"}},
				Groups: []thundersvc.ThunderGroup{{ID: "g1"}},
			}, nil
		},
		AddRoleAssigneesFunc: func(_ context.Context, _ string, req thundersvc.RoleAssignmentsRequest) error {
			gotAssign = req
			return nil
		},
		RemoveRoleAssigneesFunc: func(context.Context, string, thundersvc.RoleAssignmentsRequest) error {
			return &thundersvc.NotFoundError{}
		},
	}
	dc := directoryFor(t, c)

	roles, total, err := dc.ListRoles(ctx, "ou", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	want := identityv1.Role{
		ID: "r1", OUID: "ou", Name: "dev", IsReadOnly: true,
		Permissions: []identityv1.RolePermission{{ResourceServerID: "rs", Permissions: []string{"p1"}}},
	}
	assert.Equal(t, []identityv1.Role{want}, roles)

	got, err := dc.GetRole(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	created, err := dc.CreateRole(ctx, identityv1.RoleInput{Name: "new"})
	require.NoError(t, err)
	assert.Equal(t, "r2", created.ID)
	assert.Empty(t, created.Permissions)

	_, err = dc.UpdateRole(ctx, "r1", identityv1.RoleInput{Name: "dev", Permissions: []identityv1.RolePermission{{ResourceServerID: "rs", Permissions: []string{"p2"}}}})
	require.NoError(t, err)
	assert.Equal(t, []thundersvc.RolePermissionRequest{{ResourceServerID: "rs", Permissions: []string{"p2"}}}, gotUpdate.Permissions)
	_, err = dc.UpdateRole(ctx, "r1", identityv1.RoleInput{Name: "dev"})
	require.NoError(t, err)
	assert.Nil(t, gotUpdate.Permissions, "no permissions in the input leaves the role's permissions untouched")

	err = dc.DeleteRole(ctx, "r1")
	require.ErrorIs(t, err, errDown)
	assert.NotErrorIs(t, err, identityv1.ErrNotFound)

	require.NoError(t, dc.AddRolePermissions(ctx, "r1", identityv1.RolePermission{ResourceServerID: "rs", Permissions: []string{"p3"}}))
	assert.Equal(t, thundersvc.RolePermissionRequest{ResourceServerID: "rs", Permissions: []string{"p3"}}, gotPerm)
	require.NoError(t, dc.RemoveRolePermissions(ctx, "r1", identityv1.RolePermission{}))

	a, err := dc.GetRoleAssignments(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, []identityv1.Member{{ID: "a1", Type: "agent"}}, a.Agents)
	assert.Equal(t, "g1", a.Groups[0].ID)

	require.NoError(t, dc.AddRoleAssignees(ctx, "r1", []identityv1.Member{{ID: "g1", Type: "group"}}))
	assert.Equal(t, []thundersvc.AssignmentEntry{{ID: "g1", Type: "group"}}, gotAssign.Assignments)
	require.ErrorIs(t, dc.RemoveRoleAssignees(ctx, "r1", nil), identityv1.ErrNotFound)
}

func TestIdentityDirectoryErrorsOnEveryCall(t *testing.T) {
	ctx := context.Background()
	nf := &thundersvc.NotFoundError{}
	c := &clientmocks.EnvIdentityClientMock{
		GetDefaultOUIDFunc:   func(context.Context) (string, error) { return "", nf },
		ListGroupsByOUIdFunc: func(context.Context, string, int, int) ([]thundersvc.ThunderGroup, int, error) { return nil, 0, nf },
		CreateGroupFunc:      func(context.Context, thundersvc.CreateGroupRequest) (*thundersvc.ThunderGroup, error) { return nil, nf },
		ListGroupMemberEntriesFunc: func(context.Context, string, int, int) ([]thundersvc.GroupMember, int, error) {
			return nil, 0, nf
		},
		GetGroupRolesFunc: func(context.Context, string) ([]thundersvc.ThunderRole, error) { return nil, nf },
		ListRolesFunc:     func(context.Context, string, int, int) ([]thundersvc.ThunderRole, int, error) { return nil, 0, nf },
		GetRoleFunc:       func(context.Context, string) (*thundersvc.ThunderRole, error) { return nil, nf },
		CreateRoleFunc:    func(context.Context, thundersvc.CreateRoleRequest) (*thundersvc.ThunderRole, error) { return nil, nf },
		UpdateRoleFunc: func(context.Context, string, thundersvc.UpdateRoleRequest) (*thundersvc.ThunderRole, error) {
			return nil, nf
		},
		GetAgentRoleAssignmentsFunc: func(context.Context, string) (*thundersvc.AgentRoleAssignments, error) { return nil, nf },
		EnsureProxyResourceServerFunc: func(_ context.Context, handle, _, _ string, _ []string) (string, error) {
			return "rs-" + handle, nil
		},
		DeleteProxyResourceServerActionFunc: func(context.Context, string, string) (string, error) { return "", nf },
		DeleteProxyResourceServerFunc:       func(context.Context, string) error { return nil },
	}
	dc := directoryFor(t, c)

	_, err := dc.DefaultOUID(ctx)
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	_, _, err = dc.ListGroups(ctx, "ou", 0, 1)
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	_, err = dc.CreateGroup(ctx, identityv1.GroupInput{})
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	_, _, err = dc.ListGroupMembers(ctx, "g", 0, 1)
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	_, err = dc.GetGroupRoles(ctx, "g")
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	_, _, err = dc.ListRoles(ctx, "ou", 0, 1)
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	_, err = dc.GetRole(ctx, "r")
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	_, err = dc.CreateRole(ctx, identityv1.RoleInput{})
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	_, err = dc.UpdateRole(ctx, "r", identityv1.RoleInput{})
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	_, err = dc.GetRoleAssignments(ctx, "r")
	require.ErrorIs(t, err, identityv1.ErrNotFound)

	id, err := dc.EnsureResourceServer(ctx, "proxy", "Proxy", "https://x", []string{"invoke"})
	require.NoError(t, err)
	assert.Equal(t, "rs-proxy", id)
	_, err = dc.RemoveResourceServerAction(ctx, "proxy", "invoke")
	require.ErrorIs(t, err, identityv1.ErrNotFound)
	require.NoError(t, dc.DeleteResourceServer(ctx, "proxy"))
}
