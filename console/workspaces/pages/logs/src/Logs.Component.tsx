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

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { EnvironmentSelector } from "@agent-management-platform/shared-component";
import { LogsPanel, PageLayout, TimeRangeSelector, useTimeRangeParams } from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import {
  TraceListTimeRange,
  type LogLevel,
} from "@agent-management-platform/types";
import { debounce } from "lodash";
import { useAgentRuntimeLogs, isObserverConfigured,
  ConsoleAction,
  useTrack,
} from "@agent-management-platform/api-client";
import {
  Alert,
  CircularProgress,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Stack,
  Checkbox,
  ListItemText,
} from "@wso2/oxygen-ui";
import {
  Filter,
  RefreshCcw,
  SortAsc,
  SortDesc,
} from "@wso2/oxygen-ui-icons-react";

const ALL_LOG_LEVELS: LogLevel[] = ["DEBUG", "INFO", "WARN", "ERROR"];

const TIME_RANGE_OPTIONS = [
  { value: TraceListTimeRange.TEN_MINUTES, label: "10 Minutes" },
  { value: TraceListTimeRange.THIRTY_MINUTES, label: "30 Minutes" },
  { value: TraceListTimeRange.ONE_HOUR, label: "1 Hour" },
  { value: TraceListTimeRange.SIX_HOURS, label: "6 Hours" },
  { value: TraceListTimeRange.TWELVE_HOURS, label: "12 Hours" },
  { value: TraceListTimeRange.ONE_DAY, label: "1 Day" },
  { value: TraceListTimeRange.SEVEN_DAYS, label: "7 Days" },
];

const DEFAULT_PAGE_SIZE = 300;
const DEBOUNCE_TIME = 2000;
type SortOrder = "asc" | "desc";

