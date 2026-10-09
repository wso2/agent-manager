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
  type TraceInclude,
  getTimeRange
} from "@agent-management-platform/types";
import {
  getTraceList,
  exportTraces,
  getSpanDetail,
  listTraceSpans,
  normalizeTraceFilters,
  normalizeTraceInclude,
  type ObserverTraceListParams,
} from "../apis/traces";
import { getAgentTraceScores } from "../apis/monitors";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

/** Maximum spans fetched in a single listTraceSpans call.
 *  Increase this if traces routinely exceed 1000 spans, or implement
 *  cursor-based pagination to avoid silent truncation. */
const TRACE_SPANS_FETCH_LIMIT = 1000;

/** Merge scores from the scores endpoint into each trace item. */
function applyScores(
  traces: TraceListResponse["traces"],
  scoreMap: Map<string, { score?: number | null; totalCount: number; skippedCount: number }>,
): TraceListResponse["traces"] {
  return traces.map((t) => {
    const s = scoreMap.get(t.traceId);
    return s !== undefined ? { ...t, score: s } : t;
  });
}

/** Fetch scores for a time window and return as a map keyed by traceId. */
async function fetchScoreMap(
  orgName: string,
  projName: string,
  agentName: string,
  startTime: string,
  endTime: string,
  limit: number,
  sortOrder: string,
  getToken: (() => Promise<string>) | undefined,
  offset = 0,
  traceIds?: string[],
  evaluator?: string,
): Promise<Map<string, { score?: number | null; totalCount: number; skippedCount: number }>> {
  try {
    const res = await getAgentTraceScores(
      {
        orgName, projName, agentName, startTime, endTime, limit, offset,
        sortOrder: sortOrder as "asc" | "desc", traceIds, evaluator,
      },
      getToken,
    );
    const map = new Map<string,
      {
        score?: number | null;
        totalCount: number;
        skippedCount: number
      }>();
    for (const t of res.traces ?? []) {
      map.set(t.traceId, {
        score: t.score, totalCount: t.totalCount, skippedCount: t.skippedCount,
      });
    }
    return map;
  } catch (err) {
    // eslint-disable-next-line no-console
    console.warn("fetchScoreMap failed", { orgName, projName, agentName }, err);
    return new Map();
  }
}

/** Highest limit, and most traceIds, the scores endpoint accepts. */
const MAX_SCORES_PER_REQUEST = 100;

/** Widens a page's score window to absorb timestamp precision differences. */
const SCORE_WINDOW_PAD_MS = 1000;

/** Fetch a page's scores by trace ID, 100 per call; with filters.evaluator, that evaluator's. */
async function fetchTraceIdScoreMap(
  scope: {
    organization: string;
    project: string;
    component: string;
    sortOrder?: string;
    filters?: TraceFilters;
  },
  traces: TraceListResponse["traces"] | undefined,
  getToken: (() => Promise<string>) | undefined,
): ReturnType<typeof fetchScoreMap> {
  const all = traces ?? [];
  const chunks: TraceListResponse["traces"][] = [];
  for (let i = 0; i < all.length; i += MAX_SCORES_PER_REQUEST) {
    chunks.push(all.slice(i, i + MAX_SCORES_PER_REQUEST));
  }
  const maps = await Promise.all(chunks.map((chunk) => {
    // The endpoint still takes a window, so bound each call by its traces' start times.
    const times = chunk.map((t) => new Date(t.startTime).getTime());
    return fetchScoreMap(
      scope.organization,
      scope.project,
      scope.component,
      new Date(Math.min(...times) - SCORE_WINDOW_PAD_MS).toISOString(),
      new Date(Math.max(...times) + SCORE_WINDOW_PAD_MS).toISOString(),
      MAX_SCORES_PER_REQUEST,
      scope.sortOrder ?? "desc",
      getToken,
      0,
      chunk.map((t) => t.traceId),
      scope.filters?.evaluator,
    );
  }));
  return new Map(maps.flatMap((map) => [...map]));
}

export type TraceListWithRange = TraceListResponse & {
  fetchedRange: { startTime: string; endTime: string };
};

export interface TraceListOptions {
  enableAutoRefresh?: boolean;
  enabled?: boolean;
  filters?: TraceFilters;
  /** Fill models on every trace; costs the server one extra upstream call per trace. */
  includeModels?: boolean;
  /** Optional fields to fill on every trace, sent with includeModels as one include param. */
  include?: TraceInclude[];
  /** Turns off focus and reconnect refetches, which would drop pages loaded with loadMore. */
  paged?: boolean;
}

