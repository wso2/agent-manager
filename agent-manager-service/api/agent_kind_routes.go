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
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
)

func registerAgentKindRoutes(mux *http.ServeMux, ctrl controllers.AgentKindController) {
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/agent-kinds", ctrl.ListKinds)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/agent-kinds/{kindName}", ctrl.GetKind)
	middleware.HandleFuncWithValidation(mux, "PUT /orgs/{orgName}/agent-kinds/{kindName}", ctrl.UpdateKind)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/agent-kinds/{kindName}", ctrl.DeleteKind)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/agent-kinds/{kindName}/versions", ctrl.AddVersion)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/agent-kinds/{kindName}/versions", ctrl.ListVersions)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/agent-kinds/{kindName}/versions/{versionTag}", ctrl.GetVersion)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/agent-kinds/{kindName}/versions/{versionTag}", ctrl.DeleteVersion)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/agent-kinds/{kindName}/agents", ctrl.ListKindAgents)
}
