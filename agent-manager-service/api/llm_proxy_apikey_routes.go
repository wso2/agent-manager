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
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// RegisterLLMProxyAPIKeyRoutes registers API key routes for LLM proxies
func RegisterLLMProxyAPIKeyRoutes(rr *middleware.RouteRegistrar, ctrl controllers.LLMProxyAPIKeyController) {
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys", rbac.LLMProxyAPIKeyManage, ctrl.ListAPIKeys)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys", rbac.LLMProxyAPIKeyManage,
		growthanalytics.Track("amp.connections.llm-proxy-api-key", actionDims("issued-proxy-api-key"), ctrl.CreateAPIKey))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys/{keyName}", rbac.LLMProxyAPIKeyManage,
		growthanalytics.Track("amp.connections.llm-proxy-api-key", actionDims("revoked-proxy-api-key"), ctrl.RevokeAPIKey))
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys/{keyName}", rbac.LLMProxyAPIKeyManage,
		growthanalytics.Track("amp.connections.llm-proxy-api-key", actionDims("rotated-proxy-api-key"), ctrl.RotateAPIKey))
}
