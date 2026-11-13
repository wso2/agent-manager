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

func registerMonitorAlertRoutes(rr *middleware.RouteRegistrar, controller controllers.MonitorAlertController) {
	alerting := "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/alerting"
	rr.HandleFuncWithValidationAndAuthz(route("GET", alerting), rbac.MonitorRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz(route("PUT", alerting), rbac.MonitorUpdate, notImplemented)
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", alerting), rbac.MonitorUpdate, notImplemented)
}
