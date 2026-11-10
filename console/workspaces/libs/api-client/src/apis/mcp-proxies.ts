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
  CreateProjectMCPProxyAPIKeyRequest,
  CreateProjectMCPProxyAPIKeyResponse,
  DeleteProjectMCPProxyPathParams,
  ListAPIKeysResponse,
  ProjectMCPProxy,
  ProjectMCPProxyAPIKeyPathParams,
  ProjectMCPProxyEnvironmentPathParams,
  ProjectMCPProxyEnvironmentRequest,
  ProjectMCPProxyRequest,
  RotateProjectMCPProxyAPIKeyRequest,
  RotateProjectMCPProxyAPIKeyResponse,
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

/**
 * Queues the discovery of a project proxy binding's capabilities again; the binding
 * comes back with it Pending.
 */
export async function refreshProjectMCPProxyCapabilities(
  params: RefreshProjectMCPProxyCapabilitiesPathParams,
  getToken?: () => Promise<string>,
): Promise<ProjectMCPProxyEnvironment> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const proxyId = encodeRequired(params.proxyId, "proxyId");
  const envName = encodeRequired(params.envName, "envName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/mcp-proxies/${proxyId}/environments/${envName}/capabilities/refresh`,
    {},
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

export async function listProjectMCPProxyAPIKeys(
  params: ProjectMCPProxyEnvironmentPathParams,
  getToken?: () => Promise<string>,
): Promise<ListAPIKeysResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${projectMCPProxyEnvironmentUrl(params)}/api-keys`, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createProjectMCPProxyAPIKey(
  params: ProjectMCPProxyEnvironmentPathParams,
  body: CreateProjectMCPProxyAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateProjectMCPProxyAPIKeyResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${projectMCPProxyEnvironmentUrl(params)}/api-keys`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function rotateProjectMCPProxyAPIKey(
  params: ProjectMCPProxyAPIKeyPathParams,
  body: RotateProjectMCPProxyAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<RotateProjectMCPProxyAPIKeyResponse> {
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(
    `${projectMCPProxyEnvironmentUrl(params)}/api-keys/${keyName}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function revokeProjectMCPProxyAPIKey(
  params: ProjectMCPProxyAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${projectMCPProxyEnvironmentUrl(params)}/api-keys/${keyName}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}
