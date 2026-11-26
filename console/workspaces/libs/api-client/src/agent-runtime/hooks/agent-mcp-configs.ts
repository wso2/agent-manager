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
  createAgentMCPConfig,
  createMCPConfigAPIKey,
  deleteAgentMCPConfig,
  getAgentMCPConfig,
  listAgentMCPConfigs,
  listMCPConfigAPIKeys,
  revokeMCPConfigAPIKey,
  rotateMCPConfigAPIKey,
  updateAgentMCPConfig,
} from "../apis/agent-mcp-configs";
import {
  agentConfigDocumentFromCreateRequest,
  agentConfigDocumentFromUpdateRequest,
  agentConfigFromDocument,
  agentConfigListFromDocuments,
} from "../apis/agent-config-documents";
import type {
  AgentMCPConfigListResponse,
  AgentMCPConfigPathParams,
  AgentMCPConfigResponse,
  CreateAgentMCPConfigPathParams,
  CreateAgentMCPConfigRequest,
  CreateMCPConfigAPIKeyPathParams,
  CreateMCPConfigAPIKeyRequest,
  CreateMCPConfigAPIKeyResponse,
  DeleteAgentMCPConfigPathParams,
  ListAgentMCPConfigsPathParams,
  ListAgentMCPConfigsQuery,
  ListAPIKeysResponse,
  ListMCPConfigAPIKeysPathParams,
  RevokeMCPConfigAPIKeyPathParams,
  RotateMCPConfigAPIKeyPathParams,
  RotateMCPConfigAPIKeyRequest,
  RotateMCPConfigAPIKeyResponse,
  UpdateAgentMCPConfigPathParams,
  UpdateAgentMCPConfigRequest,
  CreateAgentModelConfigRequest,
  UpdateAgentModelConfigRequest,
} from "@agent-management-platform/types";

const QUERY_KEY = "agent-mcp-configs";

export function useListAgentMCPConfigs(
  params: ListAgentMCPConfigsPathParams,
  query?: ListAgentMCPConfigsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentMCPConfigListResponse>({
    queryKey: [QUERY_KEY, "list", params, query],
    queryFn: async () =>
      agentConfigListFromDocuments(await listAgentMCPConfigs(params, query, getToken), params.agentName ?? ""),
    enabled: !!params.orgName && !!params.projName && !!params.agentName,
  });
}

export function useGetAgentMCPConfig(params: AgentMCPConfigPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentMCPConfigResponse>({
    queryKey: [QUERY_KEY, params],
    queryFn: async () => {
      if (!params.configId) throw new Error("configId is required");
      const doc = await getAgentMCPConfig({ ...params, configId: params.configId }, getToken);
      return agentConfigFromDocument(doc, params.agentName ?? "");
    },
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.configId,
  });
}

export function useCreateAgentMCPConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentMCPConfigResponse,
    unknown,
    {
      params: CreateAgentMCPConfigPathParams;
      body: CreateAgentMCPConfigRequest;
    }
  >({
    action: { verb: "create", target: "agent MCP config" },
    mutationFn: async ({ params, body }) => {
      const doc = agentConfigDocumentFromCreateRequest(
        "AgentMcpConfig", params.projName ?? "", body as CreateAgentModelConfigRequest);
      return agentConfigFromDocument(await createAgentMCPConfig(params, doc, getToken), params.agentName ?? "");
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
      queryClient.invalidateQueries({
        queryKey: ["agent-mcp-proxies", toAgentPathParams(variables.params)],
      });
      // A new MCP config changes the agent's AgentID scope union (see
      // agent_configuration_service.go's refreshTouchedMCPEnvironments) — the
      // deploy cards' displayed AMP_AGENTID_SCOPES value comes from this query,
      // so it must invalidate too or it keeps showing the pre-change scope list.
      queryClient.invalidateQueries({
        queryKey: ["agent-configurations", toAgentPathParams(variables.params)],
      });
    },
  });
}

export function useUpdateAgentMCPConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentMCPConfigResponse,
    unknown,
    {
      params: UpdateAgentMCPConfigPathParams;
      body: UpdateAgentMCPConfigRequest;
    }
  >({
    action: { verb: "update", target: "agent MCP config" },
    mutationFn: async ({ params, body }) => {
      const current = await getAgentMCPConfig(params, getToken);
      const doc = agentConfigDocumentFromUpdateRequest(
        "AgentMcpConfig", params.projName ?? "", body as UpdateAgentModelConfigRequest, current);
      return agentConfigFromDocument(await updateAgentMCPConfig(params, doc, getToken), params.agentName ?? "");
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
      queryClient.invalidateQueries({
        queryKey: ["agent-mcp-proxies", toAgentPathParams(variables.params)],
      });
      // See useCreateAgentMCPConfig's identical invalidation for why.
      queryClient.invalidateQueries({
        queryKey: ["agent-configurations", toAgentPathParams(variables.params)],
      });
    },
  });
}

export function useDeleteAgentMCPConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteAgentMCPConfigPathParams>({
    action: { verb: "delete", target: "agent MCP config" },
    mutationFn: (params) => deleteAgentMCPConfig(params, getToken),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
      queryClient.invalidateQueries({
        queryKey: ["agent-mcp-proxies", toAgentPathParams(variables)],
      });
      // See useCreateAgentMCPConfig's identical invalidation for why.
      queryClient.invalidateQueries({
        queryKey: ["agent-configurations", toAgentPathParams(variables)],
      });
    },
  });
}

function toAgentPathParams(params: {
  orgName?: string;
  projName?: string;
  agentName?: string;
}) {
  return {
    orgName: params.orgName,
    projName: params.projName,
    agentName: params.agentName,
  };
}

// -----------------------------------------------------------------------------
// Per-config MCP API keys (external agents)
// -----------------------------------------------------------------------------

const MCP_CONFIG_API_KEY_QUERY_KEY = "mcp-config-api-keys";

export function useListMCPConfigAPIKeys(params: ListMCPConfigAPIKeysPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ListAPIKeysResponse>({
    queryKey: [
      MCP_CONFIG_API_KEY_QUERY_KEY,
      params.orgName,
      params.projName,
      params.agentName,
      params.configId,
      params.envName,
    ],
    queryFn: () => listMCPConfigAPIKeys(params, getToken),
    enabled: !!(
      params.orgName &&
      params.projName &&
      params.agentName &&
      params.configId &&
      params.envName
    ),
  });
}

export function useCreateMCPConfigAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    CreateMCPConfigAPIKeyResponse,
    unknown,
    { params: CreateMCPConfigAPIKeyPathParams; body: CreateMCPConfigAPIKeyRequest }
  >({
    action: { verb: "create", target: "MCP config api key" },
    mutationFn: ({ params, body }) =>
      createMCPConfigAPIKey(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [MCP_CONFIG_API_KEY_QUERY_KEY],
      });
    },
  });
}

export function useRotateMCPConfigAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    RotateMCPConfigAPIKeyResponse,
    unknown,
    { params: RotateMCPConfigAPIKeyPathParams; body: RotateMCPConfigAPIKeyRequest }
  >({
    action: { verb: "rotate", target: "MCP config api key" },
    mutationFn: ({ params, body }) =>
      rotateMCPConfigAPIKey(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [MCP_CONFIG_API_KEY_QUERY_KEY],
      });
    },
  });
}

export function useRevokeMCPConfigAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, RevokeMCPConfigAPIKeyPathParams>({
    action: { verb: "revoke", target: "MCP config api key" },
    mutationFn: (params) => revokeMCPConfigAPIKey(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [MCP_CONFIG_API_KEY_QUERY_KEY],
      });
    },
  });
}
