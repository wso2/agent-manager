//go:build integration

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

package gwctl_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1/identitymocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// agIdentityOU is the organization unit the environment's directory reports.
const agIdentityOU = "ou-env"

// agUnknownMember prefixes a member ID the in-memory directory does not know,
// so it refuses to add it the way the identity provider does.
const agUnknownMember = "ghost-"

// agDirectory is an in-memory identity directory for one environment: the
// identity provider the agent-identity routes front. Everything else on the
// routes (MCP proxies, scopes, gateways) is real.
type agDirectory struct {
	mu        sync.Mutex
	groups    map[string]identityv1.Group
	roles     map[string]identityv1.Role
	members   map[string][]identityv1.Member // by group id
	assignees map[string][]identityv1.Member // by role id
	// servers maps a resource server's handle to its identifier and actions.
	servers map[string]agResourceServer
}

type agResourceServer struct {
	id, identifier string
	actions        []string
}

// agUseDirectory makes the gateway's identity directory an in-memory one for
// the rest of the test; the next test's server restores the real one.
func agUseDirectory(t *testing.T) *agDirectory {
	t.Helper()
	d := &agDirectory{
		groups: map[string]identityv1.Group{}, roles: map[string]identityv1.Role{},
		members: map[string][]identityv1.Member{}, assignees: map[string][]identityv1.Member{},
		servers: map[string]agResourceServer{},
	}
	dir := &identitymocks.DirectoryMock{
		ForFunc: func(context.Context, string, string) (identityv1.Client, error) { return d.client(), nil },
		ListProvisionedAgentsFunc: func(context.Context, string, string) ([]identityv1.ProvisionedAgent, error) {
			return []identityv1.ProvisionedAgent{
				{ProjectName: agProject, AgentName: agAgent, Status: "completed", AgentID: "thunder-agent-1"},
			}, nil
		},
	}
	aigateway.Current().IdentityDirectory.Use(func() (identityv1.Directory, bool) { return dir, true })
	return d
}

func (d *agDirectory) group(id string) (identityv1.Group, error) {
	g, ok := d.groups[id]
	if !ok {
		return identityv1.Group{}, identityv1.ErrNotFound
	}
	return g, nil
}

func (d *agDirectory) role(id string) (identityv1.Role, error) {
	r, ok := d.roles[id]
	if !ok {
		return identityv1.Role{}, identityv1.ErrNotFound
	}
	return r, nil
}

// agWithout is members less every entry of drop.
func agWithout(members, drop []identityv1.Member) []identityv1.Member {
	return slices.DeleteFunc(slices.Clone(members), func(m identityv1.Member) bool { return slices.Contains(drop, m) })
}

