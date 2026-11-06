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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Role handlers for CP_MANAGED identity mode. The roles are kept on the identity control plane: they
// are listed and read from it, and a role with its MCP scopes is written there and applied to the
// request's environment, all by the configured services.IdentityRoleProvider. The handlers never
// resolve the environment's own Thunder, and without a provider they report the feature unavailable.
// What they keep from the legacy handlers: the request validation, the scope checks against the
// proxy scope catalog, the hidden native roles, and the response shapes.

// identityRoleProvider returns the role provider when roles are kept on the control plane. When it
// is not (LEGACY_DIRECT mode) it returns cp=false and the caller takes the legacy path. When it is
// but no provider is configured, it writes the 503 itself and returns ok=false.
func identityRoleProvider(w http.ResponseWriter) (provider services.IdentityRoleProvider, cp, ok bool) {
	cp, provider = services.IdentityRolesViaControlPlane()
	if !cp {
		return nil, false, true
	}
	if provider == nil {
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable,
			"Role management through the identity control plane is not configured")
		return nil, true, false
	}
	return provider, true, true
}

// writeIdentityRoleError maps a provider failure to the response.
func writeIdentityRoleError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	switch {
	case errors.Is(err, services.ErrIdentityRoleNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
	case errors.Is(err, services.ErrIdentityRoleUnavailable):
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "Role management through the identity control plane is not configured")
	default:
		logger.GetLogger(r.Context()).Error("agent-identity: role provider failed", "operation", operation,
			"env", r.PathValue("envName"), "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to "+operation+" through the identity control plane")
	}
}

func (c *agentIdentityController) listRolesViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityRoleProvider) {
	offset, limit := paginationParams(r)
	list, err := provider.ListRoles(r.Context(), services.IdentityRoleListRequest{
		OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"), Offset: offset, Limit: limit,
	})
	if err != nil {
		writeIdentityRoleError(w, r, "list roles", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"roles": list.Roles, "total": list.Total, "offset": offset, "limit": limit})
}

// managedRoleViaProvider reads the role and treats the hidden native roles as nonexistent, as
// managedRole does for the environment's own Thunder.
func (c *agentIdentityController) managedRoleViaProvider(
	w http.ResponseWriter, r *http.Request, provider services.IdentityRoleProvider, roleID string,
) (*thundersvc.ThunderRole, bool) {
	role, err := provider.GetRole(r.Context(), services.IdentityRoleRef{
		OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"), RoleID: roleID,
	})
	if err != nil {
		writeIdentityRoleError(w, r, "get role", err)
		return nil, false
	}
	if role == nil || role.Name == thundersvc.NativeAdministratorRoleName || role.Name == thundersvc.AMPSystemClientRoleName {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
		return nil, false
	}
	return role, true
}

func (c *agentIdentityController) getRoleViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityRoleProvider) {
	role, ok := c.managedRoleViaProvider(w, r, provider, r.PathValue(utils.PathParamRoleID))
	if !ok {
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, role)
}

// scopePermissionsViaProvider validates the requested scopes against the proxy scope catalog, makes
// sure each referenced proxy's resource server exists for the environment through the MCP resource
// server provider, and returns the scopes as role permissions grouped by resource server. It writes
// the error response itself when ok=false. Ensuring every resource server first means a role never
// references a permission the environment does not know.
func (c *agentIdentityController) scopePermissionsViaProvider(
	w http.ResponseWriter, r *http.Request, scopes []string,
) ([]thundersvc.RolePermissionRequest, bool) {
	ctx := r.Context()
	ouID := middleware.OUIDFromRequest(r)
	groups, err := c.resolveScopeGroups(ctx, ouID, scopes)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	permissions := make([]thundersvc.RolePermissionRequest, 0, len(groups))
	if len(groups) == 0 {
		return permissions, true
	}
	envID, ok := c.resolveScopeEnvironment(w, r)
	if !ok {
		return nil, false
	}
	envName := r.PathValue("envName")
	for _, handle := range sortedKeys(groups) {
		g := groups[handle]
		rsID, err := c.rsIdentifiers.EnsureResourceServerInEnv(ctx, ouID, envID, envName, g.proxy, g.actions)
		if err != nil {
			c.writeEnsureError(w, r, g, envName, err)
			return nil, false
		}
		permissions = append(permissions, thundersvc.RolePermissionRequest{ResourceServerID: rsID, Permissions: g.scopes})
	}
	return permissions, true
}

