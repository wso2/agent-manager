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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/identity_client_mock.go . IdentityClient:IdentityClientMock

// IdentityClient provides user, group, and role management operations via the Thunder API.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/identity_client_mock.go . IdentityClient:IdentityClientMock
type IdentityClient interface {
	// Users
	ListUsers(ctx context.Context, offset, limit int) ([]ThunderUser, int, error)
	ListUsersByOUId(ctx context.Context, ouID string, offset, limit int) ([]ThunderUser, int, error)
	GetUser(ctx context.Context, userID string) (*ThunderUser, error)
	CreateUser(ctx context.Context, req CreateUserRequest) (*ThunderUser, error)
	UpdateUser(ctx context.Context, userID string, req UpdateUserRequest) (*ThunderUser, error)
	// UpdateUserCredentials changes a user's password via Thunder's dedicated credential
	// endpoint (POST /users/{id}/update-credentials). Thunder rejects a password field
	// inside a regular UpdateUser call with USR-1028 "Credential update not allowed" —
	// credential changes are only accepted through this separate endpoint.
	UpdateUserCredentials(ctx context.Context, userID, password string) error
	DeleteUser(ctx context.Context, userID string) error
	GetUserGroups(ctx context.Context, userID string) ([]ThunderGroup, error)
	GetUserRoles(ctx context.Context, userID string) ([]ThunderRole, error)
	InviteUser(ctx context.Context, email string, ouID string) (string, error)

	// Agents (identity, not the AMP agent resource)
	// GetAgentRoles returns the roles assigned to an agent identity, by
	// fanning out over every role and checking its assignees — Thunder has no
	// reverse-lookup endpoint for this (mirrors GetUserRoles/GetGroupRoles).
	GetAgentRoles(ctx context.Context, agentID string) ([]ThunderRole, error)
	// GetAgentGroups returns the groups an agent identity belongs to, by
	// fanning out over every group in ouID and checking its members —
	// Thunder has no reverse-lookup endpoint for this.
	GetAgentGroups(ctx context.Context, ouID, agentID string) ([]ThunderGroup, error)
	// GetAgentRoleAssignments returns a role's assignments with agent-identity
	// (environment Thunder) semantics: agent assignees as raw {type, id}
	// entries and group assignees resolved to full groups. Unlike
	// GetRoleAssignments (built for the platform user store), it does not
	// resolve or return users.
	GetAgentRoleAssignments(ctx context.Context, roleID string) (*AgentRoleAssignments, error)

	// Groups
	ListGroups(ctx context.Context, ouID string, offset, limit int) ([]ThunderGroup, int, error)
	ListGroupsByOUId(ctx context.Context, ouID string, offset, limit int) ([]ThunderGroup, int, error)
	GetGroup(ctx context.Context, groupID string) (*ThunderGroup, error)
	CreateGroup(ctx context.Context, req CreateGroupRequest) (*ThunderGroup, error)
	UpdateGroup(ctx context.Context, groupID string, req UpdateGroupRequest) (*ThunderGroup, error)
	DeleteGroup(ctx context.Context, groupID string) error
	AddGroupMembers(ctx context.Context, groupID string, userIDs []string) error
	RemoveGroupMembers(ctx context.Context, groupID string, userIDs []string) error
	GetGroupMembers(ctx context.Context, groupID string, offset, limit int) ([]ThunderUser, int, error)
	GetGroupRoles(ctx context.Context, groupID string) ([]ThunderRole, error)
	// AddGroupMemberEntries adds typed members (agents, users, nested groups) to a group.
	AddGroupMemberEntries(ctx context.Context, groupID string, members []GroupMember) error
	// RemoveGroupMemberEntries removes typed members from a group.
	RemoveGroupMemberEntries(ctx context.Context, groupID string, members []GroupMember) error
	// ListGroupMemberEntries returns the raw typed member entries of a group (unlike
	// GetGroupMembers, which resolves and returns only user members).
	ListGroupMemberEntries(ctx context.Context, groupID string, offset, limit int) ([]GroupMember, int, error)

	// Roles
	ListRoles(ctx context.Context, ouID string, offset, limit int) ([]ThunderRole, int, error)
	GetRole(ctx context.Context, roleID string) (*ThunderRole, error)
	CreateRole(ctx context.Context, req CreateRoleRequest) (*ThunderRole, error)
	UpdateRole(ctx context.Context, roleID string, req UpdateRoleRequest) (*ThunderRole, error)
	DeleteRole(ctx context.Context, roleID string) error
	GetRoleAssignments(ctx context.Context, roleID string) (*RoleAssignments, error)
	AddRolePermissions(ctx context.Context, roleID string, req RolePermissionRequest) error
	RemoveRolePermissions(ctx context.Context, roleID string, req RolePermissionRequest) error
	AddRoleAssignees(ctx context.Context, roleID string, req RoleAssignmentsRequest) error
	RemoveRoleAssignees(ctx context.Context, roleID string, req RoleAssignmentsRequest) error

	// Permissions catalog
	ListAMPPermissions(ctx context.Context) ([]ThunderPermission, string, error)
	// EnsureProxyResourceServer makes sure the proxy's resource server exists
	// (handle = proxyHandle, identifier = the proxy's public invocation URI in
	// this environment, delimiter ":", type MCP), that its permission-anchor
	// resource exists (handle = proxyHandle), and every given action is
	// registered under that resource, then returns the RS ID. Anchoring under
	// a resource (rather than registering actions at the RS root) is required
	// for Thunder to derive a "<proxyHandle>:<action>" permission string — see
	// ensureProxyResource's doc comment for why. The identifier must be an
	// RFC 8707 absolute URI; it is normalized into canonical form (lowercased
	// host, default port and trailing slash stripped), so non-canonical input
	// is accepted, not rejected. A drifted identifier is updated in place; the
	// handle never changes.
	EnsureProxyResourceServer(ctx context.Context, proxyHandle, displayName, identifier string, actions []string) (string, error)
	// DeleteProxyResourceServerAction best-effort deletes one action from the
	// proxy's permission-anchor resource. Missing RS, anchor resource, or
	// action is not an error. Returns the RS ID ("" if RS absent).
	DeleteProxyResourceServerAction(ctx context.Context, proxyHandle, action string) (string, error)
	// DeleteProxyResourceServer deletes every action under the proxy's
	// permission-anchor resource, then that resource, then the RS itself
	// (Thunder blocks deletion while children exist). Missing RS is not an error.
	DeleteProxyResourceServer(ctx context.Context, proxyHandle string) error

	// Organization units
	GetOUIDByHandle(ctx context.Context, handle string) (string, error)
	ListChildOUs(ctx context.Context, parentOUID string, limit, offset int) ([]ThunderOU, int, error)
}

// NewIdentityClient creates a Thunder client for identity management operations.
// It shares the same transport and token-caching as ThunderClient.
// baseURL is the platform Thunder public/issuer URL, so it also derives the System
// resource indicator used to obtain system-scoped tokens for the admin identity APIs.
func NewIdentityClient(baseURL, clientID, clientSecret string) IdentityClient {
	return &thunderClient{
		baseURL:        baseURL,
		clientID:       clientID,
		clientSecret:   clientSecret,
		systemResource: SystemResourceIdentifier(baseURL),
		httpClient:     &http.Client{Timeout: httpClientTimeout},
	}
}

// NewIdentityClientWithDialOverride creates a Thunder identity client that dials
// resolveToHost instead of baseURL's own host, while still using baseURL for the
// HTTP Host header and to derive the System resource server identifier (see
// SystemResourceIdentifier). Needed when baseURL is Thunder's public/issuer URL
// (required, since that's the only identifier Thunder's System resource server is
// registered under) but that hostname isn't directly dialable from this process —
// e.g. agent-manager-service running in-cluster while Thunder's public URL only
// resolves via the host machine's own DNS/hosts setup. An empty resolveToHost
// behaves exactly like NewIdentityClient.
func NewIdentityClientWithDialOverride(baseURL, clientID, clientSecret, resolveToHost string) IdentityClient {
	c := NewThunderClientWithDialOverride(baseURL, clientID, clientSecret, resolveToHost, SystemResourceIdentifier(baseURL))
	return c.(IdentityClient)
}

