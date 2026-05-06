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
  getTimeRange
} from "@agent-management-platform/types";
import {
  getTraceList,
  exportTraces,
  getSpanDetail,
  listTraceSpans,
  type TraceObserverListParams,
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
): Promise<Map<string, { score?: number | null; totalCount: number; skippedCount: number }>> {
  try {
    const res = await getAgentTraceScores(
      {
        orgName, projName, agentName, startTime, endTime, limit, offset: 0,
        sortOrder: sortOrder as "asc" | "desc"
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

export type TraceListWithRange = TraceListResponse & {
  fetchedRange: { startTime: string; endTime: string };
};

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
  const [traceList, setTraceList] = useState<TraceListWithRange | null>(null);
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

      const [res, scoreMap] = await Promise.all([
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
      if (res.totalCount === 0) {
        return { traces: [], totalCount: 0, fetchedRange: range } as TraceListWithRange;
      }
      return { ...res, traces: applyScores(res.traces, scoreMap), fetchedRange: range };
    },
    enabled: !!scopeParams && (hasCustomRange || !!timeRange),
  });

  useEffect(() => {
    setTraceList(null);
    lastFetchedRangeRef.current = null;
  }, [scopeParams, timeRange, customStartTime, customEndTime]);

  useEffect(() => {
    if (!queryResult.data) return;
    setTraceList(queryResult.data);
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

  const [loadError, setLoadError] = useState<Error | null>(null);

  const loadOlder = useCallback(async () => {
    const range = lastFetchedRangeRef.current;
    if (!scopeParams || !range || !traceList?.traces?.length || isLoadingOlder) return;

    const oldest = traceList.traces.reduce((acc, trace) =>
      new Date(trace.startTime).getTime() < new Date(acc.startTime).getTime() ? trace : acc,
    );

    setIsLoadingOlder(true);
    try {
      const subRange = { ...range, endTime: oldest.startTime };
      const [response, scoreMap] = await Promise.all([
        getTraceList(
          // Use scopeParams.limit (= pageSize) as the per-call cap.
          // Use oldest.startTime as the boundary; mergeTraces deduplicates any overlap.
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
      setIsLoadingOlder(false);
    }
  }, [scopeParams, traceList, isLoadingOlder, getToken, mergeTraces]);

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

  const fullLoad = useCallback(async () => {
    const range = lastFetchedRangeRef.current;
    if (!scopeParams || !range || !traceList?.traces?.length) return;

    const findOldest = (traces: TraceListResponse["traces"]) =>
      traces.reduce((acc, trace) =>
        new Date(trace.startTime).getTime() < new Date(acc.startTime).getTime() ? trace : acc,
      );

    let localOldestCursor = findOldest(traceList.traces).startTime;

    for (let i = 0; i < 50; i += 1) {
      let response: TraceListResponse;
      try {
        // Use scopeParams.limit (= pageSize) as the per-call cap.
        response = await getTraceList(
          { ...scopeParams, ...range, endTime: localOldestCursor },
          getToken,
        );
      } catch (err) {
        setLoadError(err instanceof Error ? err : new Error(String(err)));
        break;
      }

      if (!response.traces?.length) break;

      const scoreMap = await fetchScoreMap(
        scopeParams.organization,
        scopeParams.project,
        scopeParams.component,
        range.startTime,
        localOldestCursor,
        scopeParams.limit,
        scopeParams.sortOrder ?? "desc",
        getToken,
      );
      const enriched = { ...response, traces: applyScores(response.traces, scoreMap) };
      setTraceList((prev) => mergeTraces(prev, enriched));
      const nextOldest = findOldest(response.traces).startTime;
      // Convergence: stop when the cursor didn't advance or page was smaller than
      // the page size (indicating the server has no more results).
      if (nextOldest === localOldestCursor || response.traces.length < pageSize) break;
      localOldestCursor = nextOldest;
    }
  }, [scopeParams, traceList, pageSize, getToken, mergeTraces]);

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
  TraceObserverListParams,
  "startTime" | "endTime" | "limit" | "sortOrder"
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
          sortOrder,
        },
        getToken,
      );
    },
  });
}
