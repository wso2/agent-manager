/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type {
  CreateMCPProxyPathParams,
  DeleteMCPProxyPathParams,
  FetchMCPProxyServerInfoPathParams,
  GetMCPProxyPathParams,
  GetProjectMCPProxyPathParams,
  ListAvailableMCPPoliciesPathParams,
  ListMCPProxiesPathParams,
  ListMCPProxiesQuery,
  MCPPolicyAvailabilityResponse,
  MCPProxy,
  MCPProxyDocument,
  MCPProxyDocumentList,
  MCPProxyListResponse,
  MCPProxySpec,
  MCPProxyUpdateSpec,
  MCPServerInfoFetchRequest,
  MCPServerInfoFetchResponse,
  ProjectMCPProxy,
  UpdateMCPProxyPathParams,
} from "@agent-management-platform/types";
import { encodeRequired, httpDELETE, httpGET, httpPOST, httpPUT, GATEWAY_BASE } from "../../shared/utils";

const PROXIES = `${GATEWAY_BASE}/mcp-proxies`;
const API_VERSION = "ai-control-plane.choreolabs.com/v1";
const KIND = "McpProxy";

export async function listMCPProxies(
  _params: ListMCPProxiesPathParams,
  query?: ListMCPProxiesQuery,
  getToken?: () => Promise<string>,
): Promise<MCPProxyDocumentList> {
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }
  if (query?.project) {
    searchParams.project = query.project;
  }

  const res = await httpGET(PROXIES, {
    token,
    searchParams: Object.keys(searchParams).length ? searchParams : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function listAvailableMCPPolicies(
  _params: ListAvailableMCPPoliciesPathParams,
  getToken?: () => Promise<string>,
): Promise<MCPPolicyAvailabilityResponse> {
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${PROXIES}/policies`, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createMCPProxy(
  _params: CreateMCPProxyPathParams,
  body: MCPProxyDocument,
  getToken?: () => Promise<string>,
): Promise<MCPProxyDocument> {
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(PROXIES, body, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getMCPProxy(
  params: GetMCPProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<MCPProxyDocument> {
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${PROXIES}/${proxyId}`, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

/** A project-owned proxy with its per-environment bindings, from the project route. */
export async function getProjectMCPProxy(
  params: GetProjectMCPProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<ProjectMCPProxy> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/mcp-proxies/${proxyId}`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateMCPProxy(
  params: UpdateMCPProxyPathParams,
  body: MCPProxyDocument<MCPProxyUpdateSpec>,
  getToken?: () => Promise<string>,
): Promise<MCPProxyDocument> {
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(`${PROXIES}/${proxyId}`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteMCPProxy(
  params: DeleteMCPProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(`${PROXIES}/${proxyId}`, { token });
  if (!res.ok) throw await res.json();
}

export async function fetchMCPProxyServerInfo(
  _params: FetchMCPProxyServerInfoPathParams,
  body: MCPServerInfoFetchRequest,
  getToken?: () => Promise<string>,
  signal?: AbortSignal,
): Promise<MCPServerInfoFetchResponse> {
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(`${PROXIES}/fetch-server-info`, body, { token, signal });
  if (!res.ok) throw await res.json();
  return res.json();
}

// The UI works on a flat proxy; these convert it to and from the document the API serves.

export function proxyFromDocument(doc: MCPProxyDocument): MCPProxy {
  const { metadata, spec } = doc;
  return {
    id: metadata.name,
    name: spec.displayName,
    version: spec.version,
    description: spec.description,
    context: spec.context,
    vhost: spec.vhost,
    mcpSpecVersion: spec.mcpSpecVersion,
    inCatalog: spec.inCatalog,
    endpoints: spec.endpoints,
    subscriptionPlans: spec.subscriptionPlans,
    projectName: spec.projectName,
    createdBy: metadata.createdBy,
    createdAt: metadata.creationTimestamp,
    updatedAt: metadata.updateTimestamp,
  };
}

/** List rows are sparse documents: the UI's list item is what survives. */
export function proxyListFromDocuments(list: MCPProxyDocumentList): MCPProxyListResponse {
  const items = list.list.map((doc) => {
    const proxy = proxyFromDocument(doc);
    return {
      id: proxy.id,
      name: proxy.name,
      version: proxy.version,
      description: proxy.description,
      context: proxy.context,
      mcpSpecVersion: proxy.mcpSpecVersion,
      projectName: proxy.projectName,
      createdBy: proxy.createdBy,
      createdAt: proxy.createdAt,
      updatedAt: proxy.updatedAt,
    };
  });
  return {
    count: list.count ?? items.length,
    list: items,
    pagination: {
      count: list.pagination?.total ?? items.length,
      limit: list.pagination?.limit ?? items.length,
      offset: list.pagination?.offset ?? 0,
    },
  };
}

export function documentFromProxy(body: MCPProxy): MCPProxyDocument {
  // Spelled out rather than spread: the record fields move to metadata, and a key that never
  // reaches the spec must be a decision here, not a leftover. The round-trip test holds it.
  const spec: MCPProxySpec = {
    displayName: body.name,
    version: body.version,
    description: body.description,
    context: body.context,
    vhost: body.vhost,
    mcpSpecVersion: body.mcpSpecVersion,
    inCatalog: body.inCatalog,
    projectName: body.projectName,
  };
  // An absent endpoints key leaves the stored endpoints alone; an empty one clears them. The
  // offered plans work the same way: absent keeps them, [] offers none.
  if (body.endpoints !== undefined) spec.endpoints = body.endpoints;
  if (body.subscriptionPlans !== undefined) spec.subscriptionPlans = body.subscriptionPlans;
  return { apiVersion: API_VERSION, kind: KIND, metadata: { name: body.id }, spec };
}

/**
 * The update document. The handle comes from the path's proxy rather than the body, so an edit
 * that drops `id` still names the proxy it changes.
 */
export function updateDocumentFromProxy(
  handle: string,
  body: MCPProxy,
): MCPProxyDocument<MCPProxyUpdateSpec> {
  return { ...documentFromProxy(body), metadata: { name: handle } };
}
