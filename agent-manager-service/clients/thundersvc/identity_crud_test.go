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
package thundersvc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIdentity is a ThunderID stand-in keyed by "METHOD /path". It records each
// request's JSON body and serves a canned status and body; unknown routes 500 so
// a test fails loudly when the client calls something unexpected.
type fakeIdentity struct {
	t      *testing.T
	mu     sync.Mutex
	routes map[string]fakeReply
	bodies map[string][]map[string]any
	auth   []string
}

type fakeReply struct {
	status int
	body   any
}

func newFakeIdentity(t *testing.T, routes map[string]fakeReply) (*fakeIdentity, IdentityClient) {
	t.Helper()
	f := &fakeIdentity{t: t, routes: routes, bodies: map[string][]map[string]any{}}
	srv := newTestThunderServer(t, f.serve)
	t.Cleanup(srv.Close)
	return f, NewIdentityClient(srv.URL, "sys-client", "sys-secret")
}

func (f *fakeIdentity) serve(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + r.URL.Path
	f.mu.Lock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		var m map[string]any
		require.NoError(f.t, json.Unmarshal(raw, &m))
		f.bodies[key] = append(f.bodies[key], m)
	}
	reply, ok := f.routes[key]
	f.mu.Unlock()
	if !ok {
		http.Error(w, "unexpected "+key, http.StatusInternalServerError)
		return
	}
	status := reply.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	switch b := reply.body.(type) {
	case nil:
	case string:
		_, _ = w.Write([]byte(b))
	default:
		_ = json.NewEncoder(w).Encode(b)
	}
}

func (f *fakeIdentity) lastBody(key string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	bs := f.bodies[key]
	require.NotEmpty(f.t, bs, "no request body recorded for %s", key)
	return bs[len(bs)-1]
}

func TestIdentityClientUsers(t *testing.T) {
	ctx := context.Background()
	f, c := newFakeIdentity(t, map[string]fakeReply{
		"GET /users":                        {body: map[string]any{"users": []map[string]any{{"id": "u1"}, {"id": "u2"}}, "totalResults": 7}},
		"GET /users/u1":                     {body: map[string]any{"id": "u1", "ouId": "ou"}},
		"GET /users/gone":                   {status: http.StatusNotFound, body: "user gone"},
		"POST /users":                       {status: http.StatusCreated, body: map[string]any{"id": "u3", "type": "person"}},
		"PUT /users/u1":                     {body: map[string]any{"id": "u1", "display": "New"}},
		"POST /users/u1/update-credentials": {status: http.StatusNoContent},
		"DELETE /users/u1":                  {status: http.StatusNoContent},
		"GET /organization-units/ou/users":  {body: map[string]any{"users": []map[string]any{{"id": "u1"}}, "totalResults": 1}},
	})

	users, total, err := c.ListUsers(ctx, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, 7, total)
	assert.Len(t, users, 2)

	u, err := c.GetUser(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, "ou", u.OuID)

	_, err = c.GetUser(ctx, "gone")
	require.True(t, IsNotFound(err), "404 must stay detectable: %v", err)
	assert.Contains(t, err.Error(), "user gone")

	created, err := c.CreateUser(ctx, CreateUserRequest{OuID: "ou", Type: "person", Attributes: map[string]string{"email": "a@b.c"}, Password: "pw"})
	require.NoError(t, err)
	assert.Equal(t, "u3", created.ID)
	body := f.lastBody("POST /users")
	assert.Equal(t, "ou", body["ouId"])
	assert.NotContains(t, body, "Password", "the password is never sent in the create body")
	assert.NotContains(t, body, "password")

	updated, err := c.UpdateUser(ctx, "u1", UpdateUserRequest{Attributes: map[string]string{"name": "New"}})
	require.NoError(t, err)
	assert.Equal(t, "New", updated.Display)

	require.NoError(t, c.UpdateUserCredentials(ctx, "u1", "s3cret"))
	assert.Equal(t, map[string]any{"password": "s3cret"}, f.lastBody("POST /users/u1/update-credentials")["credentials"])

	require.NoError(t, c.DeleteUser(ctx, "u1"))

	ouUsers, n, err := c.ListUsersByOUId(ctx, "ou", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, "u1", ouUsers[0].ID)

	for _, a := range f.auth {
		assert.Equal(t, "Bearer test-system-token", a)
	}
}

