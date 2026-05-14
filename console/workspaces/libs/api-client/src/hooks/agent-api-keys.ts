/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
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
import type {
  AgentAPIKeyListResponse,
  CreateAgentAPIKeyPathParams,
  CreateAgentAPIKeyRequest,
  CreateAgentAPIKeyResponse,
  IssueTestAgentAPIKeyPathParams,
  IssueTestAgentAPIKeyResponse,
  ListAgentAPIKeysPathParams,
  RevokeAgentAPIKeyPathParams,
  RotateAgentAPIKeyPathParams,
  RotateAgentAPIKeyRequest,
  RotateAgentAPIKeyResponse,
} from "@agent-management-platform/types";
import {
  createAgentAPIKey,
  issueTestAgentAPIKey,
  listAgentAPIKeys,
  revokeAgentAPIKey,
  rotateAgentAPIKey,
} from "../apis/agent-api-keys";

export function useListAgentAPIKeys(params: ListAgentAPIKeysPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentAPIKeyListResponse>({
    queryKey: ["agent-api-keys", params.orgName, params.projName, params.agentName, params.envId],
    queryFn: () => listAgentAPIKeys(params, getToken),
    enabled: !!(params.orgName && params.projName && params.agentName && params.envId),
  });
}

export function useCreateAgentAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    CreateAgentAPIKeyResponse,
    unknown,
    { params: CreateAgentAPIKeyPathParams; body: CreateAgentAPIKeyRequest }
  >({
    action: { verb: 'create', target: 'agent api key' },
    mutationFn: ({ params, body }) => createAgentAPIKey(params, body, getToken),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["agent-api-keys", variables.params.orgName, variables.params.projName, variables.params.agentName, variables.params.envId],
      });
    },
  });
}

export function useRotateAgentAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    RotateAgentAPIKeyResponse,
    unknown,
    { params: RotateAgentAPIKeyPathParams; body: RotateAgentAPIKeyRequest }
  >({
    action: { verb: 'rotate', target: 'agent api key' },
    mutationFn: ({ params, body }) => rotateAgentAPIKey(params, body, getToken),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["agent-api-keys", variables.params.orgName, variables.params.projName, variables.params.agentName, variables.params.envId],
      });
    },
  });
}

export function useRevokeAgentAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, RevokeAgentAPIKeyPathParams>({
    action: { verb: 'revoke', target: 'agent api key' },
    mutationFn: (params) => revokeAgentAPIKey(params, getToken),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["agent-api-keys", variables.orgName, variables.projName, variables.agentName, variables.envId],
      });
    },
  });
}

// useTestAgentAPIKey issues (or rotates) a 10-minute test API key for the
// agent's Try-It flow. Each refetch rotates the same logical row on the
// backend (same name, new hash + expiry), so callers can rely on the cached
// value being valid until staleTime elapses.
export function useTestAgentAPIKey(
  params: IssueTestAgentAPIKeyPathParams,
  options: { enabled: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<IssueTestAgentAPIKeyResponse>({
    queryKey: [
      "agent-test-api-key",
      params.orgName,
      params.projName,
      params.agentName,
      params.envId,
    ],
    queryFn: () => issueTestAgentAPIKey(params, getToken),
    enabled:
      options.enabled
      && !!(params.orgName && params.projName && params.agentName && params.envId),
    staleTime: 9 * 60 * 1000,
    refetchInterval: 9 * 60 * 1000,
    refetchOnWindowFocus: false,
  });
}
