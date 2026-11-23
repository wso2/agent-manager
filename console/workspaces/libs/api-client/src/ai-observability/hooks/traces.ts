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
  type TraceListResponse,
  type TraceListTimeRange,
  type GetTraceListPathParams,
  type TraceExportResponse,
  type TraceFilters,
  getTimeRange
} from "@agent-management-platform/types";
import {
  getTraceList,
  exportTraces,
  getSpanDetail,
  listTraceSpans,
  normalizeTraceFilters,
  type ObserverTraceListParams,
} from "../apis/traces";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "../../shared/react-query-notifications";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

/** Maximum spans fetched in a single listTraceSpans call.
 *  Increase this if traces routinely exceed 1000 spans, or implement
 *  cursor-based pagination to avoid silent truncation. */
const TRACE_SPANS_FETCH_LIMIT = 1000;

export type TraceListWithRange = TraceListResponse & {
  fetchedRange: { startTime: string; endTime: string };
  /** Where the next page's window ends (desc) or starts (asc); unset when the window ran out. */
  nextEdge?: string;
};

/** The next page's window edge: the page's last start time while the window holds more. */
function nextEdgeOf(res: TraceListResponse): string | undefined {
  const shown = res.traces?.length ?? 0;
  return shown > 0 && shown < res.totalCount ? res.lookedBackTo : undefined;
}

export interface TraceListOptions {
  enableAutoRefresh?: boolean;
  enabled?: boolean;
  filters?: TraceFilters;
  /** Fill models on every trace; costs the server one extra upstream call per trace. */
  includeModels?: boolean;
  /** Turns off focus and reconnect refetches, which would drop pages loaded with loadMore. */
  paged?: boolean;
}