// --- Users ---

func (c *thunderClient) ListUsers(ctx context.Context, offset, limit int) ([]ThunderUser, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	url := fmt.Sprintf("%s/users?offset=%d&limit=%d", c.baseURL, offset, limit)
	body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("thunder list users: %w", err)
	}

	var wrapped thunderUserList
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, 0, fmt.Errorf("thunder list users decode: %w", err)
	}
	return wrapped.Users, wrapped.TotalResults, nil
}

func (c *thunderClient) GetUser(ctx context.Context, userID string) (*ThunderUser, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/users/"+userID, token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get user: %w", err)
	}
	var user ThunderUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("thunder get user decode: %w", err)
	}
	return &user, nil
}

func (c *thunderClient) CreateUser(ctx context.Context, req CreateUserRequest) (*ThunderUser, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/users", token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder create user: %w", err)
	}
	var user ThunderUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("thunder create user decode: %w", err)
	}
	return &user, nil
}

func (c *thunderClient) UpdateUser(ctx context.Context, userID string, req UpdateUserRequest) (*ThunderUser, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPut, c.baseURL+"/users/"+userID, token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder update user: %w", err)
	}
	var user ThunderUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("thunder update user decode: %w", err)
	}
	return &user, nil
}

func (c *thunderClient) UpdateUserCredentials(ctx context.Context, userID, password string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	req := UpdateUserCredentialsRequest{Credentials: map[string]string{"password": password}}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/users/"+userID+"/update-credentials", token, req)
	if err != nil {
		return fmt.Errorf("thunder update user credentials: %w", err)
	}
	return nil
}

func (c *thunderClient) DeleteUser(ctx context.Context, userID string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodDelete, c.baseURL+"/users/"+userID, token, nil)
	if err != nil {
		return fmt.Errorf("thunder delete user: %w", err)
	}
	return nil
}

func (c *thunderClient) GetUserGroups(ctx context.Context, userID string) ([]ThunderGroup, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/users/"+userID+"/groups", token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get user groups: %w", err)
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("thunder get user groups: empty response")
	}
	if trimmed[0] == '[' {
		var groups []ThunderGroup
		if err := json.Unmarshal(body, &groups); err != nil {
			return nil, fmt.Errorf("thunder get user groups decode: %w", err)
		}
		return groups, nil
	}
	var wrapped thunderGroupList
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("thunder get user groups decode: %w", err)
	}
	return wrapped.Groups, nil
}

// --- Groups ---

// NativeAdministratorsGroupName is the group every ThunderID install seeds in its
// default OU. Its members inherit NativeAdministratorRoleName and therefore
// Thunder's built-in "system" scope, so group membership is a second path to
// env-Thunder admin alongside direct role assignment: OU-scoped group listing
// hides it, and both identity controllers reject reads and writes against it.
// Thunder enforces unique group names per OU, so post-bootstrap the only group
// with this name in the default OU is the native one.
const NativeAdministratorsGroupName = "Administrators"

// isVisibleGroup reports whether a group may be surfaced through the identity
// APIs. It is the single place the native-group exclusion is decided, so every
// filtered read path stays in agreement.
func isVisibleGroup(g ThunderGroup) bool {
	return g.Name != NativeAdministratorsGroupName
}

// fetchAllGroups pages endpoint to exhaustion and returns the groups keep
// accepts. Filtering during the sweep rather than after pagination is what lets
// callers apply offset/limit to the visible set only.
func (c *thunderClient) fetchAllGroups(ctx context.Context, token, endpoint, errLabel string,
	keep func(ThunderGroup) bool,
) ([]ThunderGroup, error) {
	const fetchSize = 100
	var all []ThunderGroup
	fetchOffset := 0
	for {
		url := fmt.Sprintf("%s?offset=%d&limit=%d", endpoint, fetchOffset, fetchSize)
		body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", errLabel, err)
		}
		var wrapped thunderGroupList
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return nil, fmt.Errorf("%s decode: %w", errLabel, err)
		}
		for _, g := range wrapped.Groups {
			if keep(g) {
				all = append(all, g)
			}
		}
		fetchOffset += len(wrapped.Groups)
		if fetchOffset >= wrapped.TotalResults || len(wrapped.Groups) == 0 {
			break
		}
	}
	return all, nil
}

// ListGroups returns groups scoped to ouID when non-empty, by fetching all pages
// from Thunder and filtering client-side (the instance-wide endpoint cannot scope
// by OU). The ouID=="" branch is the raw instance-wide view and keeps the native
// Administrators group; it has no production caller today.
func (c *thunderClient) ListGroups(ctx context.Context, ouID string, offset, limit int) ([]ThunderGroup, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	if ouID == "" {
		url := fmt.Sprintf("%s/groups?offset=%d&limit=%d", c.baseURL, offset, limit)
		body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("thunder list groups: %w", err)
		}
		var wrapped thunderGroupList
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return nil, 0, fmt.Errorf("thunder list groups decode: %w", err)
		}
		return wrapped.Groups, wrapped.TotalResults, nil
	}

	all, err := c.fetchAllGroups(ctx, token, c.baseURL+"/groups", "thunder list groups",
		func(g ThunderGroup) bool { return g.OuID == ouID && isVisibleGroup(g) })
	if err != nil {
		return nil, 0, err
	}
	groups, total := paginate(all, offset, limit)
	return groups, total, nil
}

// listGroupsInOU returns every visible group in ouID from Thunder's OU-scoped
// endpoint. It is the single filtered source for ListGroupsByOUId and
// GetAgentGroups, so the native group cannot leak through either surface.
func (c *thunderClient) listGroupsInOU(ctx context.Context, ouID string) ([]ThunderGroup, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	return c.fetchAllGroups(ctx, token, c.baseURL+"/organization-units/"+ouID+"/groups",
		"thunder list groups by ou id", isVisibleGroup)
}

// ListGroupsByOUId returns groups in ouID from Thunder's OU-scoped endpoint with
// the native Administrators group excluded. Pagination is applied client-side
// after the exclusion (like ListRoles) so offset/limit/total stay exact — a
// server-side page filtered after the fact would return short pages and an
// inflated total.
func (c *thunderClient) ListGroupsByOUId(ctx context.Context, ouID string, offset, limit int) ([]ThunderGroup, int, error) {
	all, err := c.listGroupsInOU(ctx, ouID)
	if err != nil {
		return nil, 0, err
	}
	groups, total := paginate(all, offset, limit)
	return groups, total, nil
}

// paginate applies offset/limit to an already-filtered slice and reports the
// post-filter total, so a caller that excludes reserved principals before
// paginating keeps offset/limit/total consistent with what it returns.
func paginate[T any](all []T, offset, limit int) ([]T, int) {
	total := len(all)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []T{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total
}

func (c *thunderClient) GetGroup(ctx context.Context, groupID string) (*ThunderGroup, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/groups/"+groupID, token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get group: %w", err)
	}
	var group ThunderGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("thunder get group decode: %w", err)
	}
	return &group, nil
}

func (c *thunderClient) CreateGroup(ctx context.Context, req CreateGroupRequest) (*ThunderGroup, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/groups", token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder create group: %w", err)
	}
	var group ThunderGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("thunder create group decode: %w", err)
	}
	return &group, nil
}

func (c *thunderClient) UpdateGroup(ctx context.Context, groupID string, req UpdateGroupRequest) (*ThunderGroup, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPut, c.baseURL+"/groups/"+groupID, token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder update group: %w", err)
	}
	var group ThunderGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("thunder update group decode: %w", err)
	}
	return &group, nil
}

func (c *thunderClient) DeleteGroup(ctx context.Context, groupID string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodDelete, c.baseURL+"/groups/"+groupID, token, nil)
	if err != nil {
		return fmt.Errorf("thunder delete group: %w", err)
	}
	return nil
}

