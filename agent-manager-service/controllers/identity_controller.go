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
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/constants"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// IdentityController defines HTTP handlers for user, group, and role management.
type IdentityController interface {
	// Users
	ListUsers(w http.ResponseWriter, r *http.Request)
	GetUser(w http.ResponseWriter, r *http.Request)
	CreateUser(w http.ResponseWriter, r *http.Request)
	UpdateUser(w http.ResponseWriter, r *http.Request)
	DeleteUser(w http.ResponseWriter, r *http.Request)
	GetUserGroups(w http.ResponseWriter, r *http.Request)
	GetUserRoles(w http.ResponseWriter, r *http.Request)
	InviteUser(w http.ResponseWriter, r *http.Request)
	GetUserProfile(w http.ResponseWriter, r *http.Request)
	UpdateCurrentUserProfile(w http.ResponseWriter, r *http.Request)

	// Groups
	ListGroups(w http.ResponseWriter, r *http.Request)
	GetGroup(w http.ResponseWriter, r *http.Request)
	CreateGroup(w http.ResponseWriter, r *http.Request)
	UpdateGroup(w http.ResponseWriter, r *http.Request)
	DeleteGroup(w http.ResponseWriter, r *http.Request)
	AddGroupMembers(w http.ResponseWriter, r *http.Request)
	RemoveGroupMembers(w http.ResponseWriter, r *http.Request)
	GetGroupMembers(w http.ResponseWriter, r *http.Request)
	GetGroupRoles(w http.ResponseWriter, r *http.Request)

	// Roles
	ListRoles(w http.ResponseWriter, r *http.Request)
	GetRole(w http.ResponseWriter, r *http.Request)
	CreateRole(w http.ResponseWriter, r *http.Request)
	UpdateRole(w http.ResponseWriter, r *http.Request)
	DeleteRole(w http.ResponseWriter, r *http.Request)
	GetRoleAssignments(w http.ResponseWriter, r *http.Request)
	AddRolePermissions(w http.ResponseWriter, r *http.Request)
	RemoveRolePermissions(w http.ResponseWriter, r *http.Request)
	AddRoleAssignees(w http.ResponseWriter, r *http.Request)
	RemoveRoleAssignees(w http.ResponseWriter, r *http.Request)

	// Permissions catalog
	ListAMPPermissions(w http.ResponseWriter, r *http.Request)
}

type identityController struct {
	client thundersvc.IdentityClient
}

// NewIdentityController creates a new identity controller.
func NewIdentityController(client thundersvc.IdentityClient) IdentityController {
	return &identityController{client: client}
}

// --- Users ---

func (c *identityController) ListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	offset := getIntQueryParam(r, "offset", 0)
	limit := getIntQueryParam(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	users, total, err := c.client.ListUsersByOUId(ctx, resolvedOrg.OUID, offset, limit)
	if err != nil {
		log.Error("ListUsers failed", "ou_id", resolvedOrg.OUID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list users")
		return
	}

	// Normalize user data: if Display is set but attributes is empty,
	// populate attributes with the username from Display field (from OU-scoped endpoint)
	for i := range users {
		if users[i].Display != "" && users[i].Attributes == nil {
			users[i].Attributes = map[string]any{"username": users[i].Display}
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"users": users, "total": total, "offset": offset, "limit": limit})
}

func (c *identityController) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	userID := r.PathValue(utils.PathParamUserID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	user, err := c.client.GetUser(ctx, userID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("GetUser failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get user")
		return
	}

	if !validateUserOwnership(w, ctx, user, resolvedOrg.OUID) {
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, user)
}

func (c *identityController) CreateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	var body spec.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Type == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "type is required")
		return
	}
	if body.Attributes == nil || body.Attributes["username"] == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "username in attributes is required")
		return
	}
	password, ok := body.Attributes["password"]
	if !ok || strings.TrimSpace(password) == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "password in attributes is required")
		return
	}

	// Convert spec.CreateUserRequest to thundersvc.CreateUserRequest. The OU comes
	// from the token-resolved org only; a body-supplied one would let a caller
	// create users in another tenant's OU.
	req := thundersvc.CreateUserRequest{
		OuID:       resolvedOrg.OUID,
		Type:       body.Type,
		Attributes: body.Attributes,
		Password:   password,
	}

	// The attributes map is free-form and is known to carry a password, so only
	// its key names and shape are recorded — never a value. See
	// audit.AttributeKeySummary.
	attrKeys, attrCount, hasSensitive := audit.AttributeKeySummary(body.Attributes)
	attempt, ok := beginAuditOrFail(
		w, r, "CreateUser", "Failed to create user", audit.ActionUserCreate,
		audit.Org(resolvedOrg.OUID), audit.OrgHandle(resolvedOrg.OuHandle),
		audit.ResourceNamed(audit.ResourceUser, body.Attributes["username"], body.Attributes["username"]),
		audit.Detail("username", body.Attributes["username"]),
		audit.Detail("userType", body.Type),
		audit.Detail("attributeKeys", attrKeys),
		audit.Detail("attributeCount", attrCount),
		audit.Detail("containsSensitiveKey", hasSensitive),
	)
	if !ok {
		return
	}

	user, err := c.client.CreateUser(ctx, req)
	if err != nil {
		attempt.Complete(ctx, err)
		log.Error("CreateUser failed", "username", body.Attributes["username"], "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create user")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusCreated, user)
}

