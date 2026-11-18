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

package gwapi

import (
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

func registerAgentIdentityRoutes(rr Router, ctrl gwctl.AgentIdentityController) {
	// Groups
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envName}/agent-identities/groups", gwperm.AgentIdentityRead, ctrl.ListGroups)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/environments/{envName}/agent-identities/groups", gwperm.AgentIdentityCreate,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("group", "created-agent-identity-group"), ctrl.CreateGroup))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}", gwperm.AgentIdentityRead, ctrl.GetGroup)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}", gwperm.AgentIdentityUpdate,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("group", "updated-agent-identity-group"), ctrl.UpdateGroup))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}", gwperm.AgentIdentityDelete,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("group", "deleted-agent-identity-group"), ctrl.DeleteGroup))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}/members", gwperm.AgentIdentityRead, ctrl.GetGroupMembers)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}/members/add", gwperm.AgentIdentityUpdate,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("group", "added-agent-identity-group-member"), ctrl.AddGroupMembers))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}/members/remove", gwperm.AgentIdentityUpdate,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("group", "removed-agent-identity-group-member"), ctrl.RemoveGroupMembers))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}/roles", gwperm.AgentIdentityRead, ctrl.GetGroupRoles)

	// Roles
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envName}/agent-identities/roles", gwperm.AgentIdentityRead, ctrl.ListRoles)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/environments/{envName}/agent-identities/roles", gwperm.AgentIdentityCreate,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("role", "created-agent-identity-role"), ctrl.CreateRole))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}", gwperm.AgentIdentityRead, ctrl.GetRole)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}", gwperm.AgentIdentityUpdate,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("role", "updated-agent-identity-role"), ctrl.UpdateRole))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}", gwperm.AgentIdentityDelete,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("role", "deleted-agent-identity-role"), ctrl.DeleteRole))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}/assignments", gwperm.AgentIdentityRead, ctrl.GetRoleAssignments)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}/assignments/add", gwperm.AgentIdentityUpdate,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("role", "added-agent-identity-role-assignee"), ctrl.AddRoleAssignees))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}/assignments/remove", gwperm.AgentIdentityUpdate,
		growthanalytics.Track("amp.security-access.identity-group-role", identityGroupRoleDims("role", "removed-agent-identity-role-assignee"), ctrl.RemoveRoleAssignees))

	// Agents picker
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envName}/agent-identities/agents", gwperm.AgentIdentityRead, ctrl.ListAgents)
}

// identityGroupRoleDims builds the growth-analytics dimensions for
// "amp.security-access.identity-group-role", tagging whether the route
// operates on a group or a role and which action it performs.
func identityGroupRoleDims(resourceType, action string) map[string]interface{} {
	return map[string]interface{}{"resource_type": resourceType, "action": action}
}
