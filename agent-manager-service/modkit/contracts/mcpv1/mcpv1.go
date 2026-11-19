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

// Package mcpv1 is the "mcp.proxy/v1" capability: giving agents access to MCP
// servers through a gateway's MCP proxies.
package mcpv1

import "context"

// ID is the capability id.
const ID = "mcp.proxy/v1"

// Proxy answers for the MCP proxies a project can use.
type Proxy interface {
	// ValidateProxies refuses MCP proxies the project cannot use.
	ValidateProxies(ctx context.Context, orgID, project string, proxies []string) error
}
