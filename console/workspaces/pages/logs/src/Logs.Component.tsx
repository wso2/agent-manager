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
import { LogsPanel, PageLayout, TimeRangeSelector, useTimeRangeParams } from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import {
  TraceListTimeRange,
  type LogLevel,
} from "@agent-management-platform/types";
import { debounce } from "lodash";
import { useAgentRuntimeLogs } from "@agent-management-platform/api-client";
import {
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

  const { customStartTime, customEndTime, hasCustomRange, handleCustomRangeApply, handleCustomRangeClear } =
    useTimeRangeParams(searchParams, setSearchParams);

  const timeRange = useMemo(
    () =>
      hasCustomRange
        ? undefined
        : (searchParams.get("timeRange") as TraceListTimeRange) || TraceListTimeRange.ONE_HOUR,
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
      startTime: hasCustomRange ? customStartTime : undefined,
      endTime: hasCustomRange ? customEndTime : undefined,
      sortOrder: sortOrder,
      searchPhrase,
      logLevels: selectedLogLevels.length > 0 ? selectedLogLevels : undefined,
    }),
    [envId, timeRange, hasCustomRange, customStartTime, customEndTime, sortOrder, searchPhrase, selectedLogLevels],
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

  return (
    <PageLayout
      title="Runtime Logs"
      disableIcon
      actions={
        <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
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
            onCustomRangeClear={handleCustomRangeClear}
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
