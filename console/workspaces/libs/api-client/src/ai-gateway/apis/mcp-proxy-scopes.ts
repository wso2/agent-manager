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

import { encodeRequired, httpDELETE, httpGET, httpPOST, httpPUT, GATEWAY_BASE } from "../../shared/utils";
import type {
  MCPProxyScopeDocument,
  MCPProxyScopeDocumentList,
  MCPProxyScopeListResponse,
  MCPProxyScopeRequest,
  MCPProxyScopeResponse,
  MCPProxyScopeSpec,
  MCPProxyScopeUpdateRequest,
  MCPProxyScopesPathParams,
  MCPProxyScopePathParams,
} from "@agent-management-platform/types";

const API_VERSION = "ai-control-plane.choreolabs.com/v1";
const KIND = "McpProxyScope";

// Required: a missing proxy would request `/mcp-proxies/undefined/scopes`.
const scopesBase = (proxyId: string | undefined) =>
  `${GATEWAY_BASE}/mcp-proxies/${encodeRequired(proxyId, "proxyId")}/scopes`;

/**
 * List an MCP proxy's scopes
 */
export async function listMCPProxyScopes(
  params: MCPProxyScopesPathParams,
  getToken?: () => Promise<string>,
): Promise<MCPProxyScopeDocumentList> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(scopesBase(params.proxyId), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Create an MCP proxy scope
 */
export async function createMCPProxyScope(
  params: MCPProxyScopesPathParams,
  body: MCPProxyScopeDocument,
  getToken?: () => Promise<string>,
): Promise<MCPProxyScopeDocument> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(scopesBase(params.proxyId), body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Update an MCP proxy scope's description and/or tools. The action itself is
 * immutable (rename = delete + create).
 */
export async function updateMCPProxyScope(
  params: MCPProxyScopePathParams,
  body: MCPProxyScopeDocument,
  getToken?: () => Promise<string>,
): Promise<MCPProxyScopeDocument> {
  const action = encodeRequired(params.scopeAction, "scopeAction");
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(`${scopesBase(params.proxyId)}/${action}`, body, { token });
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
  const action = encodeRequired(params.scopeAction, "scopeAction");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(`${scopesBase(params.proxyId)}/${action}`, { token });
  if (!res.ok && res.status !== 204) throw await res.json();
}

// The UI works on a flat scope; these convert it to and from the document the API serves.

export function scopeFromDocument(doc: MCPProxyScopeDocument): MCPProxyScopeResponse {
  const { metadata, spec } = doc;
  return {
    action: metadata.name,
    scope: spec.scope ?? "",
    description: spec.description,
    tools: spec.tools ?? [],
    createdAt: metadata.creationTimestamp,
    updatedAt: metadata.updateTimestamp,
  };
}

export function scopeListFromDocuments(list: MCPProxyScopeDocumentList): MCPProxyScopeListResponse {
  return { scopes: list.list.map(scopeFromDocument) };
}

export function documentFromScopeRequest(body: MCPProxyScopeRequest): MCPProxyScopeDocument {
  const { action, ...spec } = body;
  return { apiVersion: API_VERSION, kind: KIND, metadata: { name: action }, spec };
}

/**
 * The update document. The action is immutable, so it comes from the path; an omitted `tools`
 * stays omitted, which leaves the scope's tool list alone.
 */
export function documentFromScopeUpdate(
  action: string,
  body: MCPProxyScopeUpdateRequest,
): MCPProxyScopeDocument {
  const spec: MCPProxyScopeSpec = {};
  if (body.description !== undefined) spec.description = body.description;
  if (body.tools !== undefined) spec.tools = body.tools;
  return { apiVersion: API_VERSION, kind: KIND, metadata: { name: action }, spec };
}
