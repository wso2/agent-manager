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

import { getAgentMetrics } from "../apis/metrics";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiQuery } from "../../shared/react-query-notifications";
import {
  getTimeRange,
  type GetAgentMetricsPathParams,
  type MetricsFilterRequest,
  type MetricsResponse,
  type TraceListTimeRange,
} from "@agent-management-platform/types";
import { SLOW_POLL_INTERVAL } from "../../shared/utils";

export function useGetAgentMetrics(
  params: GetAgentMetricsPathParams,
  body: MetricsFilterRequest,
  options?: { enabled?: boolean; enableAutoRefresh?: boolean; timeRange?: TraceListTimeRange }
) {
  const { getToken } = useAuthHooks();
  const hasPreset = !!options?.timeRange;
  return useApiQuery<MetricsResponse>({
    queryKey: [
      "agent-metrics",
      params,
      body.environmentName,
      hasPreset ? options!.timeRange : { startTime: body.startTime, endTime: body.endTime },
    ],
    queryFn: () => {
      const { startTime, endTime } = hasPreset
        ? getTimeRange(options!.timeRange!)
        : { startTime: body.startTime, endTime: body.endTime };
      return getAgentMetrics(
        {
          organization: params.orgName ?? "default",
          project: params.projName ?? "default",
          agent: params.agentName!,
          environment: body.environmentName,
          startTime,
          endTime,
        },
        getToken,
      );
    },
    refetchInterval: options?.enableAutoRefresh ? SLOW_POLL_INTERVAL : undefined,
    enabled:
      (options?.enabled ?? true) &&
      !!params.agentName &&
      !!body.environmentName &&
      (hasPreset || (!!body.startTime && !!body.endTime)),
  });
}
