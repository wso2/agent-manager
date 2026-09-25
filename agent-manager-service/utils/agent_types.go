//
// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
//

package utils

type AgentProvisioningType string

const (
	InternalAgent AgentProvisioningType = "internal"
	ExternalAgent AgentProvisioningType = "external"
)

type AgentType string

const (
	AgentTypeAPI         AgentType = "agent-api"
	AgentTypeExternalAPI AgentType = "external-agent-api"
)

type AgentSubType string

const (
	AgentSubTypeChatAPI   AgentSubType = "chat-api"
	AgentSubTypeCustomAPI AgentSubType = "custom-api"
	// AgentSubTypeA2A is an agent that speaks the A2A protocol. It provisions
	// with the same agent-api component type as the other two but gets no REST
	// API: the api-configuration trait is not attached, and the agent is
	// published to the gateway as a kind: Agent resource instead.
	AgentSubTypeA2A AgentSubType = "a2a-agent"
)

// IsA2AAgentSubType reports whether a subtype string names an A2A agent. It
// exists so the several gates that branch on this subtype read the same and
// cannot drift apart.
func IsA2AAgentSubType(subType string) bool {
	return subType == string(AgentSubTypeA2A)
}

type InputInterfaceType string

const (
	InputInterfaceTypeHTTP InputInterfaceType = "HTTP"
)
