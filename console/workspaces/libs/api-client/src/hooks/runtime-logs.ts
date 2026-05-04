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

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { filterAgentRuntimeLogs } from "../apis";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  type FilterAgentRuntimeLogsPathParams,
  type LogFilterRequest,
  type LogEntry,
  type TraceListTimeRange,
  getTimeRange,
} from "@agent-management-platform/types";
import { useApiQuery } from "./react-query-notifications";

// Extended type that includes timeRange for the hook input
export type LogFilterRequestWithTimeRange = LogFilterRequest & {
  timeRange?: TraceListTimeRange;
};

type UseAgentRuntimeLogsOptions = {
  enabled?: boolean;
  refetchInterval?: number | false;
  pageSize?: number;
};

export function useAgentRuntimeLogs(
  params: FilterAgentRuntimeLogsPathParams,
  body: LogFilterRequestWithTimeRange,
  options?: UseAgentRuntimeLogsOptions,
) {
  const { getToken } = useAuthHooks();
  const pageSize = options?.pageSize ?? 10;
  const refetchInterval = options?.refetchInterval ?? false;
  const hasCustomRange = !!body.startTime && !!body.endTime && !body.timeRange;

  const [allLogs, setAllLogs] = useState<LogEntry[]>([]);
  const [isLoadingOlder, setIsLoadingOlder] = useState(false);
  const [isLoadingNewer, setIsLoadingNewer] = useState(false);
  const [loadError, setLoadError] = useState<Error | null>(null);

  // Destructure path params as scalars so scopeParams stays stable even if the
  // caller passes an inline object for params.
  const { orgName, projName, agentName } = params;

  // Body without time fields — used in load callbacks where time is managed via
  // lastFetchedRangeRef. Memoized so load callbacks don't change when only the
  // time params change.
  const bodyWithoutTime = useMemo(() => {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const { timeRange: _, startTime: __, endTime: ___, ...rest } = body;
    return rest;
  }, [body]);

  // Non-time params — stable across refetches while org/project/filters don't change.
  const scopeParams = useMemo(() => {
    if (!orgName || !projName || !agentName || !bodyWithoutTime.environmentName)
      return undefined;
    return { orgName, projName, agentName, ...bodyWithoutTime, limit: pageSize };
  }, [orgName, projName, agentName, bodyWithoutTime, pageSize]);

  // Tracks the time range used in the most recent successful fetch so that
  // loadOlder / loadNewer paginate against the same window.
  const lastFetchedRangeRef = useRef<{
    startTime: string;
    endTime: string;
  } | null>(null);

  const queryResult = useApiQuery({
    queryKey: [
      "agent-runtime-logs",
      orgName,
      projName,
      agentName,
      bodyWithoutTime,
      pageSize,
      body.timeRange,
      body.startTime,
      body.endTime,
    ],
    queryFn: async () => {
      // Always compute the range at call-time so refetches use the current clock,
      // not a timestamp frozen when the component first mounted.
      const range = hasCustomRange
        ? { startTime: body.startTime!, endTime: body.endTime! }
        : getTimeRange(body.timeRange!)!;

      lastFetchedRangeRef.current = range;

      return filterAgentRuntimeLogs(
        params,
        { ...bodyWithoutTime, ...range, limit: pageSize },
        getToken,
      );
    },
    enabled:
      (options?.enabled ?? true) &&
      !!orgName &&
      !!projName &&
      !!agentName &&
      !!bodyWithoutTime.environmentName &&
      (hasCustomRange || !!body.timeRange),
    refetchInterval,
  });

  useEffect(() => {
    setAllLogs([]);
    lastFetchedRangeRef.current = null;
  }, [scopeParams, body.timeRange, body.startTime, body.endTime]);

  useEffect(() => {
    if (!queryResult.data?.logs) return;
    setAllLogs(queryResult.data.logs);
    // Restore the range ref when React Query serves from cache without re-running
    // queryFn (which is where the ref is normally set after a live fetch).
    if (!lastFetchedRangeRef.current) {
      lastFetchedRangeRef.current = hasCustomRange
        ? { startTime: body.startTime!, endTime: body.endTime! }
        : getTimeRange(body.timeRange!)! ?? null;
    }
  }, [queryResult.data, hasCustomRange, body.startTime, body.endTime, body.timeRange]);

  const mergeLogs = useCallback(
    (current: LogEntry[], incoming: LogEntry[]): LogEntry[] => {
      const map = new Map<string, LogEntry>();
      // Use timestamp + log as a composite key since timestamps are not guaranteed unique.
      for (const log of current) map.set(`${log.timestamp}:${log.log}`, log);
      for (const log of incoming) map.set(`${log.timestamp}:${log.log}`, log);
      return Array.from(map.values()).sort((a, b) => {
        const timeA = new Date(a.timestamp).getTime();
        const timeB = new Date(b.timestamp).getTime();
        return body.sortOrder === "asc" ? timeA - timeB : timeB - timeA;
      });
    },
    [body.sortOrder],
  );

  const loadOlder = useCallback(async () => {
    const range = lastFetchedRangeRef.current;
    if (!scopeParams || !range || !allLogs.length || isLoadingOlder) return;

    const oldest = allLogs.reduce((acc, log) =>
      new Date(log.timestamp).getTime() < new Date(acc.timestamp).getTime() ? log : acc,
    );

    setIsLoadingOlder(true);
    try {
      const response = await filterAgentRuntimeLogs(
        params,
        { ...bodyWithoutTime, ...range, endTime: oldest.timestamp, limit: pageSize },
        getToken,
      );
      if (response.logs?.length) {
        setAllLogs((prev) => mergeLogs(prev, response.logs));
      }
    } catch (err) {
      setLoadError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setIsLoadingOlder(false);
    }
  }, [scopeParams, allLogs, isLoadingOlder, params, bodyWithoutTime, pageSize, getToken, mergeLogs]);

  const loadNewer = useCallback(async () => {
    const range = lastFetchedRangeRef.current;
    if (!scopeParams || !range || !allLogs.length || isLoadingNewer) return;

    const newest = allLogs.reduce((acc, log) =>
      new Date(log.timestamp).getTime() > new Date(acc.timestamp).getTime() ? log : acc,
    );

    setIsLoadingNewer(true);
    try {
      const response = await filterAgentRuntimeLogs(
        params,
        {
          ...bodyWithoutTime,
          startTime: newest.timestamp,
          endTime: hasCustomRange ? range.endTime : new Date().toISOString(),
          limit: pageSize,
        },
        getToken,
      );
      if (response.logs?.length) {
        setAllLogs((prev) => mergeLogs(prev, response.logs));
      }
    } catch (err) {
      setLoadError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setIsLoadingNewer(false);
    }
  }, [scopeParams, allLogs, isLoadingNewer, hasCustomRange, params, bodyWithoutTime, pageSize, getToken, mergeLogs]);

  // Stable refs so the interval always calls the latest versions without
  // being torn down and recreated on every render.
  const loadNewerRef = useRef(loadNewer);
  useEffect(() => { loadNewerRef.current = loadNewer; }, [loadNewer]);

  const refetchRef = useRef(queryResult.refetch);
  useEffect(() => { refetchRef.current = queryResult.refetch; }, [queryResult.refetch]);

  const allLogsRef = useRef(allLogs);
  useEffect(() => { allLogsRef.current = allLogs; }, [allLogs]);

  // Auto-refresh: incrementally load newer logs every 30 s instead of replacing
  // the whole list. Falls back to a full refetch when the list is empty.
  // Skips when a custom range or a manual refetchInterval is configured.
  useEffect(() => {
    if (hasCustomRange || !scopeParams || refetchInterval !== false) return;
    const timer = setInterval(() => {
      if (allLogsRef.current?.length) {
        loadNewerRef.current();
      } else {
        refetchRef.current();
      }
    }, 30000);
    return () => clearInterval(timer);
  }, [hasCustomRange, scopeParams, refetchInterval]);

  return {
    isLoading: queryResult.isLoading,
    isRefetching: queryResult.isRefetching,
    error: queryResult.error,
    refetch: queryResult.refetch,
    data: queryResult.data,
    logs: allLogs,
    loadOlder,
    loadNewer,
    isLoadingOlder,
    isLoadingNewer,
    loadError,
  };
}
