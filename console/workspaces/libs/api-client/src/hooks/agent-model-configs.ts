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
  createAgentModelConfig,
  deleteAgentModelConfig,
  getAgentModelConfig,
  listAgentModelConfigs,
  updateAgentModelConfig,
} from "../apis/agent-model-configs";
import type {
  AgentModelConfigListResponse,
  AgentModelConfigPathParams,
  AgentModelConfigResponse,
  CreateAgentModelConfigPathParams,
  CreateAgentModelConfigRequest,
  DeleteAgentModelConfigPathParams,
  ListAgentModelConfigsPathParams,
  ListAgentModelConfigsQuery,
  UpdateAgentModelConfigPathParams,
  UpdateAgentModelConfigRequest,
} from "@agent-management-platform/types";

const QUERY_KEY = "agent-model-configs";

export function useListAgentModelConfigs(
  params: ListAgentModelConfigsPathParams,
  query?: ListAgentModelConfigsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentModelConfigListResponse>({
    queryKey: [QUERY_KEY, "list", params, query],
    queryFn: () => listAgentModelConfigs(params, query, getToken),
    enabled:
      !!params.orgName && !!params.projName && !!params.agentName,
  });
}

export function useGetAgentModelConfig(params: AgentModelConfigPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentModelConfigResponse>({
    queryKey: [QUERY_KEY, params],
    queryFn: () => {
      if (!params.configId) throw new Error("configId is required");
      return getAgentModelConfig(
        { ...params, configId: params.configId },
        getToken,
      );
    },
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.configId,
  });
}

export function useCreateAgentModelConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentModelConfigResponse,
    unknown,
    {
      params: CreateAgentModelConfigPathParams;
      body: CreateAgentModelConfigRequest;
    }
  >({
    action: { verb: 'create', target: 'agent model config' },
    mutationFn: ({ params, body }) =>
      createAgentModelConfig(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
    },
  });
}

export function useUpdateAgentModelConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentModelConfigResponse,
    unknown,
    {
      params: UpdateAgentModelConfigPathParams;
      body: UpdateAgentModelConfigRequest;
    }
  >({
    action: { verb: 'update', target: 'agent model config' },
    mutationFn: ({ params, body }) =>
      updateAgentModelConfig(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
    },
  });
}

export function useDeleteAgentModelConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteAgentModelConfigPathParams>({
    action: { verb: 'delete', target: 'agent model config' },
    mutationFn: (params) => deleteAgentModelConfig(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
    },
  });
}
