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

import React, { useCallback, useMemo } from "react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  FadeIn,
  PageLayout,
} from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import {
  GetTraceListPathParams,
  TraceListTimeRange,
} from "@agent-management-platform/types";
import {
  CircularProgress,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Skeleton,
  Stack,
} from "@mui/material";
import {
  Clock,
  RefreshCcw,
  SortAsc,
  SortDesc,
  Workflow,
} from "@wso2/oxygen-ui-icons-react";
import { useTraceList } from "@agent-management-platform/api-client";
import { TraceDetails, TracesTable, TracesTopCards } from "./subComponents";

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

  return (
    <FadeIn>
      <PageLayout
        title="Traces"
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
              {TIME_RANGE_OPTIONS.map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
                </MenuItem>
              ))}
            </Select>
            <IconButton
              size="small"
              disabled={isRefetching}
              onClick={() => {
                refetch();
              }}
            >
              {isRefetching ? (
                <CircularProgress size={16} />
              ) : (
                <RefreshCcw size={16} />
              )}
            </IconButton>
            <IconButton
              size="small"
              onClick={() => {
                const next = new URLSearchParams(searchParams);
                next.set("sortOrder", sortOrder === "desc" ? "asc" : "desc");
                setSearchParams(next);
              }}
            >
              {sortOrder === "desc" ? (
                <SortAsc size={16} />
              ) : (
                <SortDesc size={16} />
              )}
            </IconButton>
          </Stack>
        }
        disableIcon
      >
        <Stack direction="column" gap={4}>
          <TracesTopCards timeRange={timeRange} />
          {isLoading ? (
            <Skeleton variant="rounded" height={500} width="100%" />
          ) : (
            <TracesTable
              traces={traceData?.traces ?? []}
              onTraceSelect={handleTraceSelect}
              count={count}
              page={page}
              rowsPerPage={rowsPerPage}
              onPageChange={handlePageChange}
              onRowsPerPageChange={handleRowsPerPageChange}
              selectedTrace={selectedTrace}
            />
          )}
        </Stack>
        <DrawerWrapper
          open={!!selectedTrace}
          onClose={() => setSearchParams(new URLSearchParams())}
          minWidth={"80vw"}
        >
          <DrawerHeader
            title="Trace Details"
            icon={<Workflow size={16} />}
            onClose={() => setSearchParams(new URLSearchParams())}
          />
          <DrawerContent>
            <TraceDetails traceId={selectedTrace ?? ""} />
          </DrawerContent>
        </DrawerWrapper>
      </PageLayout>
    </FadeIn>
  );
};
