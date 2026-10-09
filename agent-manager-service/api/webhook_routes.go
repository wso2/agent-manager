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
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func registerWebhookRoutes(rr *middleware.RouteRegistrar, controller controllers.WebhookController) {
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/webhook-event-types"), rbac.OrgView, controller.ListEventTypes)

	// Each scope is gated by the permissions of what it belongs to: org
	// webhooks are an admin setting, project and agent webhooks are part of
	// the project or agent. Written out per route so the authorization
	// invariant tests can read each permission.

	org := controller.ForScope(models.WebhookScopeOrg)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/webhooks"), rbac.AlertingRead, org.List)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/webhooks"), rbac.AlertingManage, org.Create)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/webhooks/{webhookId}"), rbac.AlertingRead, org.Get)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/orgs/{orgName}/webhooks/{webhookId}"), rbac.AlertingManage, org.Update)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/orgs/{orgName}/webhooks/{webhookId}"), rbac.AlertingManage, org.Delete)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/webhooks/{webhookId}/rotate-secret"), rbac.AlertingManage, org.RotateSecret)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/webhooks/{webhookId}/test"), rbac.AlertingManage, org.Test)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/webhooks/{webhookId}/deliveries"), rbac.AlertingRead, org.ListDeliveries)

	project := controller.ForScope(models.WebhookScopeProject)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/webhooks"), rbac.ProjectRead, project.List)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/projects/{projName}/webhooks"), rbac.ProjectUpdate, project.Create)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/webhooks/{webhookId}"), rbac.ProjectRead, project.Get)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/orgs/{orgName}/projects/{projName}/webhooks/{webhookId}"), rbac.ProjectUpdate, project.Update)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/orgs/{orgName}/projects/{projName}/webhooks/{webhookId}"), rbac.ProjectUpdate, project.Delete)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/projects/{projName}/webhooks/{webhookId}/rotate-secret"), rbac.ProjectUpdate, project.RotateSecret)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/projects/{projName}/webhooks/{webhookId}/test"), rbac.ProjectUpdate, project.Test)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/webhooks/{webhookId}/deliveries"), rbac.ProjectRead, project.ListDeliveries)

	agent := controller.ForScope(models.WebhookScopeAgent)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/webhooks"), rbac.AgentRead, agent.List)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/webhooks"), rbac.AgentUpdate, agent.Create)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/webhooks/{webhookId}"), rbac.AgentRead, agent.Get)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/webhooks/{webhookId}"), rbac.AgentUpdate, agent.Update)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/webhooks/{webhookId}"), rbac.AgentUpdate, agent.Delete)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/webhooks/{webhookId}/rotate-secret"), rbac.AgentUpdate, agent.RotateSecret)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/webhooks/{webhookId}/test"), rbac.AgentUpdate, agent.Test)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/webhooks/{webhookId}/deliveries"), rbac.AgentRead, agent.ListDeliveries)
}
