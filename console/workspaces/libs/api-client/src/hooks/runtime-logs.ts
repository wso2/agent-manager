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
import { useCallback, useEffect, useMemo, useState } from "react";
import { filterAgentRuntimeLogs } from "../apis";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  FilterAgentRuntimeLogsPathParams,
  LogFilterRequest,
  LogEntry,
  getTimeRange,
  TraceListTimeRange,
} from "@agent-management-platform/types";

// Extended type that includes timeRange for the hook input
export type LogFilterRequestWithTimeRange = LogFilterRequest & {
  timeRange?: TraceListTimeRange;
};

type UseAgentRuntimeLogsOptions = {
  enabled?: boolean;
  refetchInterval?: number | false; // Auto-refetch interval in milliseconds
  pageSize?: number; // Number of log lines to load per "page" for infinite scroll
};

export function useAgentRuntimeLogs(
  params: FilterAgentRuntimeLogsPathParams,
  body: LogFilterRequestWithTimeRange,
  options?: UseAgentRuntimeLogsOptions,
) {
  const { getToken } = useAuthHooks();
  
  const pageSize = options?.pageSize ?? 10;

  // Calculate startTime and endTime from timeRange if provided, and add limit
  const calculatedBody = useMemo(() => {
    let baseBody;
    if (body.timeRange) {
      const timeRangeResult = getTimeRange(body.timeRange);
      if (timeRangeResult) {
        // Remove timeRange from body before sending to API
        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        const { timeRange: _unused, ...restBody } = body;
        baseBody = {
          ...restBody,
          startTime: timeRangeResult.startTime,
          endTime: timeRangeResult.endTime,
        };
      }
    }
    // If timeRange not provided, use startTime/endTime directly
    if (!baseBody && 'timeRange' in body) {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { timeRange: _unused, ...restBody } = body;
      baseBody = restBody;
    }
    if (!baseBody) {
      baseBody = body;
    }
    // Add limit for initial fetch
    return {
      ...baseBody,
      limit: pageSize,
    };
  }, [body, pageSize]);

  // Initial fetch
  const queryResult = useQuery({
    queryKey: ["agent-runtime-logs", params, calculatedBody],
    queryFn: () => filterAgentRuntimeLogs(params, calculatedBody, getToken),
    enabled:
      (options?.enabled ?? true) &&
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!calculatedBody.environmentName &&
      !!calculatedBody.startTime &&
      !!calculatedBody.endTime,
    refetchInterval: options?.refetchInterval ?? false,
  });

  // Accumulated logs from all fetches (initial + loadUp + loadDown)
  const [allLogs, setAllLogs] = useState<LogEntry[]>([]);
  const [isLoadingUp, setIsLoadingUp] = useState(false);
  const [isLoadingDown, setIsLoadingDown] = useState(false);
  const [hasMoreUp, setHasMoreUp] = useState(true);
  const [hasMoreDown, setHasMoreDown] = useState(true);

  // Initialize allLogs with initial query data
  useEffect(() => {
    if (queryResult.data?.logs) {
      setAllLogs(queryResult.data.logs);
      // Track if we got fewer logs than requested (indicates end of data)
      setHasMoreUp(queryResult.data.logs.length >= pageSize);
      setHasMoreDown(queryResult.data.logs.length >= pageSize);
    }
  }, [queryResult.data?.logs, pageSize]);

  // Load older logs (scroll up)
  const loadUp = useCallback(async (beforeTimestamp: string) => {
    if (isLoadingUp) return;

    setIsLoadingUp(true);
    try {
      const fetchBody: LogFilterRequest = {
        ...calculatedBody,
        endTime: beforeTimestamp, // Fetch logs before the oldest visible log
        limit: pageSize,
        sortOrder: body.sortOrder || "desc",
      };

      const response = await filterAgentRuntimeLogs(params, fetchBody, getToken);
      
      if (response.logs && response.logs.length > 0) {
        // Merge new logs at the beginning, removing duplicates by timestamp
        setAllLogs((prev) => {
          const existingTimestamps = new Set(prev.map((log) => log.timestamp));
          const newLogs = response.logs.filter(
            (log) => !existingTimestamps.has(log.timestamp)
          );
          return [...newLogs, ...prev];
        });
        
        // Update hasMoreUp based on response size
        setHasMoreUp(response.logs.length >= pageSize);
      } else {
        setHasMoreUp(false);
      }
    } catch {
      // Error loading older logs - silently fail
    } finally {
      setIsLoadingUp(false);
    }
  }, [
    isLoadingUp,
    calculatedBody,
    pageSize,
    body.sortOrder,
    params,
    getToken,
  ]);

  // Load newer logs (scroll down)
  const loadDown = useCallback(async (afterTimestamp: string) => {
    if (isLoadingDown) return;

    setIsLoadingDown(true);
    try {
      const fetchBody: LogFilterRequest = {
        ...calculatedBody,
        startTime: afterTimestamp, // Fetch logs after the newest visible log
        limit: pageSize,
        sortOrder: body.sortOrder || "desc",
      };

      const response = await filterAgentRuntimeLogs(params, fetchBody, getToken);
      
      if (response.logs && response.logs.length > 0) {
        // Merge new logs at the end, removing duplicates by timestamp
        setAllLogs((prev) => {
          const existingTimestamps = new Set(prev.map((log) => log.timestamp));
          const newLogs = response.logs.filter(
            (log) => !existingTimestamps.has(log.timestamp)
          );
          return [...prev, ...newLogs];
        });
        
        // Update hasMoreDown based on response size
        setHasMoreDown(response.logs.length >= pageSize);
      } else {
        setHasMoreDown(false);
      }
    } catch {
      // Error loading newer logs - silently fail
    } finally {
      setIsLoadingDown(false);
    }
  }, [
    isLoadingDown,
    calculatedBody,
    pageSize,
    body.sortOrder,
    params,
    getToken,
  ]);

  return {
    // Original query result properties
    isLoading: queryResult.isLoading,
    isRefetching: queryResult.isRefetching,
    error: queryResult.error,
    refetch: queryResult.refetch,
    // Raw data from initial query
    data: queryResult.data,
    // All accumulated logs (from initial + loadUp + loadDown calls)
    logs: allLogs,
    // Infinite scroll controls
    hasMoreUp,
    hasMoreDown,
    isLoadingUp,
    isLoadingDown,
    loadUp,
    loadDown,
  };
}
