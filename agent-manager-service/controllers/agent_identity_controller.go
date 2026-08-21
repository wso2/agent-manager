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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// AgentIdentityController exposes env-Thunder group/role management for agent
// identities, resolved per request via EnvThunderResolver — AMS stores no
// group/role state of its own. Roles carry catalog scopes as their
// permissions. ListAgents is the exception: it reads the assignment picker
// straight from AMS's own state, not Thunder.
type AgentIdentityController interface {
	// Groups
	ListGroups(w http.ResponseWriter, r *http.Request)
	CreateGroup(w http.ResponseWriter, r *http.Request)
	GetGroup(w http.ResponseWriter, r *http.Request)
	UpdateGroup(w http.ResponseWriter, r *http.Request)
	DeleteGroup(w http.ResponseWriter, r *http.Request)
	GetGroupMembers(w http.ResponseWriter, r *http.Request)
	AddGroupMembers(w http.ResponseWriter, r *http.Request)
	RemoveGroupMembers(w http.ResponseWriter, r *http.Request)
	GetGroupRoles(w http.ResponseWriter, r *http.Request)

	// Roles
	ListRoles(w http.ResponseWriter, r *http.Request)
	CreateRole(w http.ResponseWriter, r *http.Request)
	GetRole(w http.ResponseWriter, r *http.Request)
	UpdateRole(w http.ResponseWriter, r *http.Request)
	DeleteRole(w http.ResponseWriter, r *http.Request)
	GetRoleAssignments(w http.ResponseWriter, r *http.Request)
	AddRoleAssignees(w http.ResponseWriter, r *http.Request)
	RemoveRoleAssignees(w http.ResponseWriter, r *http.Request)

	// Agents picker
	ListAgents(w http.ResponseWriter, r *http.Request)
}

// MCPResourceServerIdentifierResolver derives the absolute public URI an MCP
// proxy is invoked at in one environment — the env-Thunder RS identifier.
// The environment is resolved once per request and its UUID passed to each
// per-proxy identifier derivation.
type MCPResourceServerIdentifierResolver interface {
	EnvironmentUUIDByName(ctx context.Context, ouID, envName string) (uuid.UUID, error)
	MCPResourceServerIdentifier(ctx context.Context, ouID string, envID uuid.UUID, proxy *models.MCPProxy) (string, error)
}

type agentIdentityController struct {
	resolver      thundersvc.EnvThunderResolver
	bindingRepo   repositories.AgentThunderClientRepository
	proxyRepo     repositories.MCPProxyRepository
	scopeRepo     repositories.MCPProxyScopeRepository
	rsIdentifiers MCPResourceServerIdentifierResolver
}

// NewAgentIdentityController creates a new agent-identity passthrough controller.
func NewAgentIdentityController(
	resolver thundersvc.EnvThunderResolver,
	bindingRepo repositories.AgentThunderClientRepository,
	proxyRepo repositories.MCPProxyRepository,
	scopeRepo repositories.MCPProxyScopeRepository,
	rsIdentifiers MCPResourceServerIdentifierResolver,
) AgentIdentityController {
	return &agentIdentityController{
		resolver:      resolver,
		bindingRepo:   bindingRepo,
		proxyRepo:     proxyRepo,
		scopeRepo:     scopeRepo,
		rsIdentifiers: rsIdentifiers,
	}
}

