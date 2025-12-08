import { httpGET, httpPOST, SERVICE_BASE } from '../utils';
import {
  ProjectListResponse,
  ProjectResponse,
  ListProjectsPathParams,
  GetProjectPathParams,
  ListProjectsQuery,
  CreateProjectPathParams,
  CreateProjectRequest,
} from '@agent-management-platform/types';

export async function listProjects(
  params: ListProjectsPathParams,
  query?: ListProjectsQuery,
  getToken?: () => Promise<string>,
): Promise<ProjectListResponse> {
  const { orgName = "default" } = params;

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
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects`,
    { searchParams: search, token },
  );

  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createProject(
  params: CreateProjectPathParams,
  body: CreateProjectRequest,
  getToken?: () => Promise<string>,
): Promise<ProjectResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getProject(
  params: GetProjectPathParams,
  getToken?: () => Promise<string>,
): Promise<ProjectResponse> {
  const { orgName = "default", projName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}`;
  const res = await httpGET(url, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

