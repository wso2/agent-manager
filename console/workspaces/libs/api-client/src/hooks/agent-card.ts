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

import { useQueryClient, type QueryClient } from "@tanstack/react-query";
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
import { POLL_INTERVAL } from "../utils";

const agentCardKey = (p: AgentCardPathParams) =>
  ["agent-card", p.orgName, p.projName, p.agentName, p.envId] as const;

/** Drops the cached card so a removed source does not keep showing its old card. */
export function removeAgentCardFromCache(queryClient: QueryClient, params: AgentCardPathParams) {
  queryClient.removeQueries({ queryKey: agentCardKey(params) });
}

/** Re-poll while pending; stop on fetched, failed, or a 404 (which keeps stale data cached). */
export function agentCardRefetchInterval(
  data: AgentCardResponse | undefined,
  error?: unknown,
): number | false {
  if ((error as { status?: number } | null)?.status === 404) return false;
  return data?.status === "pending" ? POLL_INTERVAL : false;
}

/** A 404 (not an A2A agent, or no fetch queued yet) is expected: it errors silently. */
export function useGetAgentCard(params: AgentCardPathParams, options: { enabled?: boolean } = {}) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentCardResponse>({
    queryKey: agentCardKey(params),
    queryFn: () => getAgentCard(params, getToken),
    enabled: (options.enabled ?? true) &&
      !!(params.orgName && params.projName && params.agentName && params.envId),
    refetchInterval: (q) => agentCardRefetchInterval(q.state.data, q.state.error),
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
      await queryClient.invalidateQueries({ queryKey: agentCardKey(params) });
    },
  });
}

export function useSetAgentCardSource() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  type Vars = { params: AgentCardPathParams; body: SetAgentCardSourceRequest };
  return useApiMutation<void, unknown, Vars>({
    action: { verb: "update", target: "agent card source" },
    // Stays pending until the new URL is shown, so the editor can't close on the old one.
    mutationFn: async ({ params, body }) => {
      await setAgentCardSource(params, body, getToken);
      await queryClient.invalidateQueries({ queryKey: agentCardKey(params) });
    },
  });
}

export function useDeleteAgentCardSource() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, AgentCardPathParams>({
    action: { verb: "remove", target: "agent card source" },
    mutationFn: (params) => deleteAgentCardSource(params, getToken),
    onSuccess: (_d, params) =>
      removeAgentCardFromCache(queryClient, params),
  });
}
