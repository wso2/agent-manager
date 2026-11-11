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
  ProjectMCPProxyEnvironment,
  RefreshProjectMCPProxyCapabilitiesPathParams,
  ListAvailableMCPPoliciesPathParams,
  ListMCPProxiesPathParams,
  ListMCPProxiesQuery,
  MCPPolicyAvailabilityResponse,
  MCPProxy,
  MCPProxyListResponse,
  MCPServerInfoFetchRequest,
  MCPServerInfoFetchResponse,
  DeleteProjectMCPProxyPathParams,
  ProjectMCPProxy,
  ProjectMCPProxyEnvironmentPathParams,
  ProjectMCPProxyEnvironmentRequest,
  ProjectMCPProxyRequest,
  UpdateMCPProxyPathParams,
} from "@agent-management-platform/types";
import {
  encodeRequired,
  httpDELETE,
  httpGET,
  httpPOST,
  httpPUT,
  SERVICE_BASE,
} from "../utils";

/** The most proxies one list request returns (the service's MaxLimit). */
const MCP_PROXY_LIST_PAGE = 100;
/** A bound on the pages listAllMCPProxies reads, in case the total keeps moving. */
const MCP_PROXY_LIST_MAX_PAGES = 50;

/**
 * Every proxy the list returns, read a page at a time: a project's list holds the org's servers
 * and the project's, and a single page would hide the oldest of them.
 */
export async function listAllMCPProxies(
  params: ListMCPProxiesPathParams,
  query?: Omit<ListMCPProxiesQuery, "limit" | "offset">,
  getToken?: () => Promise<string>,
): Promise<MCPProxyListResponse> {
  const list: MCPProxyListResponse["list"] = [];
  let total = 0;
  for (let page = 0; page < MCP_PROXY_LIST_MAX_PAGES; page++) {
    const res = await listMCPProxies(
      params,
      { ...query, limit: MCP_PROXY_LIST_PAGE, offset: list.length },
      getToken,
    );
    list.push(...res.list);
    total = res.pagination?.count ?? list.length;
    if (res.list.length === 0 || list.length >= total) break;
  }
  return {
    count: list.length,
    list,
    pagination: { count: total, limit: list.length, offset: 0 },
  };
}

export async function listMCPProxies(
  params: ListMCPProxiesPathParams,
  query?: ListMCPProxiesQuery,
  getToken?: () => Promise<string>,
): Promise<MCPProxyListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
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

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/mcp-proxies`, {
    token,
    searchParams: Object.keys(searchParams).length ? searchParams : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function listAvailableMCPPolicies(
  params: ListAvailableMCPPoliciesPathParams,
  getToken?: () => Promise<string>,
): Promise<MCPPolicyAvailabilityResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/mcp-proxies/policies`, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createMCPProxy(
  params: CreateMCPProxyPathParams,
  body: MCPProxy,
  getToken?: () => Promise<string>,
): Promise<MCPProxy> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(`${SERVICE_BASE}/orgs/${org}/mcp-proxies`, body, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getMCPProxy(
  params: GetMCPProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<MCPProxy> {
  const org = encodeRequired(params.orgName, "orgName");
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/mcp-proxies/${proxyId}`, {
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
  // Built before the token is read, so a missing path parameter fails first.
  const url = projectMCPProxyUrl(params);
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(url, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Queues the discovery of a project proxy binding's capabilities again; the binding
 * comes back with it Pending.
 */
export async function refreshProjectMCPProxyCapabilities(
  params: RefreshProjectMCPProxyCapabilitiesPathParams,
  getToken?: () => Promise<string>,
): Promise<ProjectMCPProxyEnvironment> {
  const url = `${projectMCPProxyEnvironmentUrl(params)}/capabilities/refresh`;
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(url, {}, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateMCPProxy(
  params: UpdateMCPProxyPathParams,
  body: MCPProxy,
  getToken?: () => Promise<string>,
): Promise<MCPProxy> {
  const org = encodeRequired(params.orgName, "orgName");
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${org}/mcp-proxies/${proxyId}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteMCPProxy(
  params: DeleteMCPProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(`${SERVICE_BASE}/orgs/${org}/mcp-proxies/${proxyId}`, {
    token,
  });
  if (!res.ok) throw await res.json();
}

export async function fetchMCPProxyServerInfo(
  params: FetchMCPProxyServerInfoPathParams,
  body: MCPServerInfoFetchRequest,
  getToken?: () => Promise<string>,
  signal?: AbortSignal,
): Promise<MCPServerInfoFetchResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/mcp-proxies/fetch-server-info`,
    body,
    { token, signal },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

function projectMCPProxyUrl(params: DeleteProjectMCPProxyPathParams): string {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  return `${SERVICE_BASE}/orgs/${org}/projects/${proj}/mcp-proxies/${proxyId}`;
}

function projectMCPProxyEnvironmentUrl(
  params: ProjectMCPProxyEnvironmentPathParams,
): string {
  const envName = encodeRequired(params.envName, "envName");
  return `${projectMCPProxyUrl(params)}/environments/${envName}`;
}

/**
 * Creates a project-owned proxy, or updates the one with this id in the project. With
 * createOnly it only creates: an id the project already has is refused (412) and left as it is.
 */
export async function upsertProjectMCPProxy(
  params: DeleteProjectMCPProxyPathParams,
  body: ProjectMCPProxyRequest,
  getToken?: () => Promise<string>,
  options?: { createOnly?: boolean },
): Promise<ProjectMCPProxy> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(projectMCPProxyUrl(params), body, {
    token,
    headers: options?.createOnly ? { "If-None-Match": "*" } : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

/** Removes a project proxy's binding, and its API keys, from one environment. */
export async function deleteProjectMCPProxyEnvironment(
  params: ProjectMCPProxyEnvironmentPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(projectMCPProxyEnvironmentUrl(params), { token });
  if (!res.ok) throw await res.json();
}

/** Deletes a project-owned proxy and every environment binding it has. */
export async function deleteProjectMCPProxy(
  params: DeleteProjectMCPProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(projectMCPProxyUrl(params), { token });
  if (!res.ok) throw await res.json();
}

/** Replaces a project proxy's binding in one environment (security, policies, upstream auth). */
export async function putProjectMCPProxyEnvironment(
  params: ProjectMCPProxyEnvironmentPathParams,
  body: ProjectMCPProxyEnvironmentRequest,
  getToken?: () => Promise<string>,
): Promise<ProjectMCPProxyEnvironment> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(projectMCPProxyEnvironmentUrl(params), body, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}
