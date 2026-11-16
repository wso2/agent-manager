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

package openai

import "encoding/json"

// The wire types below declare only the fields the connector uses. Fields
// that can hold content (instructions, tool parameters, messages, metadata,
// IP addresses, user agents) are deliberately absent, so they are never
// kept after decoding. Nullable strings decode to "".

// listPage is the cursor list shape of OpenAI list endpoints: data,
// has_more and last_id (the next page is after=last_id).
type listPage[T any] struct {
	Data    *[]T    `json:"data"`
	HasMore bool    `json:"has_more"`
	LastID  *string `json:"last_id"`
}

// agent is an AgentResource (GET /v1/agents). instructions and metadata are
// not decoded.
type agent struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Model     string      `json:"model"`
	CreatedAt int64       `json:"created_at"`
	UpdatedAt int64       `json:"updated_at"`
	Tools     []agentTool `json:"tools"`
}

// agentTool is a PersistedAgentToolResource: its type, its name (function
// tools) and, for MCP tools, the server label and whatever carries the
// server URL. Parameters, descriptions, request metadata and credentials
// are not decoded.
type agentTool struct {
	Type             string          `json:"type"`
	Name             string          `json:"name"`
	ServerLabel      string          `json:"server_label"`
	ServerURL        string          `json:"server_url"`
	Transport        json.RawMessage `json:"transport"`
	ConnectionOrigin json.RawMessage `json:"connection_origin"`
}
