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

import React, { useState, useCallback, useMemo } from "react";
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
];

export const TracesComponent: React.FC = () => {
  const { agentId, orgId, projectId, envId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  const [timeRange, setTimeRange] = useState<TraceListTimeRange>(
    TraceListTimeRange.ONE_DAY
  );
  const [limit, setLimit] = useState<number>(10);
  const [offset, setOffset] = useState<number>(0);
  const [sortOrder, setSortOrder] =
    useState<GetTraceListPathParams["sortOrder"]>("desc");
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
      setOffset(newPage * rowsPerPage);
    },
    [rowsPerPage]
  );

  const handleRowsPerPageChange = useCallback((newRowsPerPage: number) => {
    setLimit(newRowsPerPage);
    setOffset(0); // Reset to first page when changing rows per page
  }, []);

  return (
    <FadeIn>
      <PageLayout
        title="Traces"
        actions={
          <Stack direction="row" gap={1}>
            {setTimeRange && (
              <Select
                size="small"
                variant="outlined"
                value={timeRange}
                startAdornment={
                  <InputAdornment position="start">
                    <Clock size={16} />
                  </InputAdornment>
                }
                onChange={(e) =>
                  setTimeRange(e.target.value as TraceListTimeRange)
                }
              >
                {TIME_RANGE_OPTIONS.map((option) => (
                  <MenuItem key={option.value} value={option.value}>
                    {option.label}
                  </MenuItem>
                ))}
              </Select>
            )}
            <IconButton
              size="small"
              disabled={isRefetching}
              color="primary"
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
              onClick={() =>
                setSortOrder(sortOrder === "desc" ? "asc" : "desc")
              }
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
