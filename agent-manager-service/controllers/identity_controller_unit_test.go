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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	callerOU    = "ou-1"
	otherOU     = "ou-other"
	callerUser  = "user-1"
	thunderDown = "thunder unavailable"
)

var errThunder = errors.New(thunderDown)

func notFound() error { return &thundersvc.NotFoundError{Message: "gone"} }

// captureRecorder keeps every audit event; failSync makes the fail-closed
// intent record fail, the way an unreachable audit sink does.
type captureRecorder struct {
	mu       sync.Mutex
	events   []audit.Event
	failSync bool
}

func (c *captureRecorder) Record(_ context.Context, e audit.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *captureRecorder) RecordSync(_ context.Context, e audit.Event) error {
	if c.failSync {
		return errors.New("audit sink down")
	}
	c.Record(context.Background(), e)
	return nil
}

func (c *captureRecorder) Close(context.Context) error { return nil }

// completed returns the outcome records (not the intent records) for action.
func (c *captureRecorder) completed(action audit.Action) []audit.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []audit.Event
	for _, e := range c.events {
		if e.Action == action && e.Outcome != audit.OutcomeUnknown {
			out = append(out, e)
		}
	}
	return out
}

type idCall struct {
	method, url, body string
	params            map[string]string
	noOrg             bool
	sub               string
	rec               audit.Recorder
}

func serveIdentity(t *testing.T, client thundersvc.IdentityClient, h func(IdentityController) http.HandlerFunc, c idCall) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(c.method, c.url, strings.NewReader(c.body))
	for k, v := range c.params {
		req.SetPathValue(k, v)
	}
	ctx := req.Context()
	if !c.noOrg {
		ctx = middleware.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: callerOU, OuHandle: "acme"})
	}
	if c.rec == nil {
		c.rec = audit.NewNoopRecorder()
	}
	ctx = audit.WithRecorder(ctx, c.rec)
	if c.sub != "" {
		ctx = jwtassertion.ContextWithTokenClaims(ctx, &jwtassertion.TokenClaims{Sub: c.sub, OuId: callerOU})
	}
	w := httptest.NewRecorder()
	h(NewIdentityController(client))(w, req.WithContext(ctx))
	return w
}

func errMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body.Message
}

func decodeMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body
}

func userIn(ou string) *thundersvc.ThunderUser {
	return &thundersvc.ThunderUser{
		ID: callerUser, OuID: ou, Type: "Person",
		Attributes: map[string]any{"username": "alice", "email": "a@example.com", "nickname": "al"},
	}
}

func groupIn(ou, name string) *thundersvc.ThunderGroup {
	return &thundersvc.ThunderGroup{ID: "group-1", OuID: ou, Name: name, Description: "desc"}
}

func roleIn(ou, name string) *thundersvc.ThunderRole {
	return &thundersvc.ThunderRole{ID: "role-1", OuID: ou, Name: name}
}

// Every org-scoped identity handler refuses a request that reached it without
// a resolved org, before touching Thunder (an empty mock panics if it does).
func TestIdentityController_MissingOrgContextIsForbidden(t *testing.T) {
	handlers := map[string]func(IdentityController) http.HandlerFunc{
		"ListUsers":                func(c IdentityController) http.HandlerFunc { return c.ListUsers },
		"GetUser":                  func(c IdentityController) http.HandlerFunc { return c.GetUser },
		"CreateUser":               func(c IdentityController) http.HandlerFunc { return c.CreateUser },
		"UpdateUser":               func(c IdentityController) http.HandlerFunc { return c.UpdateUser },
		"DeleteUser":               func(c IdentityController) http.HandlerFunc { return c.DeleteUser },
		"GetUserGroups":            func(c IdentityController) http.HandlerFunc { return c.GetUserGroups },
		"GetUserRoles":             func(c IdentityController) http.HandlerFunc { return c.GetUserRoles },
		"InviteUser":               func(c IdentityController) http.HandlerFunc { return c.InviteUser },
		"ListGroups":               func(c IdentityController) http.HandlerFunc { return c.ListGroups },
		"GetGroup":                 func(c IdentityController) http.HandlerFunc { return c.GetGroup },
		"CreateGroup":              func(c IdentityController) http.HandlerFunc { return c.CreateGroup },
		"UpdateGroup":              func(c IdentityController) http.HandlerFunc { return c.UpdateGroup },
		"DeleteGroup":              func(c IdentityController) http.HandlerFunc { return c.DeleteGroup },
		"AddGroupMembers":          func(c IdentityController) http.HandlerFunc { return c.AddGroupMembers },
		"RemoveGroupMembers":       func(c IdentityController) http.HandlerFunc { return c.RemoveGroupMembers },
		"GetGroupMembers":          func(c IdentityController) http.HandlerFunc { return c.GetGroupMembers },
		"GetGroupRoles":            func(c IdentityController) http.HandlerFunc { return c.GetGroupRoles },
		"ListRoles":                func(c IdentityController) http.HandlerFunc { return c.ListRoles },
		"GetRole":                  func(c IdentityController) http.HandlerFunc { return c.GetRole },
		"CreateRole":               func(c IdentityController) http.HandlerFunc { return c.CreateRole },
		"UpdateRole":               func(c IdentityController) http.HandlerFunc { return c.UpdateRole },
		"DeleteRole":               func(c IdentityController) http.HandlerFunc { return c.DeleteRole },
		"GetRoleAssignments":       func(c IdentityController) http.HandlerFunc { return c.GetRoleAssignments },
		"AddRolePermissions":       func(c IdentityController) http.HandlerFunc { return c.AddRolePermissions },
		"RemoveRolePermissions":    func(c IdentityController) http.HandlerFunc { return c.RemoveRolePermissions },
		"AddRoleAssignees":         func(c IdentityController) http.HandlerFunc { return c.AddRoleAssignees },
		"RemoveRoleAssignees":      func(c IdentityController) http.HandlerFunc { return c.RemoveRoleAssignees },
		"GetUserProfile":           func(c IdentityController) http.HandlerFunc { return c.GetUserProfile },
		"UpdateCurrentUserProfile": func(c IdentityController) http.HandlerFunc { return c.UpdateCurrentUserProfile },
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			w := serveIdentity(t, &clientmocks.IdentityClientMock{}, h, idCall{method: http.MethodGet, url: "/x", body: "{}", noOrg: true})
			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Equal(t, "missing org context", errMessage(t, w))
		})
	}
}

// --- Users ---

func TestIdentityController_ListUsers(t *testing.T) {
	t.Run("clamps pagination and fills attributes from the display name", func(t *testing.T) {
		var gotOU string
		var gotOffset, gotLimit int
		client := &clientmocks.IdentityClientMock{
			ListUsersByOUIdFunc: func(_ context.Context, ouID string, offset, limit int) ([]thundersvc.ThunderUser, int, error) {
				gotOU, gotOffset, gotLimit = ouID, offset, limit
				return []thundersvc.ThunderUser{{ID: "u1", Display: "bob"}, {ID: "u2", Attributes: map[string]any{"username": "carol"}}}, 2, nil
			},
		}
		w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.ListUsers },
			idCall{method: http.MethodGet, url: "/orgs/acme/identities/users?offset=-5&limit=500"})
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, callerOU, gotOU, "users are listed from the token's org only")
		assert.Equal(t, 0, gotOffset)
		assert.Equal(t, 20, gotLimit)

		body := decodeMap(t, w)
		assert.InDelta(t, 2, body["total"], 0)
		assert.InDelta(t, 0, body["offset"], 0)
		assert.InDelta(t, 20, body["limit"], 0)
		users := body["users"].([]any)
		require.Len(t, users, 2)
		assert.Equal(t, "bob", users[0].(map[string]any)["attributes"].(map[string]any)["username"])
		assert.Equal(t, "carol", users[1].(map[string]any)["attributes"].(map[string]any)["username"])
	})

	t.Run("passes a valid page through", func(t *testing.T) {
		client := &clientmocks.IdentityClientMock{
			ListUsersByOUIdFunc: func(_ context.Context, _ string, offset, limit int) ([]thundersvc.ThunderUser, int, error) {
				assert.Equal(t, 40, offset)
				assert.Equal(t, 50, limit)
				return []thundersvc.ThunderUser{}, 41, nil
			},
		}
		w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.ListUsers },
			idCall{method: http.MethodGet, url: "/x?offset=40&limit=50"})
		require.Equal(t, http.StatusOK, w.Code)
		assert.InDelta(t, 41, decodeMap(t, w)["total"], 0)
	})

	t.Run("upstream failure is a 500", func(t *testing.T) {
		client := &clientmocks.IdentityClientMock{
			ListUsersByOUIdFunc: func(context.Context, string, int, int) ([]thundersvc.ThunderUser, int, error) {
				return nil, 0, errThunder
			},
		}
		w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.ListUsers }, idCall{method: http.MethodGet, url: "/x"})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "Failed to list users", errMessage(t, w))
	})
}

