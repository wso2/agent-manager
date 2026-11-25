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
  ThunderGroup,
  ThunderRole,
  AgentIdentityGroupListResponse,
  AgentIdentityGroupRequest,
  AgentIdentityGroupMembersResponse,
  AgentIdentityMembersRequest,
  AgentIdentityGroupRolesResponse,
  AgentIdentityRoleListResponse,
  AgentIdentityRoleRequest,
  AgentIdentityAssignmentsRequest,
  AgentIdentityRoleAssignmentsResponse,
  AgentIdentityAgentListResponse,
  AgentIdentityScopeListResponse,
  AgentIdentityEnvPathParams,
  AgentIdentityGroupPathParams,
  AgentIdentityRolePathParams,
  AgentIdentityListQuery,
} from "@agent-management-platform/types";

// A missing org is refused rather than sent as another org's ("default").
const envBase = (orgName: string | undefined, envName: string) =>
  `${GATEWAY_BASE}/orgs/${encodeRequired(orgName, "orgName")}/environments/${encodeURIComponent(envName)}/agent-identities`;

const toSearch = (query?: AgentIdentityListQuery) =>
  query
    ? { offset: String(query.offset ?? 0), limit: String(query.limit ?? 20) }
    : undefined;

// --- Groups ---

export async function listAgentIdentityGroups(
  params: AgentIdentityEnvPathParams,
  query?: AgentIdentityListQuery,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityGroupListResponse> {
  const { orgName, envName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${envBase(orgName, envName)}/groups`, {
    searchParams: toSearch(query), token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createAgentIdentityGroup(
  params: AgentIdentityEnvPathParams,
  body: AgentIdentityGroupRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderGroup> {
  const { orgName, envName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${envBase(orgName, envName)}/groups`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentIdentityGroup(
  params: AgentIdentityGroupPathParams,
  getToken?: () => Promise<string>,
): Promise<ThunderGroup> {
  const { orgName, envName, groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${envBase(orgName, envName)}/groups/${encodeURIComponent(groupId)}`, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateAgentIdentityGroup(
  params: AgentIdentityGroupPathParams,
  body: AgentIdentityGroupRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderGroup> {
  const { orgName, envName, groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(
    `${envBase(orgName, envName)}/groups/${encodeURIComponent(groupId)}`, body, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteAgentIdentityGroup(
  params: AgentIdentityGroupPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName, envName, groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${envBase(orgName, envName)}/groups/${encodeURIComponent(groupId)}`, { token },
  );
  if (!res.ok && res.status !== 204) throw await res.json();
}

export async function getAgentIdentityGroupMembers(
  params: AgentIdentityGroupPathParams,
  query?: AgentIdentityListQuery,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityGroupMembersResponse> {
  const { orgName, envName, groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${envBase(orgName, envName)}/groups/${encodeURIComponent(groupId)}/members`,
    { searchParams: toSearch(query), token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function addAgentIdentityGroupMembers(
  params: AgentIdentityGroupPathParams,
  body: AgentIdentityMembersRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName, envName, groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${envBase(orgName, envName)}/groups/${encodeURIComponent(groupId)}/members/add`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

export async function removeAgentIdentityGroupMembers(
  params: AgentIdentityGroupPathParams,
  body: AgentIdentityMembersRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName, envName, groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${envBase(orgName, envName)}/groups/${encodeURIComponent(groupId)}/members/remove`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

export async function getAgentIdentityGroupRoles(
  params: AgentIdentityGroupPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityGroupRolesResponse> {
  const { orgName, envName, groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${envBase(orgName, envName)}/groups/${encodeURIComponent(groupId)}/roles`, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

// --- Roles ---

export async function listAgentIdentityRoles(
  params: AgentIdentityEnvPathParams,
  query?: AgentIdentityListQuery,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityRoleListResponse> {
  const { orgName, envName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${envBase(orgName, envName)}/roles`, {
    searchParams: toSearch(query), token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createAgentIdentityRole(
  params: AgentIdentityEnvPathParams,
  body: AgentIdentityRoleRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderRole> {
  const { orgName, envName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${envBase(orgName, envName)}/roles`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentIdentityRole(
  params: AgentIdentityRolePathParams,
  getToken?: () => Promise<string>,
): Promise<ThunderRole> {
  const { orgName, envName, roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${envBase(orgName, envName)}/roles/${encodeURIComponent(roleId)}`, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateAgentIdentityRole(
  params: AgentIdentityRolePathParams,
  body: AgentIdentityRoleRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderRole> {
  const { orgName, envName, roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(
    `${envBase(orgName, envName)}/roles/${encodeURIComponent(roleId)}`, body, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteAgentIdentityRole(
  params: AgentIdentityRolePathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName, envName, roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${envBase(orgName, envName)}/roles/${encodeURIComponent(roleId)}`, { token },
  );
  if (!res.ok && res.status !== 204) throw await res.json();
}

export async function getAgentIdentityRoleAssignments(
  params: AgentIdentityRolePathParams,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityRoleAssignmentsResponse> {
  const { orgName, envName, roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${envBase(orgName, envName)}/roles/${encodeURIComponent(roleId)}/assignments`, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function addAgentIdentityRoleAssignees(
  params: AgentIdentityRolePathParams,
  body: AgentIdentityAssignmentsRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName, envName, roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${envBase(orgName, envName)}/roles/${encodeURIComponent(roleId)}/assignments/add`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

export async function removeAgentIdentityRoleAssignees(
  params: AgentIdentityRolePathParams,
  body: AgentIdentityAssignmentsRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName, envName, roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${envBase(orgName, envName)}/roles/${encodeURIComponent(roleId)}/assignments/remove`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

// --- Agents picker ---

export async function listAgentIdentityAgents(
  params: AgentIdentityEnvPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityAgentListResponse> {
  const { orgName, envName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${envBase(orgName, envName)}/agents`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

// --- Scopes ---

export async function listAgentIdentityScopes(
  params: AgentIdentityEnvPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityScopeListResponse> {
  const { orgName, envName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${envBase(orgName, envName)}/scopes`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}
