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
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

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

  // Non-time params — stable across refetches while org/project/etc don't change.
  const scopeParams = useMemo(() => {
    if (!organization || !project || !component || !environment)
      return undefined;
    return {
      organization,
      project,
      component,
      environment,
      limit: pageSize,
      sortOrder,
    };
  }, [organization, project, component, environment, pageSize, sortOrder]);

  // Tracks the time range used in the most recent successful fetch so that
  // loadOlder / loadNewer paginate against the same window.
  const lastFetchedRangeRef = useRef<{
    startTime: string;
    endTime: string;
  } | null>(null);

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
      if (!scopeParams) {
        throw new Error("Missing required parameters");
      }
      // Always compute the range at call-time so refetches use the current clock,
      // not a timestamp frozen when the component first mounted.
      const range = hasCustomRange
        ? { startTime: customStartTime!, endTime: customEndTime! }
        : getTimeRange(timeRange!)!;

      lastFetchedRangeRef.current = range;

      const res = await getTraceList({ ...scopeParams, ...range }, getToken);
      if (res.totalCount === 0) {
        return { traces: [], totalCount: 0 } as TraceListResponse;
      }
      return res;
    },
    enabled: !!scopeParams && (hasCustomRange || !!timeRange),
  });

  useEffect(() => {
    if (!queryResult.data) return;
    setTraceList(queryResult.data);
  }, [queryResult.data]);

  const mergeTraces = useCallback(
    (
      current: TraceListResponse | null,
      incoming: TraceListResponse,
    ): TraceListResponse => {
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
    const range = lastFetchedRangeRef.current;
    if (!scopeParams || !range || !traceList?.traces?.length || isLoadingOlder) return;

    const oldest = traceList.traces.reduce((acc, trace) =>
      new Date(trace.startTime).getTime() < new Date(acc.startTime).getTime() ? trace : acc,
    );

    // Subtract 1 ms so the boundary trace is excluded from the next page,
    // preventing a wasted limit slot on an already-known trace.
    const exclusiveEndTime = new Date(
      new Date(oldest.startTime).getTime() - 1,
    ).toISOString();

    setIsLoadingOlder(true);
    try {
      const response = await getTraceList(
        // No limit cap — fetch all older traces in the window so none are dropped.
        { ...scopeParams, limit: undefined, ...range, endTime: exclusiveEndTime },
        getToken,
      );
      if ((response.traces?.length ?? 0) > 0) {
        setTraceList((prev) => mergeTraces(prev, response));
      }
    } finally {
      setIsLoadingOlder(false);
    }
  }, [scopeParams, traceList, isLoadingOlder, getToken, mergeTraces]);

  const loadNewer = useCallback(async () => {
    const range = lastFetchedRangeRef.current;
    if (!scopeParams || !range || !traceList?.traces?.length || isLoadingNewer) return;

    const newest = traceList.traces.reduce((acc, trace) =>
      new Date(trace.startTime).getTime() > new Date(acc.startTime).getTime() ? trace : acc,
    );

    // Add 1 ms so the boundary trace is excluded from the query,
    // preventing a wasted limit slot on an already-known trace.
    const exclusiveStartTime = new Date(
      new Date(newest.startTime).getTime() + 1,
    ).toISOString();

    setIsLoadingNewer(true);
    try {
      const response = await getTraceList(
        // No limit cap — fetch all newer traces so none are silently dropped.
        // Use current time as endTime so traces added since the last fetch are included.
        {
          ...scopeParams,
          limit: undefined,
          startTime: exclusiveStartTime,
          endTime: new Date().toISOString(),
        },
        getToken,
      );
      if ((response.traces?.length ?? 0) > 0) {
        setTraceList((prev) => mergeTraces(prev, response));
      }
    } finally {
      setIsLoadingNewer(false);
    }
  }, [scopeParams, traceList, isLoadingNewer, getToken, mergeTraces]);

  const fullLoad = useCallback(async () => {
    for (let i = 0; i < 50; i += 1) {
      await loadOlder();
    }
  }, [loadOlder]);

  // Stable refs so the interval always calls the latest versions without
  // being torn down and recreated on every render.
  const loadNewerRef = useRef(loadNewer);
  useEffect(() => { loadNewerRef.current = loadNewer; }, [loadNewer]);

  const refetchRef = useRef(queryResult.refetch);
  useEffect(() => { refetchRef.current = queryResult.refetch; }, [queryResult.refetch]);

  const traceListRef = useRef(traceList);
  useEffect(() => { traceListRef.current = traceList; }, [traceList]);

  // Auto-refresh: incrementally load newer traces every 30 s instead of
  // replacing the whole list. Falls back to a full refetch when the list is
  // empty (e.g. on initial load or after the user clears filters).
  useEffect(() => {
    if (hasCustomRange || !scopeParams) return;
    const timer = setInterval(() => {
      if (traceListRef.current?.traces?.length) {
        loadNewerRef.current();
      } else {
        refetchRef.current();
      }
    }, 30000);
    return () => clearInterval(timer);
  }, [hasCustomRange, scopeParams]);

  return {
    ...queryResult,
    data: traceList ?? queryResult.data,
    traceList: traceList ?? queryResult.data,
    loadOlder,
    loadNewer,
    fullLoad,
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