func (c *identityController) UpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	userID := r.PathValue(utils.PathParamUserID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	user, err := c.client.GetUser(ctx, userID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("UpdateUser failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update user")
		return
	}

	if !validateUserOwnership(w, ctx, user, resolvedOrg.OUID) {
		return
	}

	var body spec.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	attrs := make(map[string]string)
	if body.Attributes != nil {
		attrs = *body.Attributes
	}
	// This endpoint only updates attributes (spec.UpdateUserRequest has no type/ouId field), so
	// Type and OuID are carried over from the existing user record fetched above. Thunder's PUT
	// /users/{id} is a full-replace operation that requires a valid type on every call; leaving
	// it empty here fails with "user type not found" rather than preserving the existing type.
	req := thundersvc.UpdateUserRequest{
		Attributes: attrs,
		Type:       user.Type,
		OuID:       user.OuID,
	}

	updatedUser, err := c.client.UpdateUser(ctx, userID, req)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("UpdateUser failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update user")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, updatedUser)
}

func (c *identityController) DeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	userID := r.PathValue(utils.PathParamUserID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	user, err := c.client.GetUser(ctx, userID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("DeleteUser failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	if !validateUserOwnership(w, ctx, user, resolvedOrg.OUID) {
		return
	}

	attempt, ok := beginAuditOrFail(
		w, r, "DeleteUser", "Failed to delete user", audit.ActionUserDelete,
		audit.Org(resolvedOrg.OUID), audit.OrgHandle(resolvedOrg.OuHandle),
		audit.ResourceNamed(audit.ResourceUser, userID, userIdentifier(user)),
		audit.Detail("username", userIdentifier(user)),
	)
	if !ok {
		return
	}

	if err := c.client.DeleteUser(ctx, userID); err != nil {
		attempt.Complete(ctx, err)
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("DeleteUser failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}
	attempt.Complete(ctx, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (c *identityController) GetUserGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	userID := r.PathValue(utils.PathParamUserID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	user, err := c.client.GetUser(ctx, userID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("GetUserGroups failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get user groups")
		return
	}

	if !validateUserOwnership(w, ctx, user, resolvedOrg.OUID) {
		return
	}

	groups, err := c.client.GetUserGroups(ctx, userID)
	if err != nil {
		log.Error("GetUserGroups failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get user groups")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"groups": groups})
}

func (c *identityController) GetUserRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	userID := r.PathValue(utils.PathParamUserID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	user, err := c.client.GetUser(ctx, userID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("GetUserRoles failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get user roles")
		return
	}

	if !validateUserOwnership(w, ctx, user, resolvedOrg.OUID) {
		return
	}

	roles, err := c.client.GetUserRoles(ctx, userID)
	if err != nil {
		log.Error("GetUserRoles failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get user roles")
		return
	}
	for i := range roles {
		roles[i].IsReadOnly = constants.IsPredefinedRole(roles[i].Name)
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"roles": roles})
}

func (c *identityController) InviteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Email == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "email is required")
		return
	}

	// On-prem: Thunder's invite flow has no OU selection step (no child OUs).
	// Cloud: pass the child OU ID so Thunder scopes the invite correctly.
	ouIDForInvite := ""
	if !config.GetConfig().IsOnPremDeployment {
		ouIDForInvite = resolvedOrg.OUID
	}
	// The invite link grants org access to whoever holds it, so an invite is a
	// membership change and is refused when it cannot be recorded. The link
	// itself is never recorded.
	attempt, ok := beginAuditOrFail(
		w, r, "InviteUser", "Failed to invite user", audit.ActionUserInvite,
		audit.Org(resolvedOrg.OUID), audit.OrgHandle(resolvedOrg.OuHandle),
		audit.ResourceNamed(audit.ResourceUser, body.Email, body.Email),
		audit.Detail("email", body.Email),
	)
	if !ok {
		return
	}

	inviteLink, err := c.client.InviteUser(ctx, body.Email, ouIDForInvite)
	if err != nil {
		attempt.Complete(ctx, err)
		log.Error("InviteUser failed", "ou_id", resolvedOrg.OUID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to invite user")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]string{"inviteLink": inviteLink})
}

// --- Groups ---

func (c *identityController) ListGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	offset := getIntQueryParam(r, "offset", 0)
	limit := getIntQueryParam(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	groups, total, err := c.client.ListGroupsByOUId(ctx, resolvedOrg.OUID, offset, limit)
	if err != nil {
		log.Error("ListGroups failed", "ou_id", resolvedOrg.OUID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list groups")
		return
	}

	// Populate OuID for groups from OU-scoped endpoint (they don't return it)
	for i := range groups {
		if groups[i].OuID == "" {
			groups[i].OuID = resolvedOrg.OUID
		}
	}

	// The OU-scoped group listing already hides Thunder's native Administrators
	// group (thundersvc.NativeAdministratorsGroupName) and paginates after that
	// exclusion, so offset/limit/total need no adjustment here.
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"groups": groups, "total": total, "offset": offset, "limit": limit})
}

func (c *identityController) GetGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	group, err := c.client.GetGroup(ctx, groupID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("GetGroup failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get group")
		return
	}

	if !validateGroupOwnership(w, ctx, group, resolvedOrg.OUID) {
		return
	}

	if !validateSystemGroup(w, group.Name) {
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, group)
}

