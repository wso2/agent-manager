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

package services

import (
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// agentGatewayShape is how the gateway holds a platform agent: the kind of its artifact and the protocol of its exposure.
type agentGatewayShape struct {
	ArtifactKind string
	Protocol     models.AgentExposureProtocol
}

var a2aAgentShape = agentGatewayShape{ArtifactKind: models.KindAgent, Protocol: models.AgentExposureProtocolA2A}

var agentGatewayShapes = map[utils.AgentSubType]agentGatewayShape{
	utils.AgentSubTypeA2A: a2aAgentShape,
}

func agentGatewayShapeFor(subType string) (agentGatewayShape, bool) {
	shape, ok := agentGatewayShapes[utils.AgentSubType(subType)]
	return shape, ok
}

func agentGatewayShapeForProtocol(protocol models.AgentExposureProtocol) (agentGatewayShape, bool) {
	for _, shape := range agentGatewayShapes {
		if shape.Protocol == protocol {
			return shape, true
		}
	}
	return agentGatewayShape{}, false
}

// agentGatewayShapeOf is a2aAgentShape for every external agent: its artifacts are always of kind Agent.
func agentGatewayShapeOf(agent *models.AgentResponse) (agentGatewayShape, bool) {
	if isExternalProvisioned(agent) {
		return a2aAgentShape, true
	}
	return agentGatewayShapeFor(agent.Type.SubType)
}