func (c *thunderClient) AddGroupMembers(ctx context.Context, groupID string, userIDs []string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	members := make([]GroupMember, len(userIDs))
	for i, id := range userIDs {
		members[i] = GroupMember{ID: id, Type: "user"}
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/groups/"+groupID+"/members/add", token, GroupMembersRequest{Members: members})
	if err != nil {
		return fmt.Errorf("thunder add group members: %w", err)
	}
	return nil
}

func (c *thunderClient) RemoveGroupMembers(ctx context.Context, groupID string, userIDs []string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	members := make([]GroupMember, len(userIDs))
	for i, id := range userIDs {
		members[i] = GroupMember{ID: id, Type: "user"}
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/groups/"+groupID+"/members/remove", token, GroupMembersRequest{Members: members})
	if err != nil {
		return fmt.Errorf("thunder remove group members: %w", err)
	}
	return nil
}

func (c *thunderClient) GetGroupMembers(ctx context.Context, groupID string, offset, limit int) ([]ThunderUser, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	url := fmt.Sprintf("%s/groups/%s/members?offset=%d&limit=%d", c.baseURL, groupID, offset, limit)
	body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("thunder get group members: %w", err)
	}
	var resp thunderGroupMemberList
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("thunder get group members decode: %w", err)
	}
	users := make([]ThunderUser, 0, len(resp.Members))
	log := logger.GetLogger(ctx)
	for _, m := range resp.Members {
		if m.Type != "user" {
			continue
		}
		user, err := c.GetUser(ctx, m.ID)
		if err != nil {
			if IsNotFound(err) {
				log.Warn("skipping deleted group member", "group_id", groupID, "user_id", m.ID)
				continue
			}
			return nil, 0, fmt.Errorf("thunder get group member %s: %w", m.ID, err)
		}
		users = append(users, *user)
	}
	return users, resp.TotalResults, nil
}

// AddGroupMemberEntries adds typed members (agents, users, nested groups) to a
// group. Unlike AddGroupMembers it does not assume Type "user".
func (c *thunderClient) AddGroupMemberEntries(ctx context.Context, groupID string, members []GroupMember) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/groups/"+groupID+"/members/add", token, GroupMembersRequest{Members: members})
	if err != nil {
		return fmt.Errorf("thunder add group member entries: %w", err)
	}
	return nil
}

// RemoveGroupMemberEntries removes typed members from a group. Unlike
// RemoveGroupMembers it does not assume Type "user".
func (c *thunderClient) RemoveGroupMemberEntries(ctx context.Context, groupID string, members []GroupMember) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/groups/"+groupID+"/members/remove", token, GroupMembersRequest{Members: members})
	if err != nil {
		return fmt.Errorf("thunder remove group member entries: %w", err)
	}
	return nil
}

// ListGroupMemberEntries returns the raw typed member entries of a group. Unlike
// GetGroupMembers, it does not resolve entries into full user objects, so it
// preserves non-user members (agents, nested groups).
func (c *thunderClient) ListGroupMemberEntries(ctx context.Context, groupID string, offset, limit int) ([]GroupMember, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	url := fmt.Sprintf("%s/groups/%s/members?offset=%d&limit=%d", c.baseURL, groupID, offset, limit)
	body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("thunder list group member entries: %w", err)
	}
	var resp thunderGroupMemberList
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("thunder list group member entries decode: %w", err)
	}
	return resp.Members, resp.TotalResults, nil
}

// listRoleAssignmentEntries fetches only the raw {type, id} assignment entries for a role
// without expanding each entry into a full user or group object. Use this when you only
// need to check membership rather than display full objects — it is O(1) per role instead
// of O(assignments) like GetRoleAssignments.
func (c *thunderClient) listRoleAssignmentEntries(ctx context.Context, roleID string) ([]AssignmentEntry, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/roles/"+roleID+"/assignments", token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder list role assignment entries: %w", err)
	}
	var resp thunderRoleAssignmentList
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("thunder list role assignment entries decode: %w", err)
	}
	return resp.Assignments, nil
}

// rolesForAssignee returns every role that has an assignment entry matching
// assigneeType/assigneeID (e.g. "group"/groupID, "user"/userID, "agent"/
// agentID). Thunder has no reverse-lookup endpoint for "roles assigned to
// this assignee", so this pages through every role in the instance and
// checks its assignment entries client-side. Shared by GetGroupRoles,
// GetUserRoles, and GetAgentRoles.
func (c *thunderClient) rolesForAssignee(ctx context.Context, assigneeType, assigneeID string) ([]ThunderRole, error) {
	const pageSize = 50
	var allRoles []ThunderRole
	offset := 0
	for {
		page, total, err := c.ListRoles(ctx, "", offset, pageSize)
		if err != nil {
			return nil, fmt.Errorf("thunder get %s roles (list): %w", assigneeType, err)
		}
		allRoles = append(allRoles, page...)
		offset += len(page)
		if offset >= total || len(page) == 0 {
			break
		}
	}

	var assigneeRoles []ThunderRole
	for _, role := range allRoles {
		entries, err := c.listRoleAssignmentEntries(ctx, role.ID)
		if err != nil {
			return nil, fmt.Errorf("thunder get %s roles (assignments for role %s): %w", assigneeType, role.ID, err)
		}
		for _, e := range entries {
			if e.Type == assigneeType && e.ID == assigneeID {
				assigneeRoles = append(assigneeRoles, role)
				break
			}
		}
	}
	return assigneeRoles, nil
}

func (c *thunderClient) GetGroupRoles(ctx context.Context, groupID string) ([]ThunderRole, error) {
	return c.rolesForAssignee(ctx, "group", groupID)
}

func (c *thunderClient) GetUserRoles(ctx context.Context, userID string) ([]ThunderRole, error) {
	return c.rolesForAssignee(ctx, "user", userID)
}

// GetAgentRoles returns the roles assigned to an agent identity. Same
// fan-out-and-filter approach as GetUserRoles/GetGroupRoles, since Thunder has
// no reverse-lookup endpoint for "roles assigned to this assignee".
func (c *thunderClient) GetAgentRoles(ctx context.Context, agentID string) ([]ThunderRole, error) {
	return c.rolesForAssignee(ctx, "agent", agentID)
}

// GetAgentGroups returns the groups an agent identity belongs to, by fanning
// out over every group in ouID and checking its member entries — Thunder has
// no reverse-lookup endpoint for "groups this assignee belongs to". The native
// Administrators group is excluded with the rest of the group surface, so an
// agent mis-added to it is never reported as a member.
func (c *thunderClient) GetAgentGroups(ctx context.Context, ouID, agentID string) ([]ThunderGroup, error) {
	allGroups, err := c.listGroupsInOU(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("thunder get agent groups (list): %w", err)
	}

	var agentGroups []ThunderGroup
	for _, group := range allGroups {
		const memberPageSize = 100
		memberOffset := 0
		for {
			members, total, err := c.ListGroupMemberEntries(ctx, group.ID, memberOffset, memberPageSize)
			if err != nil {
				return nil, fmt.Errorf("thunder get agent groups (members for group %s): %w", group.ID, err)
			}
			matched := false
			for _, m := range members {
				if m.Type == "agent" && m.ID == agentID {
					matched = true
					break
				}
			}
			if matched {
				agentGroups = append(agentGroups, group)
				break
			}
			memberOffset += len(members)
			if memberOffset >= total || len(members) == 0 {
				break
			}
		}
	}
	return agentGroups, nil
}

// --- Roles ---

// NativeAdministratorRoleName is the role every ThunderID install seeds in its
// default OU. It carries Thunder's built-in "system" scope (the admin-API
// permission the amp-system-client authenticates with — see fetchSystemToken),
// so it must never surface as an assignable agent-identity role: OU-scoped
// ListRoles hides it, and the agent-identity controller rejects reads/writes
// against it. Thunder enforces unique role names per OU, so post-bootstrap the
// only role with this name in the default OU is the native one.
const NativeAdministratorRoleName = "Administrator"

// AMPSystemClientRoleName is the role env-Thunder's own bootstrap seeds (see
// add-environment-thunder.sh's render_system_client_role) to grant the
// amp-system-client app Thunder's "system" scope — infrastructure the agent-manager
// backend uses to provision per-agent OAuth apps, not something an operator manages
// through agent identities. Hidden the same way and for the same reason as
// NativeAdministratorRoleName: it must never surface as an assignable agent-identity
// role, since agents/groups are the only things this API is meant to expose.
const AMPSystemClientRoleName = "AMP System Client Thunder Admin"