// userLookup covers the GetUser-then-check-ownership prologue shared by every
// handler addressing one user.
func TestIdentityController_UserLookupErrors(t *testing.T) {
	handlers := map[string]struct {
		h       func(IdentityController) http.HandlerFunc
		failMsg string
	}{
		"GetUser":       {func(c IdentityController) http.HandlerFunc { return c.GetUser }, "Failed to get user"},
		"UpdateUser":    {func(c IdentityController) http.HandlerFunc { return c.UpdateUser }, "Failed to update user"},
		"DeleteUser":    {func(c IdentityController) http.HandlerFunc { return c.DeleteUser }, "Failed to delete user"},
		"GetUserGroups": {func(c IdentityController) http.HandlerFunc { return c.GetUserGroups }, "Failed to get user groups"},
		"GetUserRoles":  {func(c IdentityController) http.HandlerFunc { return c.GetUserRoles }, "Failed to get user roles"},
	}
	for name, tc := range handlers {
		t.Run(name, func(t *testing.T) {
			call := idCall{method: http.MethodGet, url: "/x", body: "{}", params: map[string]string{utils.PathParamUserID: callerUser}}

			w := serveIdentity(t, &clientmocks.IdentityClientMock{
				GetUserFunc: func(context.Context, string) (*thundersvc.ThunderUser, error) { return nil, notFound() },
			}, tc.h, call)
			assert.Equal(t, http.StatusNotFound, w.Code)
			assert.Equal(t, "User not found", errMessage(t, w))

			w = serveIdentity(t, &clientmocks.IdentityClientMock{
				GetUserFunc: func(context.Context, string) (*thundersvc.ThunderUser, error) { return nil, errThunder },
			}, tc.h, call)
			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Equal(t, tc.failMsg, errMessage(t, w))

			// A user of another org is forbidden, not hidden, and nothing else is called.
			w = serveIdentity(t, &clientmocks.IdentityClientMock{
				GetUserFunc: func(context.Context, string) (*thundersvc.ThunderUser, error) { return userIn(otherOU), nil },
			}, tc.h, call)
			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Equal(t, "User does not belong to your organization", errMessage(t, w))
		})
	}
}

func TestIdentityController_GetUser(t *testing.T) {
	client := &clientmocks.IdentityClientMock{
		GetUserFunc: func(_ context.Context, id string) (*thundersvc.ThunderUser, error) {
			assert.Equal(t, callerUser, id)
			return userIn(callerOU), nil
		},
	}
	w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetUser },
		idCall{method: http.MethodGet, url: "/x", params: map[string]string{utils.PathParamUserID: callerUser}})
	require.Equal(t, http.StatusOK, w.Code)
	body := decodeMap(t, w)
	assert.Equal(t, callerUser, body["id"])
	assert.Equal(t, callerOU, body["ouId"])
}

func TestIdentityController_CreateUser(t *testing.T) {
	createUser := func(c IdentityController) http.HandlerFunc { return c.CreateUser }

	validation := []struct {
		name, body, msg string
	}{
		{"malformed body", "{", "Invalid request body"},
		{"no type", `{"attributes":{"username":"a","password":"p"}}`, "type is required"},
		{"no username", `{"type":"Person","attributes":{"password":"p"}}`, "username in attributes is required"},
		{"no attributes", `{"type":"Person"}`, "username in attributes is required"},
		{"blank password", `{"type":"Person","attributes":{"username":"a","password":"  "}}`, "password in attributes is required"},
	}
	for _, tc := range validation {
		t.Run(tc.name, func(t *testing.T) {
			w := serveIdentity(t, &clientmocks.IdentityClientMock{}, createUser, idCall{method: http.MethodPost, url: "/x", body: tc.body})
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.msg, errMessage(t, w))
		})
	}

	body := `{"type":"Person","ouId":"ou-attacker","attributes":{"username":"alice","password":"s3cret"}}`

	t.Run("creates in the caller's org and records the outcome", func(t *testing.T) {
		rec := &captureRecorder{}
		client := &clientmocks.IdentityClientMock{
			CreateUserFunc: func(_ context.Context, req thundersvc.CreateUserRequest) (*thundersvc.ThunderUser, error) {
				assert.Equal(t, callerOU, req.OuID, "a body-supplied OU must never be used")
				assert.Equal(t, "s3cret", req.Password)
				assert.Equal(t, "Person", req.Type)
				return &thundersvc.ThunderUser{ID: "new-user", OuID: callerOU, Type: "Person"}, nil
			},
		}
		w := serveIdentity(t, client, createUser, idCall{method: http.MethodPost, url: "/x", body: body, rec: rec})
		require.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "new-user", decodeMap(t, w)["id"])

		done := rec.completed(audit.ActionUserCreate)
		require.Len(t, done, 1)
		assert.Equal(t, audit.OutcomeSuccess, done[0].Outcome)
		raw, _ := json.Marshal(rec.events)
		assert.NotContains(t, string(raw), "s3cret", "the password must never reach the audit trail")
	})

	t.Run("upstream failure is a 500 with a failure record", func(t *testing.T) {
		rec := &captureRecorder{}
		client := &clientmocks.IdentityClientMock{
			CreateUserFunc: func(context.Context, thundersvc.CreateUserRequest) (*thundersvc.ThunderUser, error) {
				return nil, errThunder
			},
		}
		w := serveIdentity(t, client, createUser, idCall{method: http.MethodPost, url: "/x", body: body, rec: rec})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "Failed to create user", errMessage(t, w))
		done := rec.completed(audit.ActionUserCreate)
		require.Len(t, done, 1)
		assert.Equal(t, audit.OutcomeFailure, done[0].Outcome)
	})

	t.Run("refuses with 503 when the audit trail is down", func(t *testing.T) {
		// CreateUserFunc is nil: reaching Thunder would panic.
		w := serveIdentity(t, &clientmocks.IdentityClientMock{}, createUser,
			idCall{method: http.MethodPost, url: "/x", body: body, rec: &captureRecorder{failSync: true}})
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "Failed to create user", errMessage(t, w))
	})
}

