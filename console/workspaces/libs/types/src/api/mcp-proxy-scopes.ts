/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { OrgPathParams } from './common';

// Per-MCP-proxy scope catalog. Each scope is an action on the proxy's resource
// server and directly owns the list of tools it authorizes; the token scope
// string ("scope") is derived as "<proxy-handle>:<action>".

export interface MCPProxyScopeRequest {
  action: string;
  description?: string;
  tools?: string[];
}

export interface MCPProxyScopeUpdateRequest {
  description?: string;
  tools?: string[];
}

export interface MCPProxyScopeResponse {
  action: string;
  scope: string;
  description?: string;
  tools: string[];
  createdAt?: string;
  updatedAt?: string;
}

export interface MCPProxyScopeListResponse {
  scopes: MCPProxyScopeResponse[];
}

// --- Path params ---

/**
 * A proxy's scopes. With projName they are a project MCP proxy's, under that project's routes;
 * without it, an org MCP proxy's.
 */
export type MCPProxyScopesPathParams = OrgPathParams & { proxyId: string; projName?: string };
export type MCPProxyScopePathParams = MCPProxyScopesPathParams & { scopeAction: string };
