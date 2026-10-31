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
)

// ThunderUser represents a user in Thunder.
type ThunderUser struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	OuID       string            `json:"ouId,omitempty"`
	Display    string            `json:"display,omitempty"`
	Attributes map[string]any    `json:"attributes,omitempty"`
	Groups     []ThunderGroupRef `json:"groups,omitempty"`
	CreatedAt  string            `json:"createdAt,omitempty"`
	UpdatedAt  string            `json:"updatedAt,omitempty"`
}

// ThunderClaim is kept for compatibility in request bodies.
type ThunderClaim struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ThunderCred is kept for compatibility in request bodies.
type ThunderCred struct {
	Password string `json:"password"`
}

type ThunderGroupRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateUserRequest is the payload for POST /users.
// Password is kept out of Attributes in memory to avoid accidental logging;
// MarshalJSON injects it into the attributes map only when serializing for Thunder.
type CreateUserRequest struct {
	OuID       string            `json:"ouId,omitempty"`
	Type       string            `json:"type"`
	Attributes map[string]string `json:"attributes"`
	Password   string            `json:"-"`
}

func (r CreateUserRequest) MarshalJSON() ([]byte, error) {
	attrs := make(map[string]string, len(r.Attributes)+1)
	for k, v := range r.Attributes {
		attrs[k] = v
	}
	if r.Password != "" {
		attrs["password"] = r.Password
	}
	type wire struct {
		OuID       string            `json:"ouId,omitempty"`
		Type       string            `json:"type"`
		Attributes map[string]string `json:"attributes"`
	}
	return json.Marshal(wire{OuID: r.OuID, Type: r.Type, Attributes: attrs})
}

// UpdateUserRequest is the payload for PUT /users/{id}.
type UpdateUserRequest struct {
	Attributes map[string]string `json:"attributes,omitempty"`
	OuID       string            `json:"ouId,omitempty"`
	Type       string            `json:"type,omitempty"`
	IsReadOnly bool              `json:"isReadOnly,omitempty"`
}

// UpdateUserCredentialsRequest is the payload for POST /users/{id}/update-credentials,
// Thunder's dedicated credential-change endpoint (separate from UpdateUserRequest; a
// password field inside a regular update is rejected with USR-1028).
type UpdateUserCredentialsRequest struct {
	Credentials map[string]string `json:"credentials"`
}

func (r UpdateUserRequest) MarshalJSON() ([]byte, error) {
	type wire struct {
		OuID       string            `json:"ouId,omitempty"`
		Type       string            `json:"type,omitempty"`
		Attributes map[string]string `json:"attributes,omitempty"`
		IsReadOnly bool              `json:"isReadOnly,omitempty"`
	}
	return json.Marshal(wire{OuID: r.OuID, Type: r.Type, Attributes: r.Attributes, IsReadOnly: r.IsReadOnly})
}

// ThunderGroup represents a user group in Thunder.
type ThunderGroup struct {
	ID          string `json:"id"`
	OuID        string `json:"ouId,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// CreateGroupRequest is the payload for POST /groups.
type CreateGroupRequest struct {
	Name        string `json:"name"`
	OuID        string `json:"ouId"`
	Description string `json:"description,omitempty"`
}

// UpdateGroupRequest is the payload for PUT /groups/{id}.
type UpdateGroupRequest struct {
	OuID        string `json:"ouId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// GroupMember is a single member entry in a group members request.
type GroupMember struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "user" | "group" | "app" | "agent"
}

// GroupMembersRequest is the payload for /groups/{id}/members/add and remove.
type GroupMembersRequest struct {
	Members []GroupMember `json:"members"`
}

// ThunderRole represents a role in Thunder.
type ThunderRole struct {
	ID          string                  `json:"id"`
	OuID        string                  `json:"ouId,omitempty"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Permissions []RolePermissionRequest `json:"permissions,omitempty"`
	IsReadOnly  bool                    `json:"isReadOnly"`
	CreatedAt   string                  `json:"createdAt,omitempty"`
	UpdatedAt   string                  `json:"updatedAt,omitempty"`
}

// CreateRoleRequest is the payload for POST /roles.
type CreateRoleRequest struct {
	Name        string `json:"name"`
	OuID        string `json:"ouId"`
	Description string `json:"description,omitempty"`
}

// UpdateRoleRequest is the payload for PUT /roles/{id}. Thunder's role update is
// a full replace and requires ouId; any permissions omitted from the body are
// dropped, so callers echo the role's current permissions here and let a
// follow-up reconcile adjust them.
type UpdateRoleRequest struct {
	OuID        string                  `json:"ouId"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Permissions []RolePermissionRequest `json:"permissions,omitempty"`
}

