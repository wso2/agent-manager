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

import {
  getTimeRange,
  TraceListResponse,
  TraceListTimeRange,
  GetTraceListPathParams,
  TraceExportResponse,
} from "@agent-management-platform/types";
import {
  getTrace,
  getTraceList,
  exportTraces,
  TraceObserverListParams,
} from "../apis/traces";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

export function useTraceList(
  namespace?: string,
  project?: string,
  component?: string,
  environment?: string,
  timeRange?: TraceListTimeRange | undefined,
  limit?: number | undefined,
  sortOrder?: GetTraceListPathParams["sortOrder"] | undefined,
  customStartTime?: string,
  customEndTime?: string,
) {
  const { getToken } = useAuthHooks();

  const hasCustomRange = !!customStartTime && !!customEndTime;

  return useApiQuery({
    queryKey: [
      "trace-list",
      namespace,
      project,
      component,
      environment,
      timeRange,
      limit,
      sortOrder,
      customStartTime,
      customEndTime,
    ],
    queryFn: async () => {
      if (!namespace || !project || !component || !environment) {
        throw new Error("Missing required parameters");
      }

      let startTime: string;
      let endTime: string;
      if (hasCustomRange) {
        startTime = customStartTime;
        endTime = customEndTime;
      } else {
        if (!timeRange) {
          throw new Error("Missing required parameters");
        }
        ({ startTime, endTime } = getTimeRange(timeRange));
      }

      const res = await getTraceList(
        {
          namespace,
          project,
          component,
          environment,
          startTime,
          endTime,
          limit,
          sortOrder,
        },
        getToken,
      );
      if (res.totalCount === 0) {
        return { traces: [], totalCount: 0 } as TraceListResponse;
      }
      return res;
    },
    refetchInterval: hasCustomRange ? false : 30000,
    enabled:
      !!namespace &&
      !!project &&
      !!component &&
      !!environment &&
      (hasCustomRange || !!timeRange),
  });
}

export function useTrace(
  namespace: string | undefined,
  project: string | undefined,
  component: string | undefined,
  environment: string | undefined,
  traceId: string,
  startTime: string | undefined,
  endTime: string | undefined,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery({
    queryKey: ["trace", namespace, project, component, environment, traceId, startTime, endTime],
    queryFn: async () => {
      return getTrace(
        {
          traceId,
          namespace: namespace!,
          project: project!,
          component: component!,
          environment: environment!,
          startTime: startTime!,
          endTime: endTime!,
        },
        getToken,
      );
    },
    enabled:
      !!namespace &&
      !!project &&
      !!component &&
      !!environment &&
      !!traceId &&
      !!startTime &&
      !!endTime,
  });
}

export type ExportTracesParams = Pick<
  TraceObserverListParams,
  "startTime" | "endTime" | "limit" | "sortOrder"
> & {
  namespace: string;
  project: string;
  component: string;
  environment: string;
};

export function useExportTraces() {
  const { getToken } = useAuthHooks();

  return useApiMutation({
    action: { verb: "create", target: "trace export" },
    mutationFn: async (
      params: ExportTracesParams,
    ): Promise<TraceExportResponse> => {
      const {
        namespace,
        project,
        component,
        environment,
        startTime,
        endTime,
        limit,
        sortOrder,
      } = params;

      return exportTraces(
        {
          namespace,
          project,
          component,
          environment,
          startTime,
          endTime,
          limit,
          sortOrder,
        },
        getToken,
      );
    },
  });
}
