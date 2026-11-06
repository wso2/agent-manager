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
	"log/slog"
	"sync"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
)

// IdentityGroupRef names one group as seen from one environment.
type IdentityGroupRef struct {
	// OrganizationID is the organization unit ID (OUID); always set.
	OrganizationID string
	// Environment is the environment the request is made in.
	Environment string
	GroupID     string
}

// IdentityGroupListRequest asks for one page of the groups of an environment.
type IdentityGroupListRequest struct {
	OrganizationID string
	Environment    string
	Offset, Limit  int
}

// IdentityGroupList is one page of groups and the total number of groups.
type IdentityGroupList struct {
	Groups []thundersvc.ThunderGroup
	Total  int
}

// IdentityGroupWrite creates or updates a group in an environment.
type IdentityGroupWrite struct {
	OrganizationID string
	Environment    string
	// GroupID is set for an update and empty for a create.
	GroupID string
	// Name and Description change the group when set. A create requires Name.
	Name        *string
	Description *string
}

// IdentityGroupMembersRequest asks for one page of the members of a group.
type IdentityGroupMembersRequest struct {
	IdentityGroupRef
	Offset, Limit int
}

// IdentityGroupMembers is one page of group members and the total number of members.
type IdentityGroupMembers struct {
	Members []thundersvc.GroupMember
	Total   int
}

// IdentityGroupMembersWrite adds or removes members of a group. Members are the agents of the
// environment.
type IdentityGroupMembersWrite struct {
	IdentityGroupRef
	Members []thundersvc.GroupMember
}

// IdentityGroupProvider is implemented by deployments that manage the groups of an environment
// through an identity control plane instead of the environment's own Thunder. It is used only in
// CP_MANAGED identity mode and covers the whole of agent identity group management: listing,
// reading, creating, updating and deleting groups, their members (agents), and the roles a group
// holds. Every call names the environment, and a group is only seen and changed in the environment
// it was made in.
//
// A group the environment does not hold is reported as ErrIdentityGroupNotFound. Groups a
// deployment hides, such as its native administrators group, are not returned by ListGroups. A
// deployment whose identity platform cannot hold groups on the control plane returns
// ErrIdentityGroupUnsupported from every method, and Agent Manager reports that to the caller.
type IdentityGroupProvider interface {
	ListGroups(ctx context.Context, request IdentityGroupListRequest) (IdentityGroupList, error)
	GetGroup(ctx context.Context, ref IdentityGroupRef) (*thundersvc.ThunderGroup, error)
	CreateGroup(ctx context.Context, write IdentityGroupWrite) (*thundersvc.ThunderGroup, error)
	UpdateGroup(ctx context.Context, write IdentityGroupWrite) (*thundersvc.ThunderGroup, error)
	DeleteGroup(ctx context.Context, ref IdentityGroupRef) error
	ListGroupMembers(ctx context.Context, request IdentityGroupMembersRequest) (IdentityGroupMembers, error)
	AddGroupMembers(ctx context.Context, write IdentityGroupMembersWrite) error
	RemoveGroupMembers(ctx context.Context, write IdentityGroupMembersWrite) error
	GetGroupRoles(ctx context.Context, ref IdentityGroupRef) ([]thundersvc.ThunderRole, error)
}

var (
	// ErrIdentityGroupNotFound means the group does not exist, or is not in the environment.
	ErrIdentityGroupNotFound = errors.New("identity group not found")
	// ErrIdentityGroupUnavailable means CP_MANAGED identity mode is on and no group provider is
	// configured. The per-environment Thunder is never used instead.
	ErrIdentityGroupUnavailable = errors.New("identity group provider is not configured")
	// ErrIdentityGroupUnsupported means the identity platform cannot hold groups on the control plane.
	ErrIdentityGroupUnsupported = errors.New("the identity platform does not support groups through the control plane")
)

// identityGroupSetting is the process-wide choice made once at startup, as for roles.
var identityGroupSetting struct {
	mu       sync.RWMutex
	mode     thundersvc.IdentityManagementMode
	provider IdentityGroupProvider
}

// SetIdentityGroupProvider chooses where groups are kept. In LEGACY_DIRECT mode (the default) groups
// are in each environment's own Thunder and provider is ignored. In CP_MANAGED mode group
// management goes to provider; with a nil provider it reports ErrIdentityGroupUnavailable.
func SetIdentityGroupProvider(mode thundersvc.IdentityManagementMode, provider IdentityGroupProvider) {
	identityGroupSetting.mu.Lock()
	defer identityGroupSetting.mu.Unlock()
	identityGroupSetting.mode = mode
	identityGroupSetting.provider = provider
	switch {
	case mode != thundersvc.IdentityManagementCPManaged && provider != nil:
		slog.Warn("identity group provider ignored: identity management mode is not CP_MANAGED", "mode", string(mode))
	case mode == thundersvc.IdentityManagementCPManaged && provider == nil:
		slog.Warn("CP_MANAGED identity mode without an identity group provider: group management is unavailable until one is configured")
	}
}

// IdentityGroupsViaControlPlane reports whether groups are managed through the control plane
// (CP_MANAGED mode) and, if so, the provider, which is nil when none is configured.
func IdentityGroupsViaControlPlane() (bool, IdentityGroupProvider) {
	identityGroupSetting.mu.RLock()
	defer identityGroupSetting.mu.RUnlock()
	if identityGroupSetting.mode != thundersvc.IdentityManagementCPManaged {
		return false, nil
	}
	return true, identityGroupSetting.provider
}
