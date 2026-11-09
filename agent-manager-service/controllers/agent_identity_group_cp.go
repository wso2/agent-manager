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
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Group handlers for CP_MANAGED identity mode. The groups of an environment, their members (agents)
// and the roles they hold are managed by the configured services.IdentityGroupProvider and never by
// the environment's own Thunder. What they keep from the legacy handlers: the request validation, the
// hidden native group, and the response shapes. Agents added to a group must be provisioned in the
// request's environment, the same set the agent picker offers.

// identityGroupProvider returns the group provider when groups are managed through the control plane.
// When they are not (LEGACY_DIRECT mode) it returns cp=false and the caller takes the legacy path.
// When they are but no provider is configured, it writes the 503 itself and returns ok=false.
func identityGroupProvider(w http.ResponseWriter) (provider services.IdentityGroupProvider, cp, ok bool) {
	cp, provider = services.IdentityGroupsViaControlPlane()
	if !cp {
		return nil, false, true
	}
	if provider == nil {
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable,
			"Group management through the identity control plane is not configured")
		return nil, true, false
	}
	return provider, true, true
}

// writeIdentityGroupError maps a provider failure to the response.
func writeIdentityGroupError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	switch {
	case errors.Is(err, services.ErrIdentityGroupNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
	case errors.Is(err, services.ErrIdentityGroupUnsupported):
		utils.WriteErrorResponse(w, http.StatusNotImplemented, "Groups are not supported through the identity control plane yet")
	case errors.Is(err, thundersvc.ErrThunderNotProvisioned):
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "The identity provider is not set up for this environment yet")
	case errors.Is(err, services.ErrIdentityGroupUnavailable):
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "Group management through the identity control plane is not configured")
	case errors.Is(err, services.ErrIdentityRoleNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
	default:
		logger.GetLogger(r.Context()).Error("agent-identity: group provider failed", "operation", operation,
			"env", r.PathValue("envName"), "error", err)
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Failed to "+operation+" through the identity control plane")
	}
}

func groupRef(r *http.Request, groupID string) services.IdentityGroupRef {
	return services.IdentityGroupRef{OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"), GroupID: groupID}
}

// managedGroupViaProvider reads the group and treats the hidden native group as nonexistent, as
// managedGroup does for the environment's own Thunder.
func (c *agentIdentityController) managedGroupViaProvider(
	w http.ResponseWriter, r *http.Request, provider services.IdentityGroupProvider, groupID string,
) (*thundersvc.ThunderGroup, bool) {
	group, err := provider.GetGroup(r.Context(), groupRef(r, groupID))
	if err != nil {
		writeIdentityGroupError(w, r, "get group", err)
		return nil, false
	}
	if group == nil {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
		return nil, false
	}
	if !validateSystemGroup(w, group.Name) {
		return nil, false
	}
	return group, true
}

func (c *agentIdentityController) listGroupsViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityGroupProvider) {
	offset, limit := paginationParams(r)
	list, err := provider.ListGroups(r.Context(), services.IdentityGroupListRequest{
		OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"), Offset: offset, Limit: limit,
	})
	if err != nil {
		writeIdentityGroupError(w, r, "list groups", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"groups": list.Groups, "total": list.Total, "offset": offset, "limit": limit})
}

func (c *agentIdentityController) getGroupViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityGroupProvider) {
	group, ok := c.managedGroupViaProvider(w, r, provider, r.PathValue(utils.PathParamGroupID))
	if !ok {
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, group)
}

func (c *agentIdentityController) createGroupViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityGroupProvider) {
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
	group, err := provider.CreateGroup(r.Context(), services.IdentityGroupWrite{
		OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"),
		Name: &body.Name, Description: body.Description,
	})
	if err != nil {
		writeIdentityGroupError(w, r, "create group", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, group)
}

// Implementations that opt in check group ownership, visibility, and placement in UpdateGroup.
type validatedGroupUpdate interface {
	ValidatesGroupUpdateTarget() bool
}

func (c *agentIdentityController) updateGroupViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityGroupProvider) {
	groupID := r.PathValue(utils.PathParamGroupID)
	var body spec.AgentIdentityGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !validateReservedGroupName(w, body.Name) {
		return
	}
	validator, validates := provider.(validatedGroupUpdate)
	if !validates || !validator.ValidatesGroupUpdateTarget() {
		if _, ok := c.managedGroupViaProvider(w, r, provider, groupID); !ok {
			return
		}
	}
	write := services.IdentityGroupWrite{
		OrganizationID: middleware.OUIDFromRequest(r), Environment: r.PathValue("envName"),
		GroupID: groupID, Description: body.Description,
	}
	if body.Name != "" {
		write.Name = &body.Name
	}
	group, err := provider.UpdateGroup(r.Context(), write)
	if err != nil {
		writeIdentityGroupError(w, r, "update group", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, group)
}

func (c *agentIdentityController) deleteGroupViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityGroupProvider) {
	groupID := r.PathValue(utils.PathParamGroupID)
	if _, ok := c.managedGroupViaProvider(w, r, provider, groupID); !ok {
		return
	}
	if err := provider.DeleteGroup(r.Context(), groupRef(r, groupID)); err != nil {
		writeIdentityGroupError(w, r, "delete group", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *agentIdentityController) getGroupMembersViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityGroupProvider) {
	groupID := r.PathValue(utils.PathParamGroupID)
	if _, ok := c.managedGroupViaProvider(w, r, provider, groupID); !ok {
		return
	}
	offset, limit := paginationParams(r)
	members, err := provider.ListGroupMembers(r.Context(), services.IdentityGroupMembersRequest{
		IdentityGroupRef: groupRef(r, groupID), Offset: offset, Limit: limit,
	})
	if err != nil {
		writeIdentityGroupError(w, r, "get group members", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"members": members.Members, "total": members.Total, "offset": offset, "limit": limit})
}

// changeGroupMembersViaProvider adds or removes agent members. Adding checks the agents against the
// environment's; removing does not, so a member that is no longer there can be cleaned up.
func (c *agentIdentityController) changeGroupMembersViaProvider(
	w http.ResponseWriter, r *http.Request, provider services.IdentityGroupProvider, add bool,
) {
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
	if _, ok := c.managedGroupViaProvider(w, r, provider, groupID); !ok {
		return
	}
	write := services.IdentityGroupMembersWrite{IdentityGroupRef: groupRef(r, groupID), Members: agentMemberEntries(body.AgentIds)}
	var err error
	operation := "remove group members"
	if add {
		if !c.agentsInEnvironment(w, r, body.AgentIds) {
			return
		}
		operation = "add group members"
		err = provider.AddGroupMembers(r.Context(), write)
	} else {
		err = provider.RemoveGroupMembers(r.Context(), write)
	}
	if err != nil {
		writeIdentityGroupError(w, r, operation, err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

func (c *agentIdentityController) getGroupRolesViaProvider(w http.ResponseWriter, r *http.Request, provider services.IdentityGroupProvider) {
	groupID := r.PathValue(utils.PathParamGroupID)
	if _, ok := c.managedGroupViaProvider(w, r, provider, groupID); !ok {
		return
	}
	roles, err := provider.GetGroupRoles(r.Context(), groupRef(r, groupID))
	if err != nil {
		writeIdentityGroupError(w, r, "get group roles", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"roles": roles})
}