// resolveScopeEnvironment resolves the request's environment UUID once for the
// resource-server ensure loop, writing the HTTP error itself when ok=false.
func (c *agentIdentityController) resolveScopeEnvironment(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	ctx := r.Context()
	ouID := middleware.OUIDFromRequest(r)
	envName := r.PathValue("envName")
	envID, err := c.rsIdentifiers.EnvironmentUUIDByName(ctx, ouID, envName)
	if err != nil {
		logger.GetLogger(ctx).Error("agent-identity: resolve environment failed", "env", envName, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to resolve the MCP proxy's gateway address")
		return uuid.Nil, false
	}
	return envID, true
}

// ensureGroupResourceServer derives the group's per-environment identifier and
// ensures its resource server, writing the HTTP error itself when ok=false.
func (c *agentIdentityController) ensureGroupResourceServer(w http.ResponseWriter, r *http.Request, client thundersvc.EnvIdentityClient, envID uuid.UUID, g *proxyScopeGroup) (string, bool) {
	ctx := r.Context()
	ouID := middleware.OUIDFromRequest(r)
	envName := r.PathValue("envName")
	identifier, err := c.rsIdentifiers.MCPResourceServerIdentifier(ctx, ouID, envID, g.proxy)
	if err != nil {
		if errors.Is(err, services.ErrMCPProxyNotDeployedToEnvironment) {
			utils.WriteErrorResponse(w, http.StatusBadRequest,
				fmt.Sprintf("MCP proxy %q is not deployed to environment %q; deploy it there before granting its scopes", g.handle, envName))
			return "", false
		}
		logger.GetLogger(ctx).Error("agent-identity: derive RS identifier failed", "proxy", g.handle, "env", envName, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to resolve the MCP proxy's gateway address")
		return "", false
	}
	rsID, err := client.EnsureProxyResourceServer(ctx, g.handle, displayName(g), identifier, g.actions)
	if err != nil {
		logger.GetLogger(ctx).Error("agent-identity: ensure proxy resource server failed", "proxy", g.handle, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to register scopes with the environment identity provider")
		return "", false
	}
	return rsID, true
}

// envClient resolves the env-Thunder identity client for the request's org+env,
// writing the error response itself when resolution fails (returns ok=false).
// An unprovisioned or unreachable environment surfaces 503 so callers know to
// retry once the environment's Thunder is available, rather than a generic 500.
func (c *agentIdentityController) envClient(w http.ResponseWriter, r *http.Request) (thundersvc.EnvIdentityClient, bool) {
	ouID := middleware.OUIDFromRequest(r)
	envName := r.PathValue("envName")
	client, err := services.ResolveEnvThunderIdentity(r.Context(), c.resolver, ouID, envName)
	if err != nil {
		logger.GetLogger(r.Context()).Error("agent-identity: env-thunder resolve failed", "ou_id", ouID, "env", envName, "error", err)
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable,
			"The environment's identity provider is not available; retry after it is provisioned")
		return nil, false
	}
	return client, true
}

// --- Groups ---

// managedGroup fetches the group and treats Thunder's native Administrators
// group as nonexistent, matching its exclusion from the OU-scoped listing — see
// thundersvc.NativeAdministratorsGroupName for why that group grants admin.
// Writes the error response itself when ok=false.
//
// Unlike managedRole, which leaves RemoveRoleAssignees and the read-only
// assignment lookups open so a mis-assignment can be cleaned up through the
// same API, this guards every group operation: an agent already mis-added to
// the native group must be removed with direct Thunder admin access.
func (c *agentIdentityController) managedGroup(w http.ResponseWriter, r *http.Request, client thundersvc.EnvIdentityClient, groupID string) (*thundersvc.ThunderGroup, bool) {
	ctx := r.Context()
	group, err := client.GetGroup(ctx, groupID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return nil, false
		}
		logger.GetLogger(ctx).Error("agent-identity: get group failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get group")
		return nil, false
	}
	if !validateSystemGroup(w, group.Name) {
		return nil, false
	}
	return group, true
}

func (c *agentIdentityController) ListGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	ouID, err := client.GetDefaultOUID(ctx)
	if err != nil {
		log.Error("agent-identity ListGroups: resolve default OU failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to resolve environment identity provider OU")
		return
	}

	offset, limit := paginationParams(r)
	// ListGroupsByOUId excludes the native Administrators group before paginating,
	// so offset/limit/total need no adjustment here.
	groups, total, err := client.ListGroupsByOUId(ctx, ouID, offset, limit)
	if err != nil {
		log.Error("agent-identity ListGroups failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list groups")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"groups": groups, "total": total, "offset": offset, "limit": limit})
}

