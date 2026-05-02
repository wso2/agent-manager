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

import { useAuthHooks } from "@agent-management-platform/auth";
import { useQueryClient } from "@tanstack/react-query";
import {
  AgentAPIKeyListResponse,
  CreateAgentAPIKeyPathParams,
  CreateAgentAPIKeyRequest,
  CreateAgentAPIKeyResponse,
  ListAgentAPIKeysPathParams,
  RevokeAgentAPIKeyPathParams,
} from "@agent-management-platform/types";
import {
  createAgentAPIKey,
  createAgentTestAPIKey,
  listAgentAPIKeys,
  revokeAgentAPIKey,
} from "../apis";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

export function useListAgentAPIKeys(
  params: ListAgentAPIKeysPathParams,
  options?: { enabled?: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentAPIKeyListResponse>({
    queryKey: ["agent-api-keys", params],
    queryFn: () => listAgentAPIKeys(params, getToken),
    enabled:
      (options?.enabled ?? true) &&
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName,
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
    action: { verb: "create", target: "agent api key" },
    mutationFn: ({ params, body }) => createAgentAPIKey(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["agent-api-keys"] });
    },
  });
}

export function useCreateAgentTestAPIKey() {
  const { getToken } = useAuthHooks();
  return useApiMutation<
    CreateAgentAPIKeyResponse,
    unknown,
    CreateAgentAPIKeyPathParams
  >({
    action: { verb: "create", target: "agent test api key" },
    mutationFn: (params) => createAgentTestAPIKey(params, getToken),
    showSuccess: false,
  });
}

export function useRevokeAgentAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, RevokeAgentAPIKeyPathParams>({
    action: { verb: "revoke", target: "agent api key" },
    mutationFn: (params) => revokeAgentAPIKey(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["agent-api-keys"] });
    },
  });
}
