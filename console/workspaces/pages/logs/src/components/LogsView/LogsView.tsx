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

import React, { useState } from "react";
import dayjs from "dayjs";
import { NoDataFound } from "@agent-management-platform/views";
import {
  ArrowUp,
  ArrowDown,
  FileText,
  Search,
  ChevronDown,
  ChevronRight,
  Info,
  AlertTriangle,
  AlertCircle,
  CheckCircle,
  Clock,
  RefreshCcw,
  SortAsc,
  SortDesc,
} from "@wso2/oxygen-ui-icons-react";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  Paper,
  Skeleton,
  Stack,
  TextField,
  Typography,
  IconButton,
  Select,
  MenuItem,
  InputAdornment,
  Card,
  CardContent,
  Collapse,
} from "@wso2/oxygen-ui";
import type { LogEntry } from "@agent-management-platform/types";

type SortOrder = "asc" | "desc";

export interface TimeRangeOption {
  value: string;
  label: string;
}

export interface LogsViewProps {
  logs?: LogEntry[];
  isLoading?: boolean;
  error?: unknown;
  // Infinite scroll props
  hasMoreUp?: boolean;
  hasMoreDown?: boolean;
  isLoadingUp?: boolean;
  isLoadingDown?: boolean;
  onLoadUp?: () => void;
  onLoadDown?: () => void;
  onSearch?: (search: string) => void;
  search?: string;
  // Time and sorting controls
  timeRange?: string;
  timeRangeOptions?: TimeRangeOption[];
  onTimeRangeChange?: (timeRange: string) => void;
  sortOrder?: SortOrder;
  onSortOrderChange?: (sortOrder: SortOrder) => void;
  onRefresh?: () => void;
  isRefreshing?: boolean;
}

interface LogEntryItemProps {
  entry: LogEntry;
}

const LogEntryItem: React.FC<LogEntryItemProps> = ({ entry }) => {
  const [expanded, setExpanded] = useState(false);

  // Determine log level/severity from log content
  const getLogLevel = (log: string): "info" | "warning" | "error" | "success" => {
    const lowerLog = log.toLowerCase();
    if (lowerLog.includes("error") || lowerLog.includes("failed")) return "error";
    if (lowerLog.includes("warning") || lowerLog.includes("warn")) return "warning";
    if (lowerLog.includes("success") || lowerLog.includes("completed")) return "success";
    return "info";
  };

  const getLevelIcon = (level: string) => {
    switch (level) {
      case "success":
        return <CheckCircle size={16} />;
      case "warning":
        return <AlertTriangle size={16} />;
      case "error":
        return <AlertCircle size={16} />;
      case "info":
      default:
        return <Info size={16} />;
    }
  };

  const getLevelColor = (level: string) => {
    switch (level) {
      case "success":
        return "success";
      case "warning":
        return "warning";
      case "error":
        return "error";
      case "info":
      default:
        return "info";
    }
  };

  const level = getLogLevel(entry.log);
  const hasDetails = entry.log.length > 100;

  return (
    <Box>
      <Box
        sx={{
          py: 1.5,
          px: 2,
          cursor: hasDetails ? "pointer" : "default",
          transition: "background-color 0.2s",
          "&:hover": hasDetails ? { bgcolor: "action.hover" } : {},
        }}
        onClick={() => hasDetails && setExpanded(!expanded)}
      >
        <Stack direction="row" spacing={1.5} alignItems="flex-start">
          {/* Level Icon */}
          <Box
            sx={{
              color: `${getLevelColor(level)}.main`,
              mt: 0.25,
              minWidth: 20,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            {getLevelIcon(level)}
          </Box>

          {/* Content */}
          <Box sx={{ flexGrow: 1, minWidth: 0 }}>
            <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 0.5 }}>
              {/* Timestamp */}
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ fontFamily: "monospace", whiteSpace: "nowrap" }}
              >
                {dayjs(entry.timestamp).format("DD/MM/YYYY HH:mm:ss")}
              </Typography>

              {/* Level Chip */}
              <Chip
                label={level.toUpperCase()}
                size="small"
                color={getLevelColor(level) as "success" | "warning" | "error" | "info"}
                sx={{ height: 18, fontSize: "0.65rem" }}
              />
            </Stack>

            {/* Log Message */}
            <Typography
              variant="body2"
              sx={{
                fontFamily: "monospace",
                fontSize: "0.8125rem",
                lineHeight: 1.5,
                wordBreak: "break-word",
                color: "text.primary",
              }}
            >
              {(!hasDetails || !expanded )&& `${entry.log.slice(0, 100)}...`}
              <Collapse
                in={hasDetails && expanded}
                timeout="auto"
                unmountOnExit
              >
                <Typography variant="caption" sx={{ fontFamily: "monospace" }}>
                  {entry.log}
                </Typography>
              </Collapse>
            </Typography>
          </Box>

          {/* Expand Icon */}
          {hasDetails && (
            <IconButton size="small">
              {expanded ? <ChevronDown size={18} /> : <ChevronRight size={18} />}
            </IconButton>
          )}
        </Stack>
      </Box>
      <Divider />
    </Box>
  );
};

