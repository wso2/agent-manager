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

package anthropic

// Wire types. Each one decodes only the fields the connector uses; content
// fields (the agent's system prompt and the custom tools' description and
// input_schema) are deliberately absent, so they are never held in a typed
// value, let alone stored.

// cursorPage is the envelope of the Managed Agents list: {data, next_page};
// the next page is page=next_page.
type cursorPage[T any] struct {
	Data     *[]T    `json:"data"`
	NextPage *string `json:"next_page"`
	HasMore  *bool   `json:"has_more"`
}

// agent is a BetaManagedAgentsAgent, without system and the custom tools'
// description and input_schema.
type agent struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Model       *struct {
		ID string `json:"id"`
	} `json:"model"`
	Tools []struct {
		Type          string `json:"type"`
		Name          string `json:"name"`
		MCPServerName string `json:"mcp_server_name"`
		Configs       []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Enabled *bool  `json:"enabled"`
		} `json:"configs"`
	} `json:"tools"`
	MCPServers []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"mcp_servers"`
	Skills []struct {
		SkillID string `json:"skill_id"`
		Type    string `json:"type"`
	} `json:"skills"`
	ExecutionIdentity *struct {
		Type    string `json:"type"`
		RoleARN string `json:"role_arn"`
	} `json:"execution_identity"`
	CreatedAt  string  `json:"created_at"`
	ArchivedAt *string `json:"archived_at"`
	Version    int64   `json:"version"`
}
