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

import React, { useCallback, useMemo, useState } from "react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  PageLayout,
} from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import {
  GetTraceListPathParams,
  TraceListTimeRange,
  TraceScoreSummary,
  getTimeRange,
} from "@agent-management-platform/types";
// import {
//   Snackbar,
//   Alert,
//   Button,
//   CircularProgress,
//   IconButton,
//   InputAdornment,
//   MenuItem,
//   Select,
//   Stack,
// } from "@mui/material";
import {
  Workflow,
  Clock,
  RefreshCcw,
  SortAsc,
  SortDesc,
  Download,
} from "@wso2/oxygen-ui-icons-react";
import { useTraceList, useExportTraces, useAgentTraceScores } from "@agent-management-platform/api-client";
import { TraceDetails, TracesView } from "./subComponents";
import { Alert, Button, CircularProgress, IconButton, InputAdornment, MenuItem, Select, Snackbar, Stack, Typography } from "@wso2/oxygen-ui";

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

export const TracesComponent: React.FC = () => {
  const { agentId, orgId, projectId, envId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const { mutateAsync: exportTracesAsync, isPending: isExporting } = useExportTraces();
  const [exportError, setExportError] = useState<string | null>(null);

  // Initialize state from URL search params with defaults.
  // Validate that both timestamps are parseable and start <= end before
  // activating custom-range mode, so malformed or inverted URLs are ignored.
  const [customStartTime, customEndTime, hasCustomRange] = useMemo((): [
    string | undefined,
    string | undefined,
    boolean,
  ] => {
    const startRaw = searchParams.get("startTime") || undefined;
    const endRaw = searchParams.get("endTime") || undefined;
    if (!startRaw || !endRaw) return [undefined, undefined, false];
    const startMs = Date.parse(startRaw);
    const endMs = Date.parse(endRaw);
    if (isNaN(startMs) || isNaN(endMs) || startMs > endMs) return [undefined, undefined, false];
    return [startRaw, endRaw, true];
  }, [searchParams]);

  const timeRange = useMemo(
    () =>
      hasCustomRange
        ? undefined
        : (searchParams.get("timeRange") as TraceListTimeRange) ||
          TraceListTimeRange.SEVEN_DAYS,
    [searchParams, hasCustomRange]
  );

  const limit = useMemo(
    () => parseInt(searchParams.get("limit") || "10", 10),
    [searchParams]
  );

  const offset = useMemo(
    () => parseInt(searchParams.get("offset") || "0", 10),
    [searchParams]
  );

  const sortOrder = useMemo(
    () =>
      (searchParams.get("sortOrder") as GetTraceListPathParams["sortOrder"]) ||
      "desc",
    [searchParams]
  );
  const {
    data: traceData,
    isLoading,
    refetch,
    isRefetching,
  } = useTraceList(
    orgId,
    projectId,
    agentId,
    envId,
    timeRange,
    limit,
    offset,
    sortOrder,
    customStartTime,
    customEndTime,
  );

  // Fetch aggregated scores for all traces in the current time range.
  // The scores endpoint only returns traces that have scores, so its pagination
  // doesn't align with the traces endpoint (different result sets, different sort).
  // We fetch up to the total trace count (capped at backend max of 100) to ensure
  // scores are available for all visible traces regardless of the current page.
  // TODO: implement filtering scores by trace IDs for exact alignment.
  const resolvedTimeRange = useMemo(
    () =>
      hasCustomRange
        ? { startTime: customStartTime!, endTime: customEndTime! }
        : timeRange
          ? getTimeRange(timeRange)
          : undefined,
    [hasCustomRange, customStartTime, customEndTime, timeRange],
  );
  const scoresLimit = useMemo(
    () => Math.min(traceData?.totalCount || 100, 100),
    [traceData?.totalCount],
  );
  const { data: scoresData, isLoading: isScoresLoading } = useAgentTraceScores({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
    startTime: resolvedTimeRange?.startTime,
    endTime: resolvedTimeRange?.endTime,
    limit: scoresLimit,
    offset: 0,
  });

  const scoreMap = useMemo(() => {
    const map = new Map<string, TraceScoreSummary>();
    if (scoresData?.traces) {
      for (const t of scoresData.traces) {
        map.set(t.traceId, t);
      }
    }
    return map;
  }, [scoresData]);

  const selectedTrace = useMemo(
    () => searchParams.get("selectedTrace"),
    [searchParams]
  );

  const handleTraceSelect = useCallback(
    (traceId: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("selectedTrace", traceId);
      setSearchParams(next);
    },
    [searchParams, setSearchParams]
  );

  // Convert limit/offset to page/rowsPerPage for TablePagination
  const page = useMemo(() => Math.floor(offset / limit), [offset, limit]);
  const rowsPerPage = useMemo(() => limit, [limit]);
  const count = useMemo(
    () => traceData?.totalCount ?? 0,
    [traceData?.totalCount]
  );

  const handlePageChange = useCallback(
    (newPage: number) => {
      const next = new URLSearchParams(searchParams);
      next.set("offset", String(newPage * rowsPerPage));
      setSearchParams(next);
    },
    [rowsPerPage, searchParams, setSearchParams]
  );

  const handleRowsPerPageChange = useCallback(
    (newRowsPerPage: number) => {
      const next = new URLSearchParams(searchParams);
      next.set("limit", String(newRowsPerPage));
      next.set("offset", "0"); // Reset to first page when changing rows per page
      setSearchParams(next);
    },
    [searchParams, setSearchParams]
  );

  const handleExportTraces = useCallback(async () => {
    if (!orgId || !projectId || !agentId || !envId) {
      setExportError("Missing required parameters for export");
      return;
    }

    try {
      setExportError(null);

      const range = hasCustomRange
        ? { startTime: customStartTime!, endTime: customEndTime! }
        : timeRange
          ? getTimeRange(timeRange)
          : null;
      if (!range) {
        setExportError("Invalid time range");
        return;
      }
      const { startTime, endTime } = range;

      const exportData = await exportTracesAsync({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
        environment: envId,
        startTime,
        endTime,
        sortOrder,
        limit,
        offset,
      });

      // Create a blob from the JSON data
      const blob = new Blob([JSON.stringify(exportData, null, 2)], {
        type: "application/json",
      });

      // Create download link
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `traces-export-${new Date().toISOString().replace(/[:.]/g, "-")}.json`;
      document.body.appendChild(link);
      link.click();

      // Cleanup
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error("Export failed:", error);
      setExportError(
        error instanceof Error ? error.message : "Failed to export traces"
      );
    }
  }, [
    orgId, projectId, agentId, envId, timeRange, sortOrder, limit, offset,
    exportTracesAsync, hasCustomRange, customStartTime, customEndTime,
  ]);

  const handleTimeRangeChange = useCallback(
    (newTimeRange: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("timeRange", newTimeRange as TraceListTimeRange);
      // Clear custom range when switching to a preset
      next.delete("startTime");
      next.delete("endTime");
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const customRangeLabel = useMemo(() => {
    if (!hasCustomRange) return null;
    const fmt = (iso: string) =>
      new Date(iso).toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    return `${fmt(customStartTime!)} – ${fmt(customEndTime!)}`;
  }, [hasCustomRange, customStartTime, customEndTime]);

  const handleSortOrderChange = useCallback(
    (newSortOrder: "asc" | "desc") => {
      const next = new URLSearchParams(searchParams);
      next.set("sortOrder", newSortOrder);
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const handleRefresh = useCallback(() => {
    refetch();
  }, [refetch]);

  return (
    <>
      <PageLayout
        title="Traces"
        disableIcon
        actions={
          <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
            {/* Time Range Selector */}
            {hasCustomRange ? (
              <Stack direction="row" spacing={0.5} alignItems="center">
                <Clock size={16} />
                <Typography variant="caption" color="text.secondary" noWrap>
                  {customRangeLabel}
                </Typography>
              </Stack>
            ) : (
              <Select
                size="small"
                variant="outlined"
                value={timeRange}
                onChange={(e) => handleTimeRangeChange(e.target.value)}
                startAdornment={
                  <InputAdornment position="start">
                    <Clock size={16} />
                  </InputAdornment>
                }
                sx={{ minWidth: 150 }}
              >
                {TIME_RANGE_OPTIONS.map((opt) => (
                  <MenuItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </MenuItem>
                ))}
              </Select>
            )}

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

            {/* Export Button */}
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
              onClick={handleExportTraces}
              disabled={isExporting || isLoading || (traceData?.traces ?? []).length === 0}
            >
              Export
            </Button>
          </Stack>
        }
      >
        <TracesView
          traces={traceData?.traces ?? []}
          count={count}
          page={page}
          rowsPerPage={rowsPerPage}
          isLoading={isLoading}
          selectedTrace={selectedTrace}
          scoreMap={scoreMap}
          isScoresLoading={isScoresLoading}
          onTraceSelect={handleTraceSelect}
          onPageChange={handlePageChange}
          onRowsPerPageChange={handleRowsPerPageChange}
        />
        <DrawerWrapper
          open={!!selectedTrace}
          disableScroll
          onClose={() => {
            const next = new URLSearchParams(searchParams);
            next.delete("selectedTrace");
            setSearchParams(next);
          }}
          minWidth={"80vw"}
        >
          <DrawerHeader
            title="Trace Details"
            icon={<Workflow size={24} />}
            onClose={() => {
              const next = new URLSearchParams(searchParams);
              next.delete("selectedTrace");
              setSearchParams(next);
            }}
          />
          <DrawerContent>
            <TraceDetails traceId={selectedTrace ?? ""} />
          </DrawerContent>
        </DrawerWrapper>
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
    </>
  );
};
