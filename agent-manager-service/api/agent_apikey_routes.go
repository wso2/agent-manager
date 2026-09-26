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

// RegisterAgentAPIKeyRoutes registers API key routes for agents
func RegisterAgentAPIKeyRoutes(rr *middleware.RouteRegistrar, ctrl controllers.AgentAPIKeyController) {
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys", rbac.AgentAPIKeyManage,
		growthanalytics.Track("amp.security-access.issue-api-key", keyPurposeDims("production"), ctrl.CreateAPIKey))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/test", rbac.AgentAPIKeyManage,
		growthanalytics.Track("amp.security-access.issue-api-key", keyPurposeDims("test"), ctrl.IssueTestAPIKey))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys", rbac.AgentAPIKeyManage, ctrl.ListAPIKeys)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}", rbac.AgentAPIKeyManage,
		growthanalytics.Track("amp.security-access.issue-api-key.rotate", actionDims("revoked-agent-api-key"), ctrl.RevokeAPIKey))
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}", rbac.AgentAPIKeyManage,
		growthanalytics.Track("amp.security-access.issue-api-key.rotate", actionDims("rotated-agent-api-key"), ctrl.RotateAPIKey))
}

// keyPurposeDims builds the growth-analytics dimensions for
// "amp.security-access.issue-api-key": key_purpose distinguishes production
// vs. test, while action is fixed since both routes perform the same action.
func keyPurposeDims(purpose string) map[string]interface{} {
	return map[string]interface{}{"key_purpose": purpose, "action": "issued-agent-api-key"}
}