// ListRoles returns roles scoped to ouID when non-empty, by fetching all pages
// from Thunder and filtering client-side (Thunder has no OU-scoped list endpoint for roles).
func (c *thunderClient) ListRoles(ctx context.Context, ouID string, offset, limit int) ([]ThunderRole, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	if ouID == "" {
		url := fmt.Sprintf("%s/roles?offset=%d&limit=%d", c.baseURL, offset, limit)
		body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("thunder list roles: %w", err)
		}
		var wrapped thunderRoleList
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return nil, 0, fmt.Errorf("thunder list roles decode: %w", err)
		}
		return wrapped.Roles, wrapped.TotalResults, nil
	}

	const fetchSize = 100
	var all []ThunderRole
	fetchOffset := 0
	for {
		url := fmt.Sprintf("%s/roles?offset=%d&limit=%d", c.baseURL, fetchOffset, fetchSize)
		body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("thunder list roles: %w", err)
		}
		var wrapped thunderRoleList
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return nil, 0, fmt.Errorf("thunder list roles decode: %w", err)
		}
		for _, role := range wrapped.Roles {
			// The exclusion happens before the client-side pagination below so
			// offset/limit/total stay consistent. The unfiltered ouID=="" branch
			// above deliberately keeps both hidden roles: rolesForAssignee and the
			// scope-cleanup sweep must still see every role in the instance.
			if role.OuID == ouID && role.Name != NativeAdministratorRoleName && role.Name != AMPSystemClientRoleName {
				all = append(all, role)
			}
		}
		fetchOffset += len(wrapped.Roles)
		if fetchOffset >= wrapped.TotalResults || len(wrapped.Roles) == 0 {
			break
		}
	}
	roles, total := paginate(all, offset, limit)
	return roles, total, nil
}

func (c *thunderClient) GetRole(ctx context.Context, roleID string) (*ThunderRole, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/roles/"+roleID, token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get role: %w", err)
	}
	var role ThunderRole
	if err := json.Unmarshal(body, &role); err != nil {
		return nil, fmt.Errorf("thunder get role decode: %w", err)
	}
	return &role, nil
}

func (c *thunderClient) CreateRole(ctx context.Context, req CreateRoleRequest) (*ThunderRole, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/roles", token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder create role: %w", err)
	}
	var role ThunderRole
	if err := json.Unmarshal(body, &role); err != nil {
		return nil, fmt.Errorf("thunder create role decode: %w", err)
	}
	return &role, nil
}

func (c *thunderClient) UpdateRole(ctx context.Context, roleID string, req UpdateRoleRequest) (*ThunderRole, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPut, c.baseURL+"/roles/"+roleID, token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder update role: %w", err)
	}
	var role ThunderRole
	if err := json.Unmarshal(body, &role); err != nil {
		return nil, fmt.Errorf("thunder update role decode: %w", err)
	}
	return &role, nil
}

func (c *thunderClient) DeleteRole(ctx context.Context, roleID string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodDelete, c.baseURL+"/roles/"+roleID, token, nil)
	if err != nil {
		return fmt.Errorf("thunder delete role: %w", err)
	}
	return nil
}

// resolveAssignmentGroup expands a group assignment entry to the full group.
// A group deleted from Thunder after being assigned is tolerated: it logs a
// warning and returns ok=false rather than failing the whole listing.
func (c *thunderClient) resolveAssignmentGroup(ctx context.Context, roleID, groupID string) (group *ThunderGroup, ok bool, err error) {
	group, err = c.GetGroup(ctx, groupID)
	if err != nil {
		if IsNotFound(err) {
			logger.GetLogger(ctx).Warn("skipping deleted role assignment group", "role_id", roleID, "group_id", groupID)
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("thunder get role assignment group %s: %w", groupID, err)
	}
	return group, true, nil
}

func (c *thunderClient) GetRoleAssignments(ctx context.Context, roleID string) (*RoleAssignments, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/roles/"+roleID+"/assignments", token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get role assignments: %w", err)
	}
	var resp thunderRoleAssignmentList
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("thunder get role assignments decode: %w", err)
	}
	result := &RoleAssignments{}
	log := logger.GetLogger(ctx)
	for _, a := range resp.Assignments {
		switch a.Type {
		case "user":
			user, err := c.GetUser(ctx, a.ID)
			if err != nil {
				if IsNotFound(err) {
					log.Warn("skipping deleted role assignment user", "role_id", roleID, "user_id", a.ID)
					continue
				}
				return nil, fmt.Errorf("thunder get role assignment user %s: %w", a.ID, err)
			}
			result.Users = append(result.Users, *user)
		case "group":
			group, ok, err := c.resolveAssignmentGroup(ctx, roleID, a.ID)
			if err != nil {
				return nil, err
			}
			if ok {
				result.Groups = append(result.Groups, *group)
			}
		}
	}

	return result, nil
}

// GetAgentRoleAssignments returns the role's assignments with agent-identity
// (environment Thunder) semantics; see the IdentityClient interface doc for
// the contract.
func (c *thunderClient) GetAgentRoleAssignments(ctx context.Context, roleID string) (*AgentRoleAssignments, error) {
	entries, err := c.listRoleAssignmentEntries(ctx, roleID)
	if err != nil {
		return nil, err
	}
	result := &AgentRoleAssignments{}
	for _, a := range entries {
		switch a.Type {
		case "agent":
			result.Agents = append(result.Agents, a)
		case "group":
			group, ok, err := c.resolveAssignmentGroup(ctx, roleID, a.ID)
			if err != nil {
				return nil, err
			}
			if ok {
				result.Groups = append(result.Groups, *group)
			}
		}
	}
	return result, nil
}

// AddRolePermissions merges new permissions into the role via PUT /roles/{id}.
func (c *thunderClient) AddRolePermissions(ctx context.Context, roleID string, req RolePermissionRequest) error {
	role, err := c.GetRole(ctx, roleID)
	if err != nil {
		return fmt.Errorf("thunder add role permissions (get role): %w", err)
	}

	perms := role.Permissions
	found := false
	for i, p := range perms {
		if p.ResourceServerID == req.ResourceServerID {
			existing := make(map[string]bool, len(p.Permissions))
			for _, perm := range p.Permissions {
				existing[perm] = true
			}
			for _, newPerm := range req.Permissions {
				if !existing[newPerm] {
					perms[i].Permissions = append(perms[i].Permissions, newPerm)
				}
			}
			found = true
			break
		}
	}
	if !found {
		perms = append(perms, req)
	}

	return c.putRolePermissions(ctx, role, perms)
}

// RemoveRolePermissions removes permissions from the role via PUT /roles/{id}.
func (c *thunderClient) RemoveRolePermissions(ctx context.Context, roleID string, req RolePermissionRequest) error {
	role, err := c.GetRole(ctx, roleID)
	if err != nil {
		return fmt.Errorf("thunder remove role permissions (get role): %w", err)
	}

	toRemove := make(map[string]bool, len(req.Permissions))
	for _, p := range req.Permissions {
		toRemove[p] = true
	}

	var perms []RolePermissionRequest
	for _, p := range role.Permissions {
		if p.ResourceServerID == req.ResourceServerID {
			var remaining []string
			for _, perm := range p.Permissions {
				if !toRemove[perm] {
					remaining = append(remaining, perm)
				}
			}
			if len(remaining) > 0 {
				perms = append(perms, RolePermissionRequest{ResourceServerID: p.ResourceServerID, Permissions: remaining})
			}
		} else {
			perms = append(perms, p)
		}
	}

	return c.putRolePermissions(ctx, role, perms)
}

func (c *thunderClient) putRolePermissions(ctx context.Context, role *ThunderRole, perms []RolePermissionRequest) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	body := thunderRolePermissionsUpdateBody{
		OuID:        role.OuID,
		Name:        role.Name,
		Description: role.Description,
		Permissions: perms,
	}
	_, err = c.doRequest(ctx, http.MethodPut, c.baseURL+"/roles/"+role.ID, token, body)
	if err != nil {
		return fmt.Errorf("thunder put role permissions: %w", err)
	}
	return nil
}

