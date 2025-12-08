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
import { createAgent, deleteAgent, getAgent, listAgents } from "../apis";
import {
  AgentListResponse,
  AgentResponse,
  CreateAgentPathParams,
  CreateAgentRequest,
  DeleteAgentPathParams,
  GetAgentPathParams,
  ListAgentsPathParams,
  ListAgentsQuery,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";

export function useListAgents(
  params: ListAgentsPathParams,
  query?: ListAgentsQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<AgentListResponse>({
    queryKey: ['agents', params, query],
    queryFn: () => listAgents(params, query, getToken),
    enabled: !!params.orgName && !!params.projName,
  });
}

export function useGetAgent(params: GetAgentPathParams) {
    const { getToken } = useAuthHooks();
    return useQuery<AgentResponse>({
        queryKey: ['agent', params],
        queryFn: () => getAgent(params, getToken),
        enabled: !!params.orgName && !!params.projName && !!params.agentName,
    });
}

export function useCreateAgent() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    AgentResponse,
    unknown,
    { params: CreateAgentPathParams; body: CreateAgentRequest }
  >({
    mutationFn: ({ params, body }) => createAgent(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    },
  });
}

export function useDeleteAgent() {
    const { getToken } = useAuthHooks();
    const queryClient = useQueryClient();
    return useMutation<void, unknown, DeleteAgentPathParams>({
        mutationFn: (params) => deleteAgent(params, getToken),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['agents'] });
        },
    });
}
