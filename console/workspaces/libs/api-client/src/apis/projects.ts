/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { httpDELETE, httpGET, httpPOST, SERVICE_BASE } from '../utils';
import {
  ProjectListResponse,
  ProjectResponse,
  ListProjectsPathParams,
  GetProjectPathParams,
  ListProjectsQuery,
  CreateProjectPathParams,
  CreateProjectRequest,
  DeleteProjectPathParams,
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

export async function deleteProject(
    params: DeleteProjectPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", projName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}`;
  const res = await httpDELETE(url, { token });
  if (!res.ok) throw await res.json();
    // DELETE may return 204 No Content
  if (res.status === 204 || res.headers.get('content-length') === '0') {
    return;
  }
  return res.json();
}
