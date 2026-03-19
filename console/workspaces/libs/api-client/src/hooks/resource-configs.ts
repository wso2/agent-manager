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

import { useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";
import {
  getAgentResourceConfigs,
  updateAgentResourceConfigs,
} from "../apis/resource-configs";
import type {
  AgentResourceConfigsResponse,
  GetAgentResourceConfigsPathParams,
  GetAgentResourceConfigsQuery,
  UpdateAgentResourceConfigsPathParams,
  UpdateAgentResourceConfigsQuery,
  UpdateAgentResourceConfigsRequest,
} from "@agent-management-platform/types";

const QUERY_KEY = "resource-configs";

export function useGetAgentResourceConfigs(
  params: GetAgentResourceConfigsPathParams,
  query?: GetAgentResourceConfigsQuery
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentResourceConfigsResponse>({
    queryKey: [QUERY_KEY, params, query],
    queryFn: () => getAgentResourceConfigs(params, query, getToken),
    enabled:
      !!params.orgName && !!params.projName && !!params.agentName,
  });
}

export function useUpdateAgentResourceConfigs() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentResourceConfigsResponse,
    unknown,
    {
      params: UpdateAgentResourceConfigsPathParams;
      body: UpdateAgentResourceConfigsRequest;
      query?: UpdateAgentResourceConfigsQuery;
    }
  >({
    action: { verb: "update", target: "agent resource configs" },
    mutationFn: ({ params, body, query }) =>
      updateAgentResourceConfigs(params, body, query, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
    },
  });
}
