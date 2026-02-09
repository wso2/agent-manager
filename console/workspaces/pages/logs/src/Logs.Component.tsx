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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { PageLayout } from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import {
  TraceListTimeRange,
} from "@agent-management-platform/types";
import { debounce } from "lodash";
import { useAgentRuntimeLogs } from "@agent-management-platform/api-client";
import { LogsView } from "./components/LogsView/LogsView";

const TIME_RANGE_OPTIONS = [
  { value: TraceListTimeRange.TEN_MINUTES, label: "10 Minutes" },
  { value: TraceListTimeRange.THIRTY_MINUTES, label: "30 Minutes" },
  { value: TraceListTimeRange.ONE_HOUR, label: "1 Hour" },
  { value: TraceListTimeRange.THREE_HOURS, label: "3 Hours" },
  { value: TraceListTimeRange.SIX_HOURS, label: "6 Hours" },
  { value: TraceListTimeRange.TWELVE_HOURS, label: "12 Hours" },
  { value: TraceListTimeRange.ONE_DAY, label: "1 Day" },
  { value: TraceListTimeRange.THREE_DAYS, label: "3 Days" },
  { value: TraceListTimeRange.SEVEN_DAYS, label: "7 Days" },
  { value: TraceListTimeRange.THIRTY_DAYS, label: "30 Days" },
];

const DEFAULT_PAGE_SIZE = 2000;
const DEBOUNCE_TIME = 2000;
type SortOrder = "asc" | "desc";

export const LogsComponent: React.FC = () => {
  const { agentId, orgId, projectId, envId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  const timeRange = useMemo(
    () =>
      (searchParams.get("timeRange") as TraceListTimeRange) ||
      TraceListTimeRange.SEVEN_DAYS,
    [searchParams],
  );

  const sortOrder = useMemo(
    () => (searchParams.get("sortOrder") as SortOrder) || "desc",
    [searchParams],
  );

  const search = useMemo(
    () => searchParams.get("search") || "",
    [searchParams],
  );
  const [searchPhrase, setSearchPhrase] = useState(search);
  const setDebouncedSearch = useMemo(
    () => debounce((searchValue: string) => setSearchPhrase(searchValue), DEBOUNCE_TIME),
    [setSearchPhrase],
  );

  useEffect(() => {
    setDebouncedSearch(search);
  }, [setDebouncedSearch, search]);

  const logFilterRequest = useMemo(
    () => ({
      environmentName: envId ?? "",
      timeRange: timeRange,
      sortOrder: sortOrder,
      searchPhrase, // API expects searchPhrase not search
    }),
    [envId, timeRange, sortOrder, searchPhrase],
  );

  const {
    logs,
    error,
    isLoading,
    isRefetching,
    refetch,
    hasMoreUp,
    hasMoreDown,
    isLoadingUp,
    isLoadingDown,
    loadUp,
    loadDown,
  } = useAgentRuntimeLogs(
    { agentName: agentId, orgName: orgId, projName: projectId },
    logFilterRequest,
    {
      refetchInterval: false,
      pageSize: DEFAULT_PAGE_SIZE,
    },
  );

  const handleRefresh = useCallback(() => {
    refetch();
  }, [refetch]);

  const handleSearch = useCallback(
    (searchValue: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("search", searchValue);
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const handleTimeRangeChange = useCallback(
    (newTimeRange: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("timeRange", newTimeRange as TraceListTimeRange);
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const handleSortOrderChange = useCallback(
    (newSortOrder: "asc" | "desc") => {
      const next = new URLSearchParams(searchParams);
      next.set("sortOrder", newSortOrder);
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  return (
    <PageLayout
      title="Runtime Logs"
      disableIcon
    >
      <LogsView
        logs={logs}
        isLoading={isLoading}
        error={error}
        hasMoreUp={hasMoreUp}
        hasMoreDown={hasMoreDown}
        isLoadingUp={isLoadingUp}
        isLoadingDown={isLoadingDown}
        onLoadUp={loadUp}
        onLoadDown={loadDown}
        onSearch={handleSearch}
        search={search}
        timeRange={timeRange}
        timeRangeOptions={TIME_RANGE_OPTIONS}
        onTimeRangeChange={handleTimeRangeChange}
        sortOrder={sortOrder}
        onSortOrderChange={handleSortOrderChange}
        onRefresh={handleRefresh}
        isRefreshing={isRefetching}
      />
    </PageLayout>
  );
};

export default LogsComponent;
