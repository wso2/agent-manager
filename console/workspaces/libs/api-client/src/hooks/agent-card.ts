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
import type {
  AgentCardPathParams,
  AgentCardResponse,
  SetAgentCardSourceRequest,
} from "@agent-management-platform/types";
import { useApiMutation, useApiQuery } from "./react-query-notifications";
import {
  deleteAgentCardSource,
  getAgentCard,
  refreshAgentCard,
  setAgentCardSource,
} from "../apis/agent-card";
import { nullIfNotFound, pendingRefetchInterval } from "../utils";
import { a2aProxyQueryKey, agentCardQueryKey, invalidateA2AQueries } from "./a2a-query-keys";

/** Resolves to null when no card source exists (a 404); poll: false stops the re-poll. */
export function useGetAgentCard(
  params: AgentCardPathParams,
  options: { enabled?: boolean; poll?: boolean } = {},
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentCardResponse | null>({
    queryKey: agentCardQueryKey(params),
    queryFn: () => nullIfNotFound(getAgentCard(params, getToken)),
    enabled: (options.enabled ?? true) &&
      !!(params.orgName && params.projName && params.agentName && params.envId),
    refetchInterval: (q) => (options.poll === false ? false : pendingRefetchInterval(q.state.data)),
    retry: false,
    silent: true,
  });
}

export function useRefreshAgentCard() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, AgentCardPathParams>({
    action: { verb: "update", target: "agent card" },
    successMessage: "Agent card refresh queued",
    // In mutationFn, since useApiMutation does not await onSuccess.
    mutationFn: async (params) => {
      await refreshAgentCard(params, getToken);
      await invalidateA2AQueries(queryClient, params);
    },
  });
}

export function useSetAgentCardSource() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  // routingFollows: the routing save that follows refetches both instead.
  type Vars = {
    params: AgentCardPathParams; body: SetAgentCardSourceRequest; routingFollows?: boolean;
  };
  return useApiMutation<void, unknown, Vars>({
    action: { verb: "update", target: "agent card source" },
    // Stays pending until the new URL is shown, so the editor can't close on the old one.
    mutationFn: async ({ params, body, routingFollows }) => {
      await setAgentCardSource(params, body, getToken);
      if (!routingFollows) await invalidateA2AQueries(queryClient, params);
    },
  });
}

export function useDeleteAgentCardSource() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, AgentCardPathParams>({
    action: { verb: "remove", target: "agent card source" },
    // The source and its proxy are gone; seeding that keeps unmounted readers off the old data.
    mutationFn: async (params) => {
      await deleteAgentCardSource(params, getToken);
      queryClient.setQueryData(agentCardQueryKey(params), null);
      queryClient.setQueryData(a2aProxyQueryKey(params), null);
      await invalidateA2AQueries(queryClient, params);
    },
  });
}
