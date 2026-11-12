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

import { encodeRequired, httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";
import type {
  MCPProxyScopeListResponse,
  MCPProxyScopeRequest,
  MCPProxyScopeResponse,
  MCPProxyScopeUpdateRequest,
  MCPProxyScopesPathParams,
  MCPProxyScopePathParams,
} from "@agent-management-platform/types";

// A project MCP proxy's scopes are under its project's routes; the org routes refuse them.
const scopesBase = (orgName: string, proxyId: string, projName?: string) =>
  `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}`
  + (projName ? `/projects/${encodeURIComponent(projName)}` : "")
  + `/mcp-proxies/${encodeURIComponent(proxyId)}/scopes`;

/**
 * List an MCP proxy's scopes
 */
export async function listMCPProxyScopes(
  params: MCPProxyScopesPathParams,
  getToken?: () => Promise<string>,
): Promise<MCPProxyScopeListResponse> {
  const { orgName = "default", proxyId, projName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(scopesBase(orgName, proxyId, projName), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Create an MCP proxy scope
 */
export async function createMCPProxyScope(
  params: MCPProxyScopesPathParams,
  body: MCPProxyScopeRequest,
  getToken?: () => Promise<string>,
): Promise<MCPProxyScopeResponse> {
  const { orgName = "default", proxyId, projName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(scopesBase(orgName, proxyId, projName), body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Update an MCP proxy scope's description and/or tools. The action itself is
 * immutable (rename = delete + create).
 */
export async function updateMCPProxyScope(
  params: MCPProxyScopePathParams,
  body: MCPProxyScopeUpdateRequest,
  getToken?: () => Promise<string>,
): Promise<MCPProxyScopeResponse> {
  const { orgName = "default", proxyId, projName, scopeAction } = params;
  const action = encodeRequired(scopeAction, "scopeAction");
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(`${scopesBase(orgName, proxyId, projName)}/${action}`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Delete an MCP proxy scope
 */
export async function deleteMCPProxyScope(
  params: MCPProxyScopePathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", proxyId, projName, scopeAction } = params;
  const action = encodeRequired(scopeAction, "scopeAction");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(`${scopesBase(orgName, proxyId, projName)}/${action}`, { token });
  if (!res.ok && res.status !== 204) throw await res.json();
}
