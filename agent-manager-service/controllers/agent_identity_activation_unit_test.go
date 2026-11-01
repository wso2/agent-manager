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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// activatingIdentityClient is a CP managed identity client: the mock plus the
// activation capability, counting how often a request activates.
type activatingIdentityClient struct {
	*clientmocks.EnvIdentityClientMock
	activations int
	err         error
}

func (c *activatingIdentityClient) ActivateIdentityChange(context.Context) error {
	c.activations++
	return c.err
}

// writeCall is one identity write handler with the request that reaches it.
type writeCall struct {
	name   string
	method string
	path   string
	body   string
	pathID map[string]string
	call   func(AgentIdentityController, http.ResponseWriter, *http.Request)
	// status is the success status.
	status int
}

func identityWriteCalls() []writeCall {
	return []writeCall{
		{"create group", http.MethodPost, "/groups", `{"name":"team"}`, nil, AgentIdentityController.CreateGroup, http.StatusCreated},
		{"update group", http.MethodPut, "/groups/g1", `{"name":"team"}`, map[string]string{"groupId": "g1"}, AgentIdentityController.UpdateGroup, http.StatusOK},
		{"delete group", http.MethodDelete, "/groups/g1", ``, map[string]string{"groupId": "g1"}, AgentIdentityController.DeleteGroup, http.StatusNoContent},
		{"add group members", http.MethodPost, "/groups/g1/members/add", `{"agentIds":["a1"]}`, map[string]string{"groupId": "g1"}, AgentIdentityController.AddGroupMembers, http.StatusOK},
		{"remove group members", http.MethodPost, "/groups/g1/members/remove", `{"agentIds":["a1"]}`, map[string]string{"groupId": "g1"}, AgentIdentityController.RemoveGroupMembers, http.StatusOK},
		{"create role", http.MethodPost, "/roles", `{"name":"reader"}`, nil, AgentIdentityController.CreateRole, http.StatusCreated},
		{"update role metadata", http.MethodPut, "/roles/r1", `{"name":"reader"}`, map[string]string{"roleId": "r1"}, AgentIdentityController.UpdateRole, http.StatusOK},
		{"update role scopes", http.MethodPut, "/roles/r1", `{"name":"reader","scopes":[]}`, map[string]string{"roleId": "r1"}, AgentIdentityController.UpdateRole, http.StatusOK},
		{"delete role", http.MethodDelete, "/roles/r1", ``, map[string]string{"roleId": "r1"}, AgentIdentityController.DeleteRole, http.StatusNoContent},
		{"add role assignees", http.MethodPost, "/roles/r1/assignments/add", `{"assignments":[{"id":"a1","type":"agent"}]}`, map[string]string{"roleId": "r1"}, AgentIdentityController.AddRoleAssignees, http.StatusOK},
		{"remove role assignees", http.MethodPost, "/roles/r1/assignments/remove", `{"assignments":[{"id":"a1","type":"agent"}]}`, map[string]string{"roleId": "r1"}, AgentIdentityController.RemoveRoleAssignees, http.StatusOK},
	}
}

// happyIdentityMock answers every write the handlers make.
func happyIdentityMock() *clientmocks.EnvIdentityClientMock {
	role := &thundersvc.ThunderRole{ID: "r1", Name: "reader"}
	group := &thundersvc.ThunderGroup{ID: "g1", Name: "team"}
	return &clientmocks.EnvIdentityClientMock{
		GetDefaultOUIDFunc: func(context.Context) (string, error) { return "ou-env", nil },
		GetRoleFunc:        func(context.Context, string) (*thundersvc.ThunderRole, error) { return role, nil },
		GetGroupFunc:       func(context.Context, string) (*thundersvc.ThunderGroup, error) { return group, nil },
		CreateRoleFunc:     func(context.Context, thundersvc.CreateRoleRequest) (*thundersvc.ThunderRole, error) { return role, nil },
		UpdateRoleFunc: func(context.Context, string, thundersvc.UpdateRoleRequest) (*thundersvc.ThunderRole, error) {
			return role, nil
		},
		DeleteRoleFunc: func(context.Context, string) error { return nil },
		CreateGroupFunc: func(context.Context, thundersvc.CreateGroupRequest) (*thundersvc.ThunderGroup, error) {
			return group, nil
		},
		UpdateGroupFunc: func(context.Context, string, thundersvc.UpdateGroupRequest) (*thundersvc.ThunderGroup, error) {
			return group, nil
		},
		DeleteGroupFunc:      func(context.Context, string) error { return nil },
		AddRoleAssigneesFunc: func(context.Context, string, thundersvc.RoleAssignmentsRequest) error { return nil },
		RemoveRoleAssigneesFunc: func(context.Context, string, thundersvc.RoleAssignmentsRequest) error {
			return nil
		},
		AddGroupMemberEntriesFunc:    func(context.Context, string, []thundersvc.GroupMember) error { return nil },
		RemoveGroupMemberEntriesFunc: func(context.Context, string, []thundersvc.GroupMember) error { return nil },
	}
}