func TestIdentityController_UpdateUser(t *testing.T) {
	updateUser := func(c IdentityController) http.HandlerFunc { return c.UpdateUser }
	params := map[string]string{utils.PathParamUserID: callerUser}
	owned := func() *clientmocks.IdentityClientMock {
		return &clientmocks.IdentityClientMock{
			GetUserFunc: func(context.Context, string) (*thundersvc.ThunderUser, error) { return userIn(callerOU), nil },
		}
	}

	t.Run("malformed body", func(t *testing.T) {
		w := serveIdentity(t, owned(), updateUser, idCall{method: http.MethodPut, url: "/x", body: "nope", params: params})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("carries the stored type and OU into Thunder's full replace", func(t *testing.T) {
		client := owned()
		client.UpdateUserFunc = func(_ context.Context, id string, req thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			assert.Equal(t, callerUser, id)
			assert.Equal(t, "Person", req.Type)
			assert.Equal(t, callerOU, req.OuID)
			assert.Equal(t, map[string]string{"email": "new@example.com"}, req.Attributes)
			return &thundersvc.ThunderUser{ID: id, Attributes: map[string]any{"email": "new@example.com"}}, nil
		}
		w := serveIdentity(t, client, updateUser,
			idCall{method: http.MethodPut, url: "/x", body: `{"attributes":{"email":"new@example.com"}}`, params: params})
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "new@example.com", decodeMap(t, w)["attributes"].(map[string]any)["email"])
	})

	t.Run("attributes the user type refuses are a 400 with Thunder's reason", func(t *testing.T) {
		client := owned()
		client.UpdateUserFunc = func(context.Context, string, thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			return nil, fmt.Errorf("thunder update user: %w", &thundersvc.BadRequestError{Message: "{}", Description: "attribute email is required"})
		}
		w := serveIdentity(t, client, updateUser,
			idCall{method: http.MethodPut, url: "/x", body: `{"attributes":{"given_name":"A"}}`, params: params})
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "attribute email is required")
	})

	t.Run("no attributes sends an empty set", func(t *testing.T) {
		client := owned()
		client.UpdateUserFunc = func(_ context.Context, _ string, req thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			assert.Empty(t, req.Attributes)
			assert.NotNil(t, req.Attributes)
			return userIn(callerOU), nil
		}
		w := serveIdentity(t, client, updateUser, idCall{method: http.MethodPut, url: "/x", body: `{}`, params: params})
		assert.Equal(t, http.StatusOK, w.Code)
	})

	for name, tc := range map[string]struct {
		err    error
		status int
	}{"vanished": {notFound(), http.StatusNotFound}, "failed": {errThunder, http.StatusInternalServerError}} {
		t.Run("update "+name, func(t *testing.T) {
			client := owned()
			client.UpdateUserFunc = func(context.Context, string, thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
				return nil, tc.err
			}
			w := serveIdentity(t, client, updateUser, idCall{method: http.MethodPut, url: "/x", body: `{}`, params: params})
			assert.Equal(t, tc.status, w.Code)
		})
	}
}

func TestIdentityController_DeleteUser(t *testing.T) {
	deleteUser := func(c IdentityController) http.HandlerFunc { return c.DeleteUser }
	params := map[string]string{utils.PathParamUserID: callerUser}
	owned := func(delErr error) *clientmocks.IdentityClientMock {
		return &clientmocks.IdentityClientMock{
			GetUserFunc:    func(context.Context, string) (*thundersvc.ThunderUser, error) { return userIn(callerOU), nil },
			DeleteUserFunc: func(context.Context, string) error { return delErr },
		}
	}

	t.Run("deletes and records the user by name", func(t *testing.T) {
		rec := &captureRecorder{}
		client := owned(nil)
		w := serveIdentity(t, client, deleteUser, idCall{method: http.MethodDelete, url: "/x", params: params, rec: rec})
		require.Equal(t, http.StatusNoContent, w.Code)
		assert.Empty(t, w.Body.String())
		require.Len(t, client.DeleteUserCalls(), 1)
		done := rec.completed(audit.ActionUserDelete)
		require.Len(t, done, 1)
		assert.Equal(t, audit.OutcomeSuccess, done[0].Outcome)
		assert.Equal(t, "alice", done[0].Details["username"])
	})

	t.Run("delete not found", func(t *testing.T) {
		w := serveIdentity(t, owned(notFound()), deleteUser, idCall{method: http.MethodDelete, url: "/x", params: params})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("delete failure", func(t *testing.T) {
		rec := &captureRecorder{}
		w := serveIdentity(t, owned(errThunder), deleteUser, idCall{method: http.MethodDelete, url: "/x", params: params, rec: rec})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, audit.OutcomeFailure, rec.completed(audit.ActionUserDelete)[0].Outcome)
	})

	t.Run("audit down refuses before deleting", func(t *testing.T) {
		client := owned(nil)
		w := serveIdentity(t, client, deleteUser,
			idCall{method: http.MethodDelete, url: "/x", params: params, rec: &captureRecorder{failSync: true}})
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Empty(t, client.DeleteUserCalls())
	})
}

func TestIdentityController_UserGroupsAndRoles(t *testing.T) {
	params := map[string]string{utils.PathParamUserID: callerUser}
	client := &clientmocks.IdentityClientMock{
		GetUserFunc: func(context.Context, string) (*thundersvc.ThunderUser, error) { return userIn(""), nil },
		GetUserGroupsFunc: func(context.Context, string) ([]thundersvc.ThunderGroup, error) {
			return []thundersvc.ThunderGroup{*groupIn(callerOU, "devs")}, nil
		},
		GetUserRolesFunc: func(context.Context, string) ([]thundersvc.ThunderRole, error) {
			return []thundersvc.ThunderRole{*roleIn(callerOU, "Agent Manager Admin"), *roleIn(callerOU, "custom")}, nil
		},
	}

	w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetUserGroups },
		idCall{method: http.MethodGet, url: "/x", params: params})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, decodeMap(t, w)["groups"], 1)

	w = serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetUserRoles },
		idCall{method: http.MethodGet, url: "/x", params: params})
	require.Equal(t, http.StatusOK, w.Code)
	roles := decodeMap(t, w)["roles"].([]any)
	require.Len(t, roles, 2)
	assert.Equal(t, true, roles[0].(map[string]any)["isReadOnly"], "a predefined role is read-only")
	assert.Equal(t, false, roles[1].(map[string]any)["isReadOnly"])

	failing := &clientmocks.IdentityClientMock{
		GetUserFunc:       client.GetUserFunc,
		GetUserGroupsFunc: func(context.Context, string) ([]thundersvc.ThunderGroup, error) { return nil, errThunder },
		GetUserRolesFunc:  func(context.Context, string) ([]thundersvc.ThunderRole, error) { return nil, errThunder },
	}
	w = serveIdentity(t, failing, func(c IdentityController) http.HandlerFunc { return c.GetUserGroups },
		idCall{method: http.MethodGet, url: "/x", params: params})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	w = serveIdentity(t, failing, func(c IdentityController) http.HandlerFunc { return c.GetUserRoles },
		idCall{method: http.MethodGet, url: "/x", params: params})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestIdentityController_InviteUser(t *testing.T) {
	invite := func(c IdentityController) http.HandlerFunc { return c.InviteUser }

	for name, tc := range map[string]struct{ body, msg string }{
		"malformed body": {"[", "Invalid request body"},
		"no email":       {`{}`, "email is required"},
	} {
		t.Run(name, func(t *testing.T) {
			w := serveIdentity(t, &clientmocks.IdentityClientMock{}, invite, idCall{method: http.MethodPost, url: "/x", body: tc.body})
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.msg, errMessage(t, w))
		})
	}

	for _, onPrem := range []bool{true, false} {
		name := map[bool]string{true: "on-prem invites without an OU", false: "cloud invites into the caller's OU"}[onPrem]
		t.Run(name, func(t *testing.T) {
			cfg := config.GetConfig()
			prev := cfg.IsOnPremDeployment
			cfg.IsOnPremDeployment = onPrem
			t.Cleanup(func() { cfg.IsOnPremDeployment = prev })

			want := callerOU
			if onPrem {
				want = ""
			}
			rec := &captureRecorder{}
			client := &clientmocks.IdentityClientMock{
				InviteUserFunc: func(_ context.Context, email, ouID string) (string, error) {
					assert.Equal(t, "new@example.com", email)
					assert.Equal(t, want, ouID)
					return "https://invite/link-token", nil
				},
			}
			w := serveIdentity(t, client, invite, idCall{method: http.MethodPost, url: "/x", body: `{"email":"new@example.com"}`, rec: rec})
			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "https://invite/link-token", decodeMap(t, w)["inviteLink"])
			raw, _ := json.Marshal(rec.events)
			assert.NotContains(t, string(raw), "link-token", "the invite link grants access and is never recorded")
		})
	}

	t.Run("upstream failure", func(t *testing.T) {
		client := &clientmocks.IdentityClientMock{
			InviteUserFunc: func(context.Context, string, string) (string, error) { return "", errThunder },
		}
		w := serveIdentity(t, client, invite, idCall{method: http.MethodPost, url: "/x", body: `{"email":"x@example.com"}`})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("audit down", func(t *testing.T) {
		w := serveIdentity(t, &clientmocks.IdentityClientMock{}, invite,
			idCall{method: http.MethodPost, url: "/x", body: `{"email":"x@example.com"}`, rec: &captureRecorder{failSync: true}})
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

// --- Groups ---

func TestIdentityController_ListGroups(t *testing.T) {
	client := &clientmocks.IdentityClientMock{
		ListGroupsByOUIdFunc: func(_ context.Context, ouID string, offset, limit int) ([]thundersvc.ThunderGroup, int, error) {
			assert.Equal(t, callerOU, ouID)
			assert.Equal(t, 0, offset)
			assert.Equal(t, 20, limit)
			return []thundersvc.ThunderGroup{{ID: "g1", Name: "devs"}, {ID: "g2", Name: "ops", OuID: "kept"}}, 2, nil
		},
	}
	w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.ListGroups },
		idCall{method: http.MethodGet, url: "/x?limit=0&offset=-1"})
	require.Equal(t, http.StatusOK, w.Code)
	groups := decodeMap(t, w)["groups"].([]any)
	assert.Equal(t, callerOU, groups[0].(map[string]any)["ouId"], "a group from the OU listing gets the caller's OU")
	assert.Equal(t, "kept", groups[1].(map[string]any)["ouId"])

	client.ListGroupsByOUIdFunc = func(context.Context, string, int, int) ([]thundersvc.ThunderGroup, int, error) {
		return nil, 0, errThunder
	}
	w = serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.ListGroups }, idCall{method: http.MethodGet, url: "/x"})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// The shared prologue of the handlers addressing one group: lookup errors,
// another org's group, and Thunder's native Administrators group, which is
// hidden as if it did not exist.
func TestIdentityController_GroupLookupErrors(t *testing.T) {
	handlers := map[string]func(IdentityController) http.HandlerFunc{
		"GetGroup":           func(c IdentityController) http.HandlerFunc { return c.GetGroup },
		"UpdateGroup":        func(c IdentityController) http.HandlerFunc { return c.UpdateGroup },
		"DeleteGroup":        func(c IdentityController) http.HandlerFunc { return c.DeleteGroup },
		"AddGroupMembers":    func(c IdentityController) http.HandlerFunc { return c.AddGroupMembers },
		"RemoveGroupMembers": func(c IdentityController) http.HandlerFunc { return c.RemoveGroupMembers },
		"GetGroupMembers":    func(c IdentityController) http.HandlerFunc { return c.GetGroupMembers },
		"GetGroupRoles":      func(c IdentityController) http.HandlerFunc { return c.GetGroupRoles },
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			call := idCall{method: http.MethodPost, url: "/x", body: `{"userIds":["u"]}`, params: map[string]string{utils.PathParamGroupID: "group-1"}}
			cases := []struct {
				group  *thundersvc.ThunderGroup
				err    error
				status int
				msg    string
			}{
				{nil, notFound(), http.StatusNotFound, "Group not found"},
				{nil, errThunder, http.StatusInternalServerError, ""},
				{groupIn(otherOU, "devs"), nil, http.StatusForbidden, "Group does not belong to your organization"},
				{groupIn(callerOU, thundersvc.NativeAdministratorsGroupName), nil, http.StatusNotFound, "Group not found"},
			}
			for _, tc := range cases {
				w := serveIdentity(t, &clientmocks.IdentityClientMock{
					GetGroupFunc: func(context.Context, string) (*thundersvc.ThunderGroup, error) { return tc.group, tc.err },
				}, h, call)
				assert.Equal(t, tc.status, w.Code)
				if tc.msg != "" {
					assert.Equal(t, tc.msg, errMessage(t, w))
				}
			}
		})
	}
}

