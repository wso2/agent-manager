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

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createProject, deleteProject, getProject, listProjects } from "../apis";
import {
  ProjectListResponse,
  ProjectResponse,
  ListProjectsPathParams,
  GetProjectPathParams,
  ListProjectsQuery,
  CreateProjectPathParams,
  CreateProjectRequest,
  DeleteProjectPathParams,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";

export function useListProjects(
  params: ListProjectsPathParams,
  query?: ListProjectsQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<ProjectListResponse>({
    queryKey: ['projects', params, query],
    queryFn: () => listProjects(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useGetProject(params: GetProjectPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<ProjectResponse>({
    queryKey: ['project', params],
    queryFn: () => getProject(params, getToken),
    enabled: !!params.orgName && !!params.projName,
  });
}

export function useCreateProject(params: CreateProjectPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    ProjectResponse,
    unknown,
    CreateProjectRequest
  >({
    mutationFn: (body) => createProject(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
    },
  });
}

export function useDeleteProject() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<void, unknown, DeleteProjectPathParams>({
    mutationFn: (params) => deleteProject(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
    },
  });
}