func (c *thunderClient) AddRoleAssignees(ctx context.Context, roleID string, req RoleAssignmentsRequest) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/roles/"+roleID+"/assignments/add", token, req)
	if err != nil {
		return fmt.Errorf("thunder add role assignees: %w", err)
	}
	return nil
}

func (c *thunderClient) RemoveRoleAssignees(ctx context.Context, roleID string, req RoleAssignmentsRequest) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/roles/"+roleID+"/assignments/remove", token, req)
	if err != nil {
		return fmt.Errorf("thunder remove role assignees: %w", err)
	}
	return nil
}

// --- Permissions catalog ---

// ListAMPPermissions returns all permissions registered under the "amp" resource
// server (each entry's Name is the scope string, e.g. "amp:agents:create") and
// the resource server ID.
func (c *thunderClient) ListAMPPermissions(ctx context.Context) ([]ThunderPermission, string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, "", err
	}

	// Find the "amp" resource server by its Thunder IDENTIFIER (a different field/value
	// than rbac.ResourceServer, the scope-string prefix — see ResourceServerIdentifier's doc comment).
	ampRSID, err := c.findResourceServerID(ctx, token, rbac.ResourceServerIdentifier)
	if err != nil {
		return nil, "", err
	}
	if ampRSID == "" {
		// Return empty list if amp resource server not found - permissions can be managed without it
		return []ThunderPermission{}, "", nil
	}

	// Fetch every resource under the amp resource server. GET .../resources
	// without parentId only returns TOP-LEVEL resources — despite Thunder's own
	// handler comment claiming it returns "all resources" when parentId is
	// omitted, it does not: every actual AMP permission resource (org, profile,
	// project, ...) lives one level below the "amp" anchor resource (see
	// amp-thunder-bootstrap.yaml's "Top-level parent for every resource below"
	// comment), so each top-level resource's children must be fetched
	// explicitly via parentId or they're silently missing from this list —
	// which is exactly why the permission-assignment UI showed an empty list.
	topLevel, err := c.listResources(ctx, token, ampRSID, "")
	if err != nil {
		return nil, "", fmt.Errorf("thunder list amp resources: %w", err)
	}
	resources := append([]ThunderResource{}, topLevel...)
	for _, top := range topLevel {
		children, err := c.listResources(ctx, token, ampRSID, top.ID)
		if err != nil {
			return nil, "", fmt.Errorf("thunder list amp resource children for %q: %w", top.Handle, err)
		}
		resources = append(resources, children...)
	}

	// For each resource fetch its actions via the dedicated endpoint.
	// Actions are not embedded in the resource list response.
	// Non-nil even when empty: an all-zero-actions result (e.g. amp exists but
	// has no children yet) must serialize as "permissions": [] to API callers,
	// not null, matching the RS-not-found early return above.
	perms := []ThunderPermission{}
	for _, res := range resources {
		actURL := fmt.Sprintf("%s/resource-servers/%s/resources/%s/actions", c.baseURL, ampRSID, res.ID)
		actBody, err := c.doRequest(ctx, http.MethodGet, actURL, token, nil)
		if err != nil {
			return nil, "", fmt.Errorf("thunder list actions for resource %s: %w", res.ID, err)
		}
		var actPage thunderActionList
		if err := json.Unmarshal(actBody, &actPage); err != nil {
			return nil, "", fmt.Errorf("thunder list actions decode for resource %s: %w", res.ID, err)
		}
		for _, action := range actPage.Actions {
			name := action.Permission
			if name == "" {
				name = res.Handle + ":" + action.Handle
			}
			perms = append(perms, ThunderPermission{
				Name:             name,
				ResourceServerID: ampRSID,
				ResourceName:     res.Name,
				ActionName:       action.Name,
			})
		}
	}

	return perms, ampRSID, nil
}

// listResources paginates through a resource server's resources and returns
// them all. When parentID is "", it lists top-level resources (Thunder's
// listing endpoint does not descend into children by default); pass a
// resource's ID to fetch that resource's direct children instead. Used by
// ListAMPPermissions to descend into the "amp" anchor resource's children.
func (c *thunderClient) listResources(ctx context.Context, token, rsID, parentID string) ([]ThunderResource, error) {
	const resPageSize = 20
	var resources []ThunderResource
	offset := 0
	for {
		url := fmt.Sprintf("%s/resource-servers/%s/resources?offset=%d&limit=%d", c.baseURL, rsID, offset, resPageSize)
		if parentID != "" {
			url += "&parentId=" + parentID
		}
		body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
		if err != nil {
			return nil, err
		}
		var page thunderResourceList
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		resources = append(resources, page.Resources...)
		offset += len(page.Resources)
		if offset >= page.TotalResults || len(page.Resources) == 0 {
			break
		}
	}
	return resources, nil
}

// findResourceServer paginates through resource servers and returns the first
// one match accepts, or nil if none match.
func (c *thunderClient) findResourceServer(ctx context.Context, token string, match func(*ThunderResourceServer) bool) (*ThunderResourceServer, error) {
	const rsPageSize = 20
	rsOffset := 0
	for {
		rsURL := fmt.Sprintf("%s/resource-servers?offset=%d&limit=%d", c.baseURL, rsOffset, rsPageSize)
		rsBody, err := c.doRequest(ctx, http.MethodGet, rsURL, token, nil)
		if err != nil {
			return nil, fmt.Errorf("thunder list resource servers: %w", err)
		}
		var page thunderResourceServerList
		if err := json.Unmarshal(rsBody, &page); err != nil {
			return nil, fmt.Errorf("thunder list resource servers decode: %w", err)
		}
		for i := range page.ResourceServers {
			if match(&page.ResourceServers[i]) {
				return &page.ResourceServers[i], nil
			}
		}
		rsOffset += len(page.ResourceServers)
		if rsOffset >= page.TotalResults || len(page.ResourceServers) == 0 {
			return nil, nil //nolint:nilnil // absent resource server is not an error
		}
	}
}

// findResourceServerID returns the ID of the RS whose identifier matches, or "".
// Still used for the platform AMP resource server lookup (rbac.ResourceServerIdentifier).
func (c *thunderClient) findResourceServerID(ctx context.Context, token, identifier string) (string, error) {
	rs, err := c.findResourceServer(ctx, token, func(rs *ThunderResourceServer) bool { return rs.Identifier == identifier })
	if err != nil || rs == nil {
		return "", err
	}
	return rs.ID, nil
}

// findProxyResourceServer locates a proxy's RS by its stable handle, or by its
// exact current identifier when the caller has one. Thunder's resource servers
// carry no handle of their own (only their child resources/actions do), so the
// handle check and the bare-handle identifier fallback below only ever match a
// pre-migration row created before identifiers switched to invocation URIs;
// the identifier check is what actually finds a proxy's RS day to day.
func (c *thunderClient) findProxyResourceServer(ctx context.Context, token, proxyHandle, identifier string) (*ThunderResourceServer, error) {
	return c.findResourceServer(ctx, token, func(rs *ThunderResourceServer) bool {
		if identifier != "" && rs.Identifier == identifier {
			return true
		}
		return rs.Handle == proxyHandle || (rs.Handle == "" && rs.Identifier == proxyHandle)
	})
}

// --- Per-proxy resource servers ---

const (
	// proxyResourceServerDelimiter joins a resource/action's own handle with its
	// parent's already-composed permission to derive its permission. The scope
	// string is "<proxy-handle>:<action>": anchoring every action under the
	// proxyHandle-named resource created by ensureProxyResource reproduces that
	// shape exactly (see that function's comment for why the anchor is
	// required). Proxy handles are kebab-case and never contain ":".
	proxyResourceServerDelimiter = ":"
	// proxyResourceServerType marks the RS as MCP, enabling Thunder's MCP
	// handle-collision guardrails.
	proxyResourceServerType = "MCP"
	// thunderHandleMaxLen is Thunder's per-handle cap (RS, resource, and action
	// handles). Over-long inputs are rejected with a clear error instead of letting
	// Thunder reject the handle with an opaque 400.
	thunderHandleMaxLen = 100
	// thunderIdentifierMaxLen is Thunder's cap on the RS identifier column, which
	// holds a URI and is far wider than a handle.
	thunderIdentifierMaxLen = 2048
)