func TestIdentityClientUserGroupsAcceptsBothShapes(t *testing.T) {
	ctx := context.Background()
	_, c := newFakeIdentity(t, map[string]fakeReply{
		"GET /users/arr/groups":   {body: []map[string]any{{"id": "g1", "name": "Devs"}}},
		"GET /users/obj/groups":   {body: map[string]any{"groups": []map[string]any{{"id": "g2"}}, "totalResults": 1}},
		"GET /users/empty/groups": {body: "  "},
		"GET /users/bad/groups":   {body: "[not json"},
	})

	groups, err := c.GetUserGroups(ctx, "arr")
	require.NoError(t, err)
	assert.Equal(t, "Devs", groups[0].Name)

	groups, err = c.GetUserGroups(ctx, "obj")
	require.NoError(t, err)
	assert.Equal(t, "g2", groups[0].ID)

	_, err = c.GetUserGroups(ctx, "empty")
	require.ErrorContains(t, err, "empty response")
	_, err = c.GetUserGroups(ctx, "bad")
	require.ErrorContains(t, err, "decode")
	_, err = c.GetUserGroups(ctx, "missing")
	require.ErrorContains(t, err, "HTTP 500")
}

func TestIdentityClientGroups(t *testing.T) {
	ctx := context.Background()
	f, c := newFakeIdentity(t, map[string]fakeReply{
		"GET /groups/g1":                 {body: map[string]any{"id": "g1", "name": "Devs"}},
		"POST /groups":                   {status: http.StatusCreated, body: map[string]any{"id": "g2", "name": "Ops"}},
		"PUT /groups/g1":                 {body: map[string]any{"id": "g1", "name": "Renamed"}},
		"DELETE /groups/g1":              {status: http.StatusNoContent},
		"POST /groups/g1/members/add":    {},
		"POST /groups/g1/members/remove": {},
		"GET /groups/g1/members": {body: map[string]any{"totalResults": 4, "members": []map[string]any{
			{"id": "u1", "type": "user"}, {"id": "agent-1", "type": "agent"}, {"id": "deleted", "type": "user"},
		}}},
		"GET /users/u1":      {body: map[string]any{"id": "u1"}},
		"GET /users/deleted": {status: http.StatusNotFound},
	})

	g, err := c.GetGroup(ctx, "g1")
	require.NoError(t, err)
	assert.Equal(t, "Devs", g.Name)

	g, err = c.CreateGroup(ctx, CreateGroupRequest{Name: "Ops", OuID: "ou"})
	require.NoError(t, err)
	assert.Equal(t, "g2", g.ID)
	assert.Equal(t, "ou", f.lastBody("POST /groups")["ouId"])

	g, err = c.UpdateGroup(ctx, "g1", UpdateGroupRequest{Name: "Renamed", OuID: "ou"})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", g.Name)

	require.NoError(t, c.DeleteGroup(ctx, "g1"))

	require.NoError(t, c.AddGroupMembers(ctx, "g1", []string{"u1", "u2"}))
	assert.Equal(t, []any{
		map[string]any{"id": "u1", "type": "user"}, map[string]any{"id": "u2", "type": "user"},
	}, f.lastBody("POST /groups/g1/members/add")["members"])
	require.NoError(t, c.RemoveGroupMembers(ctx, "g1", []string{"u2"}))
	assert.Equal(t, []any{map[string]any{"id": "u2", "type": "user"}}, f.lastBody("POST /groups/g1/members/remove")["members"])

	// Only user members are resolved; a user deleted since is skipped, not an error.
	members, total, err := c.GetGroupMembers(ctx, "g1", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 4, total)
	require.Len(t, members, 1)
	assert.Equal(t, "u1", members[0].ID)
}

func TestIdentityClientGroupMembersFailsOnUpstreamError(t *testing.T) {
	_, c := newFakeIdentity(t, map[string]fakeReply{
		"GET /groups/g1/members": {body: map[string]any{"members": []map[string]any{{"id": "broken", "type": "user"}}}},
	})
	_, _, err := c.GetGroupMembers(context.Background(), "g1", 0, 10)
	require.ErrorContains(t, err, "thunder get group member broken")
	assert.False(t, IsNotFound(err), "a 500 must not read as not-found")
}