func (c *agentIdentityController) CreateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	var body spec.AgentIdentityGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Name == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validateReservedGroupName(w, body.Name) {
		return
	}

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	ouID, err := client.GetDefaultOUID(ctx)
	if err != nil {
		log.Error("agent-identity CreateGroup: resolve default OU failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to resolve environment identity provider OU")
		return
	}

	group, err := client.CreateGroup(ctx, thundersvc.CreateGroupRequest{
		Name:        body.Name,
		OuID:        ouID,
		Description: derefString(body.Description),
	})
	if err != nil {
		log.Error("agent-identity CreateGroup failed", "name", body.Name, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create group")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, group)
}

func (c *agentIdentityController) GetGroup(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue(utils.PathParamGroupID)

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	group, ok := c.managedGroup(w, r, client, groupID)
	if !ok {
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, group)
}

func (c *agentIdentityController) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	var body spec.AgentIdentityGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !validateReservedGroupName(w, body.Name) {
		return
	}

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	current, ok := c.managedGroup(w, r, client, groupID)
	if !ok {
		return
	}
	var namePtr *string
	if body.Name != "" {
		namePtr = &body.Name
	}
	// Thunder's PUT /groups/{id} is a full replace: NewGroupReplace preserves the
	// group's current name when the body omits it, applying only the given fields.
	group, err := client.UpdateGroup(ctx, groupID, thundersvc.NewGroupReplace(*current, namePtr, body.Description))
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("agent-identity UpdateGroup failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update group")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, group)
}

func (c *agentIdentityController) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	if _, ok := c.managedGroup(w, r, client, groupID); !ok {
		return
	}
	if err := client.DeleteGroup(ctx, groupID); err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("agent-identity DeleteGroup failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *agentIdentityController) GetGroupMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	if _, ok := c.managedGroup(w, r, client, groupID); !ok {
		return
	}
	offset, limit := paginationParams(r)
	// Agent-identity group members are agents, not users, so return the raw typed
	// member entries rather than resolving user members.
	members, total, err := client.ListGroupMemberEntries(ctx, groupID, offset, limit)
	if err != nil {
		log.Error("agent-identity GetGroupMembers failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get group members")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"members": members, "total": total, "offset": offset, "limit": limit})
}

func (c *agentIdentityController) AddGroupMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	var body spec.AgentIdentityMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(body.AgentIds) == 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "agentIds must not be empty")
		return
	}

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	if _, ok := c.managedGroup(w, r, client, groupID); !ok {
		return
	}
	if err := client.AddGroupMemberEntries(ctx, groupID, agentMemberEntries(body.AgentIds)); err != nil {
		log.Error("agent-identity AddGroupMembers failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to add group members")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

func (c *agentIdentityController) RemoveGroupMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	var body spec.AgentIdentityMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(body.AgentIds) == 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "agentIds must not be empty")
		return
	}

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	if _, ok := c.managedGroup(w, r, client, groupID); !ok {
		return
	}
	if err := client.RemoveGroupMemberEntries(ctx, groupID, agentMemberEntries(body.AgentIds)); err != nil {
		log.Error("agent-identity RemoveGroupMembers failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove group members")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

func (c *agentIdentityController) GetGroupRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	if _, ok := c.managedGroup(w, r, client, groupID); !ok {
		return
	}
	roles, err := client.GetGroupRoles(ctx, groupID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("agent-identity GetGroupRoles failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get group roles")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"roles": roles})
}

// --- Roles ---

func (c *agentIdentityController) ListRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	ouID, err := client.GetDefaultOUID(ctx)
	if err != nil {
		log.Error("agent-identity ListRoles: resolve default OU failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to resolve environment identity provider OU")
		return
	}

	offset, limit := paginationParams(r)
	roles, total, err := client.ListRoles(ctx, ouID, offset, limit)
	if err != nil {
		log.Error("agent-identity ListRoles failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list roles")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"roles": roles, "total": total, "offset": offset, "limit": limit})
}