func (d *agDirectory) client() identityv1.Client {
	lock := func() func() { d.mu.Lock(); return d.mu.Unlock }
	return &identitymocks.ClientMock{
		DefaultOUIDFunc: func(context.Context) (string, error) { return agIdentityOU, nil },
		ListGroupsFunc: func(_ context.Context, _ string, _, _ int) ([]identityv1.Group, int, error) {
			defer lock()()
			out := make([]identityv1.Group, 0, len(d.groups))
			for _, g := range d.groups {
				out = append(out, g)
			}
			return out, len(out), nil
		},
		GetGroupFunc: func(_ context.Context, id string) (identityv1.Group, error) {
			defer lock()()
			return d.group(id)
		},
		CreateGroupFunc: func(_ context.Context, in identityv1.GroupInput) (identityv1.Group, error) {
			defer lock()()
			g := identityv1.Group{ID: uuid.NewString(), OUID: in.OUID, Name: in.Name, Description: in.Description}
			d.groups[g.ID] = g
			return g, nil
		},
		UpdateGroupFunc: func(_ context.Context, id string, in identityv1.GroupInput) (identityv1.Group, error) {
			defer lock()()
			if _, err := d.group(id); err != nil {
				return identityv1.Group{}, err
			}
			g := identityv1.Group{ID: id, OUID: in.OUID, Name: in.Name, Description: in.Description}
			d.groups[id] = g
			return g, nil
		},
		DeleteGroupFunc: func(_ context.Context, id string) error {
			defer lock()()
			delete(d.groups, id)
			return nil
		},
		ListGroupMembersFunc: func(_ context.Context, id string, _, _ int) ([]identityv1.Member, int, error) {
			defer lock()()
			return slices.Clone(d.members[id]), len(d.members[id]), nil
		},
		AddGroupMembersFunc: func(_ context.Context, id string, members []identityv1.Member) error {
			defer lock()()
			for _, m := range members {
				if strings.HasPrefix(m.ID, agUnknownMember) {
					return fmt.Errorf("member %s: %w", m.ID, identityv1.ErrInvalid)
				}
			}
			d.members[id] = append(d.members[id], members...)
			return nil
		},
		RemoveGroupMembersFunc: func(_ context.Context, id string, members []identityv1.Member) error {
			defer lock()()
			d.members[id] = agWithout(d.members[id], members)
			return nil
		},
		GetGroupRolesFunc: func(_ context.Context, id string) ([]identityv1.Role, error) {
			defer lock()()
			if _, err := d.group(id); err != nil {
				return nil, err
			}
			var out []identityv1.Role
			for roleID, as := range d.assignees {
				if slices.Contains(as, identityv1.Member{ID: id, Type: "group"}) {
					out = append(out, d.roles[roleID])
				}
			}
			return out, nil
		},
		ListRolesFunc: func(_ context.Context, _ string, _, _ int) ([]identityv1.Role, int, error) {
			defer lock()()
			out := make([]identityv1.Role, 0, len(d.roles))
			for _, r := range d.roles {
				out = append(out, r)
			}
			return out, len(out), nil
		},
		GetRoleFunc: func(_ context.Context, id string) (identityv1.Role, error) {
			defer lock()()
			return d.role(id)
		},
		CreateRoleFunc: func(_ context.Context, in identityv1.RoleInput) (identityv1.Role, error) {
			defer lock()()
			r := identityv1.Role{ID: uuid.NewString(), OUID: in.OUID, Name: in.Name, Description: in.Description}
			d.roles[r.ID] = r
			return r, nil
		},
		UpdateRoleFunc: func(_ context.Context, id string, in identityv1.RoleInput) (identityv1.Role, error) {
			defer lock()()
			if _, err := d.role(id); err != nil {
				return identityv1.Role{}, err
			}
			r := identityv1.Role{ID: id, OUID: in.OUID, Name: in.Name, Description: in.Description, Permissions: in.Permissions}
			d.roles[id] = r
			return r, nil
		},
		DeleteRoleFunc: func(_ context.Context, id string) error {
			defer lock()()
			delete(d.roles, id)
			delete(d.assignees, id)
			return nil
		},
		AddRolePermissionsFunc: func(_ context.Context, id string, p identityv1.RolePermission) error {
			defer lock()()
			r := d.roles[id]
			r.Permissions = agMergePermissions(r.Permissions, p, true)
			d.roles[id] = r
			return nil
		},
		RemoveRolePermissionsFunc: func(_ context.Context, id string, p identityv1.RolePermission) error {
			defer lock()()
			r := d.roles[id]
			r.Permissions = agMergePermissions(r.Permissions, p, false)
			d.roles[id] = r
			return nil
		},
		GetRoleAssignmentsFunc: func(_ context.Context, id string) (identityv1.RoleAssignments, error) {
			defer lock()()
			if _, err := d.role(id); err != nil {
				return identityv1.RoleAssignments{}, err
			}
			var out identityv1.RoleAssignments
			for _, m := range d.assignees[id] {
				if m.Type == "group" {
					out.Groups = append(out.Groups, d.groups[m.ID])
				} else {
					out.Agents = append(out.Agents, m)
				}
			}
			return out, nil
		},
		AddRoleAssigneesFunc: func(_ context.Context, id string, as []identityv1.Member) error {
			defer lock()()
			d.assignees[id] = append(d.assignees[id], as...)
			return nil
		},
		RemoveRoleAssigneesFunc: func(_ context.Context, id string, as []identityv1.Member) error {
			defer lock()()
			d.assignees[id] = agWithout(d.assignees[id], as)
			return nil
		},
		EnsureResourceServerFunc: func(_ context.Context, handle, _, identifier string, actions []string) (string, error) {
			defer lock()()
			rs, ok := d.servers[handle]
			if !ok {
				rs = agResourceServer{id: "rs-" + handle}
			}
			rs.identifier = identifier
			for _, a := range actions {
				if !slices.Contains(rs.actions, a) {
					rs.actions = append(rs.actions, a)
				}
			}
			d.servers[handle] = rs
			return rs.id, nil
		},
		RemoveResourceServerActionFunc: func(_ context.Context, handle, action string) (string, error) {
			defer lock()()
			rs := d.servers[handle]
			rs.actions = slices.DeleteFunc(rs.actions, func(a string) bool { return a == action })
			d.servers[handle] = rs
			return rs.id, nil
		},
		DeleteResourceServerFunc: func(_ context.Context, handle string) error {
			defer lock()()
			delete(d.servers, handle)
			return nil
		},
	}
}