func TestIdentityClientRoles(t *testing.T) {
	ctx := context.Background()
	role := map[string]any{"id": "r1", "ouId": "ou", "name": "dev", "description": "d", "permissions": []map[string]any{
		{"resourceServerId": "amp", "permissions": []string{"amp:a:read"}},
		{"resourceServerId": "other", "permissions": []string{"x"}},
	}}
	f, c := newFakeIdentity(t, map[string]fakeReply{
		"GET /roles/r1":                     {body: role},
		"POST /roles":                       {status: http.StatusCreated, body: map[string]any{"id": "r2", "name": "new"}},
		"PUT /roles/r1":                     {body: role},
		"DELETE /roles/r1":                  {status: http.StatusNoContent},
		"POST /roles/r1/assignments/add":    {},
		"POST /roles/r1/assignments/remove": {},
		"GET /roles/missing":                {status: http.StatusNotFound},
	})

	r, err := c.GetRole(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "dev", r.Name)

	r, err = c.CreateRole(ctx, CreateRoleRequest{Name: "new", OuID: "ou"})
	require.NoError(t, err)
	assert.Equal(t, "r2", r.ID)

	_, err = c.UpdateRole(ctx, "r1", UpdateRoleRequest{Name: "dev", OuID: "ou"})
	require.NoError(t, err)
	require.NoError(t, c.DeleteRole(ctx, "r1"))

	// Adding merges into the existing resource server entry without duplicates and keeps the role's identity.
	require.NoError(t, c.AddRolePermissions(ctx, "r1", RolePermissionRequest{ResourceServerID: "amp", Permissions: []string{"amp:a:read", "amp:a:write"}}))
	put := f.lastBody("PUT /roles/r1")
	assert.Equal(t, "dev", put["name"])
	assert.Equal(t, "ou", put["ouId"])
	assert.Equal(t, []any{
		map[string]any{"resourceServerId": "amp", "permissions": []any{"amp:a:read", "amp:a:write"}},
		map[string]any{"resourceServerId": "other", "permissions": []any{"x"}},
	}, put["permissions"])

	require.NoError(t, c.AddRolePermissions(ctx, "r1", RolePermissionRequest{ResourceServerID: "new-rs", Permissions: []string{"p"}}))
	assert.Len(t, f.lastBody("PUT /roles/r1")["permissions"], 3, "a new resource server is appended")

	// Removing the last permission of a resource server drops its entry; others stay.
	require.NoError(t, c.RemoveRolePermissions(ctx, "r1", RolePermissionRequest{ResourceServerID: "amp", Permissions: []string{"amp:a:read"}}))
	assert.Equal(t, []any{
		map[string]any{"resourceServerId": "other", "permissions": []any{"x"}},
	}, f.lastBody("PUT /roles/r1")["permissions"])

	err = c.AddRolePermissions(ctx, "missing", RolePermissionRequest{})
	require.True(t, IsNotFound(err))
	err = c.RemoveRolePermissions(ctx, "missing", RolePermissionRequest{})
	require.True(t, IsNotFound(err))

	assignees := RoleAssignmentsRequest{Assignments: []AssignmentEntry{{ID: "u1", Type: "user"}}}
	require.NoError(t, c.AddRoleAssignees(ctx, "r1", assignees))
	assert.Equal(t, []any{map[string]any{"id": "u1", "type": "user"}}, f.lastBody("POST /roles/r1/assignments/add")["assignments"])
	require.NoError(t, c.RemoveRoleAssignees(ctx, "r1", assignees))
}

func TestIdentityClientRoleAssignments(t *testing.T) {
	ctx := context.Background()
	_, c := newFakeIdentity(t, map[string]fakeReply{
		"GET /roles/r1/assignments": {body: map[string]any{"assignments": []map[string]any{
			{"id": "u1", "type": "user"},
			{"id": "u-gone", "type": "user"},
			{"id": "g1", "type": "group"},
			{"id": "g-gone", "type": "group"},
			{"id": "app1", "type": "app"},
		}}},
		"GET /users/u1":             {body: map[string]any{"id": "u1"}},
		"GET /users/u-gone":         {status: http.StatusNotFound},
		"GET /groups/g1":            {body: map[string]any{"id": "g1"}},
		"GET /groups/g-gone":        {status: http.StatusNotFound},
		"GET /roles/r2/assignments": {body: map[string]any{"assignments": []map[string]any{{"id": "u-err", "type": "user"}}}},
		"GET /roles/r3/assignments": {body: map[string]any{"assignments": []map[string]any{{"id": "g-err", "type": "group"}}}},
	})

	got, err := c.GetRoleAssignments(ctx, "r1")
	require.NoError(t, err)
	require.Len(t, got.Users, 1)
	assert.Equal(t, "u1", got.Users[0].ID)
	require.Len(t, got.Groups, 1)
	assert.Equal(t, "g1", got.Groups[0].ID)

	_, err = c.GetRoleAssignments(ctx, "r2")
	require.ErrorContains(t, err, "role assignment user u-err")
	_, err = c.GetRoleAssignments(ctx, "r3")
	require.ErrorContains(t, err, "role assignment group g-err")
	_, err = c.GetRoleAssignments(ctx, "nope")
	require.ErrorContains(t, err, "thunder get role assignments")
}

