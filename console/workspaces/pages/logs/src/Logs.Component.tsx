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
import { FadeIn, PageLayout } from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import {
  TraceListTimeRange,
  getTimeRange,
} from "@agent-management-platform/types";
import {
  Button,
  CircularProgress,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Snackbar,
  Alert,
  Stack,
} from "@mui/material";
import { debounce } from "lodash";
import {
  Clock,
  Download,
  RefreshCcw,
  SortAsc,
  SortDesc,
} from "@wso2/oxygen-ui-icons-react";
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
  const [isExporting, setIsExporting] = useState(false);
  const [exportError, setExportError] = useState<string | null>(null);

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

  const handleSortToggle = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set("sortOrder", sortOrder === "desc" ? "asc" : "desc");
    setSearchParams(next);
  }, [searchParams, setSearchParams, sortOrder]);

  const handleSearch = useCallback(
    (searchValue: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("search", searchValue);
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const handleExport = useCallback(async () => {
    if (!orgId || !projectId || !agentId || !envId) {
      setExportError("Missing required parameters for export");
      return;
    }

    try {
      setExportError(null);
      setIsExporting(true);

      const range = getTimeRange(timeRange);
      const exportPayload = {
        timeRange,
        sortOrder,
        ...(range && { startTime: range.startTime, endTime: range.endTime }),
        exportedAt: new Date().toISOString(),
        orgId,
        projectId,
        agentId,
        envId,
        logs: [] as unknown[],
      };

      const blob = new Blob([JSON.stringify(exportPayload, null, 2)], {
        type: "application/json",
      });
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `logs-export-${new Date().toISOString().replace(/[:.]/g, "-")}.json`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (exportErr) {
      setExportError(
        exportErr instanceof Error
          ? exportErr.message
          : "Failed to export logs",
      );
    } finally {
      setIsExporting(false);
    }
  }, [orgId, projectId, agentId, envId, timeRange, sortOrder]);

  return (
    <FadeIn>
      <PageLayout
        title="Runtime Logs"
        actions={
          <Stack direction="row" gap={1} alignItems="center">
            <Select
              size="small"
              variant="outlined"
              value={timeRange}
              startAdornment={
                <InputAdornment position="start">
                  <Clock size={16} />
                </InputAdornment>
              }
              onChange={(e) => {
                const next = new URLSearchParams(searchParams);
                next.set("timeRange", e.target.value as TraceListTimeRange);
                setSearchParams(next);
              }}
            >
              {TIME_RANGE_OPTIONS.map((opt) => (
                <MenuItem key={opt.value} value={opt.value}>
                  {opt.label}
                </MenuItem>
              ))}
            </Select>
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
            <IconButton
              size="small"
              onClick={handleSortToggle}
              aria-label={
                sortOrder === "desc" ? "Sort ascending" : "Sort descending"
              }
            >
              {sortOrder === "desc" ? (
                <SortAsc size={16} />
              ) : (
                <SortDesc size={16} />
              )}
            </IconButton>
            <Button
              size="small"
              variant="outlined"
              startIcon={
                isExporting ? (
                  <CircularProgress size={16} />
                ) : (
                  <Download size={16} />
                )
              }
              disabled
              onClick={handleExport}
            >
              Export
            </Button>
          </Stack>
        }
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
        />
      </PageLayout>
      <Snackbar
        open={!!exportError}
        autoHideDuration={6000}
        onClose={() => setExportError(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert onClose={() => setExportError(null)} severity="error">
          {exportError}
        </Alert>
      </Snackbar>
    </FadeIn>
  );
};

export default LogsComponent;
