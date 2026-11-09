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
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
)

// lookupPageSize is the page size for reading an environment's roles, groups and members.
const lookupPageSize = 100

// AgentIdentityRoleReader optionally resolves an agent's roles in one environment read.
type AgentIdentityRoleReader interface {
	ListAgentRoles(ctx context.Context, organizationID, environment, agentID string) ([]thundersvc.ThunderRole, error)
}

// AgentIdentityGroupReader optionally resolves an agent's groups in one environment read.
type AgentIdentityGroupReader interface {
	ListAgentGroups(ctx context.Context, organizationID, environment, agentID string) ([]thundersvc.ThunderGroup, error)
}

// agentRolesViaProvider returns the roles an AgentID holds in one environment when roles are kept on
// the identity control plane: the roles the environment runs that name the agent as an assignee. The
// environment's own Thunder is never used, as it does not exist in that mode.
func agentRolesViaProvider(
	ctx context.Context, provider IdentityRoleProvider, organizationID, environment, agentID string,
) ([]thundersvc.ThunderRole, error) {
	if provider == nil {
		return nil, ErrIdentityRoleUnavailable
	}
	if reader, ok := provider.(AgentIdentityRoleReader); ok {
		roles, err := reader.ListAgentRoles(ctx, organizationID, environment, agentID)
		if err != nil {
			return nil, fmt.Errorf("list the agent's roles: %w", err)
		}
		return roles, nil
	}
	held := []thundersvc.ThunderRole{}
	for offset := 0; ; offset += lookupPageSize {
		page, err := provider.ListRoles(ctx, IdentityRoleListRequest{
			OrganizationID: organizationID, Environment: environment, Offset: offset, Limit: lookupPageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("list the environment's roles: %w", err)
		}
		for _, role := range page.Roles {
			assignments, err := provider.GetRoleAssignments(ctx, IdentityRoleRef{
				OrganizationID: organizationID, Environment: environment, RoleID: role.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("read the assignees of role %q: %w", role.ID, err)
			}
			if assignments == nil {
				continue
			}
			for _, entry := range assignments.Agents {
				if entry.ID == agentID {
					held = append(held, role)
					break
				}
			}
		}
		if len(page.Roles) < lookupPageSize || offset+len(page.Roles) >= page.Total {
			return held, nil
		}
	}
}

// agentGroupsViaProvider returns the groups an AgentID belongs to in one environment when groups are
// kept on the identity control plane: the groups the environment runs that list the agent as a member.
func agentGroupsViaProvider(
	ctx context.Context, provider IdentityGroupProvider, organizationID, environment, agentID string,
) ([]thundersvc.ThunderGroup, error) {
	if provider == nil {
		return nil, ErrIdentityGroupUnavailable
	}
	if reader, ok := provider.(AgentIdentityGroupReader); ok {
		groups, err := reader.ListAgentGroups(ctx, organizationID, environment, agentID)
		if err != nil {
			return nil, fmt.Errorf("list the agent's groups: %w", err)
		}
		return groups, nil
	}
	held := []thundersvc.ThunderGroup{}
	for offset := 0; ; offset += lookupPageSize {
		page, err := provider.ListGroups(ctx, IdentityGroupListRequest{
			OrganizationID: organizationID, Environment: environment, Offset: offset, Limit: lookupPageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("list the environment's groups: %w", err)
		}
		for _, group := range page.Groups {
			isMember, err := groupHasAgent(ctx, provider, organizationID, environment, group.ID, agentID)
			if err != nil {
				return nil, fmt.Errorf("read the members of group %q: %w", group.ID, err)
			}
			if isMember {
				held = append(held, group)
			}
		}
		if len(page.Groups) < lookupPageSize || offset+len(page.Groups) >= page.Total {
			return held, nil
		}
	}
}

func groupHasAgent(ctx context.Context, provider IdentityGroupProvider, organizationID, environment, groupID, agentID string) (bool, error) {
	for offset := 0; ; offset += lookupPageSize {
		page, err := provider.ListGroupMembers(ctx, IdentityGroupMembersRequest{
			IdentityGroupRef: IdentityGroupRef{OrganizationID: organizationID, Environment: environment, GroupID: groupID},
			Offset:           offset, Limit: lookupPageSize,
		})
		if err != nil {
			return false, err
		}
		for _, member := range page.Members {
			if member.Type == "agent" && member.ID == agentID {
				return true, nil
			}
		}
		if len(page.Members) < lookupPageSize || offset+len(page.Members) >= page.Total {
			return false, nil
		}
	}
}