// CreateRole validates the requested "<proxy-handle>:<action>" scopes against
// mcp_proxy_scopes, ensures each referenced proxy's resource server exists in the
// environment's Thunder before writing the role, then registers the scopes as the
// role's permissions grouped by resource server. Ensuring every resource server
// before any permission write means a role never references a permission the
// environment does not yet know.
func (c *agentIdentityController) CreateRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	var body spec.AgentIdentityRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Name == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validateReservedRoleName(w, body.Name) {
		return
	}
	scopes := body.Scopes

	groups, err := c.resolveScopeGroups(ctx, middleware.OUIDFromRequest(r), scopes)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	ouID, err := client.GetDefaultOUID(ctx)
	if err != nil {
		log.Error("agent-identity CreateRole: resolve default OU failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to resolve environment identity provider OU")
		return
	}

	// Ensure every referenced proxy's resource server (and its actions) exists
	// before the role write, so no permission add below references an unknown
	// resource server. rsIDByHandle keeps the ensured RS ID per proxy handle.
	rsIDByHandle := make(map[string]string, len(groups))
	if len(groups) > 0 {
		envID, ok := c.resolveScopeEnvironment(w, r)
		if !ok {
			return
		}
		for _, handle := range sortedKeys(groups) {
			g := groups[handle]
			rsID, ok := c.ensureGroupResourceServer(w, r, client, envID, g)
			if !ok {
				return
			}
			rsIDByHandle[handle] = rsID
		}
	}

	role, err := client.CreateRole(ctx, thundersvc.CreateRoleRequest{
		Name:        body.Name,
		OuID:        ouID,
		Description: derefString(body.Description),
	})
	if err != nil {
		log.Error("agent-identity CreateRole failed", "name", body.Name, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create role")
		return
	}

	for _, handle := range sortedKeys(groups) {
		g := groups[handle]
		if err := client.AddRolePermissions(ctx, role.ID, thundersvc.RolePermissionRequest{ResourceServerID: rsIDByHandle[handle], Permissions: g.scopes}); err != nil {
			log.Error("agent-identity CreateRole: add role permissions failed", "role_id", role.ID, "proxy", g.handle, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Role created but scope permissions failed; edit the role to retry")
			return
		}
	}
	// The create response predates the permission writes above, so re-fetch to
	// return the reconciled state. The role already exists, so a read failure
	// here degrades to returning the pre-permission role rather than failing.
	if reconciled, err := client.GetRole(ctx, role.ID); err != nil {
		log.Warn("agent-identity CreateRole: re-fetch after permission write failed; returning role without reconciled scopes", "role_id", role.ID, "error", err)
	} else {
		role = reconciled
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, role)
}

// managedRole fetches the role and treats Thunder's native Administrator role and
// env-Thunder's own bootstrap-seeded AMP System Client Thunder Admin role as
// nonexistent (404, matching their exclusion from ListRoles): both carry the
// built-in "system" scope, and exposing either through this API is how agent
// identities were acquiring env-Thunder admin — see
// thundersvc.NativeAdministratorRoleName and thundersvc.AMPSystemClientRoleName.
// Writes the error response itself when ok=false. RemoveRoleAssignees deliberately
// skips this guard so an existing mis-assignment can still be cleaned up through
// the same API, and the read-only GetRoleAssignments/GetGroupRoles stay unguarded
// for the same reason — finding which agents and groups to clean up requires
// seeing them.
func (c *agentIdentityController) managedRole(w http.ResponseWriter, r *http.Request, client thundersvc.EnvIdentityClient, roleID string) (*thundersvc.ThunderRole, bool) {
	ctx := r.Context()
	role, err := client.GetRole(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return nil, false
		}
		logger.GetLogger(ctx).Error("agent-identity: get role failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get role")
		return nil, false
	}
	if role.Name == thundersvc.NativeAdministratorRoleName || role.Name == thundersvc.AMPSystemClientRoleName {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
		return nil, false
	}
	return role, true
}

