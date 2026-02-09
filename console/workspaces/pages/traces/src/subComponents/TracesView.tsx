/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import {
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Clock,
  RefreshCcw,
  SortAsc,
  SortDesc,
  Download,
} from "@wso2/oxygen-ui-icons-react";
import type { TraceOverview, TraceListTimeRange } from "@agent-management-platform/types";
import { TracesTable } from "./TracesTable";

type SortOrder = "asc" | "desc";

export interface TimeRangeOption {
  value: string;
  label: string;
}

export interface TracesViewProps {
  // Data props
  traces: TraceOverview[];
  count: number;
  page: number;
  rowsPerPage: number;
  isLoading?: boolean;
  selectedTrace: string | null;

  // Handlers
  onTraceSelect: (traceId: string) => void;
  onPageChange: (page: number) => void;
  onRowsPerPageChange: (rowsPerPage: number) => void;

  // Time and sorting controls
  timeRange: TraceListTimeRange;
  timeRangeOptions: TimeRangeOption[];
  onTimeRangeChange: (timeRange: string) => void;
  sortOrder: SortOrder;
  onSortOrderChange: (sortOrder: SortOrder) => void;
  onRefresh: () => void;
  isRefreshing?: boolean;

  // Export
  onExport?: () => void;
  isExporting?: boolean;
}

export const TracesView: React.FC<TracesViewProps> = ({
  traces,
  count,
  page,
  rowsPerPage,
  isLoading = false,
  selectedTrace,
  onTraceSelect,
  onPageChange,
  onRowsPerPageChange,
  timeRange,
  timeRangeOptions,
  onTimeRangeChange,
  sortOrder,
  onSortOrderChange,
  onRefresh,
  isRefreshing = false,
  onExport,
  isExporting = false,
}) => {
  const handleSortToggle = () => {
    onSortOrderChange(sortOrder === "desc" ? "asc" : "desc");
  };

  return (
    <Stack direction="column" gap={3}>
      {/* Filters and Controls */}
      <Card variant="outlined">
        <CardContent>
          <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
            <Box sx={{ flexGrow: 1 }} />

            {/* Time Range Selector */}
            <Select
              size="small"
              variant="outlined"
              value={timeRange}
              onChange={(e) => onTimeRangeChange(e.target.value)}
              startAdornment={
                <InputAdornment position="start">
                  <Clock size={16} />
                </InputAdornment>
              }
              sx={{ minWidth: 150 }}
            >
              {timeRangeOptions.map((opt) => (
                <MenuItem key={opt.value} value={opt.value}>
                  {opt.label}
                </MenuItem>
              ))}
            </Select>

            {/* Sort Toggle */}
            <IconButton
              size="small"
              onClick={handleSortToggle}
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
              disabled={isRefreshing}
              onClick={onRefresh}
              aria-label="Refresh"
            >
              {isRefreshing ? (
                <CircularProgress size={16} />
              ) : (
                <RefreshCcw size={16} />
              )}
            </IconButton>

            {/* Export Button */}
            {onExport && (
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
                onClick={onExport}
                disabled={isExporting || isLoading || traces.length === 0}
              >
                Export
              </Button>
            )}
          </Stack>
        </CardContent>
      </Card>

      {/* Trace Count Summary */}
      {!isLoading && traces.length > 0 && (
        <Box sx={{ textAlign: "center" }}>
          <Typography variant="body2" color="text.secondary">
            Showing {traces.length} of {count} total {count === 1 ? "trace" : "traces"}
          </Typography>
        </Box>
      )}

      {/* Traces Table */}
      <Box>
        {isLoading ? (
          <Skeleton variant="rounded" height={500} width="100%" />
        ) : (
          <TracesTable
            traces={traces}
            onTraceSelect={onTraceSelect}
            count={count}
            page={page}
            rowsPerPage={rowsPerPage}
            onPageChange={onPageChange}
            onRowsPerPageChange={onRowsPerPageChange}
            selectedTrace={selectedTrace}
          />
        )}
      </Box>
    </Stack>
  );
};
