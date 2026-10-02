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
		GatewayCreate, GatewayRead, GatewayUpdate, GatewayDelete, GatewayTokenManage,
		DataPlaneRead, DeploymentPipelineRead, DeploymentPipelineCreate, DeploymentPipelineUpdate, DeploymentPipelineDelete,
		GitSecretCreate, GitSecretRead, GitSecretDelete,
		LLMProviderTemplateCreate, LLMProviderTemplateRead, LLMProviderTemplateUpdate, LLMProviderTemplateDelete,
		LLMProviderCreate, LLMProviderRead, LLMProviderUpdate, LLMProviderDelete,
		LLMProviderConfigureGuardrail, LLMProviderConnect, LLMProviderDeploy, LLMProviderAPIKeyManage,
		MCPServerCreate, MCPServerRead, MCPServerUpdate, MCPServerDelete,
		MCPServerConfigureGuardrail, MCPServerConnect, MCPServerAPIKeyManage,
		ScopeCreate, ScopeRead, ScopeUpdate, ScopeDelete,
		LLMProxyCreate, LLMProxyRead, LLMProxyUpdate, LLMProxyDelete, LLMProxyDeploy, LLMProxyAPIKeyManage,
		EvaluatorCreate, EvaluatorRead, EvaluatorUpdate, EvaluatorDelete,
		AgentKindCreate, AgentKindRead, AgentKindUpdate, AgentKindDelete,
		AgentCreate, AgentRead, AgentUpdate, AgentDelete, AgentBuild,
		AgentRollback, AgentSuspend,
		AgentEnvNonProduction, AgentEnvProduction,
		AgentTokenManage, AgentAPIKeyManage,
		MonitorCreate, MonitorRead, MonitorUpdate, MonitorDelete, MonitorExecute,
		MonitorScoreRead, MonitorScorePublish,
		AlertingRead, AlertingManage,
		ObservabilityTraceRead, ObservabilityLogRead,
		ObservabilityBuildLogRead, ObservabilityMetricRead,
		RoleCreate, RoleRead, RoleUpdate, RoleDelete,
		GroupCreate, GroupRead, GroupUpdate, GroupDelete,
		AgentIdentityRead, AgentIdentityCreate, AgentIdentityUpdate, AgentIdentityDelete,
		CatalogRead, RepositoryRead,
		ProfileRead, ProfileUpdateAttributes,
	},

	RoleDeveloper: {
		OrgView,
		ProjectCreate, ProjectRead, ProjectUpdate, ProjectDelete,
		EnvironmentRead,
		GatewayRead,
		DataPlaneRead, DeploymentPipelineRead,
		GitSecretCreate, GitSecretRead, GitSecretDelete,
		LLMProviderTemplateRead,
		LLMProviderRead, LLMProviderConfigureGuardrail, LLMProviderConnect,
		MCPServerRead, MCPServerConfigureGuardrail, MCPServerConnect, MCPServerAPIKeyManage,
		ScopeRead,
		LLMProxyCreate, LLMProxyRead, LLMProxyUpdate, LLMProxyDelete,
		LLMProxyDeploy, LLMProxyAPIKeyManage,
		EvaluatorRead,
		AgentKindCreate, AgentKindRead, AgentKindUpdate, AgentKindDelete,
		AgentCreate, AgentRead, AgentUpdate, AgentDelete, AgentBuild,
		// Suspend belongs with the tier grant, not apart from it: a role that can
		// deploy into an environment and delete the agent outright gains nothing
		// from being unable to stop it.
		AgentEnvNonProduction, AgentSuspend, AgentTokenManage, AgentAPIKeyManage,
		MonitorCreate, MonitorRead, MonitorUpdate, MonitorDelete, MonitorExecute,
		MonitorScoreRead,
		ObservabilityTraceRead, ObservabilityLogRead, ObservabilityBuildLogRead, ObservabilityMetricRead,
		AgentIdentityRead,
		CatalogRead, RepositoryRead,
		ProfileRead, ProfileUpdateAttributes,
	},

	RoleAILead: {
		OrgView,
		ProjectRead,
		EnvironmentRead,
		DataPlaneRead, DeploymentPipelineRead,
		GitSecretRead,
		LLMProviderTemplateCreate, LLMProviderTemplateRead, LLMProviderTemplateUpdate, LLMProviderTemplateDelete,
		LLMProviderCreate, LLMProviderRead, LLMProviderUpdate, LLMProviderDelete,
		LLMProviderConfigureGuardrail, LLMProviderConnect, LLMProviderDeploy, LLMProviderAPIKeyManage,
		MCPServerCreate, MCPServerRead, MCPServerUpdate, MCPServerDelete,
		MCPServerConfigureGuardrail, MCPServerConnect, MCPServerAPIKeyManage,
		ScopeCreate, ScopeRead, ScopeUpdate, ScopeDelete,
		EvaluatorCreate, EvaluatorRead, EvaluatorUpdate, EvaluatorDelete,
		AgentKindRead,
		AgentRead, AgentBuild, AgentEnvNonProduction, AgentSuspend, AgentAPIKeyManage,
		MonitorCreate, MonitorRead, MonitorUpdate, MonitorDelete, MonitorExecute,
		MonitorScoreRead,
		ObservabilityTraceRead, ObservabilityMetricRead,
		CatalogRead, RepositoryRead,
		ProfileRead, ProfileUpdateAttributes,
	},

	RolePlatformEngineer: {
		OrgView,
		ProjectCreate, ProjectRead, ProjectUpdate, ProjectDelete,
		EnvironmentCreate, EnvironmentRead, EnvironmentUpdate, EnvironmentDelete,
		GatewayCreate, GatewayRead, GatewayUpdate, GatewayDelete, GatewayTokenManage,
		DataPlaneRead, DeploymentPipelineRead, DeploymentPipelineCreate, DeploymentPipelineUpdate, DeploymentPipelineDelete,
		GitSecretRead,
		LLMProviderTemplateCreate, LLMProviderTemplateRead, LLMProviderTemplateUpdate, LLMProviderTemplateDelete,
		LLMProviderCreate, LLMProviderRead, LLMProviderUpdate, LLMProviderDelete,
		LLMProviderConfigureGuardrail, LLMProviderConnect, LLMProviderDeploy, LLMProviderAPIKeyManage,
		MCPServerCreate, MCPServerRead, MCPServerUpdate, MCPServerDelete,
		MCPServerConfigureGuardrail, MCPServerConnect, MCPServerAPIKeyManage,
		ScopeCreate, ScopeRead, ScopeUpdate, ScopeDelete,
		EvaluatorRead,
		AgentKindRead,
		AgentRead, AgentBuild, AgentAPIKeyManage,
		AgentRollback, AgentSuspend,
		AgentEnvNonProduction, AgentEnvProduction,
		MonitorCreate, MonitorRead, MonitorUpdate, MonitorDelete, MonitorExecute,
		MonitorScoreRead,
		ObservabilityTraceRead, ObservabilityLogRead, ObservabilityBuildLogRead, ObservabilityMetricRead,
		AgentIdentityRead, AgentIdentityCreate, AgentIdentityUpdate, AgentIdentityDelete,
		CatalogRead,
		ProfileRead, ProfileUpdateAttributes,
	},
}