func (c *agentIdentityController) GetRole(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue(utils.PathParamRoleID)

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	role, ok := c.managedRole(w, r, client, roleID)
	if !ok {
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, role)
}

// UpdateRole updates the role metadata and reconciles its scope permissions to
// the requested set per resource server: for each referenced proxy's RS, scopes
// requested but not on the role are added and scopes on the role but not requested
// are removed; a proxy dropped from the role entirely has all its permissions
// removed from its RS.
func (c *agentIdentityController) UpdateRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	var body spec.AgentIdentityRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	// managedRole below blocks editing the native Administrator role itself.
	if !validateReservedRoleName(w, body.Name) {
		return
	}
	// scopes, when present, fully replaces the role's permissions. Decoding tells
	// omitted (nil, preserve all) from explicit [] (non-nil, clear all) apart.
	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	current, ok := c.managedRole(w, r, client, roleID)
	if !ok {
		return
	}

	var namePtr *string
	if body.Name != "" {
		namePtr = &body.Name
	}
	// Thunder's PUT /roles/{id} is a full replace: NewRoleReplace carries the
	// role's ouId and current permissions and preserves the name when the body
	// omits it, so a metadata change never blanks the name or drops permissions.
	// The scope reconcile below then applies the requested additions/removals.
	updated, err := client.UpdateRole(ctx, roleID, thundersvc.NewRoleReplace(*current, namePtr, body.Description))
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("agent-identity UpdateRole failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update role")
		return
	}

	// An omitted "scopes" (nil) is a metadata-only update: leave the role's
	// permissions untouched and skip every resource-server write. An explicit
	// (possibly empty) slice falls through to reconcile the permissions to it.
	if body.Scopes == nil {
		utils.WriteSuccessResponse(w, http.StatusOK, updated)
		return
	}
	scopes := body.Scopes

	groups, err := c.resolveScopeGroups(ctx, middleware.OUIDFromRequest(r), scopes)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// desired[rsID] is the requested scope set for that proxy's resource server,
	// ensured to exist before any reconcile write.
	desired := make(map[string][]string, len(groups))
	if len(groups) > 0 {
		envID, ok := c.resolveScopeEnvironment(w, r)
		if !ok {
			return
		}
		for _, handle := range sortedKeys(groups) {
			g := groups[handle]
			rsID, ok := c.ensureGroupResourceServer(w, r, client, envID, g)
			if !ok {
				return
			}
			desired[rsID] = g.scopes
		}
	}

	// currentByRS is the role's existing permission set per resource server.
	currentByRS := make(map[string][]string, len(current.Permissions))
	for _, p := range current.Permissions {
		currentByRS[p.ResourceServerID] = p.Permissions
	}

	// Reconcile every resource server the role touches — newly desired ones and
	// ones it already had. A proxy dropped from the role has no entry in desired
	// but still has one in currentByRS, so it must be visited to clear it.
	rsIDs := make(map[string]struct{}, len(desired)+len(currentByRS))
	for rsID := range desired {
		rsIDs[rsID] = struct{}{}
	}
	for rsID := range currentByRS {
		rsIDs[rsID] = struct{}{}
	}
	for _, rsID := range sortedStringSetKeys(rsIDs) {
		additions := stringSetDifference(desired[rsID], currentByRS[rsID])
		removals := stringSetDifference(currentByRS[rsID], desired[rsID])
		if len(additions) > 0 {
			if err := client.AddRolePermissions(ctx, roleID, thundersvc.RolePermissionRequest{ResourceServerID: rsID, Permissions: additions}); err != nil {
				log.Error("agent-identity UpdateRole: add role permissions failed", "role_id", roleID, "resource_server_id", rsID, "error", err)
				utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update role permissions")
				return
			}
		}
		if len(removals) > 0 {
			if err := client.RemoveRolePermissions(ctx, roleID, thundersvc.RolePermissionRequest{ResourceServerID: rsID, Permissions: removals}); err != nil {
				log.Error("agent-identity UpdateRole: remove role permissions failed", "role_id", roleID, "resource_server_id", rsID, "error", err)
				utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update role permissions")
				return
			}
		}
	}
	// `updated` carries the pre-reconcile permissions echoed by NewRoleReplace,
	// so re-fetch to return the reconciled state. The role is already updated,
	// so a read failure here degrades to returning the pre-reconcile role.
	if reconciled, err := client.GetRole(ctx, roleID); err != nil {
		log.Warn("agent-identity UpdateRole: re-fetch after reconcile failed; returning role without reconciled scopes", "role_id", roleID, "error", err)
	} else {
		updated = reconciled
	}
	utils.WriteSuccessResponse(w, http.StatusOK, updated)
}

