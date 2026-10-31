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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// recordingSyncTrigger records the changes a controller reports.
type recordingSyncTrigger struct {
	calls     []string
	assignees []thundersvc.AssignmentEntry
	members   []thundersvc.GroupMember
	// afterDelete counts the functions BeforeXDelete returned that were then called.
	afterDelete int
}

func (r *recordingSyncTrigger) RoleChanged(_ context.Context, ouID, env, roleID string) {
	r.calls = append(r.calls, "role-changed:"+ouID+":"+env+":"+roleID)
}

func (r *recordingSyncTrigger) RoleAssigneesChanged(_ context.Context, ouID, env, roleID string, assignees []thundersvc.AssignmentEntry) {
	r.calls = append(r.calls, "role-assignees-changed:"+ouID+":"+env+":"+roleID)
	r.assignees = assignees
}

func (r *recordingSyncTrigger) GroupChanged(_ context.Context, ouID, env, groupID string) {
	r.calls = append(r.calls, "group-changed:"+ouID+":"+env+":"+groupID)
}

func (r *recordingSyncTrigger) GroupMembersChanged(_ context.Context, ouID, env, groupID string, members []thundersvc.GroupMember) {
	r.calls = append(r.calls, "group-members-changed:"+ouID+":"+env+":"+groupID)
	r.members = members
}

func (r *recordingSyncTrigger) BeforeRoleDelete(_ context.Context, _, _, roleID string) func() {
	r.calls = append(r.calls, "before-role-delete:"+roleID)
	return func() { r.afterDelete++ }
}

func (r *recordingSyncTrigger) BeforeGroupDelete(_ context.Context, _, _, groupID string) func() {
	r.calls = append(r.calls, "before-group-delete:"+groupID)
	return func() { r.afterDelete++ }
}

func (r *recordingSyncTrigger) MCPProxyChanged(context.Context, string, string, string) {
	r.calls = append(r.calls, "mcp-proxy-changed")
}

// syncRequest is the call's request with the path ids the handlers read, which
// the shared table spells differently.
func syncRequest(tc writeCall) *http.Request {
	req := tc.request()
	req.SetPathValue("groupID", "g1")
	req.SetPathValue("roleID", "r1")
	return req
}

func newSyncController(client thundersvc.EnvIdentityClient, trigger services.AgentIdentitySyncTrigger) AgentIdentityController {
	ctrl := newActivationController(client, nil)
	if setter, ok := ctrl.(services.AgentIdentitySyncTriggerSetter); ok && trigger != nil {
		setter.SetAgentIdentitySyncTrigger(trigger)
	}
	return ctrl
}

func TestAgentIdentityWrites_TellTheSyncTriggerWhatChangedOnlyOnceItIsActive(t *testing.T) {
	// The environment the roles are managed in is the first environment of the
	// agents' pipelines, which is the one in the request path.
	expected := map[string][]string{
		"create group":         nil, // a new group has no members
		"update group":         {"group-changed:ou-org:dev:g1"},
		"delete group":         {"before-group-delete:g1"},
		"add group members":    {"group-members-changed:ou-org:dev:g1"},
		"remove group members": {"group-members-changed:ou-org:dev:g1"},
		"create role":          nil, // a new role has no holders
		"update role metadata": {"role-changed:ou-org:dev:r1"},
		"update role scopes":   {"role-changed:ou-org:dev:r1"},
		"delete role":          {"before-role-delete:r1"},
		"add role assignees":   {"role-assignees-changed:ou-org:dev:r1"},
		// Promotion is additive: an unassign does not remove the role in later
		// environments, but it still marks the agent so its carried set is current.
		"remove role assignees": {"role-assignees-changed:ou-org:dev:r1"},
	}
	for _, tc := range identityWriteCalls() {
		t.Run(tc.name, func(t *testing.T) {
			trigger := &recordingSyncTrigger{}
			client := &activatingIdentityClient{EnvIdentityClientMock: happyIdentityMock()}
			rec := httptest.NewRecorder()

			tc.call(newSyncController(client, trigger), rec, syncRequest(tc))

			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, expected[tc.name], trigger.calls)
			if strings.HasPrefix(tc.name, "delete") {
				assert.Equal(t, 1, trigger.afterDelete, "the agents are marked once the delete has succeeded")
			}
		})
	}
}

func TestAgentIdentityWrites_PassTheChangedAssigneesAndMembers(t *testing.T) {
	trigger := &recordingSyncTrigger{}
	for _, tc := range identityWriteCalls() {
		if tc.name != "add role assignees" && tc.name != "add group members" {
			continue
		}
		rec := httptest.NewRecorder()
		tc.call(newSyncController(&activatingIdentityClient{EnvIdentityClientMock: happyIdentityMock()}, trigger), rec, syncRequest(tc))
		require.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, []thundersvc.AssignmentEntry{{ID: "a1", Type: "agent"}}, trigger.assignees)
	assert.Equal(t, []thundersvc.GroupMember{{ID: "a1", Type: "agent"}}, trigger.members)
}

func TestAgentIdentityWrites_DoNotTellTheTriggerWhenTheChangeIsNotActiveOrNotSaved(t *testing.T) {
	for _, tc := range identityWriteCalls() {
		t.Run(tc.name+" not activated", func(t *testing.T) {
			trigger := &recordingSyncTrigger{}
			client := &activatingIdentityClient{EnvIdentityClientMock: happyIdentityMock(), err: errors.New("gateway unavailable")}
			rec := httptest.NewRecorder()

			tc.call(newSyncController(client, trigger), rec, syncRequest(tc))

			require.Equal(t, http.StatusBadGateway, rec.Code, "the write is saved but not active, as before")
			assert.Zero(t, trigger.afterDelete, "nothing is marked for a change the gateway does not run")
			for _, call := range trigger.calls {
				assert.True(t, strings.HasPrefix(call, "before-"), "only the read made before a delete happens: %s", call)
			}
		})
	}

	t.Run("a failed write", func(t *testing.T) {
		trigger := &recordingSyncTrigger{}
		mock := happyIdentityMock()
		mock.UpdateRoleFunc = func(context.Context, string, thundersvc.UpdateRoleRequest) (*thundersvc.ThunderRole, error) {
			return nil, errors.New("boom")
		}
		mock.DeleteRoleFunc = func(context.Context, string) error { return errors.New("boom") }
		for _, tc := range identityWriteCalls() {
			if tc.name != "update role metadata" && tc.name != "delete role" {
				continue
			}
			rec := httptest.NewRecorder()
			tc.call(newSyncController(&activatingIdentityClient{EnvIdentityClientMock: mock}, trigger), rec, syncRequest(tc))
			require.Equal(t, http.StatusInternalServerError, rec.Code)
		}
		assert.Zero(t, trigger.afterDelete)
		assert.NotContains(t, trigger.calls, "role-changed:ou-org:dev:r1")
	})
}

func TestAgentIdentityWrites_WorkWithoutASyncTrigger(t *testing.T) {
	for _, tc := range identityWriteCalls() {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.call(newSyncController(&activatingIdentityClient{EnvIdentityClientMock: happyIdentityMock()}, nil), rec, syncRequest(tc))
			require.Equal(t, tc.status, rec.Code, "a deployment that promotes no identity is unaffected")
		})
	}
}