export const LogsView: React.FC<LogsViewProps> = ({
  logs,
  isLoading,
  error,
  isLoadingUp,
  isLoadingDown,
  onLoadUp,
  onLoadDown,
  onSearch,
  search,
  timeRange,
  timeRangeOptions = [],
  onTimeRangeChange,
  sortOrder = "desc",
  onSortOrderChange,
  onRefresh,
  isRefreshing = false,
}) => {
  if (error) {
    return (
      <Alert severity="error">
        {error instanceof Error ? error.message : "Failed to load logs"}
      </Alert>
    );
  }

  const isNoLogs = !isLoading && (logs?.length ?? 0) === 0;
  const isShowPanel = logs && logs.length > 0 && !isLoading;

  const handleSortToggle = () => {
    onSortOrderChange?.(sortOrder === "desc" ? "asc" : "desc");
  };

  return (
    <Stack direction="column" gap={2} height="calc(100vh - 320px)">
      {/* Filters and Controls */}
      <Card variant="outlined">
        <CardContent>
          <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
            {/* Search Field */}
            <Box sx={{ flexGrow: 1, minWidth: 250 }}>
              <TextField
                placeholder="Search logs..."
                size="small"
                fullWidth
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  onSearch?.(e.target.value)
                }
                value={search}
                slotProps={{
                  input: {
                    endAdornment: <Search size={16} />,
                  },
                }}
              />
            </Box>

            {/* Time Range Selector */}
            {timeRangeOptions.length > 0 && onTimeRangeChange && (
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
            )}

            {/* Sort Toggle */}
            {onSortOrderChange && (
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
            )}

            {/* Refresh Button */}
            {onRefresh && (
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
            )}
          </Stack>
        </CardContent>
      </Card>

      {/* Log Count Summary */}
      {isShowPanel && (
        <Box sx={{ textAlign: "center" }}>
          <Typography variant="body2" color="text.secondary">
            Showing {logs.length} log {logs.length === 1 ? "entry" : "entries"}
          </Typography>
        </Box>
      )}

      {/* Empty State */}
      {isNoLogs && (
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flex: 1,
            minHeight: 300,
          }}
        >
          <NoDataFound
            message="No logs found"
            subtitle="Try adjusting your search or time range"
            icon={<FileText size={48} />}
          />
        </Box>
      )}

      {/* Loading Skeleton */}
      {isLoading && (
        <Stack direction="column" gap={1}>
          <Skeleton variant="rounded" height={60} width="100%" />
          <Skeleton variant="rounded" height={60} width="100%" />
          <Skeleton variant="rounded" height={60} width="100%" />
          <Skeleton variant="rounded" height={60} width="100%" />
          <Skeleton variant="rounded" height={60} width="100%" />
        </Stack>
      )}

      {/* Logs Panel */}
      {isShowPanel && (
        <Paper
          variant="outlined"
          sx={{
            flex: 1,
            display: "flex",
            flexDirection: "column",
            overflow: "hidden",
          }}
        >
          {/* Scrollable Content Area */}
          <Box sx={{ flex: 1, overflow: "auto" }}>
            {/* Load Older Logs Button */}
            <Box
              sx={{
                p: 1.5,
                borderBottom: 1,
                borderColor: "divider",
                bgcolor: "background.default",
              }}
            >
              <Button
                variant="outlined"
                size="small"
                fullWidth
                onClick={onLoadUp}
                startIcon={
                  isLoadingUp ? (
                    <CircularProgress size={16} />
                  ) : (
                    <ArrowUp size={16} />
                  )
                }
                sx={{
                  borderStyle: "dashed",
                }}
              >
                {isLoadingUp ? "Loading older logs..." : "Load older logs"}
              </Button>
            </Box>

            {/* Log Entries */}
            {logs.map((entry, idx) => (
              <LogEntryItem
                key={`${entry.timestamp}-${idx}`}
                entry={entry}
              />
            ))}

            {/* Load Newer Logs Button */}
            <Box
              sx={{
                p: 1.5,
                borderTop: 1,
                borderColor: "divider",
                bgcolor: "background.default",
              }}
            >
              <Button
                variant="outlined"
                size="small"
                fullWidth
                onClick={onLoadDown}
                startIcon={
                  isLoadingDown ? (
                    <CircularProgress size={16} />
                  ) : (
                    <ArrowDown size={16} />
                  )
                }
                sx={{
                  borderStyle: "dashed",
                }}
              >
                {isLoadingDown ? "Loading newer logs..." : "Load newer logs"}
              </Button>
            </Box>
          </Box>
        </Paper>
      )}
    </Stack>
  );
};
