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
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func registerEventsRoutes(rr *middleware.RouteRegistrar, controller controllers.EventsController) {
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/destination-kinds"), rbac.OrgView, controller.ListDestinationKinds)

	// Destinations are an org admin setting: only the org registers them.
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/destinations"), rbac.EventsRead, controller.ListDestinations)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/events/destinations"), rbac.EventsManage, controller.CreateDestination)
	rr.HandleFuncWithValidationAndAuthz(route("GET", "/orgs/{orgName}/events/destinations/{destinationId}"), rbac.EventsRead, controller.GetDestination)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", "/orgs/{orgName}/events/destinations/{destinationId}"), rbac.EventsManage, controller.UpdateDestination)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", "/orgs/{orgName}/events/destinations/{destinationId}"), rbac.EventsManage, controller.DeleteDestination)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/events/destinations/{destinationId}/rotate-secret"), rbac.EventsManage, controller.RotateDestinationSecret)
	rr.HandleFuncWithValidationAndAuthz(route("POST", "/orgs/{orgName}/events/destinations/{destinationId}/test"), rbac.EventsManage, controller.TestDestination)
}