// agMergePermissions adds (or removes) p's permissions on its resource server.
func agMergePermissions(perms []identityv1.RolePermission, p identityv1.RolePermission, add bool) []identityv1.RolePermission {
	out := make([]identityv1.RolePermission, 0, len(perms)+1)
	found := false
	for _, cur := range perms {
		if cur.ResourceServerID == p.ResourceServerID {
			found = true
			if add {
				cur.Permissions = append(slices.Clone(cur.Permissions), p.Permissions...)
			} else {
				cur.Permissions = slices.DeleteFunc(slices.Clone(cur.Permissions), func(s string) bool { return slices.Contains(p.Permissions, s) })
			}
		}
		if len(cur.Permissions) > 0 {
			out = append(out, cur)
		}
	}
	if add && !found {
		out = append(out, p)
	}
	return out
}

// agServer is the resource server registered for handle.
func (d *agDirectory) server(handle string) (agResourceServer, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok := d.servers[handle]
	return rs, ok
}

const agIdentityBase = "/orgs/{org}/environments/default/agent-identities"

// TestAGRoutesAgentIdentityGroups creates a group, reads it back, renames it,
// adds and removes agents and deletes it.
func TestAGRoutesAgentIdentityGroups(t *testing.T) {
	a, _ := agSetup(t)
	agUseDirectory(t)
	groups := agIdentityBase + "/groups"

	created := expect(t, a.do(http.MethodPost, groups, map[string]any{"name": "readers", "description": "can read"}), http.StatusCreated)
	id, _ := created["id"].(string)
	require.NotEmpty(t, id)
	assert.Equal(t, "readers", created["name"])
	assert.Equal(t, agIdentityOU, created["ouId"], "the group is created in the environment's default unit")

	list := expect(t, a.do(http.MethodGet, groups+"?limit=500&offset=-2", nil), http.StatusOK)
	assert.EqualValues(t, 1, list["total"])
	assert.EqualValues(t, 20, list["limit"])
	assert.EqualValues(t, 0, list["offset"])
	require.Len(t, list["groups"], 1)

	updated := expect(t, a.do(http.MethodPut, groups+"/"+id, map[string]any{"description": "reads everything"}), http.StatusOK)
	assert.Equal(t, "readers", updated["name"], "an omitted name is kept")
	got := expect(t, a.do(http.MethodGet, groups+"/"+id, nil), http.StatusOK)
	assert.Equal(t, "reads everything", got["description"])

	expect(t, a.do(http.MethodPost, groups+"/"+id+"/members/add", map[string]any{"agentIds": []string{"agent-1", "agent-2"}}), http.StatusOK)
	members := expect(t, a.do(http.MethodGet, groups+"/"+id+"/members", nil), http.StatusOK)
	assert.EqualValues(t, 2, members["total"])
	assert.Equal(t, map[string]any{"id": "agent-1", "type": "agent"}, members["members"].([]any)[0])
	expect(t, a.do(http.MethodPost, groups+"/"+id+"/members/remove", map[string]any{"agentIds": []string{"agent-1"}}), http.StatusOK)
	members = expect(t, a.do(http.MethodGet, groups+"/"+id+"/members", nil), http.StatusOK)
	assert.EqualValues(t, 1, members["total"])

	roles := expect(t, a.do(http.MethodGet, groups+"/"+id+"/roles", nil), http.StatusOK)
	assert.Empty(t, roles["roles"])

	rec := a.do(http.MethodDelete, groups+"/"+id, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, groups+"/"+id, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "Group not found", errorMessage(t, rec))
}

