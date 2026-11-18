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

package rbac

const (
	RoleAdmin            = "Agent Manager Admin"
	RoleDeveloper        = "Developer"
	RoleAILead           = "AI Lead"
	RolePlatformEngineer = "Platform Engineer"
)

// PredefinedRolePermissions maps each predefined role name to its set of permissions.
// Used at bootstrap time to register role→scope bindings on the Thunder resource server.
var PredefinedRolePermissions = map[string][]Permission{
	RoleAdmin: {
		OrgView, OrgModifySettings, OrgInviteMember, OrgRemoveMember,
		OrgAssignRole, OrgManageIDP, OrgManageServiceAccount,
		ProjectCreate, ProjectRead, ProjectUpdate, ProjectDelete,
		EnvironmentCreate, EnvironmentRead, EnvironmentUpdate, EnvironmentDelete,
		DataPlaneRead, DeploymentPipelineRead, DeploymentPipelineCreate, DeploymentPipelineUpdate, DeploymentPipelineDelete,
		GitSecretCreate, GitSecretRead, GitSecretDelete,
		AgentKindCreate, AgentKindRead, AgentKindUpdate, AgentKindDelete,
		AgentCreate, AgentRead, AgentUpdate, AgentDelete, AgentBuild,
		AgentSuspend, AgentTokenManage,
		EventsRead, EventsManage,
		RoleCreate, RoleRead, RoleUpdate, RoleDelete,
		GroupCreate, GroupRead, GroupUpdate, GroupDelete,
		RepositoryRead,
		ProfileRead, ProfileUpdateAttributes,
	},

	RoleDeveloper: {
		OrgView,
		ProjectCreate, ProjectRead, ProjectUpdate, ProjectDelete,
		EnvironmentRead,
		DataPlaneRead, DeploymentPipelineRead,
		GitSecretCreate, GitSecretRead, GitSecretDelete,
		AgentKindCreate, AgentKindRead, AgentKindUpdate, AgentKindDelete,
		AgentCreate, AgentRead, AgentUpdate, AgentDelete, AgentBuild,
		AgentSuspend, AgentTokenManage,
		RepositoryRead,
		ProfileRead, ProfileUpdateAttributes,
	},

	RoleAILead: {
		OrgView,
		ProjectRead,
		EnvironmentRead,
		DataPlaneRead, DeploymentPipelineRead,
		GitSecretRead,
		AgentKindRead,
		AgentRead, AgentBuild, AgentSuspend,
		RepositoryRead,
		ProfileRead, ProfileUpdateAttributes,
	},

	RolePlatformEngineer: {
		OrgView,
		ProjectCreate, ProjectRead, ProjectUpdate, ProjectDelete,
		EnvironmentCreate, EnvironmentRead, EnvironmentUpdate, EnvironmentDelete,
		DataPlaneRead, DeploymentPipelineRead, DeploymentPipelineCreate, DeploymentPipelineUpdate, DeploymentPipelineDelete,
		GitSecretRead,
		AgentKindRead,
		AgentRead, AgentBuild, AgentSuspend,
		ProfileRead, ProfileUpdateAttributes,
	},
}