/** Trace list for the window and filters, paged by narrowing the window, with polling. */
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
  options?: TraceListOptions,
) {
  const { getToken } = useAuthHooks();
  const hasCustomRange = !!customStartTime && !!customEndTime;
  const pageSize = limit ?? 10;
  const [traceList, setTraceList] = useState<TraceListWithRange | null>(null);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [isLoadingNewer, setIsLoadingNewer] = useState(false);
  const [loadError, setLoadError] = useState<Error | null>(null);

  // Keyed by value so a caller passing a fresh object each render doesn't reset the list.
  const filtersKey = JSON.stringify(normalizeTraceFilters(options?.filters));
  const filters = useMemo<TraceFilters>(() => JSON.parse(filtersKey), [filtersKey]);
  const includeModels = options?.includeModels === true;

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
      filters,
      includeModels,
    };
  }, [
    organization, project, component, environment, pageSize, sortOrder, filters, includeModels,
  ]);

  // Tracks the time range used in the most recent successful fetch so that
  // loadMore / loadNewer paginate against the same window.
  const lastFetchedRangeRef = useRef<{
    startTime: string;
    endTime: string;
  } | null>(null);

  // The next page's window edge; loadMore moves the window's far end to it.
  const nextEdgeRef = useRef<string | undefined>(undefined);

  // Bumped on each reset and new first page; a loadMore from an earlier list leaves state alone.
  const listGenerationRef = useRef(0);

  // Latest list, read by the auto-refresh interval and by loadMore to tell new rows.
  const traceListRef = useRef(traceList);
  useEffect(() => { traceListRef.current = traceList; }, [traceList]);

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
      filters,
      includeModels,
    ],
    queryFn: async (): Promise<TraceListWithRange> => {
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
      return { ...res, traces: res.traces ?? [], fetchedRange: range, nextEdge: nextEdgeOf(res) };
    },
    enabled: (options?.enabled ?? true) && !!scopeParams && (hasCustomRange || !!timeRange),
    // A focus or reconnect refetch replaces the list and drops pages loaded with loadMore.
    ...(options?.paged && { refetchOnWindowFocus: false, refetchOnReconnect: false }),
  });

  useEffect(() => {
    listGenerationRef.current += 1;
    setTraceList(null);
    setLoadError(null);
    setIsLoadingMore(false);
    lastFetchedRangeRef.current = null;
    nextEdgeRef.current = undefined;
  }, [scopeParams, timeRange, customStartTime, customEndTime]);

  useEffect(() => {
    if (!queryResult.data) return;
    listGenerationRef.current += 1;
    setTraceList(queryResult.data);
    setLoadError(null);
    setIsLoadingMore(false);
    nextEdgeRef.current = queryResult.data.nextEdge;
    // Restore the range ref when React Query serves from cache without re-running
    // queryFn (which is where the ref is normally set after a live fetch).
    // Use the concrete window embedded in the result instead of recomputing from
    // the preset, which drifts for relative time ranges.
    if (!lastFetchedRangeRef.current) {
      lastFetchedRangeRef.current = queryResult.data.fetchedRange;
    }
  }, [queryResult.data]);

  const mergeTraces = useCallback(
    (
      current: TraceListWithRange | null,
      incoming: TraceListResponse,
    ): TraceListWithRange | null => {
      if (!current) return null;
      const map = new Map<string, TraceListResponse["traces"][number]>();
      for (const trace of current?.traces ?? []) map.set(trace.traceId, trace);
      // Incoming traces win on all fields.
      for (const trace of incoming.traces ?? []) map.set(trace.traceId, trace);

      const traces = Array.from(map.values()).sort((a, b) => {
        const timeA = new Date(a.startTime).getTime();
        const timeB = new Date(b.startTime).getTime();
        return sortOrder === "asc" ? timeA - timeB : timeB - timeA;
      });
      return {
        ...current,
        traces,
        totalCount: Math.max(current?.totalCount ?? 0, incoming.totalCount ?? 0),
      };
    },
    [sortOrder],
  );

  // Fetches the page past `edge`: the first page's window with its far end moved to edge.
  // Returns whether the page added rows, or undefined if the list moved on.
  const fetchNextPage = useCallback(async (edge: string) => {
    const range = lastFetchedRangeRef.current;
    if (!scopeParams || !range) return undefined;

    const pageRange = sortOrder === "asc"
      ? { startTime: edge, endTime: range.endTime }
      : { startTime: range.startTime, endTime: edge };
    const response = await getTraceList({ ...scopeParams, ...pageRange }, getToken);
    // A refetch, scope change or concurrent call moved the edge while this page was in flight.
    if (nextEdgeRef.current !== edge) return undefined;

    const traces = response.traces ?? [];
    const shown = new Set(traceListRef.current?.traces.map((t) => t.traceId));
    const added = traces.some((t) => !shown.has(t.traceId));
    // The window bounds are inclusive, so the edge trace comes back; a page of only
    // traces already shown would ask for the same edge again, so paging stops.
    const next = nextEdgeOf(response);
    nextEdgeRef.current = added && next !== edge ? next : undefined;
    setTraceList((prev) => {
      const merged = mergeTraces(prev, { ...response, traces });
      return merged && {
        ...merged,
        nextEdge: nextEdgeRef.current,
        truncated: response.truncated,
        lookedBackTo: response.lookedBackTo,
      };
    });
    return added;
  }, [scopeParams, sortOrder, getToken, mergeTraces]);

  /** Loads the next page; resolves to whether it added rows, or undefined if the list moved on. */
  const loadMore = useCallback(async () => {
    const edge = nextEdgeRef.current;
    if (!edge || isLoadingMore) return;

    const generation = listGenerationRef.current;
    setLoadError(null);
    setIsLoadingMore(true);
    try {
      const added = await fetchNextPage(edge);
      if (generation === listGenerationRef.current) return added;
    } catch (err) {
      if (generation === listGenerationRef.current) {
        setLoadError(err instanceof Error ? err : new Error(String(err)));
      }
    } finally {
      if (generation === listGenerationRef.current) setIsLoadingMore(false);
    }
  }, [isLoadingMore, fetchNextPage]);

  // Used only by auto-refresh; Refresh is how the traces page shows new traces.
  const loadNewer = useCallback(async () => {
    const range = lastFetchedRangeRef.current;
    if (!scopeParams || !range || !traceList?.traces?.length || isLoadingNewer) return;

    const newest = traceList.traces.reduce((acc, trace) =>
      new Date(trace.startTime).getTime() > new Date(acc.startTime).getTime() ? trace : acc,
    );

    setIsLoadingNewer(true);
    try {
      const subRange = {
        startTime: newest.startTime,
        endTime: hasCustomRange ? range.endTime : new Date().toISOString(),
      };
      // Use scopeParams.limit (= pageSize) as the per-call cap.
      // Use newest.startTime as the boundary; mergeTraces deduplicates any overlap.
      // Respect the custom range upper bound; for live ranges use the current clock.
      const response = await getTraceList({ ...scopeParams, ...subRange }, getToken);
      if ((response.traces?.length ?? 0) > 0) {
        setTraceList((prev) => mergeTraces(prev, response));
      }
    } catch (err) {
      setLoadError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setIsLoadingNewer(false);
    }
  }, [scopeParams, traceList, isLoadingNewer, hasCustomRange, getToken, mergeTraces]);

  // Stable refs so the interval always calls the latest versions without
  // being torn down and recreated on every render.
  const loadNewerRef = useRef(loadNewer);
  useEffect(() => { loadNewerRef.current = loadNewer; }, [loadNewer]);

  const refetchRef = useRef(queryResult.refetch);
  useEffect(() => { refetchRef.current = queryResult.refetch; }, [queryResult.refetch]);

  // Auto-refresh: incrementally load newer traces every 30 s instead of
  // replacing the whole list. Falls back to a full refetch when the list is
  // empty (e.g. on initial load or after the user clears filters).
  useEffect(() => {
    if (hasCustomRange || !scopeParams || options?.enabled === false
      || !options?.enableAutoRefresh) return;
    const timer = setInterval(() => {
      if (traceListRef.current?.traces?.length) {
        loadNewerRef.current();
      } else {
        refetchRef.current();
      }
    }, 30000);
    return () => clearInterval(timer);
  }, [hasCustomRange, scopeParams, options?.enabled, options?.enableAutoRefresh]);

  const current = traceList ?? queryResult.data;

  return {
    ...queryResult,
    data: current,
    traceList: current,
    loadMore,
    hasMore: !!current?.nextEdge,
    truncated: current?.truncated ?? false,
    lookedBackTo: current?.lookedBackTo,
    isLoadingMore,
    loadError,
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
  const query = useApiQuery({
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
          limit: TRACE_SPANS_FETCH_LIMIT,
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
  const isTruncated =
    !!query.data &&
    (query.data.totalCount ?? 0) > (query.data.spans?.length ?? 0);
  return { ...query, isTruncated };
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
  ObserverTraceListParams,
  "startTime" | "endTime" | "limit" | "sortOrder" | "filters"
> & {
  organization: string;
  project: string;
  component: string;
  environment: string;
};

/** Mutation that exports traces for the current window and filters. */
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
        sortOrder,
        filters,
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
          sortOrder,
          filters,
        },
        getToken,
      );
    },
  });
}
