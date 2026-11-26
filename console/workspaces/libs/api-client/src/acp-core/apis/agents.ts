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
import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../../shared/utils";
import { listProjects } from "./projects";
import type {
  AgentDocument,
  AgentDocumentList,
  AgentListResponse,
  AgentResponse,
  AgentSummaryListResponse,
  AgentUpdateSpec,
  CreateAgentPathParams,
  DeleteAgentPathParams,
  GetAgentPathParams,
  ListAgentDocumentsQuery,
  ListAgentsPathParams,
  ListAgentsQuery,
  ListOrgAgentsPathParams,
  CreateAgentRequest,
  UpdateAgentPathParams,
  UpdateAgentRequest,
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

const AGENTS = `${SERVICE_BASE}/agents`;
const API_VERSION = "ai-control-plane.choreolabs.com/v1";
const KIND = "Agent";
// The largest page the agent list serves; enough to read a whole org in one or two pages.
const MAX_PAGE = 500;

// agentURL addresses one agent; agent names are unique only within a project.
function agentURL(projName: string | undefined, agentName: string | undefined, suffix = ""): string {
  if (!agentName) {
    throw new Error("agentName is required");
  }
  if (!projName) {
    throw new Error("projName is required");
  }
  return `${AGENTS}/${encodeURIComponent(agentName)}${suffix}`;
}

// searchEntries turns a query into entries, so array values repeat (label=a&label=b).
function searchEntries(query?: object): string[][] {
  const search: string[][] = [];
  for (const [k, v] of Object.entries(query ?? {})) {
    if (v === undefined) continue;
    if (Array.isArray(v)) {
      v.forEach((item) => search.push([k, String(item)]));
    } else {
      search.push([k, String(v)]);
    }
  }
  return search;
}

// searchRecord is a query as a record, without its undefined values.
function searchRecord(query: Record<string, string | undefined>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined) out[k] = v;
  }
  return out;
}