// TestAGRoutesAgentIdentityGroupRejections pins the 400s, 404s and 403s of the
// group routes.
func TestAGRoutesAgentIdentityGroupRejections(t *testing.T) {
	a, _ := agSetup(t)
	agUseDirectory(t)
	groups := agIdentityBase + "/groups"
	ghost := groups + "/" + uuid.NewString()

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}{
		{"create malformed json", http.MethodPost, groups, "{", http.StatusBadRequest, "Invalid request body"},
		{"create without name", http.MethodPost, groups, map[string]any{"description": "d"}, http.StatusBadRequest, "name is required"},
		{"create reserved name", http.MethodPost, groups, map[string]any{"name": identityv1.AdministratorsGroupName}, http.StatusBadRequest, `"Administrators" is a reserved group name`},
		{"update malformed json", http.MethodPut, ghost, "{", http.StatusBadRequest, "Invalid request body"},
		{"add no members", http.MethodPost, ghost + "/members/add", map[string]any{"agentIds": []string{}}, http.StatusBadRequest, "agentIds must not be empty"},
		{"remove malformed json", http.MethodPost, ghost + "/members/remove", "{", http.StatusBadRequest, "Invalid request body"},
		{"remove no members", http.MethodPost, ghost + "/members/remove", map[string]any{}, http.StatusBadRequest, "agentIds must not be empty"},
		{"get unknown", http.MethodGet, ghost, nil, http.StatusNotFound, "Group not found"},
		{"update unknown", http.MethodPut, ghost, map[string]any{"name": "n"}, http.StatusNotFound, "Group not found"},
		{"delete unknown", http.MethodDelete, ghost, nil, http.StatusNotFound, "Group not found"},
		{"members of unknown", http.MethodGet, ghost + "/members", nil, http.StatusNotFound, "Group not found"},
		{"add to unknown", http.MethodPost, ghost + "/members/add", map[string]any{"agentIds": []string{"x"}}, http.StatusNotFound, "Group not found"},
		{"remove from unknown", http.MethodPost, ghost + "/members/remove", map[string]any{"agentIds": []string{"x"}}, http.StatusNotFound, "Group not found"},
		{"roles of unknown", http.MethodGet, ghost + "/roles", nil, http.StatusNotFound, "Group not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(tc.method, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, tc.message, errorMessage(t, rec))
		})
	}

	readOnly := withScopes(scope(gwperm.AgentIdentityRead))
	for _, r := range []struct{ method, path string }{
		{http.MethodPost, groups},
		{http.MethodPut, ghost},
		{http.MethodDelete, ghost},
		{http.MethodPost, ghost + "/members/add"},
	} {
		rec := a.do(r.method, r.path, map[string]any{"name": "n"}, readOnly)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
	}
	rec := a.do(http.MethodGet, groups, nil, withScopes(scope(gwperm.AgentIdentityCreate)))
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	list := expect(t, a.do(http.MethodGet, groups, nil), http.StatusOK)
	assert.EqualValues(t, 0, list["total"], "no refused request created a group")

	// An agent the directory does not know is the caller's mistake, not a 500.
	group := expect(t, a.do(http.MethodPost, groups, map[string]any{"name": "g-" + uuid.NewString()[:8]}), http.StatusCreated)
	rec = a.do(http.MethodPost, groups+"/"+group["id"].(string)+"/members/add",
		map[string]any{"agentIds": []string{agUnknownMember + uuid.NewString()}})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "agentIds must name agent identities in this environment", errorMessage(t, rec))
}

