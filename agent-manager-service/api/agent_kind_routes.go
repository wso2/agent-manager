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
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func registerAgentKindRoutes(rr *middleware.RouteRegistrar) {
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/agent-kinds", rbac.AgentKindRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/agent-kinds/{kindName}", rbac.AgentKindRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/agent-kinds/{kindName}", rbac.AgentKindUpdate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/agent-kinds/{kindName}", rbac.AgentKindDelete,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/agent-kinds/{kindName}/versions", rbac.AgentKindUpdate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/agent-kinds/{kindName}/versions", rbac.AgentKindRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/agent-kinds/{kindName}/versions/{versionTag}", rbac.AgentKindRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/agent-kinds/{kindName}/versions/{versionTag}", rbac.AgentKindDelete,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/agent-kinds/{kindName}/agents", rbac.AgentKindRead, notImplemented)
}
