import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createProject, getProject, listProjects } from "../apis";
import {
  ProjectListResponse,
  ProjectResponse,
  ListProjectsPathParams,
  GetProjectPathParams,
  ListProjectsQuery,
  CreateProjectPathParams,
  CreateProjectRequest,
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

