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

package models

// AgentFilter holds filter options for listing agents
type AgentFilter struct {
	Search           string // search in name, displayName, description
	ProvisioningType string // "internal", "external"
	SortBy           string // "name", "createdAt", "updatedAt"
	SortOrder        string // "asc", "desc"
	Limit            int
	Offset           int
}

// DefaultAgentFilter returns filter with sensible defaults
func DefaultAgentFilter() AgentFilter {
	return AgentFilter{
		SortBy:    "created_at",
		SortOrder: "desc",
		Limit:     20,
		Offset:    0,
	}
}
