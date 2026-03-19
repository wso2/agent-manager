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

import { useQueryClient } from "@tanstack/react-query";
import { createProject, deleteProject, getProject, listProjects, updateProject } from "../apis";
import {
  ProjectListResponse,
  ProjectResponse,
  ListProjectsPathParams,
  GetProjectPathParams,
  ListProjectsQuery,
  CreateProjectPathParams,
  CreateProjectRequest,
  DeleteProjectPathParams,
  UpdateProjectPathParams,
  UpdateProjectRequest,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

export function useListProjects(
  params: ListProjectsPathParams,
  query?: ListProjectsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ProjectListResponse>({
    queryKey: ['projects', params, query],
    queryFn: () => listProjects(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useGetProject(params: GetProjectPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ProjectResponse>({
    queryKey: ['project', params],
    queryFn: () => getProject(params, getToken),
    enabled: !!params.orgName && !!params.projName,
  });
}

export function useCreateProject(params: CreateProjectPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ProjectResponse,
    unknown,
    CreateProjectRequest
  >({
    action: { verb: 'create', target: 'project' },
    mutationFn: (body) => createProject(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
    },
  });
}

export function useDeleteProject() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteProjectPathParams>({
    action: { verb: 'delete', target: 'project' },
    mutationFn: (params) => deleteProject(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
    },
  });
}

export function useUpdateProject(params: UpdateProjectPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<ProjectResponse, unknown, UpdateProjectRequest>({
    action: { verb: 'update', target: 'project' },
    mutationFn: (body) => updateProject(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['project'] });
    },
  });
}
