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

package gwctl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1/identitymocks"
)

func agentIdentityGetRequest(path string, pathValues map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("orgName", "o1")
	req.SetPathValue("envName", "dev")
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	return req.WithContext(modkit.WithOrgID(req.Context(), "ou-org"))
}

func controllerWithClient(client *identitymocks.ClientMock) AgentIdentityController {
	resolver := &identitymocks.DirectoryMock{
		ForFunc: func(_ context.Context, _, _ string) (identityv1.Client, error) { return client, nil },
	}
	return NewAgentIdentityController(resolver, nil, nil, stubRSIdentifierResolver{})
}

func TestAgentIdentityListGroupsUsesJSONFieldNames(t *testing.T) {
	client := &identitymocks.ClientMock{
		DefaultOUIDFunc: func(context.Context) (string, error) { return "ou-env", nil },
		ListGroupsFunc: func(context.Context, string, int, int) ([]identityv1.Group, int, error) {
			return []identityv1.Group{{ID: "g1", OUID: "ou-env", Name: "Readers"}}, 1, nil
		},
	}
	w := httptest.NewRecorder()
	controllerWithClient(client).ListGroups(w, agentIdentityGetRequest(
		"/orgs/o1/environments/dev/agent-identities/groups", nil,
	))

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t,
		`{"groups":[{"id":"g1","ouId":"ou-env","name":"Readers"}],"total":1,"offset":0,"limit":20}`,
		w.Body.String())
}

func TestAgentIdentityRoleResponsesUseJSONFieldNames(t *testing.T) {
	role := identityv1.Role{
		ID:          "r1",
		Name:        "readers",
		Permissions: []identityv1.RolePermission{{ResourceServerID: "rs1", Permissions: []string{"gh:read"}}},
	}
	client := &identitymocks.ClientMock{
		GetRoleFunc: func(context.Context, string) (identityv1.Role, error) { return role, nil },
		GetRoleAssignmentsFunc: func(context.Context, string) (identityv1.RoleAssignments, error) {
			return identityv1.RoleAssignments{
				Agents: []identityv1.Member{{ID: "a1", Type: "agent"}},
				Groups: []identityv1.Group{{ID: "g1", Name: "Readers"}},
			}, nil
		},
	}
	ctrl := controllerWithClient(client)
	params := map[string]string{"roleID": "r1"}

	w := httptest.NewRecorder()
	ctrl.GetRole(w, agentIdentityGetRequest("/orgs/o1/environments/dev/agent-identities/roles/r1", params))
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t,
		`{"id":"r1","name":"readers","isReadOnly":false,"permissions":[{"resourceServerId":"rs1","permissions":["gh:read"]}]}`,
		w.Body.String())

	w = httptest.NewRecorder()
	ctrl.GetRoleAssignments(w, agentIdentityGetRequest(
		"/orgs/o1/environments/dev/agent-identities/roles/r1/assignments", params,
	))
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t,
		`{"agents":[{"id":"a1","type":"agent"}],"groups":[{"id":"g1","name":"Readers"}]}`,
		w.Body.String())
}
