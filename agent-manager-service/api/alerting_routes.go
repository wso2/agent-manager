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

func registerAlertingRoutes(rr *middleware.RouteRegistrar, controller controllers.AlertingController) {
	orgBase := "/orgs/{orgName}/alerting"

	// Org alert endpoint
	rr.HandleFuncWithValidationAndAuthz(route("GET", orgBase+"/endpoint"), rbac.AlertingRead, controller.GetAlertEndpoint)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", orgBase+"/endpoint"), rbac.AlertingManage, controller.UpsertAlertEndpoint)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", orgBase+"/endpoint"), rbac.AlertingManage, controller.DeleteAlertEndpoint)
	rr.HandleFuncWithValidationAndAuthz(route("POST", orgBase+"/endpoint/test"), rbac.AlertingManage, controller.TestAlertEndpoint)
	rr.HandleFuncWithValidationAndAuthz(route("GET", orgBase+"/deliveries"), rbac.AlertingRead, controller.ListAlertDeliveries)

	// Per-monitor alert rules
	monitorBase := "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/alerting"
	rr.HandleFuncWithValidationAndAuthz(route("GET", monitorBase), rbac.MonitorRead, controller.GetMonitorAlertConfig)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", monitorBase), rbac.MonitorUpdate, controller.UpdateMonitorAlertConfig)
}