func (c *identityController) CreateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	var body spec.CreateGroupRequest
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

	description := ""
	if body.Description != nil {
		description = *body.Description
	}

	// The OU comes from the token-resolved org only; a body-supplied one would let
	// a caller create groups in another tenant's OU.
	req := thundersvc.CreateGroupRequest{
		Name:        body.Name,
		OuID:        resolvedOrg.OUID,
		Description: description,
	}

	group, err := c.client.CreateGroup(ctx, req)
	if err != nil {
		log.Error("CreateGroup failed", "name", req.Name, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create group")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, group)
}

func (c *identityController) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	group, err := c.client.GetGroup(ctx, groupID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("UpdateGroup failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update group")
		return
	}

	if !validateGroupOwnership(w, ctx, group, resolvedOrg.OUID) {
		return
	}

	if !validateSystemGroup(w, group.Name) {
		return
	}

	var body spec.UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !validateReservedGroupName(w, derefString(body.Name)) {
		return
	}

	// Thunder's PUT /groups/{id} is a full replace: NewGroupReplace preserves the
	// group's current name/description when the body omits them.
	req := thundersvc.NewGroupReplace(*group, body.Name, body.Description)

	updatedGroup, err := c.client.UpdateGroup(ctx, groupID, req)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("UpdateGroup failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update group")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, updatedGroup)
}