func TestIdentityClientRolesForAssignee(t *testing.T) {
	ctx := context.Background()
	_, c := newFakeIdentity(t, map[string]fakeReply{
		"GET /roles": {body: map[string]any{"totalResults": 3, "roles": []map[string]any{
			{"id": "r1", "name": "one"}, {"id": "r2", "name": "two"}, {"id": "r3", "name": "three"},
		}}},
		"GET /roles/r1/assignments": {body: map[string]any{"assignments": []map[string]any{{"id": "x", "type": "user"}, {"id": "x", "type": "group"}}}},
		"GET /roles/r2/assignments": {body: map[string]any{"assignments": []map[string]any{{"id": "x", "type": "agent"}}}},
		"GET /roles/r3/assignments": {body: map[string]any{"assignments": []map[string]any{}}},
	})

	names := func(roles []ThunderRole, err error) []string {
		require.NoError(t, err)
		out := []string{}
		for _, r := range roles {
			out = append(out, r.Name)
		}
		return out
	}
	assert.Equal(t, []string{"one"}, names(c.GetUserRoles(ctx, "x")))
	assert.Equal(t, []string{"one"}, names(c.GetGroupRoles(ctx, "x")))
	assert.Equal(t, []string{"two"}, names(c.GetAgentRoles(ctx, "x")))
	assert.Empty(t, names(c.GetUserRoles(ctx, "someone-else")))
}

func TestIdentityClientRolesForAssigneeFailsOnUpstreamError(t *testing.T) {
	_, c := newFakeIdentity(t, map[string]fakeReply{
		"GET /roles": {body: map[string]any{"totalResults": 1, "roles": []map[string]any{{"id": "r1"}}}},
	})
	_, err := c.GetUserRoles(context.Background(), "x")
	require.ErrorContains(t, err, "assignments for role r1")

	_, c = newFakeIdentity(t, map[string]fakeReply{"GET /roles": {status: http.StatusBadGateway, body: "down"}})
	_, err = c.GetGroupRoles(context.Background(), "x")
	require.ErrorContains(t, err, "HTTP 502: down")
}

func TestIdentityClientOrganizationUnits(t *testing.T) {
	ctx := context.Background()
	_, c := newFakeIdentity(t, map[string]fakeReply{
		"GET /organization-units/parent/ous": {body: map[string]any{"totalResults": 2, "organizationUnits": []map[string]any{{"id": "c1", "handle": "acme"}}}},
		"GET /organization-units/tree/acme":  {body: map[string]any{"id": "c1"}},
		"GET /organization-units/tree/noid":  {body: map[string]any{}},
		"GET /organization-units/tree/bad":   {body: "{"},
		"GET /organization-units/tree/gone":  {status: http.StatusNotFound},
	})

	ous, total, err := c.ListChildOUs(ctx, "parent", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, "acme", ous[0].Handle)

	id, err := c.GetOUIDByHandle(ctx, "acme")
	require.NoError(t, err)
	assert.Equal(t, "c1", id)

	_, err = c.GetOUIDByHandle(ctx, "noid")
	require.ErrorContains(t, err, "returned no id")
	_, err = c.GetOUIDByHandle(ctx, "bad")
	require.ErrorContains(t, err, "decode")
	_, err = c.GetOUIDByHandle(ctx, "gone")
	require.True(t, IsNotFound(err))
	_, err = c.GetOUHandleByID(ctx, "not-a-uuid")
	require.ErrorContains(t, err, "not a UUID")
}

