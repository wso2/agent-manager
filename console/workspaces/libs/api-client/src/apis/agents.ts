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

import { cloneDeep } from "lodash";
import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";
import type {
  AgentListResponse,
  AgentResponse,
  AgentSummaryListResponse,
  CreateAgentPathParams,
  DeleteAgentPathParams,
  GetAgentPathParams,
  ListAgentsPathParams,
  ListAgentsQuery,
  ListOrgAgentsPathParams,
  CreateAgentRequest,
  UpdateAgentPathParams,
  UpdateAgentRequest,
  UpdateAgentBuildParametersPathParams,
  UpdateAgentBuildParametersRequest,
  GenerateAgentTokenPathParams,
  GenerateAgentTokenQuery,
  TokenRequest,
  TokenResponse,
  GetAgentRolesPathParams,
  GetAgentRolesQuery,
  AgentRolesResponse,
  GetAgentGroupsPathParams,
  GetAgentGroupsQuery,
  AgentGroupsResponse,
  GetAgentIdentityPathParams,
  GetAgentIdentityQuery,
  AgentIdentityEnvironmentView,
  ProvisionAgentIdentityPathParams,
  ProvisionAgentIdentityQuery,
  RegenerateAgentIdentitySecretPathParams,
  RetryAgentIdentityProvisioningPathParams,
  AgentIdentityActionRequest,
  AgentRegenerateSecretResponse,
  RevokeAgentIdentitySecretPathParams,
  RevokeAgentIdentitySecretQuery,
  AgentRevokeSecretResponse,
} from "@agent-management-platform/types";

export async function listAgents(
  params: ListAgentsPathParams,
  query?: ListAgentsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentListResponse> {
  const { orgName = "default", projName = "default" } = params;

  // Entries form (not an object) so array values expand into repeated
  // parameters, e.g. label=env:prod&label=team:ml.
  const search: string[][] = [];
  if (query) {
    for (const [k, v] of Object.entries(query)) {
      if (v === undefined) continue;
      if (Array.isArray(v)) {
        v.forEach((item) => search.push([k, String(item)]));
      } else {
        search.push([k, String(v)]);
      }
    }
  }
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(
      orgName
    )}/projects/${encodeURIComponent(projName)}/agents`,
    { searchParams: search, token: token }
  );

  if (!res.ok) throw await res.json();
  return res.json();
}

// Lists every agent across all projects in the org — lightweight (name +
// displayName only) and unpaginated, unlike listAgents which is per-project.
export async function listOrgAgents(
  params: ListOrgAgentsPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentSummaryListResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/agents`,
    { token }
  );

  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createAgent(
  params: CreateAgentPathParams,
  body: CreateAgentRequest,
  getToken?: () => Promise<string>,
): Promise<AgentResponse> {
  const { orgName = "default", projName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(
      orgName
    )}/projects/${encodeURIComponent(projName)}/agents`,
    cloneDeep(body),
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgent(
  params: GetAgentPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentResponse> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}`;
  const res = await httpGET(url, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteAgent(
  params: DeleteAgentPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}`;
  const res = await httpDELETE(url, { token });
  if (!res.ok) throw await res.json();
}

export async function updateAgent(
  params: UpdateAgentPathParams,
  body: UpdateAgentRequest,
  getToken?: () => Promise<string>,
): Promise<AgentResponse> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}`;
  const res = await httpPUT(url, cloneDeep(body), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateAgentBuildParameters(
  params: UpdateAgentBuildParametersPathParams,
  body: UpdateAgentBuildParametersRequest,
  getToken?: () => Promise<string>,
): Promise<AgentResponse> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}/build-parameters`;
  const res = await httpPUT(url, cloneDeep(body), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function generateAgentToken(
  params: GenerateAgentTokenPathParams,
  body?: TokenRequest,
  query?: GenerateAgentTokenQuery,
  getToken?: () => Promise<string>,
): Promise<TokenResponse> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          .filter(([_, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    : undefined;

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}/token`;
  
  const res = await httpPOST(url, body || {}, { searchParams: search, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

// --- Agent identity: roles/groups (read-only) ---

export async function getAgentRoles(
  params: GetAgentRolesPathParams,
  query: GetAgentRolesQuery,
  getToken?: () => Promise<string>,
): Promise<AgentRolesResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName ?? "")}/roles`;
  const res = await httpGET(url, { searchParams: { environment: query.environment }, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentGroups(
  params: GetAgentGroupsPathParams,
  query: GetAgentGroupsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentGroupsResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName ?? "")}/groups`;
  const res = await httpGET(url, { searchParams: { environment: query.environment }, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

// --- Agent identity: AgentID lifecycle (per environment) ---

const identityBase = (orgName: string, projName: string, agentName: string) =>
  `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
  `/projects/${encodeURIComponent(projName)}` +
  `/agents/${encodeURIComponent(agentName)}/identities`;

// Lists the agent's AgentID binding for every environment in the project's
// deployment pipeline (or one, if `environment` is passed). Safe read: never
// returns or removes a secret.
export async function getAgentIdentity(
  params: GetAgentIdentityPathParams,
  query?: GetAgentIdentityQuery,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityEnvironmentView[]> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(identityBase(orgName, projName, agentName ?? ""), {
    searchParams: query?.environment ? { environment: query.environment } : undefined,
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

// Creates an AgentID for an internal or externally hosted agent in an
// environment whose binding is missing. Idempotent: an existing binding is
// left as is and returned unchanged.
export async function provisionAgentIdentity(
  params: ProvisionAgentIdentityPathParams,
  query: ProvisionAgentIdentityQuery,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityEnvironmentView> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(identityBase(orgName, projName, agentName ?? ""), {}, {
    searchParams: { environment: query.environment },
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

// Rotates the AgentID secret for one environment and returns the new value
// straight away, for both platform-hosted and externally hosted agents.
export async function regenerateAgentIdentitySecret(
  params: RegenerateAgentIdentitySecretPathParams,
  body: AgentIdentityActionRequest,
  getToken?: () => Promise<string>,
): Promise<AgentRegenerateSecretResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(identityBase(orgName, projName, agentName ?? ""), body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

// Resets a binding stuck in "failed" status back to "pending" and
// re-attempts provisioning. Only valid while the binding is currently
// failed — a 409 means it moved on (or was never failed) before this call
// landed.
export async function retryAgentIdentityProvisioning(
  params: RetryAgentIdentityProvisioningPathParams,
  body: AgentIdentityActionRequest,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityEnvironmentView> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${identityBase(orgName, projName, agentName ?? "")}/retry`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

// Turns off the AgentID secret for one environment without issuing a new
// one. Call regenerate afterward to restore access.
export async function revokeAgentIdentitySecret(
  params: RevokeAgentIdentitySecretPathParams,
  query: RevokeAgentIdentitySecretQuery,
  getToken?: () => Promise<string>,
): Promise<AgentRevokeSecretResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(identityBase(orgName, projName, agentName ?? ""), {
    searchParams: { environment: query.environment },
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}