func (c *identityController) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	group, err := c.client.GetGroup(ctx, groupID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("DeleteGroup failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete group")
		return
	}

	if !validateGroupOwnership(w, ctx, group, resolvedOrg.OUID) {
		return
	}

	if !validateSystemGroup(w, group.Name) {
		return
	}

	if err := c.client.DeleteGroup(ctx, groupID); err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("DeleteGroup failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *identityController) AddGroupMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	group, err := c.client.GetGroup(ctx, groupID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("AddGroupMembers failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to add group members")
		return
	}

	if !validateGroupOwnership(w, ctx, group, resolvedOrg.OUID) {
		return
	}

	if !validateSystemGroup(w, group.Name) {
		return
	}

	var req struct {
		UserIDs []string `json:"userIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.UserIDs) == 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "userIds must not be empty")
		return
	}

	attempt, ok := beginAuditOrFail(
		w, r, "AddGroupMembers", "Failed to add group members", audit.ActionGroupAddMember,
		audit.Org(resolvedOrg.OUID), audit.OrgHandle(resolvedOrg.OuHandle),
		audit.ResourceNamed(audit.ResourceGroup, groupID, group.Name),
		audit.Detail("groupName", group.Name),
		audit.Detail("members", req.UserIDs),
		audit.Detail("memberCount", len(req.UserIDs)),
	)
	if !ok {
		return
	}

	if err := c.client.AddGroupMembers(ctx, groupID, req.UserIDs); err != nil {
		attempt.Complete(ctx, err)
		log.Error("AddGroupMembers failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to add group members")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

func (c *identityController) RemoveGroupMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	group, err := c.client.GetGroup(ctx, groupID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("RemoveGroupMembers failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove group members")
		return
	}

	if !validateGroupOwnership(w, ctx, group, resolvedOrg.OUID) {
		return
	}

	if !validateSystemGroup(w, group.Name) {
		return
	}

	var req struct {
		UserIDs []string `json:"userIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.UserIDs) == 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "userIds must not be empty")
		return
	}

	attempt, ok := beginAuditOrFail(
		w, r, "RemoveGroupMembers", "Failed to remove group members", audit.ActionGroupRemoveMember,
		audit.Org(resolvedOrg.OUID), audit.OrgHandle(resolvedOrg.OuHandle),
		audit.ResourceNamed(audit.ResourceGroup, groupID, group.Name),
		audit.Detail("groupName", group.Name),
		audit.Detail("members", req.UserIDs),
		audit.Detail("memberCount", len(req.UserIDs)),
	)
	if !ok {
		return
	}

	if err := c.client.RemoveGroupMembers(ctx, groupID, req.UserIDs); err != nil {
		attempt.Complete(ctx, err)
		log.Error("RemoveGroupMembers failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove group members")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

func (c *identityController) GetGroupMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	group, err := c.client.GetGroup(ctx, groupID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("GetGroupMembers failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get group members")
		return
	}

	if !validateGroupOwnership(w, ctx, group, resolvedOrg.OUID) {
		return
	}

	if !validateSystemGroup(w, group.Name) {
		return
	}

	offset := getIntQueryParam(r, "offset", 0)
	limit := getIntQueryParam(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	members, total, err := c.client.GetGroupMembers(ctx, groupID, offset, limit)
	if err != nil {
		log.Error("GetGroupMembers failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get group members")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"users": members, "total": total, "offset": offset, "limit": limit})
}

func (c *identityController) GetGroupRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	groupID := r.PathValue(utils.PathParamGroupID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	group, err := c.client.GetGroup(ctx, groupID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
			return
		}
		log.Error("GetGroupRoles failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get group roles")
		return
	}

	if !validateGroupOwnership(w, ctx, group, resolvedOrg.OUID) {
		return
	}

	if !validateSystemGroup(w, group.Name) {
		return
	}

	roles, err := c.client.GetGroupRoles(ctx, groupID)
	if err != nil {
		log.Error("GetGroupRoles failed", "group_id", groupID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get group roles")
		return
	}
	for i := range roles {
		roles[i].IsReadOnly = constants.IsPredefinedRole(roles[i].Name)
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"roles": roles})
}

// --- Roles ---

func (c *identityController) ListRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	offset := getIntQueryParam(r, "offset", 0)
	limit := getIntQueryParam(r, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	roles, total, err := c.client.ListRoles(ctx, resolvedOrg.OUID, offset, limit)
	if err != nil {
		log.Error("ListRoles failed", "ou_id", resolvedOrg.OUID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list roles")
		return
	}

	// The OU-scoped ListRoles already restricts to the caller's OU and hides
	// Thunder's native Administrator role and the AMP system-client role
	// (thundersvc.NativeAdministratorRoleName, AMPSystemClientRoleName), so its
	// total is the true post-filter count; only the display-only IsReadOnly
	// flag is computed here.
	for i := range roles {
		roles[i].IsReadOnly = constants.IsPredefinedRole(roles[i].Name)
	}

	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"roles": roles, "total": total, "offset": offset, "limit": limit})
}

func (c *identityController) GetRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	role, err := c.client.GetRole(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("GetRole failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get role")
		return
	}

	if !validateRoleOwnership(w, ctx, role, resolvedOrg.OUID) {
		return
	}

	role.IsReadOnly = constants.IsPredefinedRole(role.Name)
	utils.WriteSuccessResponse(w, http.StatusOK, role)
}

