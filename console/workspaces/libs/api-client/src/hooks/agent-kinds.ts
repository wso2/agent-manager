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

import { useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";
import {
  listAgentKinds,
  getAgentKind,
  updateAgentKind,
  deleteAgentKind,
  listAgentKindVersions,
  addAgentKindVersion,
  getAgentKindVersion,
  deleteAgentKindVersion,
  publishAgentKind,
  listKindAgents,
} from "../apis";
import type {
  AgentKindListResponse,
  AgentKindResponse,
  AgentKindVersionResponse,
  AgentResponse,
  AddAgentKindVersionPathParams,
  AddAgentKindVersionRequest,
  DeleteAgentKindPathParams,
  DeleteAgentKindVersionPathParams,
  GetAgentKindPathParams,
  GetAgentKindVersionPathParams,
  ListAgentKindsPathParams,
  ListAgentKindsQuery,
  ListAgentKindVersionsPathParams,
  ListKindAgentsPathParams,
  PublishAgentKindPathParams,
  PublishAgentKindRequest,
  UpdateAgentKindPathParams,
  UpdateAgentKindRequest,
} from "@agent-management-platform/types";

export const agentKindKeys = {
  lists: () => ['agent-kinds'] as const,
  list: (params: ListAgentKindsPathParams, query?: ListAgentKindsQuery) => ['agent-kinds', params, query] as const,
  details: () => ['agent-kind'] as const,
  detail: (params: GetAgentKindPathParams) => ['agent-kind', params] as const,
  versionLists: () => ['agent-kind-versions'] as const,
  versionList: (params: ListAgentKindVersionsPathParams) => ['agent-kind-versions', params] as const,
  versionDetails: () => ['agent-kind-version'] as const,
  versionDetail: (params: GetAgentKindVersionPathParams) => ['agent-kind-version', params] as const,
  kindAgentLists: () => ['agent-kind-agents'] as const,
  kindAgentList: (params: ListKindAgentsPathParams) => ['agent-kind-agents', params] as const,
};

/**
 * Hook to list all Agent Kinds for an organization
 */
export function useListAgentKinds(
  params: ListAgentKindsPathParams,
  query?: ListAgentKindsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentKindListResponse>({
    queryKey: agentKindKeys.list(params, query),
    queryFn: () => listAgentKinds(params, query, getToken),
    enabled: !!params.orgName,
  });
}

/**
 * Hook to get details of an Agent Kind
 */
export function useGetAgentKind(params: GetAgentKindPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentKindResponse>({
    queryKey: agentKindKeys.detail(params),
    queryFn: () => getAgentKind(params, getToken),
    enabled: !!params.orgName && !!params.kindName,
  });
}

/**
 * Hook to update display name or description of an Agent Kind
 */
export function useUpdateAgentKind() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentKindResponse,
    unknown,
    { params: UpdateAgentKindPathParams; body: UpdateAgentKindRequest }
  >({
    action: { verb: 'update', target: 'agent kind' },
    mutationFn: ({ params, body }) => updateAgentKind(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKindKeys.lists() });
      queryClient.invalidateQueries({ queryKey: agentKindKeys.details() });
    },
  });
}

/**
 * Hook to delete an Agent Kind and all its versions
 */
export function useDeleteAgentKind() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteAgentKindPathParams>({
    action: { verb: 'delete', target: 'agent kind' },
    mutationFn: (params) => deleteAgentKind(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKindKeys.lists() });
    },
  });
}

/**
 * Hook to list all versions of an Agent Kind
 */
export function useListAgentKindVersions(params: ListAgentKindVersionsPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentKindVersionResponse[]>({
    queryKey: agentKindKeys.versionList(params),
    queryFn: () => listAgentKindVersions(params, getToken),
    enabled: !!params.orgName && !!params.kindName,
  });
}

/**
 * Hook to add a new version to an existing Agent Kind
 */
export function useAddAgentKindVersion() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentKindVersionResponse,
    unknown,
    { params: AddAgentKindVersionPathParams; body: AddAgentKindVersionRequest }
  >({
    action: { verb: 'create', target: 'agent kind version' },
    mutationFn: ({ params, body }) => addAgentKindVersion(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKindKeys.versionLists() });
      queryClient.invalidateQueries({ queryKey: agentKindKeys.details() });
      queryClient.invalidateQueries({ queryKey: agentKindKeys.lists() });
    },
  });
}

/**
 * Hook to get a specific version of an Agent Kind
 */
export function useGetAgentKindVersion(params: GetAgentKindVersionPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentKindVersionResponse>({
    queryKey: agentKindKeys.versionDetail(params),
    queryFn: () => getAgentKindVersion(params, getToken),
    enabled: !!params.orgName && !!params.kindName && !!params.versionTag,
  });
}

/**
 * Hook to delete a specific version of an Agent Kind
 */
export function useDeleteAgentKindVersion() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteAgentKindVersionPathParams>({
    action: { verb: 'delete', target: 'agent kind version' },
    mutationFn: (params) => deleteAgentKindVersion(params, getToken),
    onSuccess: (_data, params) => {
      queryClient.invalidateQueries({ queryKey: agentKindKeys.versionLists() });
      queryClient.invalidateQueries({
        queryKey: agentKindKeys.detail({
          orgName: params.orgName,
          kindName: params.kindName,
        }),
      });
      queryClient.invalidateQueries({ queryKey: agentKindKeys.lists() });
    },
  });
}

/**
 * Hook to publish an agent build as an Agent Kind version
 */
export function usePublishAgentKind() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentKindVersionResponse,
    unknown,
    { params: PublishAgentKindPathParams; body: PublishAgentKindRequest }
  >({
    action: { verb: 'publish', target: 'agent kind' },
    mutationFn: ({ params, body }) => publishAgentKind(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKindKeys.lists() });
      queryClient.invalidateQueries({ queryKey: agentKindKeys.details() });
      queryClient.invalidateQueries({ queryKey: agentKindKeys.versionLists() });
    },
  });
}

/**
 * Hook to list all agents deployed from a given Agent Kind across all projects in the org
 */
export function useListKindAgents(params: ListKindAgentsPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentResponse[]>({
    queryKey: agentKindKeys.kindAgentList(params),
    queryFn: () => listKindAgents(params, getToken),
    enabled: !!params.orgName && !!params.kindName,
  });
}