// canonicalMCPResourceIdentifier returns raw in RFC 8707 canonical form, or an error if it cannot be one.
func canonicalMCPResourceIdentifier(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("resource identifier %q is not a valid URI: %w", raw, err)
	}
	switch {
	case u.Scheme == "":
		return "", fmt.Errorf("resource identifier %q has no scheme; RFC 8707 requires an absolute URI", raw)
	case u.Scheme != "http" && u.Scheme != "https":
		return "", fmt.Errorf("resource identifier %q must use scheme http or https, got %q", raw, u.Scheme)
	case u.Host == "" || u.Hostname() == "":
		return "", fmt.Errorf("resource identifier %q has no host", raw)
	case u.User != nil:
		return "", fmt.Errorf("resource identifier %q must not carry userinfo", raw)
	case u.RawQuery != "" || u.ForceQuery:
		return "", fmt.Errorf("resource identifier %q must not carry a query", raw)
	case u.Fragment != "":
		return "", fmt.Errorf("resource identifier %q must not carry a fragment", raw)
	}
	// url.Parse lowercases the scheme but not the host; the path stays as-is, being case-sensitive.
	host := strings.ToLower(u.Host)
	if port := u.Port(); port == defaultPortForScheme(u.Scheme) {
		host = strings.TrimSuffix(host, ":"+port)
	}
	return u.Scheme + "://" + host + strings.TrimRight(u.EscapedPath(), "/"), nil
}

// defaultPortForScheme is the port an http(s) URI omits in canonical form.
func defaultPortForScheme(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

// ensureProxyResourceServerMu returns the mutex serializing check-then-create
// calls for one proxy handle. Entries are never removed; the map only grows by
// the small set of proxy handles a client touches.
func (c *thunderClient) ensureProxyResourceServerMu(proxyHandle string) *sync.Mutex {
	mu, _ := c.ensureResourceServerMus.LoadOrStore(proxyHandle, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// EnsureProxyResourceServer makes sure the resource server for a proxy exists
// (handle = proxyHandle, identifier = the proxy's public invocation URI,
// delimiter ":", type MCP), that its permission-anchor resource exists (see
// ensureProxyResource), and that every given action is registered under that
// resource, then returns the resource server ID. The identifier must be an
// RFC 8707 absolute URI; it is canonicalized here, so both create and
// drift-update write a canonical form. A drifted identifier is rewritten in
// place. Idempotent; called lazily before role writes.
func (c *thunderClient) EnsureProxyResourceServer(ctx context.Context, proxyHandle, displayName, identifier string, actions []string) (string, error) {
	identifier, err := canonicalMCPResourceIdentifier(identifier)
	if err != nil {
		return "", err
	}
	if len(proxyHandle) > thunderHandleMaxLen {
		return "", fmt.Errorf("proxy handle %q exceeds the Thunder handle limit of %d characters", proxyHandle, thunderHandleMaxLen)
	}
	if len(identifier) > thunderIdentifierMaxLen {
		return "", fmt.Errorf("resource identifier %q exceeds the Thunder limit of %d characters", identifier, thunderIdentifierMaxLen)
	}
	for _, action := range actions {
		if len(action) > thunderHandleMaxLen {
			return "", fmt.Errorf("action %q exceeds the Thunder handle limit of %d characters", action, thunderHandleMaxLen)
		}
	}

	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", err
	}

	// Avoids a TOCTOU race where concurrent callers both create a duplicate
	// resource server, anchor resource, or action; keyed per handle so
	// unrelated proxies don't serialize behind each other's Thunder round-trips.
	mu := c.ensureProxyResourceServerMu(proxyHandle)
	mu.Lock()
	defer mu.Unlock()

	rs, err := c.findProxyResourceServer(ctx, token, proxyHandle, identifier)
	if err != nil {
		return "", err
	}
	var rsID string
	if rs == nil {
		ouID, err := c.getDefaultOUID(ctx, token)
		if err != nil {
			return "", fmt.Errorf("thunder ensure proxy resource server (default ou): %w", err)
		}
		body, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/resource-servers", token,
			proxyResourceServerBody(displayName, identifier, proxyHandle, ouID))
		if err != nil {
			// A 409 means a concurrent caller (different AMS replica) created
			// the resource server first — re-run the same lookup to find the
			// winner instead of failing the whole role write over a benign race.
			if IsConflict(err) {
				foundRS, findErr := c.findProxyResourceServer(ctx, token, proxyHandle, identifier)
				if findErr != nil {
					return "", findErr
				}
				if foundRS != nil {
					rsID = foundRS.ID
				}
			}
			if rsID == "" {
				return "", fmt.Errorf("thunder create proxy resource server: %w", err)
			}
		} else {
			var created ThunderResourceServer
			if err := json.Unmarshal(body, &created); err != nil {
				return "", fmt.Errorf("thunder create proxy resource server decode: %w", err)
			}
			rsID = created.ID
		}
	} else {
		rsID = rs.ID
		if rs.Identifier != identifier {
			if err := c.updateProxyResourceServerIdentifier(ctx, token, rs, proxyHandle, identifier); err != nil {
				return "", err
			}
		}
	}

	resourceID, err := c.ensureProxyResource(ctx, token, rsID, proxyHandle, displayName)
	if err != nil {
		return "", err
	}

	existing, err := c.listProxyActions(ctx, token, rsID, resourceID)
	if err != nil {
		return "", err
	}
	for _, action := range actions {
		if action == "" {
			continue
		}
		if _, ok := existing[action]; ok {
			continue
		}
		// A concurrent caller (different AMS replica, or racing after this
		// action was missing on the listing above) may have created it first;
		// Thunder's 409 RES-1014 "Handle conflict" means it now exists either
		// way, so this is not a failure — see ensureProxyResource's comment.
		if err := c.createProxyAction(ctx, token, rsID, resourceID, action); err != nil && !IsConflict(err) {
			return "", err
		}
		existing[action] = "" // guard against duplicate actions in the input slice
	}
	return rsID, nil
}

// proxyResourceServerBody is the request payload shared by the proxy RS create
// (POST) and identifier-update (PUT) calls.
func proxyResourceServerBody(name, identifier, proxyHandle, ouID string) map[string]string {
	return map[string]string{
		"name":       name,
		"identifier": identifier,
		"handle":     proxyHandle,
		"ouId":       ouID,
		"delimiter":  proxyResourceServerDelimiter,
		"type":       proxyResourceServerType,
	}
}

// updateProxyResourceServerIdentifier rewrites a drifted identifier in place via
// PUT /resource-servers/{id}.
func (c *thunderClient) updateProxyResourceServerIdentifier(ctx context.Context, token string, rs *ThunderResourceServer, proxyHandle, identifier string) error {
	ouID, err := c.getDefaultOUID(ctx, token)
	if err != nil {
		return fmt.Errorf("thunder update proxy resource server (default ou): %w", err)
	}
	name := rs.Name
	if name == "" {
		name = proxyHandle
	}
	_, err = c.doRequest(ctx, http.MethodPut, c.baseURL+"/resource-servers/"+rs.ID, token,
		proxyResourceServerBody(name, identifier, proxyHandle, ouID))
	if err != nil {
		return fmt.Errorf("thunder update proxy resource server identifier: %w", err)
	}
	return nil
}

// DeleteProxyResourceServerAction best-effort deletes a single action from the
// proxy's permission-anchor resource. A missing resource server, missing
// anchor resource, or missing action is not an error. Returns the resource
// server ID ("" if the RS is absent).
func (c *thunderClient) DeleteProxyResourceServerAction(ctx context.Context, proxyHandle, action string) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", err
	}
	// No identifier is known here (only the handle), so this can only find a
	// proxy RS by the pre-migration bare-handle-identifier fallback — see
	// findProxyResourceServer's comment.
	rs, err := c.findProxyResourceServer(ctx, token, proxyHandle, "")
	if err != nil {
		return "", err
	}
	if rs == nil {
		return "", nil
	}
	rsID := rs.ID
	resourceID, err := c.findProxyResourceID(ctx, token, rsID, proxyHandle)
	if err != nil {
		return rsID, err
	}
	if resourceID == "" {
		return rsID, nil
	}
	actions, err := c.listProxyActions(ctx, token, rsID, resourceID)
	if err != nil {
		return rsID, err
	}
	actionID, ok := actions[action]
	if !ok {
		return rsID, nil
	}
	if _, err := c.doRequest(ctx, http.MethodDelete, c.baseURL+"/resource-servers/"+rsID+"/resources/"+resourceID+"/actions/"+actionID, token, nil); err != nil && !IsNotFound(err) {
		return rsID, fmt.Errorf("thunder delete proxy action %q: %w", action, err)
	}
	return rsID, nil
}

