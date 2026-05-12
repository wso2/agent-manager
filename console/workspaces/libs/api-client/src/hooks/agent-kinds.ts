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
} from "../apis";
import type {
  AgentKindListResponse,
  AgentKindResponse,
  AgentKindVersionResponse,
  AddAgentKindVersionPathParams,
  AddAgentKindVersionRequest,
  DeleteAgentKindPathParams,
  DeleteAgentKindVersionPathParams,
  GetAgentKindPathParams,
  GetAgentKindVersionPathParams,
  ListAgentKindsPathParams,
  ListAgentKindsQuery,
  ListAgentKindVersionsPathParams,
  PublishAgentKindPathParams,
  PublishAgentKindRequest,
  UpdateAgentKindPathParams,
  UpdateAgentKindRequest,
} from "@agent-management-platform/types";

/**
 * Hook to list all Agent Kinds for an organization
 */
export function useListAgentKinds(
  params: ListAgentKindsPathParams,
  query?: ListAgentKindsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentKindListResponse>({
    queryKey: ['agent-kinds', params, query],
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
    queryKey: ['agent-kind', params],
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
      queryClient.invalidateQueries({ queryKey: ['agent-kinds'] });
      queryClient.invalidateQueries({ queryKey: ['agent-kind'] });
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
      queryClient.invalidateQueries({ queryKey: ['agent-kinds'] });
    },
  });
}

/**
 * Hook to list all versions of an Agent Kind
 */
export function useListAgentKindVersions(params: ListAgentKindVersionsPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentKindVersionResponse[]>({
    queryKey: ['agent-kind-versions', params],
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
      queryClient.invalidateQueries({ queryKey: ['agent-kind-versions'] });
      queryClient.invalidateQueries({ queryKey: ['agent-kind'] });
      queryClient.invalidateQueries({ queryKey: ['agent-kinds'] });
    },
  });
}

/**
 * Hook to get a specific version of an Agent Kind
 */
export function useGetAgentKindVersion(params: GetAgentKindVersionPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentKindVersionResponse>({
    queryKey: ['agent-kind-version', params],
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
      queryClient.invalidateQueries({ queryKey: ['agent-kind-versions'] });
      queryClient.invalidateQueries({ queryKey: ['agent-kind', { orgName: params.orgName, kindName: params.kindName }] });
      queryClient.invalidateQueries({ queryKey: ['agent-kinds'] });
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
      queryClient.invalidateQueries({ queryKey: ['agent-kinds'] });
      queryClient.invalidateQueries({ queryKey: ['agent-kind'] });
      queryClient.invalidateQueries({ queryKey: ['agent-kind-versions'] });
    },
  });
}