func TestIdentityController_GetGroup(t *testing.T) {
	client := &clientmocks.IdentityClientMock{
		GetGroupFunc: func(context.Context, string) (*thundersvc.ThunderGroup, error) { return groupIn(callerOU, "devs"), nil },
	}
	w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetGroup },
		idCall{method: http.MethodGet, url: "/x", params: map[string]string{utils.PathParamGroupID: "group-1"}})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "devs", decodeMap(t, w)["name"])
}

func TestIdentityController_CreateGroup(t *testing.T) {
	create := func(c IdentityController) http.HandlerFunc { return c.CreateGroup }
	for name, tc := range map[string]struct{ body, msg string }{
		"malformed body": {"{", "Invalid request body"},
		"no name":        {`{}`, "name is required"},
		"reserved name":  {`{"name":"Administrators"}`, `"Administrators" is a reserved group name`},
	} {
		t.Run(name, func(t *testing.T) {
			w := serveIdentity(t, &clientmocks.IdentityClientMock{}, create, idCall{method: http.MethodPost, url: "/x", body: tc.body})
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.msg, errMessage(t, w))
		})
	}

	t.Run("creates in the caller's OU", func(t *testing.T) {
		client := &clientmocks.IdentityClientMock{
			CreateGroupFunc: func(_ context.Context, req thundersvc.CreateGroupRequest) (*thundersvc.ThunderGroup, error) {
				assert.Equal(t, thundersvc.CreateGroupRequest{Name: "devs", OuID: callerOU, Description: "the devs"}, req)
				return groupIn(callerOU, "devs"), nil
			},
		}
		w := serveIdentity(t, client, create, idCall{method: http.MethodPost, url: "/x", body: `{"name":"devs","description":"the devs","ouId":"evil"}`})
		require.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "group-1", decodeMap(t, w)["id"])
	})

	t.Run("no description and upstream failure", func(t *testing.T) {
		client := &clientmocks.IdentityClientMock{
			CreateGroupFunc: func(_ context.Context, req thundersvc.CreateGroupRequest) (*thundersvc.ThunderGroup, error) {
				assert.Empty(t, req.Description)
				return nil, errThunder
			},
		}
		w := serveIdentity(t, client, create, idCall{method: http.MethodPost, url: "/x", body: `{"name":"devs"}`})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "Failed to create group", errMessage(t, w))
	})
}