/** Trace list for the window and filters, with cursor paging, newer-trace polling and scores. */
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
  const hasFilters = filtersKey !== "{}";
  const includeModels = options?.includeModels === true;
  const includeKey = normalizeTraceInclude(options?.include).join(",");
  const include = useMemo(
    () => (includeKey ? (includeKey.split(",") as TraceInclude[]) : undefined),
    [includeKey],
  );

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
      include,
    };
  }, [
    organization, project, component, environment, pageSize, sortOrder, filters, includeModels,
    include,
  ]);

  // Tracks the time range used in the most recent successful fetch so that
  // loadMore / loadNewer paginate against the same window.
  const lastFetchedRangeRef = useRef<{
    startTime: string;
    endTime: string;
  } | null>(null);

  // Server cursor for the next page in sort order; loadMore sends it with the unchanged window.
  const nextCursorRef = useRef<string | undefined>(undefined);

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
      includeKey,
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

      let res: TraceListResponse;
      let scoreMap: Awaited<ReturnType<typeof fetchScoreMap>>;
      if (hasFilters) {
        // Matches can sit anywhere in the window, so score the returned page itself.
        res = await getTraceList({ ...scopeParams, ...range }, getToken);
        scoreMap = await fetchTraceIdScoreMap(scopeParams, res.traces, getToken);
      } else {
        [res, scoreMap] = await Promise.all([
          getTraceList({ ...scopeParams, ...range }, getToken),
          fetchScoreMap(
            scopeParams.organization,
            scopeParams.project,
            scopeParams.component,
            range.startTime,
            range.endTime,
            // Use the same page size as the trace list; scores now support sortOrder
            // so their result set aligns with the current page.
            scopeParams.limit,
            scopeParams.sortOrder ?? "desc",
            getToken,
          ),
        ]);
      }
      // A filtered page can be empty yet still carry nextCursor and truncated.
      if (res.totalCount === 0) {
        return { ...res, traces: [], fetchedRange: range } as TraceListWithRange;
      }
      return { ...res, traces: applyScores(res.traces, scoreMap), fetchedRange: range };
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
    nextCursorRef.current = undefined;
  }, [scopeParams, timeRange, customStartTime, customEndTime]);

  useEffect(() => {
    if (!queryResult.data) return;
    listGenerationRef.current += 1;
    setTraceList(queryResult.data);
    setLoadError(null);
    setIsLoadingMore(false);
    nextCursorRef.current = queryResult.data.nextCursor;
    // Restore the range ref when React Query serves from cache without re-running
    // queryFn (which is where the ref is normally set after a live fetch).
    // Use the concrete window embedded in the result instead of recomputing from
    // the preset, which drifts for relative time ranges.
    if (!lastFetchedRangeRef.current) {
      lastFetchedRangeRef.current = (queryResult.data as TraceListWithRange).fetchedRange;
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
      // Incoming traces win on all fields; preserve existing score if incoming has none.
      for (const trace of incoming.traces ?? []) {
        const existing = map.get(trace.traceId);
        map.set(trace.traceId, {
          ...trace,
          score: trace.score ?? existing?.score,
        });
      }

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

  // Fetches the page after `cursor` over the first page's unchanged window and merges it in.
  // Returns the next cursor and whether the page added rows, or undefined if the list moved on.
  const fetchCursorPage = useCallback(async (cursor: string) => {
    const range = lastFetchedRangeRef.current;
    if (!scopeParams || !range) return undefined;

    const response = await getTraceList({ ...scopeParams, ...range, cursor }, getToken);
    const scoreMap = await fetchTraceIdScoreMap(scopeParams, response.traces, getToken);
    // A refetch, scope change or concurrent call moved the cursor while this page was in flight.
    if (nextCursorRef.current !== cursor) return undefined;

    nextCursorRef.current = response.nextCursor;
    const traces = applyScores(response.traces ?? [], scoreMap);
    const shown = new Set(traceListRef.current?.traces.map((t) => t.traceId));
    const added = traces.some((t) => !shown.has(t.traceId));
    setTraceList((prev) => {
      const merged = mergeTraces(prev, { ...response, traces });
      return merged && {
        ...merged,
        nextCursor: response.nextCursor,
        truncated: response.truncated,
        lookedBackTo: response.lookedBackTo,
      };
    });
    return { nextCursor: response.nextCursor, added };
  }, [scopeParams, getToken, mergeTraces]);

  /** Loads the next page; resolves to whether it added rows, or undefined if the list moved on. */
  const loadMore = useCallback(async () => {
    const cursor = nextCursorRef.current;
    if (!cursor || isLoadingMore) return;

    const generation = listGenerationRef.current;
    setLoadError(null);
    setIsLoadingMore(true);
    try {
      const page = await fetchCursorPage(cursor);
      if (generation === listGenerationRef.current) return page?.added;
    } catch (err) {
      if (generation === listGenerationRef.current) {
        setLoadError(err instanceof Error ? err : new Error(String(err)));
      }
    } finally {
      if (generation === listGenerationRef.current) setIsLoadingMore(false);
    }
  }, [isLoadingMore, fetchCursorPage]);

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
      const [response, scoreMap] = await Promise.all([
        getTraceList(
          // Use scopeParams.limit (= pageSize) as the per-call cap.
          // Use newest.startTime as the boundary; mergeTraces deduplicates any overlap.
          // Respect the custom range upper bound; for live ranges use the current clock.
          { ...scopeParams, ...subRange },
          getToken,
        ),
        fetchScoreMap(
          scopeParams.organization,
          scopeParams.project,
          scopeParams.component,
          subRange.startTime,
          subRange.endTime,
          // Use the same page size as the trace list; scores now support sortOrder
          // so their result set aligns with the current page.
          scopeParams.limit,
          scopeParams.sortOrder ?? "desc",
          getToken,
        ),
      ]);
      if ((response.traces?.length ?? 0) > 0) {
        const enriched = { ...response, traces: applyScores(response.traces, scoreMap) };
        setTraceList((prev) => mergeTraces(prev, enriched));
      }
    } catch (err) {
      setLoadError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setIsLoadingNewer(false);
    }
  }, [scopeParams, traceList, isLoadingNewer, hasCustomRange, getToken, mergeTraces]);

  // Walks cursor pages until the window runs out, capped at 50 pages.
  const fullLoad = useCallback(async () => {
    let cursor = nextCursorRef.current;
    for (let i = 0; i < 50 && cursor; i += 1) {
      try {
        cursor = (await fetchCursorPage(cursor))?.nextCursor;
      } catch (err) {
        setLoadError(err instanceof Error ? err : new Error(String(err)));
        break;
      }
    }
  }, [fetchCursorPage]);

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
    fullLoad,
    hasMore: !!current?.nextCursor,
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
