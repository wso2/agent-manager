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
  MCPProxyListResponse,
  MCPServerInfoFetchRequest,
  MCPServerInfoFetchResponse,
  ProjectMCPProxy,
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
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/mcp-proxies/${proxyId}`,
    { token },
  );
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
): Promise<MCPServerInfoFetchResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/mcp-proxies/fetch-server-info`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}