func TestIdentityController_UpdateGroup(t *testing.T) {
	update := func(c IdentityController) http.HandlerFunc { return c.UpdateGroup }
	params := map[string]string{utils.PathParamGroupID: "group-1"}
	owned := func() *clientmocks.IdentityClientMock {
		return &clientmocks.IdentityClientMock{
			GetGroupFunc: func(context.Context, string) (*thundersvc.ThunderGroup, error) { return groupIn(callerOU, "devs"), nil },
		}
	}

	w := serveIdentity(t, owned(), update, idCall{method: http.MethodPut, url: "/x", body: "{", params: params})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = serveIdentity(t, owned(), update, idCall{method: http.MethodPut, url: "/x", body: `{"name":"Administrators"}`, params: params})
	assert.Equal(t, http.StatusBadRequest, w.Code, "renaming to the native admin group is refused")

	t.Run("omitted fields keep their stored values", func(t *testing.T) {
		client := owned()
		client.UpdateGroupFunc = func(_ context.Context, id string, req thundersvc.UpdateGroupRequest) (*thundersvc.ThunderGroup, error) {
			assert.Equal(t, "group-1", id)
			assert.Equal(t, "devs", req.Name)
			assert.Equal(t, "new desc", req.Description)
			return groupIn(callerOU, req.Name), nil
		}
		w := serveIdentity(t, client, update, idCall{method: http.MethodPut, url: "/x", body: `{"description":"new desc"}`, params: params})
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "devs", decodeMap(t, w)["name"])
	})

	for name, tc := range map[string]struct {
		err    error
		status int
	}{"vanished": {notFound(), http.StatusNotFound}, "failed": {errThunder, http.StatusInternalServerError}} {
		t.Run(name, func(t *testing.T) {
			client := owned()
			client.UpdateGroupFunc = func(context.Context, string, thundersvc.UpdateGroupRequest) (*thundersvc.ThunderGroup, error) {
				return nil, tc.err
			}
			w := serveIdentity(t, client, update, idCall{method: http.MethodPut, url: "/x", body: `{}`, params: params})
			assert.Equal(t, tc.status, w.Code)
		})
	}
}

func TestIdentityController_DeleteGroup(t *testing.T) {
	params := map[string]string{utils.PathParamGroupID: "group-1"}
	for name, tc := range map[string]struct {
		err    error
		status int
	}{
		"deleted":  {nil, http.StatusNoContent},
		"vanished": {notFound(), http.StatusNotFound},
		"failed":   {errThunder, http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			client := &clientmocks.IdentityClientMock{
				GetGroupFunc:    func(context.Context, string) (*thundersvc.ThunderGroup, error) { return groupIn(callerOU, "devs"), nil },
				DeleteGroupFunc: func(context.Context, string) error { return tc.err },
			}
			w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.DeleteGroup },
				idCall{method: http.MethodDelete, url: "/x", params: params})
			assert.Equal(t, tc.status, w.Code)
		})
	}
}

func TestIdentityController_GroupMembership(t *testing.T) {
	params := map[string]string{utils.PathParamGroupID: "group-1"}
	ops := map[string]struct {
		h      func(IdentityController) http.HandlerFunc
		action audit.Action
		set    func(*clientmocks.IdentityClientMock, error)
		calls  func(*clientmocks.IdentityClientMock) int
	}{
		"add": {
			func(c IdentityController) http.HandlerFunc { return c.AddGroupMembers }, audit.ActionGroupAddMember,
			func(m *clientmocks.IdentityClientMock, err error) {
				m.AddGroupMembersFunc = func(_ context.Context, _ string, ids []string) error {
					assert.Equal(t, []string{"u1", "u2"}, ids)
					return err
				}
			},
			func(m *clientmocks.IdentityClientMock) int { return len(m.AddGroupMembersCalls()) },
		},
		"remove": {
			func(c IdentityController) http.HandlerFunc { return c.RemoveGroupMembers }, audit.ActionGroupRemoveMember,
			func(m *clientmocks.IdentityClientMock, err error) {
				m.RemoveGroupMembersFunc = func(_ context.Context, _ string, ids []string) error {
					assert.Equal(t, []string{"u1", "u2"}, ids)
					return err
				}
			},
			func(m *clientmocks.IdentityClientMock) int { return len(m.RemoveGroupMembersCalls()) },
		},
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			newClient := func(err error) *clientmocks.IdentityClientMock {
				m := &clientmocks.IdentityClientMock{
					GetGroupFunc: func(context.Context, string) (*thundersvc.ThunderGroup, error) { return groupIn(callerOU, "devs"), nil },
				}
				op.set(m, err)
				return m
			}
			body := `{"userIds":["u1","u2"]}`

			for bname, b := range map[string]struct{ body, msg string }{
				"malformed": {"{", "Invalid request body"},
				"empty":     {`{"userIds":[]}`, "userIds must not be empty"},
			} {
				w := serveIdentity(t, newClient(nil), op.h, idCall{method: http.MethodPost, url: "/x", body: b.body, params: params})
				assert.Equal(t, http.StatusBadRequest, w.Code, bname)
				assert.Equal(t, b.msg, errMessage(t, w), bname)
			}

			rec := &captureRecorder{}
			w := serveIdentity(t, newClient(nil), op.h, idCall{method: http.MethodPost, url: "/x", body: body, params: params, rec: rec})
			require.Equal(t, http.StatusOK, w.Code)
			done := rec.completed(op.action)
			require.Len(t, done, 1)
			assert.Equal(t, audit.OutcomeSuccess, done[0].Outcome)

			rec = &captureRecorder{}
			w = serveIdentity(t, newClient(errThunder), op.h, idCall{method: http.MethodPost, url: "/x", body: body, params: params, rec: rec})
			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Equal(t, audit.OutcomeFailure, rec.completed(op.action)[0].Outcome)

			client := newClient(nil)
			w = serveIdentity(t, client, op.h, idCall{method: http.MethodPost, url: "/x", body: body, params: params, rec: &captureRecorder{failSync: true}})
			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
			assert.Zero(t, op.calls(client), "membership must not change unrecorded")
		})
	}
}

func TestIdentityController_GetGroupMembersAndRoles(t *testing.T) {
	params := map[string]string{utils.PathParamGroupID: "group-1"}
	client := &clientmocks.IdentityClientMock{
		GetGroupFunc: func(context.Context, string) (*thundersvc.ThunderGroup, error) { return groupIn(callerOU, "devs"), nil },
		GetGroupMembersFunc: func(_ context.Context, _ string, offset, limit int) ([]thundersvc.ThunderUser, int, error) {
			assert.Equal(t, 5, offset)
			assert.Equal(t, 20, limit, "an out-of-range limit falls back to the default")
			return []thundersvc.ThunderUser{*userIn(callerOU)}, 6, nil
		},
		GetGroupRolesFunc: func(context.Context, string) ([]thundersvc.ThunderRole, error) {
			return []thundersvc.ThunderRole{*roleIn(callerOU, "Agent Manager Admin")}, nil
		},
	}
	w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetGroupMembers },
		idCall{method: http.MethodGet, url: "/x?offset=5&limit=101", params: params})
	require.Equal(t, http.StatusOK, w.Code)
	body := decodeMap(t, w)
	assert.InDelta(t, 6, body["total"], 0)
	assert.Len(t, body["users"], 1)

	w = serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetGroupRoles },
		idCall{method: http.MethodGet, url: "/x", params: params})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, decodeMap(t, w)["roles"].([]any)[0].(map[string]any)["isReadOnly"])

	client.GetGroupMembersFunc = func(context.Context, string, int, int) ([]thundersvc.ThunderUser, int, error) {
		return nil, 0, errThunder
	}
	client.GetGroupRolesFunc = func(context.Context, string) ([]thundersvc.ThunderRole, error) { return nil, errThunder }
	w = serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetGroupMembers },
		idCall{method: http.MethodGet, url: "/x", params: params})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	w = serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetGroupRoles },
		idCall{method: http.MethodGet, url: "/x", params: params})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- Roles ---