// Lists one project's agents, or every project's when query.project is omitted.
export async function listAgentDocuments(
  query?: ListAgentDocumentsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentDocumentList> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(AGENTS, { searchParams: searchEntries(query), token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function listAgents(
  params: ListAgentsPathParams,
  query?: ListAgentsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentDocumentList> {
  const { projName = "default" } = params;
  return listAgentDocuments({ ...query, project: projName }, getToken);
}

// Every agent in every project, with its project's display name; reads every page of both.
export async function listOrgAgents(
  params: ListOrgAgentsPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentSummaryListResponse> {
  const [agents, projectNames] = await Promise.all([
    readAllAgents(getToken),
    readProjectDisplayNames(params, getToken),
  ]);
  return {
    agents: agents.map((doc) => ({
      name: doc.metadata.name,
      displayName: doc.spec.displayName,
      projectName: doc.metadata.project,
      projectDisplayName: projectNames.get(doc.metadata.project) ?? doc.metadata.project,
    })),
  };
}

async function readAllAgents(getToken?: () => Promise<string>): Promise<AgentDocument[]> {
  const all: AgentDocument[] = [];
  for (;;) {
    const page = await listAgentDocuments({ limit: MAX_PAGE, offset: all.length }, getToken);
    all.push(...page.list);
    const total = page.pagination?.total ?? all.length;
    if (page.list.length === 0 || all.length >= total) return all;
  }
}

async function readProjectDisplayNames(
  params: ListOrgAgentsPathParams,
  getToken?: () => Promise<string>,
): Promise<Map<string, string>> {
  const names = new Map<string, string>();
  for (let offset = 0; ; ) {
    const page = await listProjects({ orgName: params.orgName }, { limit: 100, offset }, getToken);
    page.projects.forEach((p) => names.set(p.name, p.displayName));
    offset += page.projects.length;
    if (page.projects.length === 0 || offset >= page.total) return names;
  }
}

export async function createAgent(
  _params: CreateAgentPathParams,
  body: AgentDocument,
  getToken?: () => Promise<string>,
): Promise<AgentDocument> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(AGENTS, cloneDeep(body), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgent(
  params: GetAgentPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentDocument> {
  const url = agentURL(params.projName, params.agentName);
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(url, { searchParams: { project: params.projName ?? "" }, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteAgent(
  params: DeleteAgentPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const url = agentURL(params.projName, params.agentName);
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(url, { searchParams: { project: params.projName ?? "" }, token });
  if (!res.ok) throw await res.json();
}

export async function updateAgent(
  params: UpdateAgentPathParams,
  body: AgentDocument<AgentUpdateSpec>,
  getToken?: () => Promise<string>,
): Promise<AgentDocument> {
  const url = agentURL(params.projName, params.agentName);
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(url, cloneDeep(body), {
    searchParams: { project: params.projName ?? "" },
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function generateAgentToken(
  params: GenerateAgentTokenPathParams,
  body?: TokenRequest,
  query?: GenerateAgentTokenQuery,
  getToken?: () => Promise<string>,
): Promise<TokenResponse> {
  const url = agentURL(params.projName, params.agentName, "/token");
  const token = getToken ? await getToken() : undefined;
  const search = searchRecord({ environment: query?.environment, project: params.projName });
  const res = await httpPOST(url, body || {}, { searchParams: search, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

// The UI works on a flat agent; these convert it to and from the document the API serves.

export function agentFromDocument(doc: AgentDocument): AgentResponse {
  const { metadata, spec } = doc;
  return {
    uuid: metadata.uid,
    name: metadata.name,
    displayName: spec.displayName,
    description: spec.description ?? "",
    createdAt: metadata.creationTimestamp ?? "",
    projectName: metadata.project,
    status: doc.status?.state,
    provisioning: { type: spec.provisioning.type },
    agentType: spec.agentType,
    labels: spec.labels,
    createdBy: metadata.createdBy ? { id: metadata.createdBy } : undefined,
  };
}

export function agentListFromDocuments(list: AgentDocumentList): AgentListResponse {
  const agents = list.list.map(agentFromDocument);
  return {
    agents,
    total: list.pagination?.total ?? agents.length,
    limit: list.pagination?.limit ?? agents.length,
    offset: list.pagination?.offset ?? 0,
  };
}

export function agentDocumentFromCreateRequest(
  project: string,
  body: CreateAgentRequest,
): AgentDocument {
  const { name, displayName, description, provisioning, agentType, labels, extensions } = body;
  return {
    apiVersion: API_VERSION,
    kind: KIND,
    metadata: { name, project },
    spec: {
      displayName,
      description,
      provisioning: { type: provisioning.type },
      agentType: agentType ?? { type: "" },
      labels,
      extensions,
    },
  };
}

// Leaves the fields fixed at creation out; absent fields keep their stored values.
export function agentDocumentFromUpdateRequest(
  project: string,
  agentName: string,
  body: UpdateAgentRequest,
): AgentDocument<AgentUpdateSpec> {
  return {
    apiVersion: API_VERSION,
    kind: KIND,
    metadata: { name: agentName, project },
    spec: {
      displayName: body.displayName,
      description: body.description,
      labels: body.labels,
    },
  };
}

// --- Agent identity: roles/groups (read-only) ---

export async function getAgentRoles(
  params: GetAgentRolesPathParams,
  query: GetAgentRolesQuery,
  getToken?: () => Promise<string>,
): Promise<AgentRolesResponse> {
  const url = agentURL(params.projName, params.agentName, "/roles");
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(url, {
    searchParams: searchEntries({ project: params.projName, environment: query.environment }),
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentGroups(
  params: GetAgentGroupsPathParams,
  query: GetAgentGroupsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentGroupsResponse> {
  const url = agentURL(params.projName, params.agentName, "/groups");
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(url, {
    searchParams: searchEntries({ project: params.projName, environment: query.environment }),
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

// --- Agent identity: AgentID lifecycle (per environment) ---

// Lists the agent's AgentID binding for every environment in the organization
// (or one, if `environment` is passed). Safe read: never
// returns or removes a secret.
export async function getAgentIdentity(
  params: GetAgentIdentityPathParams,
  query?: GetAgentIdentityQuery,
  getToken?: () => Promise<string>,
): Promise<AgentIdentityEnvironmentView[]> {
  const url = agentURL(params.projName, params.agentName, "/identities");
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(url, {
    searchParams: searchEntries({ project: params.projName, environment: query?.environment }),
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
  const url = agentURL(params.projName, params.agentName, "/identities");
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(url, {}, {
    searchParams: searchRecord({ project: params.projName, environment: query.environment }),
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

// Rotates the AgentID secret for one environment and returns the new value
// straight away.
export async function regenerateAgentIdentitySecret(
  params: RegenerateAgentIdentitySecretPathParams,
  body: AgentIdentityActionRequest,
  getToken?: () => Promise<string>,
): Promise<AgentRegenerateSecretResponse> {
  const url = agentURL(params.projName, params.agentName, "/identities");
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(url, body, { searchParams: { project: params.projName ?? "" }, token });
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
  const url = agentURL(params.projName, params.agentName, "/identities/retry");
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(url, body, { searchParams: { project: params.projName ?? "" }, token });
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
  const url = agentURL(params.projName, params.agentName, "/identities");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(url, {
    searchParams: searchRecord({ project: params.projName, environment: query.environment }),
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}
