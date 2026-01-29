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

import { useQuery } from "@tanstack/react-query";
import { getAgentMetrics } from "../apis";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  GetAgentMetricsPathParams,
  MetricsFilterRequest,
  MetricsResponse,
} from "@agent-management-platform/types";

export function useGetAgentMetrics(
  params: GetAgentMetricsPathParams,
  body: MetricsFilterRequest,
  options?: { enabled?: boolean }
) {
  const { getToken } = useAuthHooks();
  return useQuery<MetricsResponse>({
    queryKey: ["agent-metrics", params, body],
    queryFn: () => getAgentMetrics(params, body, getToken),
    enabled:
      (options?.enabled ?? true) &&
      !!params.agentName &&
      !!body.environmentName &&
      !!body.startTime &&
      !!body.endTime,
  });
}
