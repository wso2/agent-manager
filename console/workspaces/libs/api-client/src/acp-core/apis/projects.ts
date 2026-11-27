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

import { cloneDeep } from 'lodash';
import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from '../../shared/utils';
import type {
  CreateProjectInput,
  ProjectDocument,
  ProjectDocumentList,
  ProjectSpec,
  ListProjectsPathParams,
  GetProjectPathParams,
  ListProjectsQuery,
  CreateProjectPathParams,
  DeleteProjectPathParams,
  UpdateProjectPathParams,
} from '@agent-management-platform/types';

const PROJECTS = `${SERVICE_BASE}/projects`;
const API_VERSION = 'ai-control-plane.choreolabs.com/v1';

// projectURL addresses one project.
function projectURL(projName: string | undefined): string {
  if (!projName) {
    throw new Error('projName is required');
  }
  return `${PROJECTS}/${encodeURIComponent(projName)}`;
}

/** Builds a Project document from its handle and spec. */
export function projectDocument(name: string, spec: ProjectSpec): ProjectDocument {
  return {
    apiVersion: API_VERSION,
    kind: 'Project',
    metadata: { name },
    spec: { displayName: spec.displayName, description: spec.description },
  };
}

export async function listProjects(
  _params: ListProjectsPathParams,
  query?: ListProjectsQuery,
  getToken?: () => Promise<string>,
): Promise<ProjectDocumentList> {
  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          .filter(([_, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)]),
      )
    : undefined;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(PROJECTS, { searchParams: search, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createProject(
  _params: CreateProjectPathParams,
  body: CreateProjectInput,
  getToken?: () => Promise<string>,
): Promise<ProjectDocument> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(PROJECTS, projectDocument(body.name, body), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getProject(
  params: GetProjectPathParams,
  getToken?: () => Promise<string>,
): Promise<ProjectDocument> {
  const url = projectURL(params.projName);
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(url, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteProject(
  params: DeleteProjectPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const url = projectURL(params.projName);
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(url, { token });
  if (!res.ok) throw await res.json();
}

export async function updateProject(
  params: UpdateProjectPathParams,
  body: ProjectSpec,
  getToken?: () => Promise<string>,
): Promise<ProjectDocument> {
  const url = projectURL(params.projName);
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(url, cloneDeep(projectDocument(params.projName ?? '', body)), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}