// TestAGRoutesAgentIdentityRoles creates a role granting a real MCP proxy's
// scope, which registers the proxy's resource server, then reconciles its
// scopes, assigns it and deletes it.
func TestAGRoutesAgentIdentityRoles(t *testing.T) {
	a, _ := agSetup(t)
	agCreateToolProxy(t, a, "secured", true, "get_forecast")
	for _, action := range []string{"read", "write"} {
		expect(t, a.do(http.MethodPost, "/orgs/{org}/mcp-proxies/secured/scopes", map[string]any{"action": action}), http.StatusCreated)
	}
	d := agUseDirectory(t)
	roles := agIdentityBase + "/roles"

	created := expect(t, a.do(http.MethodPost, roles,
		map[string]any{"name": "forecasters", "description": "reads", "scopes": []string{"secured:read"}}), http.StatusCreated)
	id := created["id"].(string)
	assert.Equal(t, agIdentityOU, created["ouId"])
	assert.Equal(t, []any{map[string]any{"resourceServerId": "rs-secured", "permissions": []any{"secured:read"}}},
		created["permissions"], "the response carries the permissions written after the create")
	rs, ok := d.server("secured")
	require.True(t, ok, "the proxy's resource server is registered before the role is")
	assert.Equal(t, "https://gw.example.com/secured/mcp", rs.identifier, "identified by the proxy's public URL")
	assert.Contains(t, rs.actions, "read")

	updated := expect(t, a.do(http.MethodPut, roles+"/"+id,
		map[string]any{"scopes": []string{"secured:write"}}), http.StatusOK)
	assert.Equal(t, "forecasters", updated["name"])
	assert.Equal(t, []any{map[string]any{"resourceServerId": "rs-secured", "permissions": []any{"secured:write"}}},
		updated["permissions"], "the scopes replace the role's permissions")
	meta := expect(t, a.do(http.MethodPut, roles+"/"+id, map[string]any{"description": "writes"}), http.StatusOK)
	assert.Equal(t, "writes", meta["description"])
	got := expect(t, a.do(http.MethodGet, roles+"/"+id, nil), http.StatusOK)
	assert.Equal(t, updated["permissions"], got["permissions"], "a metadata update keeps the permissions")
	list := expect(t, a.do(http.MethodGet, roles, nil), http.StatusOK)
	assert.EqualValues(t, 1, list["total"])

	group := expect(t, a.do(http.MethodPost, agIdentityBase+"/groups", map[string]any{"name": "team"}), http.StatusCreated)
	expect(t, a.do(http.MethodPost, roles+"/"+id+"/assignments/add", map[string]any{"assignments": []any{
		map[string]any{"type": "agent", "id": "thunder-agent-1"},
		map[string]any{"type": "group", "id": group["id"]},
	}}), http.StatusOK)
	assignments := expect(t, a.do(http.MethodGet, roles+"/"+id+"/assignments", nil), http.StatusOK)
	assert.Equal(t, []any{map[string]any{"id": "thunder-agent-1", "type": "agent"}}, assignments["agents"])
	require.Len(t, assignments["groups"], 1)
	groupRoles := expect(t, a.do(http.MethodGet, agIdentityBase+"/groups/"+group["id"].(string)+"/roles", nil), http.StatusOK)
	require.Len(t, groupRoles["roles"], 1)
	assert.Equal(t, "forecasters", groupRoles["roles"].([]any)[0].(map[string]any)["name"])

	expect(t, a.do(http.MethodPost, roles+"/"+id+"/assignments/remove", map[string]any{"assignments": []any{
		map[string]any{"type": "agent", "id": "thunder-agent-1"},
	}}), http.StatusOK)
	assignments = expect(t, a.do(http.MethodGet, roles+"/"+id+"/assignments", nil), http.StatusOK)
	assert.NotContains(t, assignments, "agents")

	rec := a.do(http.MethodDelete, roles+"/"+id, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, roles+"/"+id, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "Role not found", errorMessage(t, rec))

	agents := expect(t, a.do(http.MethodGet, agIdentityBase+"/agents", nil), http.StatusOK)
	assert.Equal(t, []any{map[string]any{
		"agentName": agAgent, "projectName": agProject, "status": "completed", "thunderAgentId": "thunder-agent-1",
	}}, agents["agents"])
}

