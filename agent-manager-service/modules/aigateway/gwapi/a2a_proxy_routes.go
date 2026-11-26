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

const a2aProxyBase = "/agents/{agentName}/a2a-proxy"

// registerA2AProxyRoutes gates writes like the deploy-settings save and the card-source routes.
func registerA2AProxyRoutes(rr Router, ctrl gwctl.A2AProxyController) {
	rr.HandleFuncWithValidationAndAuthz("GET "+a2aProxyBase, gwperm.AgentRead, inProject(ctrl.GetA2AProxy))
	rr.HandleFuncWithValidationAndAllAuthz("PUT "+a2aProxyBase, inProject(ctrl.SetA2AProxy),
		gwperm.AgentUpdate, gwperm.DeployNonProduction)
	rr.HandleFuncWithValidationAndAllAuthz("DELETE "+a2aProxyBase, inProject(ctrl.DeleteA2AProxy),
		gwperm.AgentUpdate, gwperm.DeployNonProduction)
	rr.HandleFuncWithValidationAndAllAuthz("POST "+a2aProxyBase+"/refresh", inProject(ctrl.RefreshA2AProxy),
		gwperm.AgentUpdate, gwperm.DeployNonProduction)
}