// DeleteProxyResourceServer deletes every action under the proxy's permission-
// anchor resource, then that resource, then the resource server itself.
// Thunder blocks deletion while children exist, so they are removed bottom-up.
// A missing resource server is not an error.
func (c *thunderClient) DeleteProxyResourceServer(ctx context.Context, proxyHandle string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	// Same limitation as DeleteProxyResourceServerAction: no identifier is
	// known here, so this can only match a pre-migration bare-handle row.
	rs, err := c.findProxyResourceServer(ctx, token, proxyHandle, "")
	if err != nil {
		return err
	}
	if rs == nil {
		return nil
	}
	rsID := rs.ID
	resourceID, err := c.findProxyResourceID(ctx, token, rsID, proxyHandle)
	if err != nil {
		return err
	}
	if resourceID != "" {
		actions, err := c.listProxyActions(ctx, token, rsID, resourceID)
		if err != nil {
			return err
		}
		for action, actionID := range actions {
			if _, err := c.doRequest(ctx, http.MethodDelete, c.baseURL+"/resource-servers/"+rsID+"/resources/"+resourceID+"/actions/"+actionID, token, nil); err != nil && !IsNotFound(err) {
				return fmt.Errorf("thunder delete proxy action %q: %w", action, err)
			}
		}
		if _, err := c.doRequest(ctx, http.MethodDelete, c.baseURL+"/resource-servers/"+rsID+"/resources/"+resourceID, token, nil); err != nil && !IsNotFound(err) {
			return fmt.Errorf("thunder delete proxy resource %q: %w", proxyHandle, err)
		}
	}
	if _, err := c.doRequest(ctx, http.MethodDelete, c.baseURL+"/resource-servers/"+rsID, token, nil); err != nil && !IsNotFound(err) {
		return fmt.Errorf("thunder delete proxy resource server %q: %w", proxyHandle, err)
	}
	return nil
}

// findProxyResourceID returns the ID of the proxy's permission-anchor resource
// (the top-level resource with Handle == proxyHandle, created by
// ensureProxyResource), or "" if it does not exist yet.
func (c *thunderClient) findProxyResourceID(ctx context.Context, token, rsID, proxyHandle string) (string, error) {
	resources, err := c.listResources(ctx, token, rsID, "")
	if err != nil {
		return "", fmt.Errorf("thunder list proxy resources: %w", err)
	}
	for _, res := range resources {
		if res.Handle == proxyHandle {
			return res.ID, nil
		}
	}
	return "", nil
}

// ensureProxyResource makes sure a single top-level resource exists on the
// resource server with Handle == proxyHandle, and returns its ID.
//
// Thunder only derives an item's permission by composing its own handle with
// its resource *parent* chain: a root-level item with no parent resource
// keeps its bare handle verbatim, with no resource-server-handle prefix —
// resource servers themselves carry no handle at all. So an action
// registered directly at the resource server's root ends up with permission
// == its own handle (e.g. "read"), never "<proxyHandle>:read". Anchoring
// every action under this resource instead (its own permission is just
// proxyHandle, since it too has no parent) composes to
// "<proxyHandle>:<action>" one level down, matching the scope string
// exactly. This mirrors the same workaround the AMP resource server's own
// bootstrap uses for its top-level "amp" resource (see
// amp-thunder-bootstrap.yaml).
func (c *thunderClient) ensureProxyResource(ctx context.Context, token, rsID, proxyHandle, displayName string) (string, error) {
	resourceID, err := c.findProxyResourceID(ctx, token, rsID, proxyHandle)
	if err != nil {
		return "", err
	}
	if resourceID != "" {
		return resourceID, nil
	}
	body, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/resource-servers/"+rsID+"/resources", token,
		map[string]string{"name": displayName, "handle": proxyHandle})
	if err != nil {
		// ensureProxyResourceServerMu only serializes calls within this
		// process; a concurrent request handled by a different AMS replica
		// (or a client recreated after the cache entry expired mid-flight)
		// can race to create the same anchor resource. Thunder rejects the
		// loser with 409 RES-1014 "Handle conflict" — look the winner's
		// resource up by handle instead of failing the whole role write over
		// a benign race.
		if IsConflict(err) {
			resourceID, findErr := c.findProxyResourceID(ctx, token, rsID, proxyHandle)
			if findErr != nil {
				return "", findErr
			}
			if resourceID != "" {
				return resourceID, nil
			}
		}
		return "", fmt.Errorf("thunder create proxy resource: %w", err)
	}
	var created ThunderResource
	if err := json.Unmarshal(body, &created); err != nil {
		return "", fmt.Errorf("thunder create proxy resource decode: %w", err)
	}
	return created.ID, nil
}

// listProxyActions returns the actions registered under the proxy's
// permission-anchor resource as a map of action handle to action ID,
// paginating through all pages.
func (c *thunderClient) listProxyActions(ctx context.Context, token, rsID, resourceID string) (map[string]string, error) {
	const actPageSize = 20
	actions := make(map[string]string)
	offset := 0
	for {
		url := fmt.Sprintf("%s/resource-servers/%s/resources/%s/actions?offset=%d&limit=%d", c.baseURL, rsID, resourceID, offset, actPageSize)
		body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
		if err != nil {
			return nil, fmt.Errorf("thunder list proxy actions: %w", err)
		}
		var page thunderActionList
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("thunder list proxy actions decode: %w", err)
		}
		for _, a := range page.Actions {
			if a.Handle != "" {
				actions[a.Handle] = a.ID
			}
		}
		offset += len(page.Actions)
		if offset >= page.TotalResults || len(page.Actions) == 0 {
			break
		}
	}
	return actions, nil
}

// createProxyAction registers one action under the proxy's permission-anchor
// resource. Thunder derives the action's permission by joining the parent
// resource's own permission (== proxyHandle) with the action handle using the
// RS delimiter (":"), so the derived permission equals the
// "<proxy-handle>:<action>" scope exactly. Callers must ensure the action is
// within thunderHandleMaxLen before invoking this.
func (c *thunderClient) createProxyAction(ctx context.Context, token, rsID, resourceID, action string) error {
	_, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/resource-servers/"+rsID+"/resources/"+resourceID+"/actions", token,
		map[string]string{"name": action, "handle": action})
	if err != nil {
		return fmt.Errorf("thunder create proxy action %q: %w", action, err)
	}
	return nil
}

// ListUsersByOUId fetches users directly from Thunder's OU-scoped endpoint.
// Uses /organization-units/{ouId}/users which is more efficient than fetching all users.
func (c *thunderClient) ListUsersByOUId(ctx context.Context, ouID string, offset, limit int) ([]ThunderUser, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}

	url := fmt.Sprintf("%s/organization-units/%s/users?offset=%d&limit=%d&include=display", c.baseURL, ouID, offset, limit)
	body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("thunder list users by ou id: %w", err)
	}

	var wrapped thunderUserList
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, 0, fmt.Errorf("thunder list users by ou id decode: %w", err)
	}
	return wrapped.Users, wrapped.TotalResults, nil
}