func TestIdentityController_ListRoles(t *testing.T) {
	client := &clientmocks.IdentityClientMock{
		ListRolesFunc: func(_ context.Context, ouID string, offset, limit int) ([]thundersvc.ThunderRole, int, error) {
			assert.Equal(t, callerOU, ouID)
			assert.Equal(t, 10, offset)
			assert.Equal(t, 5, limit)
			return []thundersvc.ThunderRole{*roleIn(callerOU, "Agent Manager Admin"), *roleIn(callerOU, "mine")}, 12, nil
		},
	}
	w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.ListRoles },
		idCall{method: http.MethodGet, url: "/x?offset=10&limit=5"})
	require.Equal(t, http.StatusOK, w.Code)
	body := decodeMap(t, w)
	assert.InDelta(t, 12, body["total"], 0)
	roles := body["roles"].([]any)
	assert.Equal(t, true, roles[0].(map[string]any)["isReadOnly"])
	assert.Equal(t, false, roles[1].(map[string]any)["isReadOnly"])

	client.ListRolesFunc = func(context.Context, string, int, int) ([]thundersvc.ThunderRole, int, error) {
		return nil, 0, errThunder
	}
	w = serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.ListRoles }, idCall{method: http.MethodGet, url: "/x?limit=0"})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestIdentityController_RoleLookupErrors(t *testing.T) {
	handlers := map[string]func(IdentityController) http.HandlerFunc{
		"GetRole":               func(c IdentityController) http.HandlerFunc { return c.GetRole },
		"UpdateRole":            func(c IdentityController) http.HandlerFunc { return c.UpdateRole },
		"DeleteRole":            func(c IdentityController) http.HandlerFunc { return c.DeleteRole },
		"GetRoleAssignments":    func(c IdentityController) http.HandlerFunc { return c.GetRoleAssignments },
		"AddRolePermissions":    func(c IdentityController) http.HandlerFunc { return c.AddRolePermissions },
		"RemoveRolePermissions": func(c IdentityController) http.HandlerFunc { return c.RemoveRolePermissions },
		"AddRoleAssignees":      func(c IdentityController) http.HandlerFunc { return c.AddRoleAssignees },
		"RemoveRoleAssignees":   func(c IdentityController) http.HandlerFunc { return c.RemoveRoleAssignees },
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			call := idCall{method: http.MethodPost, url: "/x", body: "{}", params: map[string]string{utils.PathParamRoleID: "role-1"}}
			cases := []struct {
				role   *thundersvc.ThunderRole
				err    error
				status int
				msg    string
			}{
				{nil, notFound(), http.StatusNotFound, "Role not found"},
				{nil, errThunder, http.StatusInternalServerError, ""},
				{roleIn(otherOU, "theirs"), nil, http.StatusForbidden, "Role does not belong to your organization"},
			}
			for _, tc := range cases {
				w := serveIdentity(t, &clientmocks.IdentityClientMock{
					GetRoleFunc: func(context.Context, string) (*thundersvc.ThunderRole, error) { return tc.role, tc.err },
				}, h, call)
				assert.Equal(t, tc.status, w.Code)
				if tc.msg != "" {
					assert.Equal(t, tc.msg, errMessage(t, w))
				}
			}
		})
	}
}

func TestIdentityController_PredefinedRoleIsImmutable(t *testing.T) {
	for name, h := range map[string]func(IdentityController) http.HandlerFunc{
		"UpdateRole": func(c IdentityController) http.HandlerFunc { return c.UpdateRole },
		"DeleteRole": func(c IdentityController) http.HandlerFunc { return c.DeleteRole },
	} {
		t.Run(name, func(t *testing.T) {
			w := serveIdentity(t, &clientmocks.IdentityClientMock{
				GetRoleFunc: func(context.Context, string) (*thundersvc.ThunderRole, error) {
					return roleIn(callerOU, "Agent Manager Admin"), nil
				},
			}, h, idCall{method: http.MethodPut, url: "/x", body: `{}`, params: map[string]string{utils.PathParamRoleID: "role-1"}})
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "Predefined roles cannot be edited or deleted", errMessage(t, w))
		})
	}
}

func TestIdentityController_GetRole(t *testing.T) {
	client := &clientmocks.IdentityClientMock{
		GetRoleFunc: func(context.Context, string) (*thundersvc.ThunderRole, error) {
			return roleIn(callerOU, "Agent Manager Admin"), nil
		},
	}
	w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetRole },
		idCall{method: http.MethodGet, url: "/x", params: map[string]string{utils.PathParamRoleID: "role-1"}})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, decodeMap(t, w)["isReadOnly"])
}

func TestIdentityController_CreateRole(t *testing.T) {
	create := func(c IdentityController) http.HandlerFunc { return c.CreateRole }
	for name, tc := range map[string]struct{ body, msg string }{
		"malformed body":          {"{", "Invalid request body"},
		"no name":                 {`{}`, "name is required"},
		"native admin name":       {`{"name":"Administrator"}`, `"Administrator" is a reserved role name`},
		"system client role name": {`{"name":"AMP System Client Thunder Admin"}`, `"AMP System Client Thunder Admin" is a reserved role name`},
	} {
		t.Run(name, func(t *testing.T) {
			w := serveIdentity(t, &clientmocks.IdentityClientMock{}, create, idCall{method: http.MethodPost, url: "/x", body: tc.body})
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.msg, errMessage(t, w))
		})
	}

	client := &clientmocks.IdentityClientMock{
		CreateRoleFunc: func(_ context.Context, req thundersvc.CreateRoleRequest) (*thundersvc.ThunderRole, error) {
			assert.Equal(t, thundersvc.CreateRoleRequest{Name: "auditors", OuID: callerOU, Description: "read only"}, req)
			return roleIn(callerOU, req.Name), nil
		},
	}
	w := serveIdentity(t, client, create, idCall{method: http.MethodPost, url: "/x", body: `{"name":"auditors","description":"read only"}`})
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, false, decodeMap(t, w)["isReadOnly"])

	client.CreateRoleFunc = func(context.Context, thundersvc.CreateRoleRequest) (*thundersvc.ThunderRole, error) {
		return nil, errThunder
	}
	w = serveIdentity(t, client, create, idCall{method: http.MethodPost, url: "/x", body: `{"name":"auditors"}`})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestIdentityController_UpdateRole(t *testing.T) {
	update := func(c IdentityController) http.HandlerFunc { return c.UpdateRole }
	params := map[string]string{utils.PathParamRoleID: "role-1"}
	owned := func() *clientmocks.IdentityClientMock {
		role := roleIn(callerOU, "auditors")
		role.Permissions = []thundersvc.RolePermissionRequest{{ResourceServerID: "rs", Permissions: []string{"amp:org:view"}}}
		return &clientmocks.IdentityClientMock{
			GetRoleFunc: func(context.Context, string) (*thundersvc.ThunderRole, error) { return role, nil },
		}
	}

	w := serveIdentity(t, owned(), update, idCall{method: http.MethodPut, url: "/x", body: "{", params: params})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = serveIdentity(t, owned(), update, idCall{method: http.MethodPut, url: "/x", body: `{"name":"Administrator"}`, params: params})
	assert.Equal(t, http.StatusBadRequest, w.Code, "renaming to a reserved role is refused")

	t.Run("a rename keeps the OU and permissions", func(t *testing.T) {
		client := owned()
		client.UpdateRoleFunc = func(_ context.Context, _ string, req thundersvc.UpdateRoleRequest) (*thundersvc.ThunderRole, error) {
			assert.Equal(t, "renamed", req.Name)
			assert.Equal(t, callerOU, req.OuID)
			require.Len(t, req.Permissions, 1)
			return roleIn(callerOU, req.Name), nil
		}
		w := serveIdentity(t, client, update, idCall{method: http.MethodPut, url: "/x", body: `{"name":"renamed"}`, params: params})
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "renamed", decodeMap(t, w)["name"])
	})

	for name, tc := range map[string]struct {
		err    error
		status int
	}{"vanished": {notFound(), http.StatusNotFound}, "failed": {errThunder, http.StatusInternalServerError}} {
		t.Run(name, func(t *testing.T) {
			client := owned()
			client.UpdateRoleFunc = func(context.Context, string, thundersvc.UpdateRoleRequest) (*thundersvc.ThunderRole, error) {
				return nil, tc.err
			}
			w := serveIdentity(t, client, update, idCall{method: http.MethodPut, url: "/x", body: `{}`, params: params})
			assert.Equal(t, tc.status, w.Code)
		})
	}
}

