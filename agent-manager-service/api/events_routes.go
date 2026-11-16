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
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func registerEventsRoutes(rr *middleware.RouteRegistrar, controller controllers.EventsController) {
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/event-types"), rbac.OrgView, controller.ListEventTypes)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/destination-kinds"), rbac.OrgView, controller.ListDestinationKinds)

	// Destinations are an org admin setting: only the org registers them.
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/destinations"), rbac.EventsRead, controller.ListDestinations)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/events/destinations"), rbac.EventsManage,
		growthanalytics.Track("amp.events.manage-destination", actionDims("created-event-destination"), controller.CreateDestination))
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/destinations/{destinationId}"), rbac.EventsRead, controller.GetDestination)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/orgs/{orgName}/events/destinations/{destinationId}"), rbac.EventsManage,
		growthanalytics.Track("amp.events.manage-destination", actionDims("updated-event-destination"), controller.UpdateDestination))
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/orgs/{orgName}/events/destinations/{destinationId}"), rbac.EventsManage,
		growthanalytics.Track("amp.events.manage-destination", actionDims("deleted-event-destination"), controller.DeleteDestination))
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/events/destinations/{destinationId}/rotate-secret"), rbac.EventsManage,
		growthanalytics.Track("amp.events.manage-destination", actionDims("rotated-event-destination-secret"), controller.RotateDestinationSecret))
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/events/destinations/{destinationId}/test"), rbac.EventsManage,
		growthanalytics.Track("amp.events.manage-destination", actionDims("tested-event-destination"), controller.TestDestination))

	// Subscriptions are gated by the permissions of what they belong to: org
	// subscriptions are an admin setting, project and agent subscriptions
	// part of the project or agent. Written out per route so the
	// authorization invariant tests can read each permission.

	org := controller.ForScope(models.EventScopeOrg)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/subscriptions"), rbac.EventsRead, org.ListSubscriptions)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/events/subscriptions"), rbac.EventsManage,
		growthanalytics.Track("amp.events.manage-subscription", subscriptionDims("org", "created-event-subscription"), org.CreateSubscription))
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/subscriptions/{subscriptionId}"), rbac.EventsRead, org.GetSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/orgs/{orgName}/events/subscriptions/{subscriptionId}"), rbac.EventsManage,
		growthanalytics.Track("amp.events.manage-subscription", subscriptionDims("org", "updated-event-subscription"), org.UpdateSubscription))
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/orgs/{orgName}/events/subscriptions/{subscriptionId}"), rbac.EventsManage,
		growthanalytics.Track("amp.events.manage-subscription", subscriptionDims("org", "deleted-event-subscription"), org.DeleteSubscription))
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/deliveries"), rbac.EventsRead, org.ListDeliveries)

	project := controller.ForScope(models.EventScopeProject)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/events/destinations"), rbac.ProjectRead, project.AvailableDestinations)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/events/subscriptions"), rbac.ProjectRead, project.ListSubscriptions)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/projects/{projName}/events/subscriptions"), rbac.ProjectUpdate,
		growthanalytics.Track("amp.events.manage-subscription", subscriptionDims("project", "created-event-subscription"), project.CreateSubscription))
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/events/subscriptions/{subscriptionId}"), rbac.ProjectRead, project.GetSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/orgs/{orgName}/projects/{projName}/events/subscriptions/{subscriptionId}"), rbac.ProjectUpdate,
		growthanalytics.Track("amp.events.manage-subscription", subscriptionDims("project", "updated-event-subscription"), project.UpdateSubscription))
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/orgs/{orgName}/projects/{projName}/events/subscriptions/{subscriptionId}"), rbac.ProjectUpdate,
		growthanalytics.Track("amp.events.manage-subscription", subscriptionDims("project", "deleted-event-subscription"), project.DeleteSubscription))
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/events/deliveries"), rbac.ProjectRead, project.ListDeliveries)

	agent := controller.ForScope(models.EventScopeAgent)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/events/destinations"), rbac.AgentRead, agent.AvailableDestinations)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/events/subscriptions"), rbac.AgentRead, agent.ListSubscriptions)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/events/subscriptions"), rbac.AgentUpdate,
		growthanalytics.Track("amp.events.manage-subscription", subscriptionDims("agent", "created-event-subscription"), agent.CreateSubscription))
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/events/subscriptions/{subscriptionId}"), rbac.AgentRead, agent.GetSubscription)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/events/subscriptions/{subscriptionId}"), rbac.AgentUpdate,
		growthanalytics.Track("amp.events.manage-subscription", subscriptionDims("agent", "updated-event-subscription"), agent.UpdateSubscription))
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/events/subscriptions/{subscriptionId}"), rbac.AgentUpdate,
		growthanalytics.Track("amp.events.manage-subscription", subscriptionDims("agent", "deleted-event-subscription"), agent.DeleteSubscription))
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/projects/{projName}/agents/{agentName}/events/deliveries"), rbac.AgentRead, agent.ListDeliveries)
}

func subscriptionDims(scope, action string) map[string]interface{} {
	return map[string]interface{}{"scope": scope, "action": action}
}
