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

package gwapi

import (
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// RegisterAgentAPIKeyRoutes registers API key routes for agents
func RegisterAgentAPIKeyRoutes(rr Router, ctrl gwctl.AgentAPIKeyController) {
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys", gwperm.AgentAPIKeyManage,
		rr.Track("amp.security-access.issue-api-key", keyPurposeDims("production"), ctrl.CreateAPIKey))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/test", gwperm.AgentAPIKeyManage,
		rr.Track("amp.security-access.issue-api-key", keyPurposeDims("test"), ctrl.IssueTestAPIKey))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys", gwperm.AgentAPIKeyManage, ctrl.ListAPIKeys)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}", gwperm.AgentAPIKeyManage,
		rr.Track("amp.security-access.issue-api-key.rotate", actionDims("revoked-agent-api-key"), ctrl.RevokeAPIKey))
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}", gwperm.AgentAPIKeyManage,
		rr.Track("amp.security-access.issue-api-key.rotate", actionDims("rotated-agent-api-key"), ctrl.RotateAPIKey))
}

// keyPurposeDims builds the growth-analytics dimensions for
// "amp.security-access.issue-api-key": key_purpose distinguishes production
// vs. test, while action is fixed since both routes perform the same action.
func keyPurposeDims(purpose string) map[string]any {
	return map[string]any{"key_purpose": purpose, "action": "issued-agent-api-key"}
}
