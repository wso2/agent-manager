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

// IdentityRoleRef names one role as seen from one environment.
type IdentityRoleRef struct {
	// OrganizationID is the organization unit ID (OUID); always set.
	OrganizationID string
	// Environment is the environment the request is made in.
	Environment string
	RoleID      string
}

// IdentityRoleListRequest asks for one page of the roles of an environment.
type IdentityRoleListRequest struct {
	OrganizationID string
	Environment    string
	Offset, Limit  int
}

// IdentityRoleList is one page of roles and the total number of roles.
type IdentityRoleList struct {
	Roles []thundersvc.ThunderRole
	Total int
}

// IdentityRoleWrite creates or updates a role in an environment.
type IdentityRoleWrite struct {
	OrganizationID string
	Environment    string
	// RoleID is set for an update and empty for a create.
	RoleID string
	// Name and Description change the role when set. A create requires Name.
	Name        *string
	Description *string
	// Permissions are the role's MCP scope permissions grouped by resource server. For a create they
	// are the role's permissions. For an update, nil leaves the role's permissions as they are and a
	// non-nil value, even an empty one, replaces them: scopes missing from it are removed.
	Permissions *[]thundersvc.RolePermissionRequest
}

// IdentityRoleAssignmentWrite adds or removes the assignees of a role: agents, and groups of the
// environment.
type IdentityRoleAssignmentWrite struct {
	OrganizationID string
	Environment    string
	RoleID         string
	Assignments    []thundersvc.AssignmentEntry
}

// IdentityRoleProvider is implemented by deployments that keep roles on an identity control plane
// instead of in each environment's own Thunder. It is used only in CP_MANAGED identity mode, and
// covers the whole of agent identity role management: listing, reading, creating, updating and
// deleting roles, the MCP scope permissions on them, and the agents assigned to them.
//
// Roles are control plane objects. ListRoles and GetRole read them from the control plane, a write
// goes to the control plane first and is then applied to request.Environment, so the environment's
// gateway runs the role as the control plane holds it. Resource server IDs in a role's permissions
// are the ones MCPResourceServerProvider returned for the same environment. A role the environment
// does not run is not listed there, and a write for it is refused as not found, so a role is only
// ever changed through the environment it was made in.
//
// Roles a deployment hides from agent identity management, such as its native administrator roles,
// are not returned by ListRoles. A role that does not exist, or is not in the environment, is
// reported as ErrIdentityRoleNotFound. Every method is idempotent where repeating it can be.
//
// DeleteRole removes the role from the control plane and from the environment. GetRoleAssignments
// returns the agents assigned to the role and the groups it can resolve to groups of the
// environment; a deployment that cannot hold groups on the control plane returns the agents and no
// groups, not an error. AddRoleAssignees and RemoveRoleAssignees change the assignees on the control
// plane and apply the role. Only adding a group assignee needs the groups of the environment
// (IdentityGroupProvider), and a deployment that cannot hold groups reports
// ErrIdentityGroupUnsupported for it. Removing an assignee that is no longer there succeeds.
type IdentityRoleProvider interface {
	ListRoles(ctx context.Context, request IdentityRoleListRequest) (IdentityRoleList, error)
	GetRole(ctx context.Context, ref IdentityRoleRef) (*thundersvc.ThunderRole, error)
	CreateRole(ctx context.Context, write IdentityRoleWrite) (*thundersvc.ThunderRole, error)
	UpdateRole(ctx context.Context, write IdentityRoleWrite) (*thundersvc.ThunderRole, error)
	DeleteRole(ctx context.Context, ref IdentityRoleRef) error
	GetRoleAssignments(ctx context.Context, ref IdentityRoleRef) (*thundersvc.AgentRoleAssignments, error)
	AddRoleAssignees(ctx context.Context, write IdentityRoleAssignmentWrite) error
	RemoveRoleAssignees(ctx context.Context, write IdentityRoleAssignmentWrite) error
}

var (
	// ErrIdentityRoleNotFound means the role does not exist, or is not in the environment.
	ErrIdentityRoleNotFound = errors.New("identity role not found")
	// ErrIdentityRoleUnavailable means CP_MANAGED identity mode is on and no role provider is
	// configured. The per-environment Thunder is never used instead.
	ErrIdentityRoleUnavailable = errors.New("identity role provider is not configured")
)

// identityRoleSetting is the process-wide choice made once at startup, as for MCP resource servers.
var identityRoleSetting struct {
	mu       sync.RWMutex
	mode     thundersvc.IdentityManagementMode
	provider IdentityRoleProvider
}

// SetIdentityRoleProvider chooses where roles are kept. In LEGACY_DIRECT mode (the default) roles
// are in each environment's own Thunder and provider is ignored. In CP_MANAGED mode role listing,
// reading and writing go to provider; with a nil provider they report ErrIdentityRoleUnavailable.
func SetIdentityRoleProvider(mode thundersvc.IdentityManagementMode, provider IdentityRoleProvider) {
	identityRoleSetting.mu.Lock()
	defer identityRoleSetting.mu.Unlock()
	identityRoleSetting.mode = mode
	identityRoleSetting.provider = provider
	switch {
	case mode != thundersvc.IdentityManagementCPManaged && provider != nil:
		slog.Warn("identity role provider ignored: identity management mode is not CP_MANAGED", "mode", string(mode))
	case mode == thundersvc.IdentityManagementCPManaged && provider == nil:
		slog.Warn("CP_MANAGED identity mode without an identity role provider: role listing and scope assignment are unavailable until one is configured")
	}
}

// IdentityRolesViaControlPlane reports whether roles are kept on the control plane (CP_MANAGED mode)
// and, if so, the provider, which is nil when none is configured.
func IdentityRolesViaControlPlane() (bool, IdentityRoleProvider) {
	identityRoleSetting.mu.RLock()
	defer identityRoleSetting.mu.RUnlock()
	if identityRoleSetting.mode != thundersvc.IdentityManagementCPManaged {
		return false, nil
	}
	return true, identityRoleSetting.provider
}