// InviteUser executes Thunder's USER_ONBOARDING flow for the given email address and
// returns the invite link from the final step's additionalData.
//
// The flow is adaptive: after the user-type step, Thunder may present an OU-selection
// step (multi-OU / cloud deployments) or skip straight to the invite-mode choice
// (single-OU / on-prem). The code inspects data.actions in each response to decide
// which step comes next, so it handles both topologies without separate code paths.
func (c *thunderClient) InviteUser(ctx context.Context, email string, ouID string) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", err
	}

	// flowStep holds only what we need from each intermediate response.
	type inviteFlowStep struct {
		ExecutionID    string `json:"executionId"`
		ChallengeToken string `json:"challengeToken"`
		Data           struct {
			Actions []struct {
				Ref string `json:"ref"`
			} `json:"actions"`
		} `json:"data"`
	}
	var flowStep inviteFlowStep

	unmarshalStep := func(body []byte, label string) error {
		flowStep = inviteFlowStep{}
		if err := json.Unmarshal(body, &flowStep); err != nil {
			return fmt.Errorf("thunder invite user %s decode: %w", label, err)
		}
		return nil
	}

	hasAction := func(ref string) bool {
		for _, a := range flowStep.Data.Actions {
			if a.Ref == ref {
				return true
			}
		}
		return false
	}

	// Step 1: start the onboarding flow.
	body, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/flow/execute", token,
		map[string]any{"flowType": "USER_ONBOARDING", "verbose": true})
	if err != nil {
		return "", fmt.Errorf("thunder invite user start flow: %w", err)
	}
	if err := unmarshalStep(body, "start flow"); err != nil {
		return "", err
	}
	execID := flowStep.ExecutionID
	challengeToken := flowStep.ChallengeToken

	// Step 2: select user type.
	body, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/flow/execute", token,
		map[string]any{
			"executionId":    execID,
			"challengeToken": challengeToken,
			"inputs":         map[string]string{"userType": "engineer"},
			"verbose":        true,
			"action":         "action_usertype",
		})
	if err != nil {
		return "", fmt.Errorf("thunder invite user submit type: %w", err)
	}
	if err := unmarshalStep(body, "submit type"); err != nil {
		return "", err
	}
	challengeToken = flowStep.ChallengeToken

	// Step 3 (conditional): OU selection. Only presented in multi-OU deployments where
	// Thunder cannot infer the target org unit. Single-OU instances skip this step.
	if hasAction("action_ou_selection") {
		body, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/flow/execute", token,
			map[string]any{
				"executionId":    execID,
				"challengeToken": challengeToken,
				"inputs":         map[string]string{"ouId": ouID},
				"verbose":        true,
				"action":         "action_ou_selection",
			})
		if err != nil {
			return "", fmt.Errorf("thunder invite user submit ou: %w", err)
		}
		if err := unmarshalStep(body, "submit ou"); err != nil {
			return "", err
		}
		challengeToken = flowStep.ChallengeToken
	}

	// Step 4: select invite mode (create-user-now vs invite-user).
	body, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/flow/execute", token,
		map[string]any{
			"executionId":    execID,
			"challengeToken": challengeToken,
			"verbose":        true,
			"action":         "action_invite_user",
		})
	if err != nil {
		return "", fmt.Errorf("thunder invite user select invite mode: %w", err)
	}
	if err := unmarshalStep(body, "select invite mode"); err != nil {
		return "", err
	}
	challengeToken = flowStep.ChallengeToken

	// Step 5: submit the invitee's email address.
	// TODO: The groups input still appears in the invitee onboarding form. It must be hidden and
	// left empty — supplying any value for groups breaks user signup completion. Needs a fix to
	// suppress the groups field from the form while keeping its value empty for provisioning.
	body, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/flow/execute", token,
		map[string]any{
			"executionId":    execID,
			"challengeToken": challengeToken,
			"inputs":         map[string]string{"email": email},
			"verbose":        true,
			"action":         "action_submit_email",
		})
	if err != nil {
		return "", fmt.Errorf("thunder invite user submit email: %w", err)
	}
	if err := unmarshalStep(body, "submit email"); err != nil {
		return "", err
	}
	challengeToken = flowStep.ChallengeToken

	// Step 6: request a manually-shareable invite link.
	// action_send_email_invite produces a link that only works via email delivery;
	// action_share_manually produces a link that can be pasted directly in the browser.
	body, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/flow/execute", token,
		map[string]any{
			"executionId":    execID,
			"challengeToken": challengeToken,
			"verbose":        true,
			"action":         "action_share_manually",
		})
	if err != nil {
		return "", fmt.Errorf("thunder invite user share link: %w", err)
	}

	// Parse the final response and walk all known locations where Thunder may embed the link.
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("thunder invite user share link decode: %w", err)
	}

	link := extractInviteLink(raw)
	if link == "" {
		return "", fmt.Errorf("thunder invite user: inviteLink not found in response: %s", string(body))
	}
	return link, nil
}

// extractInviteLink walks common Thunder flow response shapes looking for inviteLink.
func extractInviteLink(m map[string]any) string {
	// Candidate key names Thunder might use for the invite link.
	linkKeys := []string{"inviteLink", "invite_link", "link", "invitationLink"}

	// Check a map for any of the candidate keys.
	findLink := func(src map[string]any) string {
		for _, k := range linkKeys {
			if v, ok := src[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}

	// Top-level link field.
	if link := findLink(m); link != "" {
		return link
	}

	// Top-level additionalData / additionalInfo.
	for _, adKey := range []string{"additionalData", "additionalInfo"} {
		if ad, ok := m[adKey].(map[string]any); ok {
			if link := findLink(ad); link != "" {
				return link
			}
		}
	}

	// One level of wrapping (data / output / result / response).
	for _, wrapKey := range []string{"data", "output", "result", "response"} {
		if nested, ok := m[wrapKey].(map[string]any); ok {
			if link := findLink(nested); link != "" {
				return link
			}
			for _, adKey := range []string{"additionalData", "additionalInfo"} {
				if ad, ok := nested[adKey].(map[string]any); ok {
					if link := findLink(ad); link != "" {
						return link
					}
				}
			}
		}
	}
	return ""
}

// --- HTTP helper ---

// doRequest executes an authenticated HTTP request and returns the response body.
// For DELETE responses with no content it returns nil without error.
func (c *thunderClient) doRequest(ctx context.Context, method, url, token string, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, &NotFoundError{Message: string(body)}
	}
	if resp.StatusCode == http.StatusConflict {
		return nil, &ConflictError{Message: string(body)}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// NotFoundError is returned when Thunder responds with 404.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return "not found: " + e.Message
}

// IsNotFound returns true if the error is a Thunder 404.
func IsNotFound(err error) bool {
	var nfe *NotFoundError
	return errors.As(err, &nfe)
}

// ConflictError is returned when Thunder responds with 409, e.g. a handle or
// name that already exists (RES-1014 "Handle conflict" for resources/actions,
// AGT-1013 for agents).
type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string {
	return "conflict: " + e.Message
}

// IsConflict returns true if the error is a Thunder 409.
func IsConflict(err error) bool {
	var ce *ConflictError
	return errors.As(err, &ce)
}

// ListChildOUs returns the direct child OUs of the given parent OU ID.
func (c *thunderClient) ListChildOUs(ctx context.Context, parentOUID string, limit, offset int) ([]ThunderOU, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	url := fmt.Sprintf("%s/organization-units/%s/ous?limit=%d&offset=%d", c.baseURL, parentOUID, limit, offset)
	body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("thunder list child ous: %w", err)
	}
	var wrapped thunderChildOUList
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, 0, fmt.Errorf("thunder list child ous decode: %w", err)
	}
	return wrapped.OrganizationUnits, wrapped.TotalResults, nil
}

// GetOUIDByHandle returns the Thunder OU ID for the given org handle by calling
// GET /organization-units/tree/{handle}. The result should be cached by callers.
func (c *thunderClient) GetOUIDByHandle(ctx context.Context, handle string) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/organization-units/tree/"+handle, token, nil)
	if err != nil {
		return "", fmt.Errorf("thunder get ou by handle %q: %w", handle, err)
	}
	var ou struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &ou); err != nil {
		return "", fmt.Errorf("thunder get ou by handle %q decode: %w", handle, err)
	}
	if ou.ID == "" {
		return "", fmt.Errorf("thunder ou with handle %q returned no id", handle)
	}
	return ou.ID, nil
}