func TestIdentityClientErrorMapping(t *testing.T) {
	ctx := context.Background()
	_, c := newFakeIdentity(t, map[string]fakeReply{
		"POST /groups":   {status: http.StatusConflict, body: "name taken"},
		"GET /groups/g1": {body: "not json"},
		"PUT /roles/r1":  {status: http.StatusBadRequest, body: "bad role"},
	})

	_, err := c.CreateGroup(ctx, CreateGroupRequest{Name: "dup"})
	require.True(t, IsConflict(err), "409 maps to ConflictError: %v", err)
	assert.False(t, IsNotFound(err))
	assert.Contains(t, err.Error(), "conflict: name taken")

	_, err = c.GetGroup(ctx, "g1")
	require.ErrorContains(t, err, "thunder get group decode")

	_, err = c.UpdateRole(ctx, "r1", UpdateRoleRequest{})
	require.True(t, IsBadRequest(err), "400 maps to BadRequestError: %v", err)
	require.ErrorContains(t, err, "bad request: bad role")
	assert.False(t, IsConflict(err))
	assert.False(t, IsNotFound(err))
	assert.Equal(t, "fallback", BadRequestDescription(err, "fallback"), "a body that is not Thunder's JSON has no description")
}

// A 400's JSON description is kept for the caller to show.
func TestBadRequestKeepsThunderDescription(t *testing.T) {
	err := error(newBadRequestError([]byte(`{"code":"USR-1019","message":"Invalid request","description":"attribute email is required"}`)))
	require.True(t, IsBadRequest(err))
	assert.Equal(t, "attribute email is required", BadRequestDescription(err, "fallback"))
	assert.False(t, IsBadRequest(errors.New("other")))
}

// Without a system token no identity call reaches ThunderID.
func TestIdentityClientTokenFailure(t *testing.T) {
	var apiCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
	})
	mux.HandleFunc("/", func(http.ResponseWriter, *http.Request) { apiCalls++ })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewIdentityClient(srv.URL, "sys-client", "wrong")
	ctx := context.Background()

	calls := map[string]func() error{
		"ListUsers":   func() error { _, _, err := c.ListUsers(ctx, 0, 1); return err },
		"GetUser":     func() error { _, err := c.GetUser(ctx, "u"); return err },
		"CreateUser":  func() error { _, err := c.CreateUser(ctx, CreateUserRequest{}); return err },
		"UpdateUser":  func() error { _, err := c.UpdateUser(ctx, "u", UpdateUserRequest{}); return err },
		"Creds":       func() error { return c.UpdateUserCredentials(ctx, "u", "p") },
		"DeleteUser":  func() error { return c.DeleteUser(ctx, "u") },
		"UserGroups":  func() error { _, err := c.GetUserGroups(ctx, "u"); return err },
		"GetGroup":    func() error { _, err := c.GetGroup(ctx, "g"); return err },
		"CreateGroup": func() error { _, err := c.CreateGroup(ctx, CreateGroupRequest{}); return err },
		"UpdateGroup": func() error { _, err := c.UpdateGroup(ctx, "g", UpdateGroupRequest{}); return err },
		"DeleteGroup": func() error { return c.DeleteGroup(ctx, "g") },
		"AddMembers":  func() error { return c.AddGroupMembers(ctx, "g", nil) },
		"RmMembers":   func() error { return c.RemoveGroupMembers(ctx, "g", nil) },
		"Members":     func() error { _, _, err := c.GetGroupMembers(ctx, "g", 0, 1); return err },
		"GetRole":     func() error { _, err := c.GetRole(ctx, "r"); return err },
		"CreateRole":  func() error { _, err := c.CreateRole(ctx, CreateRoleRequest{}); return err },
		"UpdateRole":  func() error { _, err := c.UpdateRole(ctx, "r", UpdateRoleRequest{}); return err },
		"DeleteRole":  func() error { return c.DeleteRole(ctx, "r") },
		"Assignments": func() error { _, err := c.GetRoleAssignments(ctx, "r"); return err },
		"AddAssign":   func() error { return c.AddRoleAssignees(ctx, "r", RoleAssignmentsRequest{}) },
		"RmAssign":    func() error { return c.RemoveRoleAssignees(ctx, "r", RoleAssignmentsRequest{}) },
		"ListRoles":   func() error { _, _, err := c.ListRoles(ctx, "", 0, 1); return err },
		"ChildOUs":    func() error { _, _, err := c.ListChildOUs(ctx, "p", 1, 0); return err },
		"OUByHandle":  func() error { _, err := c.GetOUIDByHandle(ctx, "h"); return err },
		"OUUsers":     func() error { _, _, err := c.ListUsersByOUId(ctx, "o", 0, 1); return err },
		"Invite":      func() error { _, err := c.InviteUser(ctx, "a@b.c", "o"); return err },
	}
	for name, call := range calls {
		require.Error(t, call(), name)
	}
	assert.Zero(t, apiCalls, "no API call is made without a token")
}
