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
import { useApiMutation, useApiQuery } from "../../shared/react-query-notifications";
import {
  createAgentModelConfig,
  createLLMConfigAPIKey,
  deleteAgentModelConfig,
  getAgentModelConfig,
  listAgentModelConfigs,
  listLLMConfigAPIKeys,
  revokeLLMConfigAPIKey,
  rotateLLMConfigAPIKey,
  updateAgentModelConfig,
} from "../apis/agent-model-configs";
import {
  agentConfigDocumentFromCreateRequest,
  agentConfigDocumentFromUpdateRequest,
  agentConfigFromDocument,
  agentConfigListFromDocuments,
} from "../apis/agent-config-documents";
import type {
  AgentModelConfigListResponse,
  AgentModelConfigPathParams,
  AgentModelConfigResponse,
  CreateAgentModelConfigPathParams,
  CreateAgentModelConfigRequest,
  CreateLLMConfigAPIKeyPathParams,
  CreateLLMConfigAPIKeyRequest,
  CreateLLMConfigAPIKeyResponse,
  DeleteAgentModelConfigPathParams,
  ListAgentModelConfigsPathParams,
  ListAgentModelConfigsQuery,
  ListAPIKeysResponse,
  ListLLMConfigAPIKeysPathParams,
  RevokeLLMConfigAPIKeyPathParams,
  RotateLLMConfigAPIKeyPathParams,
  RotateLLMConfigAPIKeyRequest,
  RotateLLMConfigAPIKeyResponse,
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
    queryFn: async () =>
      agentConfigListFromDocuments(await listAgentModelConfigs(params, query, getToken), params.agentName ?? ""),
    enabled:
      !!params.orgName && !!params.projName && !!params.agentName,
  });
}

export function useGetAgentModelConfig(params: AgentModelConfigPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentModelConfigResponse>({
    queryKey: [QUERY_KEY, params],
    queryFn: async () => {
      if (!params.configId) throw new Error("configId is required");
      const doc = await getAgentModelConfig({ ...params, configId: params.configId }, getToken);
      return agentConfigFromDocument(doc, params.agentName ?? "");
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
    mutationFn: async ({ params, body }) => {
      const doc = agentConfigDocumentFromCreateRequest(
        "AgentModelConfig", params.projName ?? "", body as CreateAgentModelConfigRequest);
      return agentConfigFromDocument(await createAgentModelConfig(params, doc, getToken), params.agentName ?? "");
    },
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
    mutationFn: async ({ params, body }) => {
      const current = await getAgentModelConfig(params, getToken);
      const doc = agentConfigDocumentFromUpdateRequest(
        "AgentModelConfig", params.projName ?? "", body as UpdateAgentModelConfigRequest, current);
      return agentConfigFromDocument(await updateAgentModelConfig(params, doc, getToken), params.agentName ?? "");
    },
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

// -----------------------------------------------------------------------------
// Per-config LLM API keys (external agents)
// -----------------------------------------------------------------------------

const LLM_CONFIG_API_KEY_QUERY_KEY = "llm-config-api-keys";

export function useListLLMConfigAPIKeys(params: ListLLMConfigAPIKeysPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ListAPIKeysResponse>({
    queryKey: [
      LLM_CONFIG_API_KEY_QUERY_KEY,
      params.orgName,
      params.projName,
      params.agentName,
      params.configId,
      params.envName,
    ],
    queryFn: () => listLLMConfigAPIKeys(params, getToken),
    enabled: !!(
      params.orgName &&
      params.projName &&
      params.agentName &&
      params.configId &&
      params.envName
    ),
  });
}

export function useCreateLLMConfigAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    CreateLLMConfigAPIKeyResponse,
    unknown,
    { params: CreateLLMConfigAPIKeyPathParams; body: CreateLLMConfigAPIKeyRequest }
  >({
    action: { verb: "create", target: "LLM config api key" },
    mutationFn: ({ params, body }) =>
      createLLMConfigAPIKey(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [LLM_CONFIG_API_KEY_QUERY_KEY],
      });
    },
  });
}

export function useRotateLLMConfigAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    RotateLLMConfigAPIKeyResponse,
    unknown,
    { params: RotateLLMConfigAPIKeyPathParams; body: RotateLLMConfigAPIKeyRequest }
  >({
    action: { verb: "rotate", target: "LLM config api key" },
    mutationFn: ({ params, body }) =>
      rotateLLMConfigAPIKey(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [LLM_CONFIG_API_KEY_QUERY_KEY],
      });
    },
  });
}

export function useRevokeLLMConfigAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, RevokeLLMConfigAPIKeyPathParams>({
    action: { verb: "revoke", target: "LLM config api key" },
    mutationFn: (params) => revokeLLMConfigAPIKey(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [LLM_CONFIG_API_KEY_QUERY_KEY],
      });
    },
  });
}