// TestAGRoutesAgentIdentityRoleRejections pins the 400s, 404s and 403s of the
// role routes, including scopes the organization's MCP proxies do not define.
func TestAGRoutesAgentIdentityRoleRejections(t *testing.T) {
	a, _ := agSetup(t)
	agCreateToolProxy(t, a, "secured", true, "get_forecast")
	expect(t, a.do(http.MethodPost, "/orgs/{org}/mcp-proxies/secured/scopes", map[string]any{"action": "read"}), http.StatusCreated)
	// A proxy bound only to another environment is not deployed to this one.
	other := uuid.New()
	dbtest.EnvironmentWithID(t, a.db, a.org, other)
	expect(t, a.do(http.MethodPost, "/orgs/{org}/mcp-proxies", agMCPProxyBody("undeployed", other.String())), http.StatusCreated)
	expect(t, a.do(http.MethodPost, "/orgs/{org}/mcp-proxies/undeployed/scopes", map[string]any{"action": "read"}), http.StatusCreated)
	d := agUseDirectory(t)
	roles := agIdentityBase + "/roles"
	ghost := roles + "/" + uuid.NewString()

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}{
		{"create malformed json", http.MethodPost, roles, "{", http.StatusBadRequest, "Invalid request body"},
		{"create without name", http.MethodPost, roles, map[string]any{}, http.StatusBadRequest, "name is required"},
		{"create reserved name", http.MethodPost, roles, map[string]any{"name": identityv1.AdministratorRoleName}, http.StatusBadRequest, `"Administrator" is a reserved role name`},
		{"create malformed scope", http.MethodPost, roles, map[string]any{"name": "r", "scopes": []string{"secured"}}, http.StatusBadRequest, `scope "secured" is not of the form <proxy-handle>:<action>`},
		{"create unknown proxy", http.MethodPost, roles, map[string]any{"name": "r", "scopes": []string{"ghost:read"}}, http.StatusBadRequest, `scope "ghost:read" references unknown MCP proxy "ghost"`},
		{"create undefined action", http.MethodPost, roles, map[string]any{"name": "r", "scopes": []string{"secured:delete"}}, http.StatusBadRequest, `scope "secured:delete" is not defined on MCP proxy "secured"`},
		{"create proxy not deployed", http.MethodPost, roles, map[string]any{"name": "r", "scopes": []string{"undeployed:read"}}, http.StatusBadRequest, `MCP proxy "undeployed" is not deployed to environment "default"; deploy it there before granting its scopes`},
		{"update malformed json", http.MethodPut, ghost, "{", http.StatusBadRequest, "Invalid request body"},
		{"update reserved name", http.MethodPut, ghost, map[string]any{"name": identityv1.SystemClientRoleName}, http.StatusBadRequest, `"AMP System Client Thunder Admin" is a reserved role name`},
		{"update unknown", http.MethodPut, ghost, map[string]any{"description": "d"}, http.StatusNotFound, "Role not found"},
		{"get unknown", http.MethodGet, ghost, nil, http.StatusNotFound, "Role not found"},
		{"delete unknown", http.MethodDelete, ghost, nil, http.StatusNotFound, "Role not found"},
		{"assignments of unknown", http.MethodGet, ghost + "/assignments", nil, http.StatusNotFound, "Role not found"},
		{"assign malformed json", http.MethodPost, ghost + "/assignments/add", "{", http.StatusBadRequest, "invalid request body"},
		{"assign nothing", http.MethodPost, ghost + "/assignments/add", map[string]any{"assignments": []any{}}, http.StatusBadRequest, "assignments must not be empty"},
		{"assign a user", http.MethodPost, ghost + "/assignments/add", map[string]any{"assignments": []any{map[string]any{"type": "user", "id": "u"}}}, http.StatusBadRequest, `assignment type "user" is not allowed (must be agent or group)`},
		{"assign without id", http.MethodPost, ghost + "/assignments/remove", map[string]any{"assignments": []any{map[string]any{"type": "agent", "id": ""}}}, http.StatusBadRequest, "assignment id must not be empty"},
		{"assign to unknown", http.MethodPost, ghost + "/assignments/add", map[string]any{"assignments": []any{map[string]any{"type": "agent", "id": "x"}}}, http.StatusNotFound, "Role not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(tc.method, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, tc.message, errorMessage(t, rec))
		})
	}

	created := expect(t, a.do(http.MethodPost, roles, map[string]any{"name": "r"}), http.StatusCreated)
	rec := a.do(http.MethodPut, roles+"/"+created["id"].(string), map[string]any{"scopes": []string{"secured:delete"}})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, `scope "secured:delete" is not defined on MCP proxy "secured"`, errorMessage(t, rec))
	got := expect(t, a.do(http.MethodGet, roles+"/"+created["id"].(string), nil), http.StatusOK)
	assert.NotContains(t, got, "permissions", "the refused update granted nothing")
	_, registered := d.server("undeployed")
	assert.False(t, registered, "no resource server is registered for an undeployed proxy")

	readOnly := withScopes(scope(gwperm.AgentIdentityRead))
	for _, r := range []struct{ method, path string }{
		{http.MethodPost, roles},
		{http.MethodPut, ghost},
		{http.MethodDelete, ghost},
		{http.MethodPost, ghost + "/assignments/add"},
		{http.MethodPost, ghost + "/assignments/remove"},
	} {
		rec := a.do(r.method, r.path, map[string]any{"name": "n"}, readOnly)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
	}
	for _, path := range []string{roles, agIdentityBase + "/agents", agIdentityBase + "/scopes"} {
		rec := a.do(http.MethodGet, path, nil, withScopes(scope(gwperm.AgentIdentityUpdate)))
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s: %s", path, rec.Body.String())
	}
}

