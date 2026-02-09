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
  getTimeRange,
} from "@agent-management-platform/types";
import {
  Snackbar,
  Alert,
} from "@mui/material";
import {
  Workflow,
} from "@wso2/oxygen-ui-icons-react";
import { useTraceList, useExportTraces } from "@agent-management-platform/api-client";
import { TraceDetails, TracesView } from "./subComponents";

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

  // Initialize state from URL search params with defaults
  const timeRange = useMemo(
    () =>
      (searchParams.get("timeRange") as TraceListTimeRange) ||
      TraceListTimeRange.SEVEN_DAYS,
    [searchParams]
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
    sortOrder
  );
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

      const range = getTimeRange(timeRange);
      if (!range) {
        setExportError("Invalid time range");
        return;
      }
      const { startTime, endTime } = range;

      // Export ALL traces matching the current filters (time range, environment, sort order)
      // Backend caps at 1000 traces for safety
      const exportData = await exportTracesAsync({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
        environment: envId,
        startTime,
        endTime,
        sortOrder,
        // No limit/offset - backend handles fetching all traces
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
  }, [orgId, projectId, agentId, envId, timeRange, sortOrder, exportTracesAsync]);

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

  const handleRefresh = useCallback(() => {
    refetch();
  }, [refetch]);

  return (
    <>
      <PageLayout
        title="Traces"
        disableIcon
      >
        <TracesView
          traces={traceData?.traces ?? []}
          count={count}
          page={page}
          rowsPerPage={rowsPerPage}
          isLoading={isLoading}
          selectedTrace={selectedTrace}
          onTraceSelect={handleTraceSelect}
          onPageChange={handlePageChange}
          onRowsPerPageChange={handleRowsPerPageChange}
          timeRange={timeRange}
          timeRangeOptions={TIME_RANGE_OPTIONS}
          onTimeRangeChange={handleTimeRangeChange}
          sortOrder={sortOrder}
          onSortOrderChange={handleSortOrderChange}
          onRefresh={handleRefresh}
          isRefreshing={isRefetching}
          onExport={handleExportTraces}
          isExporting={isExporting}
        />
        <DrawerWrapper
          open={!!selectedTrace}
          disableScroll
          onClose={() => setSearchParams(new URLSearchParams())}
          minWidth={"80vw"}
        >
          <DrawerHeader
            title="Trace Details"
            icon={<Workflow size={24} />}
            onClose={() => setSearchParams(new URLSearchParams())}
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