func TestIdentityController_DeleteRole(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status int
	}{
		"deleted":  {nil, http.StatusNoContent},
		"vanished": {notFound(), http.StatusNotFound},
		"failed":   {errThunder, http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			client := &clientmocks.IdentityClientMock{
				GetRoleFunc:    func(context.Context, string) (*thundersvc.ThunderRole, error) { return roleIn(callerOU, "mine"), nil },
				DeleteRoleFunc: func(context.Context, string) error { return tc.err },
			}
			w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.DeleteRole },
				idCall{method: http.MethodDelete, url: "/x", params: map[string]string{utils.PathParamRoleID: "role-1"}})
			assert.Equal(t, tc.status, w.Code)
		})
	}
}

func TestIdentityController_GetRoleAssignments(t *testing.T) {
	client := &clientmocks.IdentityClientMock{
		GetRoleFunc: func(context.Context, string) (*thundersvc.ThunderRole, error) { return roleIn(callerOU, "mine"), nil },
		GetRoleAssignmentsFunc: func(context.Context, string) (*thundersvc.RoleAssignments, error) {
			return &thundersvc.RoleAssignments{Users: []thundersvc.ThunderUser{*userIn(callerOU)}}, nil
		},
	}
	call := idCall{method: http.MethodGet, url: "/x", params: map[string]string{utils.PathParamRoleID: "role-1"}}
	w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetRoleAssignments }, call)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, decodeMap(t, w)["users"], 1)

	client.GetRoleAssignmentsFunc = func(context.Context, string) (*thundersvc.RoleAssignments, error) { return nil, errThunder }
	w = serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.GetRoleAssignments }, call)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestIdentityController_RolePermissionChanges(t *testing.T) {
	params := map[string]string{utils.PathParamRoleID: "role-1"}
	ops := map[string]struct {
		h      func(IdentityController) http.HandlerFunc
		action audit.Action
		set    func(*clientmocks.IdentityClientMock, error)
	}{
		"add": {
			func(c IdentityController) http.HandlerFunc { return c.AddRolePermissions }, audit.ActionRoleGrantPermission,
			func(m *clientmocks.IdentityClientMock, err error) {
				m.AddRolePermissionsFunc = func(context.Context, string, thundersvc.RolePermissionRequest) error { return err }
			},
		},
		"remove": {
			func(c IdentityController) http.HandlerFunc { return c.RemoveRolePermissions }, audit.ActionRoleRevokePermission,
			func(m *clientmocks.IdentityClientMock, err error) {
				m.RemoveRolePermissionsFunc = func(context.Context, string, thundersvc.RolePermissionRequest) error { return err }
			},
		},
	}
	body := `{"resourceServerId":"rs-1","permissions":["amp:org:view","amp:agent:read"]}`
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			newClient := func(err error) *clientmocks.IdentityClientMock {
				m := &clientmocks.IdentityClientMock{
					GetRoleFunc: func(context.Context, string) (*thundersvc.ThunderRole, error) { return roleIn(callerOU, "mine"), nil },
				}
				op.set(m, err)
				return m
			}
			w := serveIdentity(t, newClient(nil), op.h, idCall{method: http.MethodPost, url: "/x", body: "{", params: params})
			assert.Equal(t, http.StatusBadRequest, w.Code)

			rec := &captureRecorder{}
			w = serveIdentity(t, newClient(nil), op.h, idCall{method: http.MethodPost, url: "/x", body: body, params: params, rec: rec})
			require.Equal(t, http.StatusOK, w.Code)
			done := rec.completed(op.action)
			require.Len(t, done, 1)
			assert.Equal(t, audit.OutcomeSuccess, done[0].Outcome)
			assert.EqualValues(t, 2, done[0].Details["permissionCount"], "the granted scopes are recorded")

			w = serveIdentity(t, newClient(errThunder), op.h, idCall{method: http.MethodPost, url: "/x", body: body, params: params})
			assert.Equal(t, http.StatusInternalServerError, w.Code)

			w = serveIdentity(t, newClient(nil), op.h,
				idCall{method: http.MethodPost, url: "/x", body: body, params: params, rec: &captureRecorder{failSync: true}})
			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	}
}

func TestIdentityController_RoleAssigneeChanges(t *testing.T) {
	params := map[string]string{utils.PathParamRoleID: "role-1"}
	ops := map[string]struct {
		h      func(IdentityController) http.HandlerFunc
		action audit.Action
		set    func(*clientmocks.IdentityClientMock, error)
	}{
		"add": {
			func(c IdentityController) http.HandlerFunc { return c.AddRoleAssignees }, audit.ActionRoleAssign,
			func(m *clientmocks.IdentityClientMock, err error) {
				m.AddRoleAssigneesFunc = func(_ context.Context, _ string, req thundersvc.RoleAssignmentsRequest) error {
					assert.Equal(t, []thundersvc.AssignmentEntry{{ID: "u1", Type: "user"}, {ID: "g1", Type: "group"}}, req.Assignments)
					return err
				}
			},
		},
		"remove": {
			func(c IdentityController) http.HandlerFunc { return c.RemoveRoleAssignees }, audit.ActionRoleUnassign,
			func(m *clientmocks.IdentityClientMock, err error) {
				m.RemoveRoleAssigneesFunc = func(_ context.Context, _ string, req thundersvc.RoleAssignmentsRequest) error {
					assert.Len(t, req.Assignments, 2)
					return err
				}
			},
		},
	}
	body := `{"userIds":["u1"],"groupIds":["g1"]}`
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			newClient := func(err error) *clientmocks.IdentityClientMock {
				// A predefined role: assigning principals to one is allowed.
				m := &clientmocks.IdentityClientMock{
					GetRoleFunc: func(context.Context, string) (*thundersvc.ThunderRole, error) {
						return roleIn(callerOU, "Agent Manager Admin"), nil
					},
				}
				op.set(m, err)
				return m
			}
			for _, bad := range []string{"{", `{"userIds":[],"groupIds":[]}`} {
				w := serveIdentity(t, newClient(nil), op.h, idCall{method: http.MethodPost, url: "/x", body: bad, params: params})
				assert.Equal(t, http.StatusBadRequest, w.Code, bad)
			}

			rec := &captureRecorder{}
			w := serveIdentity(t, newClient(nil), op.h, idCall{method: http.MethodPost, url: "/x", body: body, params: params, rec: rec})
			require.Equal(t, http.StatusOK, w.Code)
			done := rec.completed(op.action)
			require.Len(t, done, 1)
			assert.Equal(t, []string{"u1", "g1"}, done[0].Details["assignees"])
			assert.Equal(t, []string{"user", "group"}, done[0].Details["assigneeTypes"])

			w = serveIdentity(t, newClient(errThunder), op.h, idCall{method: http.MethodPost, url: "/x", body: body, params: params})
			assert.Equal(t, http.StatusInternalServerError, w.Code)

			w = serveIdentity(t, newClient(nil), op.h,
				idCall{method: http.MethodPost, url: "/x", body: body, params: params, rec: &captureRecorder{failSync: true}})
			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	}
}