func newActivationController(client thundersvc.EnvIdentityClient, resolveErr error) AgentIdentityController {
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
			return client, resolveErr
		},
	}
	return NewAgentIdentityController(resolver, &repomocks.AgentThunderClientRepositoryMock{},
		&repomocks.MCPProxyRepositoryMock{}, &repomocks.MCPProxyScopeRepositoryMock{}, stubRSIdentifierResolver{})
}

func (w writeCall) request() *http.Request {
	req := httptest.NewRequest(w.method, "/orgs/o1/environments/dev/agent-identities"+w.path, strings.NewReader(w.body))
	req.SetPathValue("orgName", "o1")
	req.SetPathValue("envName", "dev")
	for key, value := range w.pathID {
		req.SetPathValue(key, value)
	}
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-org"}))
}

func TestAgentIdentityWrites_ActivateExactlyOncePerRequestInCPMode(t *testing.T) {
	for _, tc := range identityWriteCalls() {
		t.Run(tc.name, func(t *testing.T) {
			client := &activatingIdentityClient{EnvIdentityClientMock: happyIdentityMock()}
			rec := httptest.NewRecorder()

			tc.call(newActivationController(client, nil), rec, tc.request())

			assert.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, 1, client.activations, "one write request is one activation")
		})
	}
}

func TestAgentIdentityWrites_NeverActivateInLegacyDirectMode(t *testing.T) {
	for _, tc := range identityWriteCalls() {
		t.Run(tc.name, func(t *testing.T) {
			// A plain client has no activation capability, as in LEGACY_DIRECT.
			rec := httptest.NewRecorder()

			tc.call(newActivationController(happyIdentityMock(), nil), rec, tc.request())

			assert.Equal(t, tc.status, rec.Code, rec.Body.String())
		})
	}
}

// A failed activation is reported, says the change was saved and that doing it
// again is safe, and does not leak the cause.
func TestAgentIdentityWrites_ActivationFailureIsReportedAsSavedButNotActive(t *testing.T) {
	for _, tc := range identityWriteCalls() {
		t.Run(tc.name, func(t *testing.T) {
			client := &activatingIdentityClient{
				EnvIdentityClientMock: happyIdentityMock(),
				err:                   errors.New("apply failed at https://cp.internal.svc/gateways/gw-1/apply"),
			}
			rec := httptest.NewRecorder()

			tc.call(newActivationController(client, nil), rec, tc.request())

			require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "was saved but could not be activated")
			assert.Contains(t, rec.Body.String(), "Try the same action again")
			assert.NotContains(t, rec.Body.String(), "cp.internal")
			assert.Equal(t, 1, client.activations)
		})
	}
}

// An environment that receives its identity by promotion refuses every write with
// a 409 that says where to manage roles and groups, and reaches no identity client.
func TestAgentIdentityWrites_PromotedEnvironmentIsAConflictNotARetry(t *testing.T) {
	for _, tc := range identityWriteCalls() {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			tc.call(newActivationController(nil, thundersvc.ErrIdentityEnvironmentUnsupported), rec, tc.request())

			require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "receives its identity by promotion")
			assert.NotContains(t, rec.Body.String(), "retry")
		})
	}

	// A different resolver failure stays a retryable 503.
	rec := httptest.NewRecorder()
	call := identityWriteCalls()[0]
	call.call(newActivationController(nil, errors.New("thunder not provisioned")), rec, call.request())
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// Creating a role that also ensures resource servers activates once, after the
// role writes, and the ensure itself never activates.
func TestAgentIdentityCreateRole_WithResourceServersActivatesOnceAfterTheWrites(t *testing.T) {
	ghUUID := uuid.New()
	var order []string
	mock := happyIdentityMock()
	mock.EnsureProxyResourceServerFunc = func(_ context.Context, handle, _, _ string, _ []string) (string, error) {
		order = append(order, "ensure:"+handle)
		return "rs-" + handle, nil
	}
	mock.AddRolePermissionsFunc = func(context.Context, string, thundersvc.RolePermissionRequest) error {
		order = append(order, "permissions")
		return nil
	}
	client := &activatingIdentityClient{EnvIdentityClientMock: mock}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
			return client, nil
		},
	}
	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) {
			return &models.MCPProxy{UUID: ghUUID, Handle: handle}, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(_ context.Context, id uuid.UUID, action string) (*models.MCPProxyScope, error) {
			return &models.MCPProxyScope{MCPProxyUUID: id, Action: action}, nil
		},
	}
	ctrl := NewAgentIdentityController(resolver, &repomocks.AgentThunderClientRepositoryMock{}, proxyRepo, scopeRepo, stubRSIdentifierResolver{})
	call := writeCall{method: http.MethodPost, path: "/roles", body: `{"name":"reader","scopes":["gh:read","gh:write"]}`}

	rec := httptest.NewRecorder()
	ctrl.CreateRole(rec, call.request())

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"ensure:gh", "permissions"}, order)
	assert.Equal(t, 1, client.activations, "the ensure did not activate, the role path did, once")
}
