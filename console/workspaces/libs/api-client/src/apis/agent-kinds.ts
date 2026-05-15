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

import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";
import type {
  AgentKindListResponse,
  AgentKindResponse,
  AgentKindVersionResponse,
  AddAgentKindVersionRequest,
  AddAgentKindVersionPathParams,
  AgentResponse,
  DeleteAgentKindPathParams,
  DeleteAgentKindVersionPathParams,
  GetAgentKindPathParams,
  GetAgentKindVersionPathParams,
  ListAgentKindsPathParams,
  ListAgentKindsQuery,
  ListAgentKindVersionsPathParams,
  ListKindAgentsPathParams,
  PublishAgentKindPathParams,
  PublishAgentKindRequest,
  UpdateAgentKindPathParams,
  UpdateAgentKindRequest,
} from "@agent-management-platform/types";

async function throwHttpError(res: Response): Promise<never> {
  const body = await res.json().catch(() => undefined);
  const err = new Error(`Request failed: ${res.status} ${res.statusText}`);
  throw Object.assign(err, { status: res.status, body });
}

/**
 * List all Agent Kinds for an organization
 */
export async function listAgentKinds(
  params: ListAgentKindsPathParams,
  query?: ListAgentKindsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentKindListResponse> {
  const { orgName = "default" } = params;

  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          .filter(([, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    : undefined;

  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agent-kinds`,
    { searchParams: search, token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
  return res.json();
}

/**
 * Get details of an Agent Kind
 */
export async function getAgentKind(
  params: GetAgentKindPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentKindResponse> {
  const { orgName = "default", kindName } = params;

  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agent-kinds/${encodeURIComponent(kindName!)}`,
    { token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
  return res.json();
}

/**
 * Update display name or description of an Agent Kind
 */
export async function updateAgentKind(
  params: UpdateAgentKindPathParams,
  body: UpdateAgentKindRequest,
  getToken?: () => Promise<string>,
): Promise<AgentKindResponse> {
  const { orgName = "default", kindName } = params;

  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agent-kinds/${encodeURIComponent(kindName!)}`,
    body,
    { token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
  return res.json();
}

/**
 * Delete an Agent Kind and all its versions
 */
export async function deleteAgentKind(
  params: DeleteAgentKindPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", kindName } = params;

  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agent-kinds/${encodeURIComponent(kindName!)}`,
    { token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
}

/**
 * List all versions of an Agent Kind
 */
export async function listAgentKindVersions(
  params: ListAgentKindVersionsPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentKindVersionResponse[]> {
  const { orgName = "default", kindName } = params;

  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agent-kinds/${encodeURIComponent(kindName!)}/versions`,
    { token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
  return res.json();
}

/**
 * Add a new version to an existing Agent Kind
 */
export async function addAgentKindVersion(
  params: AddAgentKindVersionPathParams,
  body: AddAgentKindVersionRequest,
  getToken?: () => Promise<string>,
): Promise<AgentKindVersionResponse> {
  const { orgName = "default", kindName } = params;

  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agent-kinds/${encodeURIComponent(kindName!)}/versions`,
    body,
    { token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
  return res.json();
}

/**
 * Get a specific version of an Agent Kind
 */
export async function getAgentKindVersion(
  params: GetAgentKindVersionPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentKindVersionResponse> {
  const { orgName = "default", kindName, versionTag } = params;

  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agent-kinds/${encodeURIComponent(kindName!)}/versions/${encodeURIComponent(versionTag!)}`,
    { token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
  return res.json();
}

/**
 * Delete a specific version of an Agent Kind
 */
export async function deleteAgentKindVersion(
  params: DeleteAgentKindVersionPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", kindName, versionTag } = params;

  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agent-kinds/${encodeURIComponent(kindName!)}/versions/${encodeURIComponent(versionTag!)}`,
    { token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
}

/**
 * Publish an agent build as an Agent Kind version
 */
export async function publishAgentKind(
  params: PublishAgentKindPathParams,
  body: PublishAgentKindRequest,
  getToken?: () => Promise<string>,
): Promise<AgentKindVersionResponse> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/publish-kind`,
    body,
    { token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
  return res.json();
}

/**
 * List all agents deployed from a given Agent Kind across all projects in the org
 */
export async function listKindAgents(
  params: ListKindAgentsPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentResponse[]> {
  const { orgName = "default", kindName } = params;

  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agent-kinds/${encodeURIComponent(kindName!)}/agents`,
    { token }
  );

  if (!res.ok) {
    await throwHttpError(res);
  }
  return res.json();
}
