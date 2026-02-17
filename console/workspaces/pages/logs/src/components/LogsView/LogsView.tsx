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

import React, { useState, useEffect, useRef, useMemo } from "react";
import { format } from "date-fns";
import {
  ArrowUp,
  ArrowDown,
  FileText,
  ChevronDown,
  ChevronRight,
  Info,
  AlertTriangle,
  AlertCircle,
  CheckCircle,
  Copy,
} from "@wso2/oxygen-ui-icons-react";
import {
  Alert,
  Box,
  Button,
  Chip,
  Divider,
  Paper,
  Skeleton,
  Stack,
  Typography,
  IconButton,
  Collapse,
  ListingTable,
  CircularProgress,
  SearchBar,
} from "@wso2/oxygen-ui";
import type { LogEntry } from "@agent-management-platform/types";

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
}

interface LogEntryItemProps {
  entry: LogEntry;
}

const LogEntryItem: React.FC<LogEntryItemProps> = ({ entry }) => {
  const [expanded, setExpanded] = useState(false);

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(entry.log);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

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
    <>
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
                {format(new Date(entry.timestamp), "dd/MM/yyyy HH:mm:ss")}
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
              {(!hasDetails || !expanded) && `${entry.log.slice(0, 100)}...`}
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

          {/* Action Buttons */}
          <Stack direction="row" spacing={0.5}>
            {/* Copy Button */}
            <IconButton
              size="small"
              onClick={handleCopy}
              aria-label="Copy log"
            >
              <Copy size={16} />
            </IconButton>

            {/* Expand Icon */}
            {hasDetails && (
              <IconButton size="small">
                {expanded ? <ChevronDown size={18} /> : <ChevronRight size={18} />}
              </IconButton>
            )}
          </Stack>
        </Stack>
      </Box>
      <Divider />
    </>
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
}) => {
  const scrollContainerRef = useRef<HTMLDivElement>(null);

  // Scroll to bottom on initial load and when logs change
  useEffect(() => {
    if (scrollContainerRef.current && logs && logs.length > 0 && !isLoading) {
      scrollContainerRef.current.scrollTop = scrollContainerRef.current.scrollHeight;
    }
  }, [logs, isLoading]);

  const isNoLogs = !isLoading && (logs?.length ?? 0) === 0;
  const isShowPanel = logs && logs.length > 0 && !isLoading;
  const reversedLogs = useMemo(() => (logs ? [...logs].reverse() : []), [logs]);


  if (error) {
    return (
      <Alert severity="error">
        {error instanceof Error ? error.message : "Failed to load logs"}
      </Alert>
    );
  }


  return (
    <Stack direction="column" gap={2} maxHeight="calc(100vh - 340px)">
      {/* Logs Panel */}
      <Paper
        variant="outlined"
        sx={{
          flex: 1,
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
        }}
      >
        <Stack direction="row" p={2} spacing={2} alignItems="center" flexWrap="wrap">
          <Box
            alignItems="center"
            justifyContent="flex-start"
            display="flex"
            sx={{
              flexGrow: 1,
              minWidth: 250,
            }}
          >
            <SearchBar
              placeholder="Search logs..."
              size="small"
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                onSearch?.(e.target.value)
              }
              value={search}
            />
          </Box>
        </Stack>

        {isLoading && (
          <Stack direction="column" gap={1} p={2}>
            <Skeleton variant="rounded" height={60} width="100%" />
            <Skeleton variant="rounded" height={60} width="100%" />
            <Skeleton variant="rounded" height={60} width="100%" />
            <Skeleton variant="rounded" height={60} width="100%" />
            <Skeleton variant="rounded" height={60} width="100%" />
          </Stack>
        )}

        {!isLoading && isNoLogs && (
          <ListingTable.EmptyState
            illustration={<FileText size={64} />}
            title="No logs found"
            description="Try adjusting your search or time range"
          />
        )}
        {/* Scrollable Content Area */}
        {isShowPanel && (
          <Box ref={scrollContainerRef} sx={{ flex: 1, overflow: "auto" }}>
            {/* Load Up Button */}
            <Box
              sx={{
                p: 1.5,
              }}
            >
              <Button
                variant="text"
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

              >
                {isLoadingUp ? "Loading more logs..." : "Load more logs"}
              </Button>
            </Box>

            {/* Log Entries */}
            {reversedLogs.map((entry, idx) => (
              <LogEntryItem
                key={`${entry.timestamp}-${idx}`}
                entry={entry}
              />
            ))}

            {/* Load Down Button */}
            <Box
              sx={{
                p: 1.5,
              }}
            >
              <Button
                variant="text"
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
              >
                {isLoadingDown ? "Loading more logs..." : "Load more logs"}
              </Button>
            </Box>
          </Box>
        )}
      </Paper>

    </Stack>
  );
};