func (c *agentIdentityController) DeleteRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	if _, ok := c.managedRole(w, r, client, roleID); !ok {
		return
	}
	if err := client.DeleteRole(ctx, roleID); err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("agent-identity DeleteRole failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete role")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *agentIdentityController) GetRoleAssignments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	assignments, err := client.GetAgentRoleAssignments(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("agent-identity GetRoleAssignments failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get role assignments")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, assignments)
}

func (c *agentIdentityController) AddRoleAssignees(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	assignments, err := decodeAgentIdentityAssignments(r)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	if _, ok := c.managedRole(w, r, client, roleID); !ok {
		return
	}
	if err := client.AddRoleAssignees(ctx, roleID, assignments); err != nil {
		log.Error("agent-identity AddRoleAssignees failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to add role assignees")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

func (c *agentIdentityController) RemoveRoleAssignees(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	assignments, err := decodeAgentIdentityAssignments(r)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	client, ok := c.envClient(w, r)
	if !ok {
		return
	}
	if err := client.RemoveRoleAssignees(ctx, roleID, assignments); err != nil {
		log.Error("agent-identity RemoveRoleAssignees failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove role assignees")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

// --- Agents picker ---

// ListAgents returns the agent-identity bindings provisioned for this
// environment straight from AMS state (no env-Thunder round-trip). It is the
// source for the assignment picker: which agents can be added to a group or
// assigned a role.
func (c *agentIdentityController) ListAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := middleware.OrgHandleFromRequest(r)
	envName := r.PathValue("envName")
	ouID := middleware.OUIDFromRequest(r)

	rows, err := c.bindingRepo.FindByOuAndEnvironment(ctx, ouID, envName)
	if err != nil {
		log.Error("agent-identity ListAgents failed", "org", orgName, "env", envName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list agent identity bindings")
		return
	}
	agents := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		agents = append(agents, map[string]any{
			"agentName":      row.AgentName,
			"projectName":    row.ProjectName,
			"status":         row.Status,
			"thunderAgentId": row.ThunderAgentID,
		})
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"agents": agents})
}

// --- helpers ---

// paginationParams parses and clamps the offset/limit query parameters.
func paginationParams(r *http.Request) (offset, limit int) {
	offset = getIntQueryParam(r, "offset", 0)
	limit = getIntQueryParam(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return offset, limit
}

// agentMemberEntries maps agent IDs to typed group member entries.
func agentMemberEntries(agentIDs []string) []thundersvc.GroupMember {
	members := make([]thundersvc.GroupMember, 0, len(agentIDs))
	for _, id := range agentIDs {
		members = append(members, thundersvc.GroupMember{ID: id, Type: "agent"})
	}
	return members
}

// decodeAgentIdentityAssignments converts the request payload into the Thunder
// assignments shape, accepting only agent and group assignee types.
func decodeAgentIdentityAssignments(r *http.Request) (thundersvc.RoleAssignmentsRequest, error) {
	var body spec.AgentIdentityAssignmentsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return thundersvc.RoleAssignmentsRequest{}, errors.New("invalid request body")
	}
	if len(body.Assignments) == 0 {
		return thundersvc.RoleAssignmentsRequest{}, errors.New("assignments must not be empty")
	}
	entries := make([]thundersvc.AssignmentEntry, 0, len(body.Assignments))
	for _, a := range body.Assignments {
		if a.Type != "agent" && a.Type != "group" {
			return thundersvc.RoleAssignmentsRequest{}, fmt.Errorf("assignment type %q is not allowed (must be agent or group)", a.Type)
		}
		if a.Id == "" {
			return thundersvc.RoleAssignmentsRequest{}, errors.New("assignment id must not be empty")
		}
		entries = append(entries, thundersvc.AssignmentEntry{ID: a.Id, Type: a.Type})
	}
	return thundersvc.RoleAssignmentsRequest{Assignments: entries}, nil
}

// proxyScopeGroup is one proxy's slice of a role's requested scopes.
type proxyScopeGroup struct {
	proxy   *models.MCPProxy
	handle  string
	actions []string // parsed actions, sorted
	scopes  []string // full "<handle>:<action>" strings, sorted
}

// resolveScopeGroups parses and validates requested scope strings against
// mcp_proxy_scopes, grouped by owning proxy. Every scope must be of the form
// "<proxy-handle>:<action>", name an existing proxy in the org, and name an
// action defined on that proxy. Errors name the offending string and map to
// HTTP 400.
func (c *agentIdentityController) resolveScopeGroups(ctx context.Context, ouID string, scopes []string) (map[string]*proxyScopeGroup, error) {
	groups := map[string]*proxyScopeGroup{}
	for _, s := range scopes {
		idx := strings.Index(s, ":")
		if idx <= 0 || idx == len(s)-1 {
			return nil, fmt.Errorf("scope %q is not of the form <proxy-handle>:<action>", s)
		}
		handle, action := s[:idx], s[idx+1:]
		g, ok := groups[handle]
		if !ok {
			proxy, err := c.proxyRepo.GetByHandle(ctx, handle, ouID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("scope %q references unknown MCP proxy %q", s, handle)
				}
				return nil, fmt.Errorf("failed to resolve MCP proxy for scope %q: %w", s, err)
			}
			g = &proxyScopeGroup{proxy: proxy, handle: handle}
			groups[handle] = g
		}
		if _, err := c.scopeRepo.Get(ctx, g.proxy.UUID, action); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("scope %q is not defined on MCP proxy %q", s, handle)
			}
			return nil, fmt.Errorf("failed to look up scope %q: %w", s, err)
		}
		g.actions = append(g.actions, action)
		g.scopes = append(g.scopes, s)
	}
	for _, g := range groups {
		sort.Strings(g.actions)
		sort.Strings(g.scopes)
	}
	return groups, nil
}

// displayName is the Thunder resource-server display name for a proxy group.
func displayName(g *proxyScopeGroup) string {
	if g.proxy.Artifact != nil && g.proxy.Artifact.Name != "" {
		return g.proxy.Artifact.Name
	}
	return g.handle
}

// sortedKeys gives deterministic iteration order over the handle-keyed groups.
func sortedKeys(groups map[string]*proxyScopeGroup) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedStringSetKeys gives deterministic iteration order over a set of strings.
func sortedStringSetKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stringSetDifference returns the elements of a that are not present in b.
func stringSetDifference(a, b []string) []string {
	inB := make(map[string]struct{}, len(b))
	for _, s := range b {
		inB[s] = struct{}{}
	}
	diff := make([]string, 0)
	for _, s := range a {
		if _, ok := inB[s]; !ok {
			diff = append(diff, s)
		}
	}
	return diff
}