func TestIdentityController_ListAMPPermissions(t *testing.T) {
	client := &clientmocks.IdentityClientMock{
		ListAMPPermissionsFunc: func(context.Context) ([]thundersvc.ThunderPermission, string, error) {
			return []thundersvc.ThunderPermission{{}}, "rs-amp", nil
		},
	}
	w := serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.ListAMPPermissions }, idCall{method: http.MethodGet, url: "/x"})
	require.Equal(t, http.StatusOK, w.Code)
	body := decodeMap(t, w)
	assert.Equal(t, "rs-amp", body["resourceServerId"])
	assert.Len(t, body["permissions"], 1)

	client.ListAMPPermissionsFunc = func(context.Context) ([]thundersvc.ThunderPermission, string, error) { return nil, "", errThunder }
	w = serveIdentity(t, client, func(c IdentityController) http.HandlerFunc { return c.ListAMPPermissions }, idCall{method: http.MethodGet, url: "/x"})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- Profile ---

func TestIdentityController_GetUserProfile(t *testing.T) {
	get := func(c IdentityController) http.HandlerFunc { return c.GetUserProfile }
	client := &clientmocks.IdentityClientMock{
		GetUserFunc: func(context.Context, string) (*thundersvc.ThunderUser, error) { return userIn(callerOU), nil },
	}

	w := serveIdentity(t, client, get, idCall{method: http.MethodGet, url: "/x", sub: callerUser})
	assert.Equal(t, http.StatusBadRequest, w.Code, "no user id")

	for name, sub := range map[string]string{"another user's profile": "user-2", "no token": ""} {
		w = serveIdentity(t, client, get, idCall{method: http.MethodGet, url: "/x", sub: sub, params: map[string]string{utils.PathParamUserID: callerUser}})
		assert.Equal(t, http.StatusForbidden, w.Code, name)
		assert.Equal(t, "You can only view your own profile", errMessage(t, w), name)
	}

	call := idCall{method: http.MethodGet, url: "/x", sub: callerUser, params: map[string]string{utils.PathParamUserID: callerUser}}
	w = serveIdentity(t, client, get, call)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, callerUser, decodeMap(t, w)["id"])

	client.GetUserFunc = func(context.Context, string) (*thundersvc.ThunderUser, error) { return nil, notFound() }
	assert.Equal(t, http.StatusNotFound, serveIdentity(t, client, get, call).Code)
	client.GetUserFunc = func(context.Context, string) (*thundersvc.ThunderUser, error) { return nil, errThunder }
	assert.Equal(t, http.StatusInternalServerError, serveIdentity(t, client, get, call).Code)
}

func TestIdentityController_UpdateCurrentUserProfile(t *testing.T) {
	update := func(c IdentityController) http.HandlerFunc { return c.UpdateCurrentUserProfile }
	params := map[string]string{utils.PathParamUserID: callerUser}
	self := func(body string) idCall {
		return idCall{method: http.MethodPut, url: "/x", body: body, sub: callerUser, params: params}
	}
	owned := func() *clientmocks.IdentityClientMock {
		return &clientmocks.IdentityClientMock{
			GetUserFunc: func(context.Context, string) (*thundersvc.ThunderUser, error) { return userIn(callerOU), nil },
		}
	}

	assert.Equal(t, http.StatusBadRequest, serveIdentity(t, owned(), update, idCall{method: http.MethodPut, url: "/x", body: "{}", sub: callerUser}).Code)
	w := serveIdentity(t, owned(), update, idCall{method: http.MethodPut, url: "/x", body: "{}", sub: "user-2", params: params})
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "You can only update your own profile", errMessage(t, w))
	assert.Equal(t, http.StatusBadRequest, serveIdentity(t, owned(), update, self("{")).Code)

	t.Run("merges known fields and changes the password separately", func(t *testing.T) {
		client := owned()
		client.UpdateUserFunc = func(_ context.Context, id string, req thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			assert.Equal(t, map[string]string{
				"username":    "alice",         // kept from the stored user
				"email":       "a@example.com", // kept
				"given_name":  "Alice",         // set by the request
				"family_name": "Liddell",       // set by the request
			}, req.Attributes, "unknown attributes and the password are never sent to the regular update")
			assert.Equal(t, "Person", req.Type)
			assert.Equal(t, callerOU, req.OuID)
			return &thundersvc.ThunderUser{ID: id, Attributes: map[string]any{"given_name": "Alice"}}, nil
		}
		client.UpdateUserCredentialsFunc = func(_ context.Context, id, password string) error {
			assert.Equal(t, callerUser, id)
			assert.Equal(t, "n3w-pass", password)
			return nil
		}
		w := serveIdentity(t, client, update,
			self(`{"attributes":{"given_name":"Alice","family_name":"Liddell","nickname":"x","password":"n3w-pass"}}`))
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, client.UpdateUserCredentialsCalls(), 1)
	})

	t.Run("no password leaves credentials alone", func(t *testing.T) {
		client := owned()
		client.UpdateUserFunc = func(context.Context, string, thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			return userIn(callerOU), nil
		}
		// UpdateUserCredentialsFunc is nil: calling it would panic.
		assert.Equal(t, http.StatusOK, serveIdentity(t, client, update, self(`{}`)).Code)
	})

	t.Run("password change failure is reported", func(t *testing.T) {
		client := owned()
		client.UpdateUserFunc = func(context.Context, string, thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			return userIn(callerOU), nil
		}
		client.UpdateUserCredentialsFunc = func(context.Context, string, string) error { return errThunder }
		w := serveIdentity(t, client, update, self(`{"attributes":{"password":"x"}}`))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "Profile updated, but password change failed", errMessage(t, w))
	})

	t.Run("lookup and update failures", func(t *testing.T) {
		for _, tc := range []struct {
			get, upd error
			owner    string
			status   int
		}{
			{get: notFound(), status: http.StatusNotFound},
			{get: errThunder, status: http.StatusInternalServerError},
			{owner: otherOU, status: http.StatusForbidden},
			{upd: notFound(), owner: callerOU, status: http.StatusNotFound},
			{upd: errThunder, owner: callerOU, status: http.StatusInternalServerError},
		} {
			client := &clientmocks.IdentityClientMock{
				GetUserFunc: func(context.Context, string) (*thundersvc.ThunderUser, error) {
					if tc.get != nil {
						return nil, tc.get
					}
					return userIn(tc.owner), nil
				},
				UpdateUserFunc: func(context.Context, string, thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
					return nil, tc.upd
				},
			}
			assert.Equal(t, tc.status, serveIdentity(t, client, update, self(`{}`)).Code)
		}
	})
}

func TestSanitizeAttributesForLogging(t *testing.T) {
	plain := map[string]string{"username": "alice", "password": "secret"}
	assert.Equal(t, map[string]string{"username": "alice"}, sanitizeAttributesForLogging(plain))
	assert.Equal(t, map[string]string{"username": "alice"}, sanitizeAttributesForLogging(&plain))
	var nilPtr *map[string]string
	assert.Empty(t, sanitizeAttributesForLogging(nilPtr))
	assert.Equal(t, map[string]string{"email": "[set]"},
		sanitizeAttributesForLogging(map[string]any{"email": "a@example.com", "password": "x"}),
		"free-form values are never logged, only that they are set")
	assert.Empty(t, sanitizeAttributesForLogging(42))
}

func TestUserIdentifier(t *testing.T) {
	assert.Empty(t, userIdentifier(nil))
	assert.Equal(t, "alice", userIdentifier(&thundersvc.ThunderUser{ID: "id", Attributes: map[string]any{"username": "alice", "email": "e"}}))
	assert.Equal(t, "e@example.com", userIdentifier(&thundersvc.ThunderUser{ID: "id", Attributes: map[string]any{"email": "e@example.com"}}))
	assert.Equal(t, "legacy", userIdentifier(&thundersvc.ThunderUser{ID: "id", Attributes: map[string]any{"userName": "legacy"}}))
	assert.Equal(t, "id", userIdentifier(&thundersvc.ThunderUser{ID: "id", Attributes: map[string]any{"username": 7}}),
		"a non-string attribute falls back to the id instead of panicking")
}
