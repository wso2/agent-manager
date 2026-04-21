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
  getTraceList,
  exportTraces,
  getSpanDetail,
  listTraceSpans,
  TraceObserverListParams,
} from "../apis/traces";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";
import { useCallback, useEffect, useMemo, useState } from "react";

export function useTraceList(
  organization?: string,
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
  const pageSize = limit ?? 10;
  const [traceList, setTraceList] = useState<TraceListResponse | null>(null);
  const [isLoadingOlder, setIsLoadingOlder] = useState(false);
  const [isLoadingNewer, setIsLoadingNewer] = useState(false);
  const [hasMoreOlder, setHasMoreOlder] = useState(true);
  const [hasMoreNewer, setHasMoreNewer] = useState(true);

  const resolvedRange = useMemo(() => {
    if (hasCustomRange) {
      return { startTime: customStartTime, endTime: customEndTime };
    }
    if (!timeRange) {
      return undefined;
    }
    return getTimeRange(timeRange);
  }, [hasCustomRange, customStartTime, customEndTime, timeRange]);

  const baseParams = useMemo(() => {
    if (!organization || !project || !component || !environment || !resolvedRange) {
      return undefined;
    }
    return {
      organization,
      project,
      component,
      environment,
      startTime: resolvedRange.startTime,
      endTime: resolvedRange.endTime,
      limit: pageSize,
      sortOrder,
    } satisfies TraceObserverListParams;
  }, [
    organization,
    project,
    component,
    environment,
    resolvedRange,
    pageSize,
    sortOrder,
  ]);

  const queryResult = useApiQuery({
    queryKey: [
      "trace-list",
      organization,
      project,
      component,
      environment,
      timeRange,
      pageSize,
      sortOrder,
      customStartTime,
      customEndTime,
    ],
    queryFn: async () => {
      if (!baseParams) {
        throw new Error("Missing required parameters");
      }
      const res = await getTraceList(baseParams, getToken);
      if (res.totalCount === 0) {
        return { traces: [], totalCount: 0 } as TraceListResponse;
      }
      return res;
    },
    refetchInterval: hasCustomRange ? false : 30000,
    enabled: !!baseParams,
  });

  useEffect(() => {
    if (!queryResult.data) return;
    setTraceList(queryResult.data);
    setHasMoreOlder((queryResult.data.traces?.length ?? 0) >= pageSize);
    setHasMoreNewer((queryResult.data.traces?.length ?? 0) >= pageSize);
  }, [queryResult.data, pageSize]);

  const mergeTraces = useCallback(
    (current: TraceListResponse | null, incoming: TraceListResponse): TraceListResponse => {
      const map = new Map<string, TraceListResponse["traces"][number]>();
      for (const trace of current?.traces ?? []) map.set(trace.traceId, trace);
      for (const trace of incoming.traces ?? []) map.set(trace.traceId, trace);

      const traces = Array.from(map.values()).sort((a, b) => {
        const timeA = new Date(a.startTime).getTime();
        const timeB = new Date(b.startTime).getTime();
        return sortOrder === "asc" ? timeA - timeB : timeB - timeA;
      });
      return { traces, totalCount: traces.length };
    },
    [sortOrder],
  );

  const loadOlder = useCallback(async () => {
    if (!baseParams || !traceList?.traces?.length || isLoadingOlder || !hasMoreOlder) return;

    const oldest = traceList.traces.reduce((acc, trace) =>
      new Date(trace.startTime).getTime() < new Date(acc.startTime).getTime() ? trace : acc,
    );

    setIsLoadingOlder(true);
    try {
      const response = await getTraceList(
        { ...baseParams, endTime: oldest.startTime, offset: undefined },
        getToken,
      );
      if ((response.traces?.length ?? 0) === 0) {
        setHasMoreOlder(false);
        return;
      }
      setTraceList((prev) => mergeTraces(prev, response));
      setHasMoreOlder((response.traces?.length ?? 0) >= pageSize);
    } finally {
      setIsLoadingOlder(false);
    }
  }, [baseParams, traceList, isLoadingOlder, hasMoreOlder, getToken, mergeTraces, pageSize]);

  const loadNewer = useCallback(async () => {
    if (!baseParams || !traceList?.traces?.length || isLoadingNewer || !hasMoreNewer) return;

    const newest = traceList.traces.reduce((acc, trace) =>
      new Date(trace.startTime).getTime() > new Date(acc.startTime).getTime() ? trace : acc,
    );

    setIsLoadingNewer(true);
    try {
      const response = await getTraceList(
        { ...baseParams, startTime: newest.startTime, offset: undefined },
        getToken,
      );
      if ((response.traces?.length ?? 0) === 0) {
        setHasMoreNewer(false);
        return;
      }
      setTraceList((prev) => mergeTraces(prev, response));
      setHasMoreNewer((response.traces?.length ?? 0) >= pageSize);
    } finally {
      setIsLoadingNewer(false);
    }
  }, [baseParams, traceList, isLoadingNewer, hasMoreNewer, getToken, mergeTraces, pageSize]);

  const fullLoad = useCallback(async () => {
    for (let i = 0; i < 50; i += 1) {
      if (!hasMoreOlder) break;
      await loadOlder();
    }
  }, [hasMoreOlder, loadOlder]);

  return {
    ...queryResult,
    data: traceList ?? queryResult.data,
    traceList: traceList ?? queryResult.data,
    loadOlder,
    loadNewer,
    fullLoad,
    hasMoreOlder,
    hasMoreNewer,
    isLoadingOlder,
    isLoadingNewer,
  };
}

export function useTrace(
  organization: string | undefined,
  project: string | undefined,
  component: string | undefined,
  environment: string | undefined,
  traceId: string,
  startTime: string | undefined,
  endTime: string | undefined,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery({
    queryKey: [
      "trace",
      organization,
      project,
      component,
      environment,
      traceId,
      startTime,
      endTime,
    ],
    queryFn: () =>
      listTraceSpans(
        {
          traceId,
          organization: organization!,
          project: project!,
          component: component!,
          environment: environment!,
          startTime: startTime!,
          endTime: endTime!,
          limit: 1000,
          sortOrder: "asc",
        },
        getToken,
      ),
    enabled:
      !!organization &&
      !!project &&
      !!component &&
      !!environment &&
      !!traceId &&
      !!startTime &&
      !!endTime,
  });
}

export function useSpanDetail(
  traceId: string | undefined,
  spanId: string | null,
  enabled: boolean,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery({
    queryKey: ["span-detail", traceId, spanId],
    queryFn: async () => {
      return getSpanDetail({ traceId: traceId!, spanId: spanId! }, getToken);
    },
    enabled: enabled && !!traceId && !!spanId,
  });
}

export type ExportTracesParams = Pick<
  TraceObserverListParams,
  "startTime" | "endTime" | "limit" | "offset" | "sortOrder"
> & {
  organization: string;
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
        organization,
        project,
        component,
        environment,
        startTime,
        endTime,
        limit,
        offset,
        sortOrder,
      } = params;

      return exportTraces(
        {
          organization,
          project,
          component,
          environment,
          startTime,
          endTime,
          limit,
          offset,
          sortOrder,
        },
        getToken,
      );
    },
  });
}
