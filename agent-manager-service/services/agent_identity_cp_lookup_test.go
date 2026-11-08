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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clientmocks "github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	repomocks "github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// cpRoleLookup answers the two calls an agent's role lookup makes, from fixed data.
type cpRoleLookup struct {
	IdentityRoleProvider
	roles     []thundersvc.ThunderRole
	agentsOf  map[string][]string // role ID -> agent assignees
	listCalls int
}

func (p *cpRoleLookup) ListRoles(_ context.Context, request IdentityRoleListRequest) (IdentityRoleList, error) {
	p.listCalls++
	end := min(request.Offset+request.Limit, len(p.roles))
	start := min(request.Offset, end)
	return IdentityRoleList{Roles: p.roles[start:end], Total: len(p.roles)}, nil
}

func (p *cpRoleLookup) GetRoleAssignments(_ context.Context, ref IdentityRoleRef) (*thundersvc.AgentRoleAssignments, error) {
	var assignments thundersvc.AgentRoleAssignments
	for _, id := range p.agentsOf[ref.RoleID] {
		assignments.Agents = append(assignments.Agents, thundersvc.AssignmentEntry{ID: id, Type: "agent"})
	}
	return &assignments, nil
}

// cpGroupLookup answers an agent's group lookup from fixed data.
type cpGroupLookup struct {
	IdentityGroupProvider
	groups  []thundersvc.ThunderGroup
	members map[string][]thundersvc.GroupMember
}

func (p *cpGroupLookup) ListGroups(_ context.Context, request IdentityGroupListRequest) (IdentityGroupList, error) {
	end := min(request.Offset+request.Limit, len(p.groups))
	start := min(request.Offset, end)
	return IdentityGroupList{Groups: p.groups[start:end], Total: len(p.groups)}, nil
}

func (p *cpGroupLookup) ListGroupMembers(_ context.Context, request IdentityGroupMembersRequest) (IdentityGroupMembers, error) {
	all := p.members[request.GroupID]
	end := min(request.Offset+request.Limit, len(all))
	start := min(request.Offset, end)
	return IdentityGroupMembers{Members: all[start:end], Total: len(all)}, nil
}

// An environment's own Thunder does not exist in CP_MANAGED mode, so looking one up is a failure.
func failingEnvResolver(t *testing.T) *clientmocks.EnvThunderResolverMock {
	return &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
			t.Error("the environment's own Thunder must not be used when roles and groups are on the control plane")
			return nil, thundersvc.ErrThunderNotProvisioned
		},
	}
}

func boundAgentRepo() *repomocks.AgentThunderClientRepositoryMock {
	return &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New(), ThunderAgentID: "agent-1"}, nil
		},
	}
}

func TestGetAgentRolesThroughTheControlPlane(t *testing.T) {
	provider := &cpRoleLookup{
		roles: []thundersvc.ThunderRole{{ID: "r1", Name: "reader"}, {ID: "r2", Name: "writer"}, {ID: "r3", Name: "admin"}},
		agentsOf: map[string][]string{
			"r1": {"agent-1", "agent-2"},
			"r2": {"agent-2"},
			"r3": {"agent-1"},
		},
	}
	SetIdentityRoleProvider(thundersvc.IdentityManagementCPManaged, provider)
	t.Cleanup(func() { SetIdentityRoleProvider(thundersvc.IdentityManagementLegacyDirect, nil) })

	svc := newTestProvisioningService(boundAgentRepo(), failingEnvResolver(t), &clientmocks.SecretManagementClientMock{})
	roles, err := svc.GetAgentRoles(context.Background(), "org-1", "proj", "my-agent", "qa")
	require.NoError(t, err)
	require.Len(t, roles, 2, "only the roles that name the agent")
	assert.Equal(t, []string{"r1", "r3"}, []string{roles[0].ID, roles[1].ID})
}

func TestGetAgentRolesThroughTheControlPlaneReadsEveryPage(t *testing.T) {
	roles := make([]thundersvc.ThunderRole, 0, lookupPageSize+5)
	agentsOf := map[string][]string{}
	for i := range lookupPageSize + 5 {
		id := uuid.NewString()
		roles = append(roles, thundersvc.ThunderRole{ID: id})
		if i == lookupPageSize+2 { // on the second page
			agentsOf[id] = []string{"agent-1"}
		}
	}
	provider := &cpRoleLookup{roles: roles, agentsOf: agentsOf}
	got, err := agentRolesViaProvider(context.Background(), provider, "org-1", "qa", "agent-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 2, provider.listCalls)
}

func TestGetAgentGroupsThroughTheControlPlane(t *testing.T) {
	provider := &cpGroupLookup{
		groups: []thundersvc.ThunderGroup{{ID: "g1", Name: "ops"}, {ID: "g2", Name: "dev"}},
		members: map[string][]thundersvc.GroupMember{
			"g1": {{ID: "user-1", Type: "user"}, {ID: "agent-1", Type: "agent"}},
			"g2": {{ID: "agent-1", Type: "user"}}, // same ID, but not an agent
		},
	}
	SetIdentityGroupProvider(thundersvc.IdentityManagementCPManaged, provider)
	t.Cleanup(func() { SetIdentityGroupProvider(thundersvc.IdentityManagementLegacyDirect, nil) })

	svc := newTestProvisioningService(boundAgentRepo(), failingEnvResolver(t), &clientmocks.SecretManagementClientMock{})
	groups, err := svc.GetAgentGroups(context.Background(), "org-1", "proj", "my-agent", "qa")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "g1", groups[0].ID)
}

func TestAgentLookupWithoutAProviderIsUnavailable(t *testing.T) {
	_, err := agentRolesViaProvider(context.Background(), nil, "org-1", "qa", "agent-1")
	assert.ErrorIs(t, err, ErrIdentityRoleUnavailable)
	_, err = agentGroupsViaProvider(context.Background(), nil, "org-1", "qa", "agent-1")
	assert.ErrorIs(t, err, ErrIdentityGroupUnavailable)
}