export const LogsComponent: React.FC = () => {
  const { agentId, orgId, projectId, envId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  const {
    customStartTime,
    customEndTime,
    hasCustomRange,
    handleCustomRangeApply,
  } = useTimeRangeParams(searchParams, setSearchParams);

  const timeRange = useMemo(
    () =>
      hasCustomRange
        ? undefined
        : (Object.values(TraceListTimeRange) as string[]).includes(
            searchParams.get("timeRange") ?? "",
          )
          ? (searchParams.get("timeRange") as TraceListTimeRange)
          : TraceListTimeRange.ONE_HOUR,
    [searchParams, hasCustomRange],
  );

  const sortOrder = useMemo(
    () => (searchParams.get("sortOrder") as SortOrder) || "desc",
    [searchParams],
  );

  const search = useMemo(
    () => searchParams.get("search") || "",
    [searchParams],
  );

  const selectedLogLevels = useMemo((): LogLevel[] => {
    const raw = searchParams.get("logLevels");
    if (!raw) return [];
    return raw.split(",").filter(Boolean) as LogLevel[];
  }, [searchParams]);

  const handleLogLevelChange = useCallback(
    (levels: LogLevel[]) => {
      const next = new URLSearchParams(searchParams);
      if (levels.length === 0) {
        next.delete("logLevels");
      } else {
        next.set("logLevels", levels.join(","));
      }
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );
  const { track } = useTrack();
  const [searchPhrase, setSearchPhrase] = useState(search);

  // One action per settled query rather than per keystroke: searchPhrase is
  // already debounced above, and the level filter changes discretely. The
  // phrase is never reported — users grep their own logs for their own data.
  // Keyed on a string rather than the array: selectedLogLevels is rebuilt by
  // useMemo on every searchParams change, so depending on it would re-report a
  // query when only the sort order or an unrelated param moved.
  const logLevelKey = selectedLogLevels.join(",");
  // The phrase itself goes into the key so that changing one search to another
  // (timeout → connection refused) counts as a new query. That is safe because
  // the key never leaves the browser: it is only compared against
  // lastReportedQuery below. What is reported is filter_count, which records
  // only whether a phrase is present.
  const logQueryKey = [
    hasCustomRange ? "custom" : (timeRange ?? "unspecified"),
    logLevelKey,
    searchPhrase,
  ].join("|");

  // Seeded with the query the page opens on, so the first effect run reports
  // nothing: arriving at Logs is a page view, not a query, and counting it as
  // one would make this metric a copy of page views. Seeding (rather than a
  // "first run" flag) also survives StrictMode's double-invoke in dev, where a
  // flag would let the second run through.
  const lastReportedQuery = useRef(logQueryKey);
  useEffect(() => {
    if (lastReportedQuery.current === logQueryKey) {
      return;
    }
    lastReportedQuery.current = logQueryKey;
    track(ConsoleAction.LogQuery, {
      time_range: hasCustomRange ? "custom" : (timeRange ?? "unspecified"),
      filter_count:
        (logLevelKey ? logLevelKey.split(",").length : 0) + (searchPhrase ? 1 : 0),
    });
  }, [track, timeRange, hasCustomRange, logLevelKey, searchPhrase, logQueryKey]);
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
      startTime: hasCustomRange ? customStartTime : undefined,
      endTime: hasCustomRange ? customEndTime : undefined,
      sortOrder: sortOrder,
      searchPhrase,
      logLevels: selectedLogLevels.length > 0 ? selectedLogLevels : undefined,
    }),
    [
      envId, timeRange, hasCustomRange, customStartTime, customEndTime, sortOrder, searchPhrase,
      selectedLogLevels,
    ],
  );

  const logParams = useMemo(
    () => ({ agentName: agentId, orgName: orgId, projName: projectId }),
    [agentId, orgId, projectId],
  );

  const {
    logs,
    error,
    isLoading,
    isRefetching,
    refetch,
    isLoadingOlder,
    isLoadingNewer,
    loadOlder,
    loadNewer,
    hasMoreOlder,
    hasMoreNewer,
  } = useAgentRuntimeLogs(
    logParams,
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
      next.delete("startTime");
      next.delete("endTime");
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

  // Mirror Traces.Component: without a configured observer the request fails
  // as an opaque "Failed to fetch" snackbar, so surface an actionable empty
  // state instead.
  if (!isObserverConfigured()) {
    return (
      <PageLayout title="Runtime Logs" disableIcon>
        <Alert severity="error" sx={{ mt: 2 }}>
          <strong>Observer not configured.</strong> Ask your platform
          administrator to set <code>AM_OBSERVER_PUBLIC_URL</code> on the
          agent-manager service.
        </Alert>
      </PageLayout>
    );
  }

  return (
    <PageLayout
      title="Runtime Logs"
      disableIcon
      actions={
        <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
          <EnvironmentSelector />
          {/* Log Level Filter */}
          <Select
            size="small"
            variant="outlined"
            multiple
            value={selectedLogLevels}
            onChange={(e) => handleLogLevelChange(e.target.value as LogLevel[])}
            displayEmpty
            renderValue={(selected) =>
              selected.length === 0 ? "All Levels" : (selected as LogLevel[]).join(", ")
            }
            startAdornment={
              <InputAdornment position="start">
                <Filter size={16} />
              </InputAdornment>
            }
            sx={{ minWidth: 150 }}
          >
            {ALL_LOG_LEVELS.map((level) => (
              <MenuItem key={level} value={level}>
                <Checkbox checked={selectedLogLevels.includes(level)} size="small" />
                <ListItemText primary={level} />
              </MenuItem>
            ))}
          </Select>

          <TimeRangeSelector
            preset={timeRange}
            customStart={customStartTime}
            customEnd={customEndTime}
            options={TIME_RANGE_OPTIONS}
            onPresetChange={handleTimeRangeChange}
            onCustomRangeApply={handleCustomRangeApply}
          />

          {/* Sort Toggle */}
          <IconButton
            size="small"
            onClick={() => handleSortOrderChange(sortOrder === "desc" ? "asc" : "desc")}
            aria-label={
              sortOrder === "desc" ? "Sort ascending" : "Sort descending"
            }
          >
            {sortOrder === "desc" ? (
              <SortDesc size={16} />
            ) : (
              <SortAsc size={16} />
            )}
          </IconButton>

          {/* Refresh Button */}
          <IconButton
            size="small"
            disabled={isRefetching}
            onClick={handleRefresh}
            aria-label="Refresh"
          >
            {isRefetching ? (
              <CircularProgress size={16} />
            ) : (
              <RefreshCcw size={16} />
            )}
          </IconButton>
        </Stack>
      }
    >
      <LogsPanel
        logs={logs}
        isLoading={isLoading}
        error={error}
        isLoadingUp={sortOrder === "asc" ? isLoadingNewer : isLoadingOlder}
        isLoadingDown={sortOrder === "asc" ? isLoadingOlder : isLoadingNewer}
        hasMoreUp={sortOrder === "asc" ? hasMoreNewer : hasMoreOlder}
        hasMoreDown={sortOrder === "asc" ? hasMoreOlder : hasMoreNewer}
        onLoadUp={sortOrder === "asc" ? loadNewer : loadOlder}
        onLoadDown={sortOrder === "asc" ? loadOlder : loadNewer}
        onSearch={handleSearch}
        search={search}
      />
    </PageLayout>
  );
};

export default LogsComponent;
