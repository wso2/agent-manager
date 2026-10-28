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

import { hashKey, useIsMutating, useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import type {
  A2AProxyPathParams,
  A2AProxyResponse,
  SetA2AProxyRequest,
} from "@agent-management-platform/types";
import { useApiMutation, useApiQuery } from "./react-query-notifications";
import { deleteA2AProxy, getA2AProxy, refreshA2AProxy, setA2AProxy } from "../apis/a2a-proxy";
import { a2aProxyQueryKey, invalidateA2AQueries } from "./a2a-query-keys";
import { nullIfNotFound, pendingRefetchInterval } from "../utils";

/** Resolves to null while the environment is direct (a 404). */
export function useGetA2AProxy(params: A2AProxyPathParams, options: { enabled?: boolean } = {}) {
  const { getToken } = useAuthHooks();
  return useApiQuery<A2AProxyResponse | null>({
    queryKey: a2aProxyQueryKey(params),
    queryFn: () => nullIfNotFound(getA2AProxy(params, getToken)),
    enabled: (options.enabled ?? true) &&
      !!(params.orgName && params.projName && params.agentName && params.envId),
    refetchInterval: (q) => pendingRefetchInterval(q.state.data),
    retry: false,
    silent: true,
  });
}

const setA2AProxyMutationKey = ["a2a-proxy", "set"] as const;

/** True while a routing save for this agent environment is in flight, from any component. */
export function useIsSavingA2AProxy(params: A2AProxyPathParams): boolean {
  const key = hashKey(a2aProxyQueryKey(params));
  return useIsMutating({
    mutationKey: setA2AProxyMutationKey,
    predicate: (m) => {
      const p = (m.state.variables as { params?: A2AProxyPathParams } | undefined)?.params;
      return !!p && hashKey(a2aProxyQueryKey(p)) === key;
    },
  }) > 0;
}

export function useSetA2AProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  type Vars = { params: A2AProxyPathParams; body: SetA2AProxyRequest };
  return useApiMutation<void, unknown, Vars>({
    mutationKey: setA2AProxyMutationKey,
    action: { verb: "update", target: "gateway routing" },
    // In mutationFn (onSuccess is not awaited) and in finally, as a card save may defer to it.
    mutationFn: async ({ params, body }) => {
      try {
        await setA2AProxy(params, body, getToken);
      } finally {
        await invalidateA2AQueries(queryClient, params);
      }
    },
  });
}

export function useDeleteA2AProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, A2AProxyPathParams>({
    action: { verb: "remove", target: "gateway routing" },
    // Known direct now, so unmounted readers do not briefly show the old routing.
    mutationFn: async (params) => {
      await deleteA2AProxy(params, getToken);
      queryClient.setQueryData(a2aProxyQueryKey(params), null);
      await invalidateA2AQueries(queryClient, params);
    },
  });
}

export function useRefreshA2AProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, A2AProxyPathParams>({
    action: { verb: "update", target: "gateway routing" },
    successMessage: "Gateway republish queued",
    mutationFn: async (params) => {
      await refreshA2AProxy(params, getToken);
      await invalidateA2AQueries(queryClient, params);
    },
  });
}