func (c *identityController) CreateRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	var body spec.CreateRoleRequest
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

	description := ""
	if body.Description != nil {
		description = *body.Description
	}

	// The OU comes from the token-resolved org only; a body-supplied one would let
	// a caller create roles in another tenant's OU.
	req := thundersvc.CreateRoleRequest{
		Name:        body.Name,
		OuID:        resolvedOrg.OUID,
		Description: description,
	}

	role, err := c.client.CreateRole(ctx, req)
	if err != nil {
		log.Error("CreateRole failed", "name", req.Name, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create role")
		return
	}
	role.IsReadOnly = constants.IsPredefinedRole(role.Name)
	utils.WriteSuccessResponse(w, http.StatusCreated, role)
}

func (c *identityController) UpdateRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	role, err := c.client.GetRole(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("UpdateRole failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update role")
		return
	}

	if !validateRoleOwnership(w, ctx, role, resolvedOrg.OUID) {
		return
	}

	if !validatePredefinedRole(w, role.Name) {
		return
	}

	var body spec.UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !validateReservedRoleName(w, derefString(body.Name)) {
		return
	}

	// Thunder's PUT /roles/{id} is a full replace: NewRoleReplace carries the
	// role's ouId and current permissions and preserves name/description when the
	// body omits them, so a metadata edit never blanks the OU or drops permissions.
	req := thundersvc.NewRoleReplace(*role, body.Name, body.Description)

	updatedRole, err := c.client.UpdateRole(ctx, roleID, req)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("UpdateRole failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update role")
		return
	}
	updatedRole.IsReadOnly = constants.IsPredefinedRole(updatedRole.Name)
	utils.WriteSuccessResponse(w, http.StatusOK, updatedRole)
}

func (c *identityController) DeleteRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	role, err := c.client.GetRole(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("DeleteRole failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete role")
		return
	}

	if !validateRoleOwnership(w, ctx, role, resolvedOrg.OUID) {
		return
	}

	if !validatePredefinedRole(w, role.Name) {
		return
	}

	if err := c.client.DeleteRole(ctx, roleID); err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("DeleteRole failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete role")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *identityController) GetRoleAssignments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	role, err := c.client.GetRole(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("GetRoleAssignments failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get role assignments")
		return
	}

	if !validateRoleOwnership(w, ctx, role, resolvedOrg.OUID) {
		return
	}

	assignments, err := c.client.GetRoleAssignments(ctx, roleID)
	if err != nil {
		log.Error("GetRoleAssignments failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get role assignments")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, assignments)
}