// thunderRolePermissionsUpdateBody is used when patching only the permissions via PUT /roles/{id}.
type thunderRolePermissionsUpdateBody struct {
	OuID        string                  `json:"ouId,omitempty"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Permissions []RolePermissionRequest `json:"permissions,omitempty"`
}

// RoleAssignments represents the current assignments on a role.
type RoleAssignments struct {
	Users  []ThunderUser  `json:"users,omitempty"`
	Groups []ThunderGroup `json:"groups,omitempty"`
}

// AgentRoleAssignments represents a role's current assignments with
// agent-identity (environment Thunder) semantics: agent assignees as raw
// assignment entries, group assignees resolved to full groups.
type AgentRoleAssignments struct {
	Agents []AssignmentEntry `json:"agents,omitempty"`
	Groups []ThunderGroup    `json:"groups,omitempty"`
}

// RolePermissionRequest is a single resource-server permissions entry.
type RolePermissionRequest struct {
	ResourceServerID string   `json:"resourceServerId"`
	Permissions      []string `json:"permissions"`
}

// AssignmentEntry is a single entry in a role assignments request.
type AssignmentEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "user" | "group" | "app" | "agent"
}

// RoleAssignmentsRequest is the payload for /roles/{id}/assignments/add and remove.
type RoleAssignmentsRequest struct {
	Assignments []AssignmentEntry `json:"assignments"`
}

// ThunderResourceServer represents a resource server registered in Thunder.
type ThunderResourceServer struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Handle     string            `json:"handle"`
	Identifier string            `json:"identifier"`
	Resources  []ThunderResource `json:"resources,omitempty"`
}

// ThunderResource represents a resource within a resource server.
type ThunderResource struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Handle     string          `json:"handle"`
	Permission string          `json:"permission"`
	Actions    []ThunderAction `json:"actions,omitempty"`
}

// ThunderAction represents an action on a resource.
type ThunderAction struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Handle     string `json:"handle"`
	Permission string `json:"permission"`
}

// ThunderPermission represents one action on a resource server resource.
type ThunderPermission struct {
	Name             string `json:"name"` // permission handle, e.g. "project:create"
	ResourceServerID string `json:"resourceServerId"`
	ActionName       string `json:"actionName"`   // display name of action, e.g. "Create"
	ResourceName     string `json:"resourceName"` // display name of resource, e.g. "Project"
}

// thunderUserList is used to decode paginated user list responses.
type thunderUserList struct {
	Users        []ThunderUser `json:"users"`
	TotalResults int           `json:"totalResults"`
}

// thunderGroupMemberList decodes the GET /groups/{id}/members response.
type thunderGroupMemberList struct {
	TotalResults int           `json:"totalResults"`
	Members      []GroupMember `json:"members"`
}

// thunderRoleAssignmentList decodes the GET /roles/{id}/assignments response.
type thunderRoleAssignmentList struct {
	Assignments []AssignmentEntry `json:"assignments"`
}

// thunderGroupList is used to decode paginated group list responses.
type thunderGroupList struct {
	Groups       []ThunderGroup `json:"groups"`
	TotalResults int            `json:"totalResults"`
}

// thunderRoleList is used to decode paginated role list responses.
type thunderRoleList struct {
	Roles        []ThunderRole `json:"roles"`
	TotalResults int           `json:"totalResults"`
}

// thunderResourceServerList is used to decode paginated resource server list responses.
type thunderResourceServerList struct {
	ResourceServers []ThunderResourceServer `json:"resourceServers"`
	TotalResults    int                     `json:"totalResults"`
}

// thunderResourceList is used to decode paginated resource list responses.
type thunderResourceList struct {
	Resources    []ThunderResource `json:"resources"`
	TotalResults int               `json:"totalResults"`
}

// thunderActionList is used to decode the GET /resource-servers/{id}/resources/{id}/actions response.
type thunderActionList struct {
	Actions      []ThunderAction `json:"actions"`
	TotalResults int             `json:"totalResults"`
}

// ThunderOU represents a child organization unit in Thunder.
type ThunderOU struct {
	ID         string `json:"id"`
	Handle     string `json:"handle"`
	Name       string `json:"name"`
	IsReadOnly bool   `json:"isReadOnly"`
}

// thunderChildOUList decodes the GET /organization-units/{id}/ous response.
type thunderChildOUList struct {
	TotalResults      int         `json:"totalResults"`
	OrganizationUnits []ThunderOU `json:"organizationUnits"`
}

// ProxyResourceServerReader is an optional read-only capability of an identity client.
type ProxyResourceServerReader interface {
	ProxyResourceServerExists(ctx context.Context, proxyHandle, identifier string) (bool, error)
}