// TestAGRoutesAgentIdentityWithoutDirectory pins the 503 every directory-backed
// route answers while the environment has no identity provider.
func TestAGRoutesAgentIdentityWithoutDirectory(t *testing.T) {
	a, _ := agSetup(t)
	id := uuid.NewString()
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, agIdentityBase + "/groups", nil},
		{http.MethodPost, agIdentityBase + "/groups", map[string]any{"name": "g"}},
		{http.MethodGet, agIdentityBase + "/groups/" + id + "/members", nil},
		{http.MethodGet, agIdentityBase + "/roles", nil},
		{http.MethodPost, agIdentityBase + "/roles", map[string]any{"name": "r"}},
		{http.MethodGet, agIdentityBase + "/roles/" + id + "/assignments", nil},
		{http.MethodPost, agIdentityBase + "/roles/" + id + "/assignments/remove", map[string]any{"assignments": []any{map[string]any{"type": "agent", "id": "x"}}}},
	} {
		rec := a.do(r.method, r.path, r.body)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
		assert.Equal(t, "The environment's identity provider is not available; retry after it is provisioned", errorMessage(t, rec))
	}
}

// TestAGRoutesAgentIdentityDirectoryFailures pins the 502 when the directory
// cannot name its organization unit, and the 500s when its calls fail.
func TestAGRoutesAgentIdentityDirectoryFailures(t *testing.T) {
	down := errors.New("directory down")
	id := uuid.NewString()
	for _, tc := range []struct {
		name    string
		ouFails bool
		method  string
		path    string
		body    any
		status  int
		message string
	}{
		{"list groups ou", true, http.MethodGet, "/groups", nil, http.StatusBadGateway, "Failed to resolve environment identity provider OU"},
		{"create group ou", true, http.MethodPost, "/groups", map[string]any{"name": "g"}, http.StatusBadGateway, "Failed to resolve environment identity provider OU"},
		{"list roles ou", true, http.MethodGet, "/roles", nil, http.StatusBadGateway, "Failed to resolve environment identity provider OU"},
		{"create role ou", true, http.MethodPost, "/roles", map[string]any{"name": "r"}, http.StatusBadGateway, "Failed to resolve environment identity provider OU"},
		{"list groups", false, http.MethodGet, "/groups", nil, http.StatusInternalServerError, "Failed to list groups"},
		{"create group", false, http.MethodPost, "/groups", map[string]any{"name": "g"}, http.StatusInternalServerError, "Failed to create group"},
		{"get group", false, http.MethodGet, "/groups/" + id, nil, http.StatusInternalServerError, "Failed to get group"},
		{"list roles", false, http.MethodGet, "/roles", nil, http.StatusInternalServerError, "Failed to list roles"},
		{"create role", false, http.MethodPost, "/roles", map[string]any{"name": "r"}, http.StatusInternalServerError, "Failed to create role"},
		{"get role", false, http.MethodGet, "/roles/" + id, nil, http.StatusInternalServerError, "Failed to get role"},
		{"role assignments", false, http.MethodGet, "/roles/" + id + "/assignments", nil, http.StatusInternalServerError, "Failed to get role assignments"},
		{"unassign role", false, http.MethodPost, "/roles/" + id + "/assignments/remove", map[string]any{"assignments": []any{map[string]any{"type": "agent", "id": "x"}}}, http.StatusInternalServerError, "Failed to remove role assignees"},
		{"list agents", false, http.MethodGet, "/agents", nil, http.StatusInternalServerError, "Failed to list agent identity bindings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := agSetup(t)
			client := &identitymocks.ClientMock{
				DefaultOUIDFunc: func(context.Context) (string, error) {
					if tc.ouFails {
						return "", down
					}
					return agIdentityOU, nil
				},
				ListGroupsFunc: func(context.Context, string, int, int) ([]identityv1.Group, int, error) { return nil, 0, down },
				CreateGroupFunc: func(context.Context, identityv1.GroupInput) (identityv1.Group, error) {
					return identityv1.Group{}, down
				},
				GetGroupFunc:   func(context.Context, string) (identityv1.Group, error) { return identityv1.Group{}, down },
				ListRolesFunc:  func(context.Context, string, int, int) ([]identityv1.Role, int, error) { return nil, 0, down },
				CreateRoleFunc: func(context.Context, identityv1.RoleInput) (identityv1.Role, error) { return identityv1.Role{}, down },
				GetRoleFunc:    func(context.Context, string) (identityv1.Role, error) { return identityv1.Role{}, down },
				GetRoleAssignmentsFunc: func(context.Context, string) (identityv1.RoleAssignments, error) {
					return identityv1.RoleAssignments{}, down
				},
				RemoveRoleAssigneesFunc: func(context.Context, string, []identityv1.Member) error { return down },
			}
			dir := &identitymocks.DirectoryMock{
				ForFunc: func(context.Context, string, string) (identityv1.Client, error) { return client, nil },
				ListProvisionedAgentsFunc: func(context.Context, string, string) ([]identityv1.ProvisionedAgent, error) {
					return nil, down
				},
			}
			aigateway.Current().IdentityDirectory.Use(func() (identityv1.Directory, bool) { return dir, true })

			rec := a.do(tc.method, agIdentityBase+tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, tc.message, errorMessage(t, rec))
			assert.NotContains(t, rec.Body.String(), "directory down", "the directory's error stays in the log")
		})
	}
}