// writeEnsureError maps a resource server ensure failure to the response.
func (c *agentIdentityController) writeEnsureError(w http.ResponseWriter, r *http.Request, g *proxyScopeGroup, envName string, err error) {
	switch {
	case errors.Is(err, services.ErrMCPProxyNotDeployedToEnvironment):
		utils.WriteErrorResponse(w, http.StatusBadRequest,
			fmt.Sprintf("MCP proxy %q is not deployed to environment %q; deploy it there before granting its scopes", g.handle, envName))
	case errors.Is(err, services.ErrMCPResourceServerUnavailable):
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "MCP resource servers are not configured for this deployment")
	default:
		logger.GetLogger(r.Context()).Error("agent-identity: ensure proxy resource server failed", "proxy", g.handle, "env", envName, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to register scopes with the environment identity provider")
	}
}

func (c *agentIdentityController) createRoleViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityRoleProvider) {
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
	permissions, ok := c.scopePermissionsViaProvider(w, r, body.Scopes)
	if !ok {
		return
	}
	role, err := provider.CreateRole(r.Context(), services.IdentityRoleWrite{
		OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"),
		Name: &body.Name, Description: body.Description, Permissions: &permissions,
	})
	if err != nil {
		writeIdentityRoleError(w, r, "create role", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, role)
}

func (c *agentIdentityController) updateRoleViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityRoleProvider) {
	roleID := r.PathValue(utils.PathParamRoleID)
	var body spec.AgentIdentityRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !validateReservedRoleName(w, body.Name) {
		return
	}
	// Reading the role first blocks editing the hidden native roles, as managedRole does.
	if _, ok := c.managedRoleViaProvider(w, r, provider, roleID); !ok {
		return
	}
	write := services.IdentityRoleWrite{
		OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"),
		RoleID: roleID, Description: body.Description,
	}
	if body.Name != "" {
		write.Name = &body.Name
	}
	// An omitted "scopes" (nil) is a metadata-only update that leaves the role's permissions alone;
	// an explicit, possibly empty, list replaces them.
	if body.Scopes != nil {
		permissions, ok := c.scopePermissionsViaProvider(w, r, body.Scopes)
		if !ok {
			return
		}
		write.Permissions = &permissions
	}
	role, err := provider.UpdateRole(r.Context(), write)
	if err != nil {
		writeIdentityRoleError(w, r, "update role", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, role)
}

// deleteRoleViaProvider removes the role from the control plane and the environment, after checking
// it is not a hidden native role.
func (c *agentIdentityController) deleteRoleViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityRoleProvider) {
	roleID := r.PathValue(utils.PathParamRoleID)
	if _, ok := c.managedRoleViaProvider(w, r, provider, roleID); !ok {
		return
	}
	if err := provider.DeleteRole(r.Context(), roleRef(r, roleID)); err != nil {
		writeIdentityRoleError(w, r, "delete role", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func roleRef(r *http.Request, roleID string) services.IdentityRoleRef {
	return services.IdentityRoleRef{OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"), RoleID: roleID}
}

// getRoleAssignmentsViaProvider reads the role's agent and group assignees. Like the legacy handler
// it does not hide the native roles, so an existing mis-assignment can be found.
func (c *agentIdentityController) getRoleAssignmentsViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityRoleProvider) {
	assignments, err := provider.GetRoleAssignments(r.Context(), roleRef(r, r.PathValue(utils.PathParamRoleID)))
	if err != nil {
		writeIdentityRoleError(w, r, "get role assignments", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, assignments)
}

// addRoleAssigneesViaProvider assigns agents and groups to the role. Agents must be provisioned in the
// request's environment, and groups must be groups of it, so a role never names an assignee the
// environment does not have.
func (c *agentIdentityController) addRoleAssigneesViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityRoleProvider) {
	roleID := r.PathValue(utils.PathParamRoleID)
	assignments, err := decodeAgentIdentityAssignments(r)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := c.managedRoleViaProvider(w, r, provider, roleID); !ok {
		return
	}
	var agentIDs, groupIDs []string
	for _, entry := range assignments.Assignments {
		if entry.Type == "agent" {
			agentIDs = append(agentIDs, entry.ID)
		} else {
			groupIDs = append(groupIDs, entry.ID)
		}
	}
	if !c.agentsInEnvironment(w, r, agentIDs) || !c.groupsInEnvironment(w, r, groupIDs) {
		return
	}
	if err := provider.AddRoleAssignees(r.Context(), services.IdentityRoleAssignmentWrite{
		OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"), RoleID: roleID,
		Assignments: assignments.Assignments,
	}); err != nil {
		writeIdentityRoleError(w, r, "add role assignees", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

// removeRoleAssigneesViaProvider removes assignees. It does not check the environment's agents, so an
// assignee that is no longer there can still be cleaned up, and like the legacy handler it does not
// hide the native roles.
func (c *agentIdentityController) removeRoleAssigneesViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityRoleProvider) {
	assignments, err := decodeAgentIdentityAssignments(r)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := provider.RemoveRoleAssignees(r.Context(), services.IdentityRoleAssignmentWrite{
		OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"),
		RoleID: r.PathValue(utils.PathParamRoleID), Assignments: assignments.Assignments,
	}); err != nil {
		writeIdentityRoleError(w, r, "remove role assignees", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

// agentsInEnvironment checks that every agent ID is an agent identity provisioned for the request's
// environment, the same set the agent picker offers. It writes a 400 itself when one is not.
func (c *agentIdentityController) agentsInEnvironment(w http.ResponseWriter, r *http.Request, agentIDs []string) bool {
	if len(agentIDs) == 0 {
		return true
	}
	envName := r.PathValue("envName")
	rows, err := c.bindingRepo.FindByOuAndEnvironment(r.Context(), middleware.OUIDFromRequest(r), envName)
	if err != nil {
		logger.GetLogger(r.Context()).Error("agent-identity: list environment agents failed", "env", envName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to check the agents of the environment")
		return false
	}
	known := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.ThunderAgentID != "" {
			known[row.ThunderAgentID] = struct{}{}
		}
	}
	for _, id := range agentIDs {
		if _, ok := known[id]; !ok {
			utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("agent %q is not provisioned in environment %q", id, envName))
			return false
		}
	}
	return true
}

// groupsInEnvironment checks that every group ID is a group of the request's environment, through the
// group provider. It writes the error response itself when one is not, or groups are unavailable.
func (c *agentIdentityController) groupsInEnvironment(w http.ResponseWriter, r *http.Request, groupIDs []string) bool {
	if len(groupIDs) == 0 {
		return true
	}
	provider, _, ok := identityGroupProvider(w)
	if !ok {
		return false
	}
	for _, id := range groupIDs {
		if _, err := provider.GetGroup(r.Context(), services.IdentityGroupRef{
			OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"), GroupID: id,
		}); err != nil {
			if errors.Is(err, services.ErrIdentityGroupNotFound) {
				utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("group %q is not a group of environment %q", id, r.PathValue("envName")))
				return false
			}
			writeIdentityGroupError(w, r, "check the group", err)
			return false
		}
	}
	return true
}
