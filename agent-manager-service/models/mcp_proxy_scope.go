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

import (
	"time"

	"github.com/google/uuid"
)

// MCPProxyScope is one scope on an MCP proxy: an action on the resource server
// the proxy is. The full token scope string is "<proxy-handle>:<action>"; Tools
// is the set of the proxy's MCP tools this scope authorizes.
type MCPProxyScope struct {
	UUID         uuid.UUID `gorm:"column:id;primaryKey" json:"uuid"`
	OrgID        string    `gorm:"column:org_id" json:"-"`
	MCPProxyUUID uuid.UUID `gorm:"column:mcp_proxy_id" json:"mcpProxyUuid"`
	Action       string    `gorm:"column:action" json:"action"`
	Description  string    `gorm:"column:description" json:"description"`
	// Tools are stored in mcp_proxy_scope_tools; the scope repository reads and writes them.
	Tools     []string  `gorm:"-" json:"tools"`
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName returns the table name for the MCPProxyScope model.
func (MCPProxyScope) TableName() string { return "mcp_proxy_scopes" }

// ScopeString derives the token scope string for this row.
func (s MCPProxyScope) ScopeString(proxyHandle string) string { return proxyHandle + ":" + s.Action }

// MCPProxyScopeInput carries a scope create request into the service layer.
type MCPProxyScopeInput struct {
	Action      string
	Description string
	Tools       []string
}

// MCPProxyScopeUpdateInput carries a scope update; nil Description / nil Tools
// mean "leave unchanged".
type MCPProxyScopeUpdateInput struct {
	Description *string
	Tools       []string
}

// MCPProxyScopesResult pairs a proxy's scopes with its handle so callers can
// derive scope strings without a second lookup.
type MCPProxyScopesResult struct {
	ProxyHandle string
	Scopes      []MCPProxyScope
}

// MCPProxyScopeResult is a single-scope variant of MCPProxyScopesResult.
type MCPProxyScopeResult struct {
	ProxyHandle string
	Scope       MCPProxyScope
}

// EnvironmentScopeEntry is one row of the env-filtered scope aggregate that
// powers role building: a grantable scope from a proxy deployed to the env.
type EnvironmentScopeEntry struct {
	Scope        string
	Description  string
	MCPProxyID   string
	MCPProxyName string
}
