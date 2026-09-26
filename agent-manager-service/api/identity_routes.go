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

package api

import (
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func registerIdentityRoutes(rr *middleware.RouteRegistrar, ctrl controllers.IdentityController) {
	// Users
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/identities/users", ctrl.ListUsers, rbac.OrgInviteMember, rbac.OrgRemoveMember)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/users/invite", rbac.OrgInviteMember,
		growthanalytics.Track("amp.org-management.invite-user", inviteUserDims("invited-user", "invited-user"), ctrl.InviteUser))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/users", rbac.OrgInviteMember,
		growthanalytics.Track("amp.org-management.invite-user", inviteUserDims("created-user-directly", "created-user-directly"), ctrl.CreateUser))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/users/{userID}/profile", rbac.ProfileRead, ctrl.GetUserProfile)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/identities/users/{userID}/profile", rbac.ProfileUpdateAttributes,
		growthanalytics.Track("amp.org-management.update-profile", actionDims("updated-profile"), ctrl.UpdateCurrentUserProfile))
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/identities/users/{userID}", ctrl.GetUser, rbac.OrgInviteMember, rbac.OrgRemoveMember)
	// UpdateUser/DeleteUser share invite-user's feature code per the taxonomy
	// mapping, but its "method" dimension only covers the two creation paths —
	// so these two set only the always-present "action" dimension, no method.
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/identities/users/{userID}", rbac.OrgInviteMember,
		growthanalytics.Track("amp.org-management.invite-user", inviteUserDims("", "updated-user"), ctrl.UpdateUser))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/identities/users/{userID}", rbac.OrgRemoveMember,
		growthanalytics.Track("amp.org-management.invite-user", inviteUserDims("", "deleted-user"), ctrl.DeleteUser))
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/identities/users/{userID}/groups", ctrl.GetUserGroups, rbac.OrgInviteMember, rbac.OrgRemoveMember)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/users/{userID}/roles", rbac.RoleRead, ctrl.GetUserRoles)

	// Groups
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/groups", rbac.GroupRead, ctrl.ListGroups)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/groups", rbac.GroupCreate,
		growthanalytics.Track("amp.org-management.manage-group", actionDims("created-org-group"), ctrl.CreateGroup))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/groups/{groupID}", rbac.GroupRead, ctrl.GetGroup)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/identities/groups/{groupID}", rbac.GroupUpdate,
		growthanalytics.Track("amp.org-management.manage-group", actionDims("updated-org-group"), ctrl.UpdateGroup))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/identities/groups/{groupID}", rbac.GroupDelete,
		growthanalytics.Track("amp.org-management.manage-group", actionDims("deleted-org-group"), ctrl.DeleteGroup))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/groups/{groupID}/members", rbac.GroupRead, ctrl.GetGroupMembers)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/groups/{groupID}/members/add", rbac.GroupUpdate,
		growthanalytics.Track("amp.org-management.manage-group", actionDims("added-org-group-members"), ctrl.AddGroupMembers))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/groups/{groupID}/members/remove", rbac.GroupUpdate,
		growthanalytics.Track("amp.org-management.manage-group", actionDims("removed-org-group-members"), ctrl.RemoveGroupMembers))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/groups/{groupID}/roles", rbac.GroupRead, ctrl.GetGroupRoles)

	// Roles
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/roles", rbac.RoleRead, ctrl.ListRoles)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/roles", rbac.RoleCreate,
		growthanalytics.Track("amp.org-management.manage-role", actionDims("created-org-role"), ctrl.CreateRole))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/roles/{roleID}", rbac.RoleRead, ctrl.GetRole)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/identities/roles/{roleID}", rbac.RoleUpdate,
		growthanalytics.Track("amp.org-management.manage-role", actionDims("updated-org-role"), ctrl.UpdateRole))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/identities/roles/{roleID}", rbac.RoleDelete,
		growthanalytics.Track("amp.org-management.manage-role", actionDims("deleted-org-role"), ctrl.DeleteRole))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/roles/{roleID}/assignments", rbac.RoleRead, ctrl.GetRoleAssignments)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/roles/{roleID}/permissions/add", rbac.RoleUpdate,
		growthanalytics.Track("amp.org-management.manage-role", actionDims("added-org-role-permissions"), ctrl.AddRolePermissions))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/roles/{roleID}/permissions/remove", rbac.RoleUpdate,
		growthanalytics.Track("amp.org-management.manage-role", actionDims("removed-org-role-permissions"), ctrl.RemoveRolePermissions))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/roles/{roleID}/assignees/add", rbac.RoleUpdate,
		growthanalytics.Track("amp.org-management.manage-role", actionDims("added-org-role-assignees"), ctrl.AddRoleAssignees))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/identities/roles/{roleID}/assignees/remove", rbac.RoleUpdate,
		growthanalytics.Track("amp.org-management.manage-role", actionDims("removed-org-role-assignees"), ctrl.RemoveRoleAssignees))

	// Permissions catalog
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/identities/permissions", rbac.RoleRead, ctrl.ListAMPPermissions)
}

// inviteUserDims builds the growth-analytics dimensions for
// "amp.org-management.invite-user": "action" is always set to the full
// lifecycle action, while "method" is set only on the two creation paths
// (invited-user/created-user-directly) per the taxonomy's method enum, which
// doesn't cover update/delete — pass method="" to omit it for those.
func inviteUserDims(method, action string) map[string]interface{} {
	dims := map[string]interface{}{"action": action}
	if method != "" {
		dims["method"] = method
	}
	return dims
}
