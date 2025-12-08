import { httpGET, httpPOST, SERVICE_BASE } from '../utils';
import {
  BuildAgentPathParams,
  BuildAgentQuery,
  BuildLogsResponse,
  BuildResponse,
  BuildsListResponse,
  GetAgentBuildsPathParams,
  GetAgentBuildsQuery,
  GetBuildLogsPathParams,
  GetBuildPathParams,
  BuildDetailsResponse,
} from '@agent-management-platform/types/src/api/builds';

export async function buildAgent(
  params: BuildAgentPathParams,
  query?: BuildAgentQuery,
  getToken?: () => Promise<string>,
): Promise<BuildResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/builds`,
    {},
    { searchParams: query?.commitId ? { commitId: query.commitId } : undefined, token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentBuilds(
  params: GetAgentBuildsPathParams,
  query?: GetAgentBuildsQuery,
  getToken?: () => Promise<string>,
): Promise<BuildsListResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
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
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/builds`,
    { searchParams: search, token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

// eslint-disable-next-line max-len
export async function getBuild(params: GetBuildPathParams, getToken?: () => Promise<string>): Promise<BuildDetailsResponse> {
  const { orgName = "default", projName = "default", agentName, buildName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/builds/${encodeURIComponent(buildName)}`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getBuildLogs(
  params: GetBuildLogsPathParams,
  getToken?: () => Promise<string>,
): Promise<BuildLogsResponse> {
  const { orgName = "default", projName = "default", agentName, buildName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/builds/${encodeURIComponent(buildName)}/build-logs`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}


