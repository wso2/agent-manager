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

// Package identityv1 is the identity.directory/v1 capability: the agent-identity directory of an organization's
// environment, where the roles and groups agents hold live. The core provides it; a module reaches it with
// modkit.Lookup and needs no identity-provider client of its own.
//
// It offers only what the AI Gateway's agent-identity pages use today. In Lite the directory is the platform
// ThunderID, reached for the default environment; For answers ErrNotProvisioned for any other.
package identityv1

import (
	"context"
	"errors"
)

//go:generate moq -rm -fmt goimports -skip-ensure -pkg identitymocks -out identitymocks/identity_mock.go . Directory:DirectoryMock Client:ClientMock

// ID is the capability id.
const ID = "identity.directory/v1"

var (
	// ErrNotProvisioned means the environment has no directory.
	ErrNotProvisioned = errors.New("the environment has no identity directory")
	// ErrNotFound means the group, role or resource server does not exist.
	ErrNotFound = errors.New("not found in the identity directory")
	// ErrInvalid means the directory refused the request's content, such as a
	// member that is not in it.
	ErrInvalid = errors.New("refused by the identity directory")
)

// Names of the directory's own administrator role and group and of the core's system client role, which a module
// must not let callers change.
const (
	AdministratorRoleName   = "Administrator"
	AdministratorsGroupName = "Administrators"
	SystemClientRoleName    = "AMP System Client Thunder Admin"
)

// Group is a group in the directory.
type Group struct {
	ID          string
	OUID        string
	Name        string
	Description string
	CreatedAt   string
	UpdatedAt   string
}

// Role is a role in the directory, with the permissions it grants.
type Role struct {
	ID          string
	OUID        string
	Name        string
	Description string
	Permissions []RolePermission
	IsReadOnly  bool
	CreatedAt   string
	UpdatedAt   string
}

// RolePermission is a set of permissions (scopes) on one resource server.
type RolePermission struct {
	ResourceServerID string
	Permissions      []string
}

// Member is an entry of a group or a role assignment; Type is "user", "group", "app" or "agent".
type Member struct {
	ID   string
	Type string
}

// RoleAssignments are the agents and groups a role is assigned to.
type RoleAssignments struct {
	Agents []Member
	Groups []Group
}

// GroupInput creates or replaces a group in the organization unit OUID.
type GroupInput struct {
	OUID        string
	Name        string
	Description string
}

// RoleInput creates or replaces a role in the organization unit OUID. On create, Permissions are ignored: add
// them with AddRolePermissions.
type RoleInput struct {
	OUID        string
	Name        string
	Description string
	Permissions []RolePermission
}

// ProvisionedAgent is an agent the core has provisioned (or is provisioning) an identity for in an environment.
type ProvisionedAgent struct {
	ProjectName string
	AgentName   string
	// Status is the provisioning status: pending, completed or failed.
	Status string
	// AgentID is the agent's id in the directory; empty until provisioning completes.
	AgentID string
}

// Directory opens the directory of one environment of an organization.
type Directory interface {
	// For returns the directory of the organization's environment; ErrNotProvisioned when it has none.
	For(ctx context.Context, orgID, environment string) (Client, error)
	// ListProvisionedAgents lists the agents of the organization the core provisions identities for in the
	// environment, from the core's own records rather than the directory.
	ListProvisionedAgents(ctx context.Context, orgID, environment string) ([]ProvisionedAgent, error)
}

// Client works on one environment's directory. Errors for a missing group, role or resource server wrap
// ErrNotFound.
type Client interface {
	// DefaultOUID is the organization unit new groups and roles go into.
	DefaultOUID(ctx context.Context) (string, error)

	ListGroups(ctx context.Context, ouID string, offset, limit int) ([]Group, int, error)
	GetGroup(ctx context.Context, groupID string) (Group, error)
	CreateGroup(ctx context.Context, in GroupInput) (Group, error)
	UpdateGroup(ctx context.Context, groupID string, in GroupInput) (Group, error)
	DeleteGroup(ctx context.Context, groupID string) error
	ListGroupMembers(ctx context.Context, groupID string, offset, limit int) ([]Member, int, error)
	AddGroupMembers(ctx context.Context, groupID string, members []Member) error
	RemoveGroupMembers(ctx context.Context, groupID string, members []Member) error
	GetGroupRoles(ctx context.Context, groupID string) ([]Role, error)

	ListRoles(ctx context.Context, ouID string, offset, limit int) ([]Role, int, error)
	GetRole(ctx context.Context, roleID string) (Role, error)
	CreateRole(ctx context.Context, in RoleInput) (Role, error)
	UpdateRole(ctx context.Context, roleID string, in RoleInput) (Role, error)
	DeleteRole(ctx context.Context, roleID string) error
	AddRolePermissions(ctx context.Context, roleID string, p RolePermission) error
	RemoveRolePermissions(ctx context.Context, roleID string, p RolePermission) error
	GetRoleAssignments(ctx context.Context, roleID string) (RoleAssignments, error)
	AddRoleAssignees(ctx context.Context, roleID string, assignees []Member) error
	RemoveRoleAssignees(ctx context.Context, roleID string, assignees []Member) error

	// EnsureResourceServer makes sure the resource server named handle exists with the given actions, and returns
	// its id. It never removes actions.
	EnsureResourceServer(ctx context.Context, handle, displayName, identifier string, actions []string) (string, error)
	// RemoveResourceServerAction removes one action and returns the resource server's id; "" when the server
	// does not exist.
	RemoveResourceServerAction(ctx context.Context, handle, action string) (string, error)
	// DeleteResourceServer deletes the resource server; a missing one is success.
	DeleteResourceServer(ctx context.Context, handle string) error
}
