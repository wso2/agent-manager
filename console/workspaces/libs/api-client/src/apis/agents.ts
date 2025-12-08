import { httpDELETE, httpGET, httpPOST, SERVICE_BASE } from '../utils';
import {
  AgentListResponse,
  AgentResponse,
  CreateAgentPathParams,
  DeleteAgentPathParams,
  GetAgentPathParams,
  ListAgentsPathParams,
  ListAgentsQuery,
  CreateAgentRequest
} from '@agent-management-platform/types';


export async function listAgents(
  params: ListAgentsPathParams,
  query?: ListAgentsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentListResponse> {
  const { orgName = "default", projName = "default" } = params;

  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          .filter(([_, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)]),
      )
    : undefined;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents`,
    {searchParams: search, token: token},
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
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgent(
  params: GetAgentPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
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
  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}`;
  const res = await httpDELETE(url, { token });
  if (!res.ok) throw await res.json();
}


