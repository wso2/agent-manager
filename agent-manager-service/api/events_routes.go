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

func registerEventsRoutes(rr *middleware.RouteRegistrar, controller controllers.EventsController) {
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/events/event-types"), rbac.OrgView, controller.ListEventTypes)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/events/destination-kinds"), rbac.OrgView, controller.ListDestinationKinds)

	// Destinations are an org admin setting: only the org registers them.
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/events/destinations"), rbac.EventsRead, controller.ListDestinations)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/events/destinations"), rbac.EventsManage, controller.CreateDestination)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/events/destinations/{destinationId}"), rbac.EventsRead, controller.GetDestination)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/events/destinations/{destinationId}"), rbac.EventsManage, controller.UpdateDestination)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/events/destinations/{destinationId}"), rbac.EventsManage, controller.DeleteDestination)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/events/destinations/{destinationId}/rotate-secret"), rbac.EventsManage, controller.RotateDestinationSecret)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/events/destinations/{destinationId}/test"), rbac.EventsManage, controller.TestDestination)

	// Subscriptions are gated by the permissions of what they belong to: org
	// subscriptions are an admin setting, project and agent subscriptions
	// part of the project or agent. Written out per route so the
	// authorization invariant tests can read each permission.

	org := controller.ForScope(models.EventScopeOrg)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/events/subscriptions"), rbac.EventsRead, org.ListSubscriptions)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/events/subscriptions"), rbac.EventsManage, org.CreateSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/events/subscriptions/{subscriptionId}"), rbac.EventsRead, org.GetSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/events/subscriptions/{subscriptionId}"), rbac.EventsManage, org.UpdateSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/events/subscriptions/{subscriptionId}"), rbac.EventsManage, org.DeleteSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/events/deliveries"), rbac.EventsRead, org.ListDeliveries)

	project := controller.ForScope(models.EventScopeProject)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/projects/{projName}/events/destinations"), rbac.ProjectRead, project.AvailableDestinations)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/projects/{projName}/events/subscriptions"), rbac.ProjectRead, project.ListSubscriptions)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/projects/{projName}/events/subscriptions"), rbac.ProjectUpdate, project.CreateSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/projects/{projName}/events/subscriptions/{subscriptionId}"), rbac.ProjectRead, project.GetSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/projects/{projName}/events/subscriptions/{subscriptionId}"), rbac.ProjectUpdate, project.UpdateSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/projects/{projName}/events/subscriptions/{subscriptionId}"), rbac.ProjectUpdate, project.DeleteSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/projects/{projName}/events/deliveries"), rbac.ProjectRead, project.ListDeliveries)

	agent := controller.ForScope(models.EventScopeAgent)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/projects/{projName}/agents/{agentName}/events/destinations"), rbac.AgentRead, agent.AvailableDestinations)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/projects/{projName}/agents/{agentName}/events/subscriptions"), rbac.AgentRead, agent.ListSubscriptions)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/projects/{projName}/agents/{agentName}/events/subscriptions"), rbac.AgentUpdate, agent.CreateSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/projects/{projName}/agents/{agentName}/events/subscriptions/{subscriptionId}"), rbac.AgentRead, agent.GetSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/projects/{projName}/agents/{agentName}/events/subscriptions/{subscriptionId}"), rbac.AgentUpdate, agent.UpdateSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/projects/{projName}/agents/{agentName}/events/subscriptions/{subscriptionId}"), rbac.AgentUpdate, agent.DeleteSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/projects/{projName}/agents/{agentName}/events/deliveries"), rbac.AgentRead, agent.ListDeliveries)
}
