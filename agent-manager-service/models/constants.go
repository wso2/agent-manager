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

package models

// Agent configuration type strings (API contract)
const (
	AgentConfigTypeLLM   = "llm"
	AgentConfigTypeMCP   = "mcp"
	AgentConfigTypeAgent = "agent"
)

// Agent configuration type IDs (persisted in DB)
const (
	AgentConfigTypeIDLLM   uint = 1
	AgentConfigTypeIDMCP   uint = 2
	AgentConfigTypeIDAgent uint = 3
)

// agentConfigTypeNames maps numeric TypeID → API type string.
var agentConfigTypeNames = map[uint]string{
	AgentConfigTypeIDLLM:   AgentConfigTypeLLM,
	AgentConfigTypeIDMCP:   AgentConfigTypeMCP,
	AgentConfigTypeIDAgent: AgentConfigTypeAgent,
}

// agentConfigTypeIDs maps API type string → numeric TypeID.
var agentConfigTypeIDs = map[string]uint{
	AgentConfigTypeLLM:   AgentConfigTypeIDLLM,
	AgentConfigTypeMCP:   AgentConfigTypeIDMCP,
	AgentConfigTypeAgent: AgentConfigTypeIDAgent,
}

// AgentConfigTypeFromID converts a persisted numeric TypeID to the API string.
// Returns "llm" for unknown IDs.
func AgentConfigTypeFromID(id uint) string {
	if name, ok := agentConfigTypeNames[id]; ok {
		return name
	}
	return AgentConfigTypeLLM
}

// AgentConfigTypeToID converts an API type string to the stable numeric TypeID.
// Returns AgentConfigTypeIDLLM for unknown strings.
func AgentConfigTypeToID(t string) uint {
	if id, ok := agentConfigTypeIDs[t]; ok {
		return id
	}
	return AgentConfigTypeIDLLM
}

// Authentication types
const (
	AuthTypeAPIKey = "api-key"
)

// User roles
const (
	UserRoleSystem = "system"
)

// Status values
const (
	StatusActive  = "active"
	StatusPending = "pending"
	StatusFailed  = "failed"
)

// Version defaults
const (
	DefaultProxyVersion = "1.0.0"
)