func (c *identityController) AddRolePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	role, err := c.client.GetRole(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("AddRolePermissions failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to add role permissions")
		return
	}

	if !validateRoleOwnership(w, ctx, role, resolvedOrg.OUID) {
		return
	}

	if !validatePredefinedRole(w, role.Name) {
		return
	}

	var req thundersvc.RolePermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// The granted scopes are recorded in full. This is the privilege-escalation
	// path, and a record naming only the role cannot answer "who granted what
	// to whom" — which is the question this event exists for.
	attempt, ok := beginAuditOrFail(
		w, r, "AddRolePermissions", "Failed to add role permissions", audit.ActionRoleGrantPermission,
		audit.Org(resolvedOrg.OUID), audit.OrgHandle(resolvedOrg.OuHandle),
		audit.ResourceNamed(audit.ResourceRole, roleID, role.Name),
		audit.Detail("roleName", role.Name),
		audit.Detail("permissions", req.Permissions),
		audit.Detail("permissionCount", len(req.Permissions)),
		audit.Detail("resourceServerId", req.ResourceServerID),
	)
	if !ok {
		return
	}

	if err := c.client.AddRolePermissions(ctx, roleID, req); err != nil {
		attempt.Complete(ctx, err)
		log.Error("AddRolePermissions failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to add role permissions")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

func (c *identityController) RemoveRolePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	role, err := c.client.GetRole(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("RemoveRolePermissions failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove role permissions")
		return
	}

	if !validateRoleOwnership(w, ctx, role, resolvedOrg.OUID) {
		return
	}

	if !validatePredefinedRole(w, role.Name) {
		return
	}

	var req thundersvc.RolePermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	attempt, ok := beginAuditOrFail(
		w, r, "RemoveRolePermissions", "Failed to remove role permissions", audit.ActionRoleRevokePermission,
		audit.Org(resolvedOrg.OUID), audit.OrgHandle(resolvedOrg.OuHandle),
		audit.ResourceNamed(audit.ResourceRole, roleID, role.Name),
		audit.Detail("roleName", role.Name),
		audit.Detail("permissions", req.Permissions),
		audit.Detail("permissionCount", len(req.Permissions)),
		audit.Detail("resourceServerId", req.ResourceServerID),
	)
	if !ok {
		return
	}

	if err := c.client.RemoveRolePermissions(ctx, roleID, req); err != nil {
		attempt.Complete(ctx, err)
		log.Error("RemoveRolePermissions failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove role permissions")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

func (c *identityController) AddRoleAssignees(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	role, err := c.client.GetRole(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("AddRoleAssignees failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to add role assignees")
		return
	}

	if !validateRoleOwnership(w, ctx, role, resolvedOrg.OUID) {
		return
	}

	req, err := decodeRoleAssigneeRequest(r)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	assigneeIDs, assigneeTypes := assignmentSummary(req.Assignments)
	attempt, ok := beginAuditOrFail(
		w, r, "AddRoleAssignees", "Failed to add role assignees", audit.ActionRoleAssign,
		audit.Org(resolvedOrg.OUID), audit.OrgHandle(resolvedOrg.OuHandle),
		audit.ResourceNamed(audit.ResourceRole, roleID, role.Name),
		audit.Detail("roleName", role.Name),
		audit.Detail("assignees", assigneeIDs),
		audit.Detail("assigneeTypes", assigneeTypes),
		audit.Detail("assigneeCount", len(req.Assignments)),
	)
	if !ok {
		return
	}

	if err := c.client.AddRoleAssignees(ctx, roleID, req); err != nil {
		attempt.Complete(ctx, err)
		log.Error("AddRoleAssignees failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to add role assignees")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

func (c *identityController) RemoveRoleAssignees(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	roleID := r.PathValue(utils.PathParamRoleID)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	role, err := c.client.GetRole(ctx, roleID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Role not found")
			return
		}
		log.Error("RemoveRoleAssignees failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove role assignees")
		return
	}

	if !validateRoleOwnership(w, ctx, role, resolvedOrg.OUID) {
		return
	}

	req, err := decodeRoleAssigneeRequest(r)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	assigneeIDs, assigneeTypes := assignmentSummary(req.Assignments)
	attempt, ok := beginAuditOrFail(
		w, r, "RemoveRoleAssignees", "Failed to remove role assignees", audit.ActionRoleUnassign,
		audit.Org(resolvedOrg.OUID), audit.OrgHandle(resolvedOrg.OuHandle),
		audit.ResourceNamed(audit.ResourceRole, roleID, role.Name),
		audit.Detail("roleName", role.Name),
		audit.Detail("assignees", assigneeIDs),
		audit.Detail("assigneeTypes", assigneeTypes),
		audit.Detail("assigneeCount", len(req.Assignments)),
	)
	if !ok {
		return
	}

	if err := c.client.RemoveRoleAssignees(ctx, roleID, req); err != nil {
		attempt.Complete(ctx, err)
		log.Error("RemoveRoleAssignees failed", "role_id", roleID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove role assignees")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, struct{}{})
}

// decodeRoleAssigneeRequest converts the frontend { userIds, groupIds } payload
// into the Thunder { assignments: [{type, id}] } format.
func decodeRoleAssigneeRequest(r *http.Request) (thundersvc.RoleAssignmentsRequest, error) {
	var body struct {
		UserIDs  []string `json:"userIds"`
		GroupIDs []string `json:"groupIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return thundersvc.RoleAssignmentsRequest{}, err
	}
	var entries []thundersvc.AssignmentEntry
	for _, id := range body.UserIDs {
		entries = append(entries, thundersvc.AssignmentEntry{ID: id, Type: "user"})
	}
	for _, id := range body.GroupIDs {
		entries = append(entries, thundersvc.AssignmentEntry{ID: id, Type: "group"})
	}
	if len(entries) == 0 {
		return thundersvc.RoleAssignmentsRequest{}, errors.New("at least one userId or groupId is required")
	}
	return thundersvc.RoleAssignmentsRequest{Assignments: entries}, nil
}

// --- Permissions catalog ---

func (c *identityController) ListAMPPermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	perms, rsID, err := c.client.ListAMPPermissions(ctx)
	if err != nil {
		log.Error("ListAMPPermissions failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list AMP permissions")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"permissions": perms, "resourceServerId": rsID})
}

// GetUserProfile retrieves a user's profile information
func (c *identityController) GetUserProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	_, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	userID := r.PathValue(utils.PathParamUserID)
	if userID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "User ID is required")
		return
	}

	// Enforce self-check: caller can only read their own profile
	claims := jwtassertion.GetTokenClaims(ctx)
	if claims == nil || claims.Sub != userID {
		utils.WriteErrorResponse(w, http.StatusForbidden, "You can only view your own profile")
		return
	}

	user, err := c.client.GetUser(ctx, userID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("GetUserProfile failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get user profile")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, user)
}

// UpdateCurrentUserProfile updates the current user's profile information
func (c *identityController) UpdateCurrentUserProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return
	}

	userID := r.PathValue(utils.PathParamUserID)
	if userID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "User ID is required")
		return
	}

	// Enforce self-check: caller can only update their own profile
	claims := jwtassertion.GetTokenClaims(ctx)
	if claims == nil || claims.Sub != userID {
		utils.WriteErrorResponse(w, http.StatusForbidden, "You can only update your own profile")
		return
	}

	var body spec.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	sanitized := sanitizeAttributesForLogging(body.Attributes)
	log.Info("UpdateCurrentUserProfile - received from frontend", "attributes", sanitized)

	// Fetch current user to get their type and existing attributes
	currentUser, err := c.client.GetUser(ctx, userID)
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("Failed to fetch current user", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update user profile")
		return
	}

	// Validate tenant ownership: ensure user belongs to caller's organization
	if !validateUserOwnership(w, ctx, currentUser, resolvedOrg.OUID) {
		return
	}

	// password is deliberately excluded from knownFields here: Thunder's regular update
	// endpoint rejects a password field with USR-1028 "Credential update not allowed" —
	// credential changes are only accepted through the dedicated update-credentials
	// endpoint (see the newPassword handling below).
	knownFields := map[string]bool{"username": true, "given_name": true, "family_name": true, "email": true}

	// Start with existing attributes from current user
	attrs := make(map[string]string)
	if currentUser.Attributes != nil {
		for k, v := range currentUser.Attributes {
			if knownFields[k] {
				if str, ok := v.(string); ok {
					attrs[k] = str
				}
			}
		}
	}

	var newPassword string
	if body.Attributes != nil {
		for k, v := range *body.Attributes {
			if knownFields[k] {
				attrs[k] = v
			} else if k == "password" {
				newPassword = v
			}
		}
	}

	log.Info("UpdateCurrentUserProfile - request details", "user_id", userID, "ou_id", resolvedOrg.OUID, "type", currentUser.Type)
	sanitizedMerged := sanitizeAttributesForLogging(attrs)
	log.Info("UpdateCurrentUserProfile - attributes being sent", "attrs", sanitizedMerged)

	updatedUser, err := c.client.UpdateUser(ctx, userID, thundersvc.UpdateUserRequest{
		Attributes: attrs,
		OuID:       resolvedOrg.OUID,
		Type:       currentUser.Type,
	})
	if err != nil {
		if thundersvc.IsNotFound(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "User not found")
			return
		}
		log.Error("UpdateCurrentUserProfile failed", "user_id", userID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update user profile")
		return
	}

	if newPassword != "" {
		if err := c.client.UpdateUserCredentials(ctx, userID, newPassword); err != nil {
			log.Error("UpdateCurrentUserProfile: password update failed", "user_id", userID, "ou_id", resolvedOrg.OUID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Profile updated, but password change failed")
			return
		}
	}

	sanitizedResponse := sanitizeAttributesForLogging(updatedUser.Attributes)
	log.Info("UpdateCurrentUserProfile - response from Thunder", "user_id", userID, "attributes", sanitizedResponse)
	utils.WriteSuccessResponse(w, http.StatusOK, updatedUser)
}

// validateUserOwnership checks if a user belongs to the caller's OU
func validateUserOwnership(w http.ResponseWriter, ctx context.Context, user *thundersvc.ThunderUser, callerOuID string) bool {
	if user.OuID != "" && user.OuID != callerOuID {
		utils.WriteErrorResponse(w, http.StatusForbidden, "User does not belong to your organization")
		return false
	}
	return true
}

// validateGroupOwnership checks if a group belongs to the caller's OU
func validateGroupOwnership(w http.ResponseWriter, ctx context.Context, group *thundersvc.ThunderGroup, callerOuID string) bool {
	if group.OuID != "" && group.OuID != callerOuID {
		utils.WriteErrorResponse(w, http.StatusForbidden, "Group does not belong to your organization")
		return false
	}
	return true
}

// validateRoleOwnership checks if a role belongs to the caller's OU
func validateRoleOwnership(w http.ResponseWriter, ctx context.Context, role *thundersvc.ThunderRole, callerOuID string) bool {
	if role.OuID != "" && role.OuID != callerOuID {
		utils.WriteErrorResponse(w, http.StatusForbidden, "Role does not belong to your organization")
		return false
	}
	return true
}

// sanitizeAttributesForLogging removes sensitive fields before logging
func sanitizeAttributesForLogging(attrs interface{}) map[string]string {
	result := make(map[string]string)
	sensitiveFields := map[string]bool{"password": true}

	switch v := attrs.(type) {
	case map[string]string:
		for k, val := range v {
			if !sensitiveFields[k] {
				result[k] = val
			}
		}
	case *map[string]string:
		if v != nil {
			for k, val := range *v {
				if !sensitiveFields[k] {
					result[k] = val
				}
			}
		}
	case map[string]interface{}:
		for k := range v {
			if !sensitiveFields[k] {
				result[k] = "[set]"
			}
		}
	}

	return result
}

func isPredefinedRole(roleName string) bool {
	return constants.IsPredefinedRole(roleName)
}

func validatePredefinedRole(w http.ResponseWriter, roleName string) bool {
	if isPredefinedRole(roleName) {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Predefined roles cannot be edited or deleted")
		return false
	}
	return true
}

// validateSystemGroup checks if a group is a system group and blocks access if so
func validateSystemGroup(w http.ResponseWriter, groupName string) bool {
	if groupName == thundersvc.NativeAdministratorsGroupName {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Group not found")
		return false
	}
	return true
}

// validateReservedName rejects a request that would claim a name Thunder or our
// own bootstrap reserves for one of its seeded principals, which would shadow the
// hidden system resource and confuse an operator about which one grants admin.
//
// Callers pass the *requested* name, never the resource's current one — that is
// validateSystemGroup/validatePredefinedRole's job. An absent name means the
// caller is not renaming, and passes.
func validateReservedName(w http.ResponseWriter, requested string, reserved []string, kind string) bool {
	for _, r := range reserved {
		if requested == r {
			utils.WriteErrorResponse(w, http.StatusBadRequest,
				fmt.Sprintf("%q is a reserved %s name", r, kind))
			return false
		}
	}
	return true
}

func validateReservedGroupName(w http.ResponseWriter, groupName string) bool {
	return validateReservedName(w, groupName, []string{thundersvc.NativeAdministratorsGroupName}, "group")
}

// validateReservedRoleName complements validatePredefinedRole, which only
// inspects a role's current name — without this an ordinary role could be
// renamed to Administrator or AMP System Client Thunder Admin.
func validateReservedRoleName(w http.ResponseWriter, roleName string) bool {
	return validateReservedName(w, roleName,
		[]string{thundersvc.NativeAdministratorRoleName, thundersvc.AMPSystemClientRoleName}, "role")
}

// assignmentSummary flattens role assignments into parallel id and type lists
// for an audit record. Principal ids and types are identifiers, not secrets,
// and "which principals were assigned" is the point of the event.
func assignmentSummary(assignments []thundersvc.AssignmentEntry) (ids, types []string) {
	ids = make([]string, 0, len(assignments))
	types = make([]string, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.ID)
		types = append(types, a.Type)
	}
	return ids, types
}

// userIdentifier returns the most useful human-readable handle for a user in an
// audit record, falling back through the attributes Thunder populates.
func userIdentifier(user *thundersvc.ThunderUser) string {
	if user == nil {
		return ""
	}
	for _, key := range []string{"username", "email", "userName"} {
		// Attributes is a free-form map, so a non-string value is possible and
		// must not panic on the audit path.
		if v, ok := user.Attributes[key].(string); ok && v != "" {
			return v
		}
	}
	return user.ID
}
