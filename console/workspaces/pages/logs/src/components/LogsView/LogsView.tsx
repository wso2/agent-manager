import React from "react";
import dayjs from "dayjs";
import { NoDataFound } from "@agent-management-platform/views";
import { ArrowUp, ArrowDown, FileText, Search } from "@wso2/oxygen-ui-icons-react";
import {
  Alert,
  Button,
  CircularProgress,
  Paper,
  Skeleton,
  Stack,
  TextField,
  Typography,
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
  if (error) {
    return (
      <Alert severity="error">
        {error instanceof Error ? error.message : "Failed to load logs"}
      </Alert>
    );
  }

  const isNoLogs = !isLoading && (logs?.length ?? 0) === 0;
  const isShowPanel = logs && logs.length > 0 && !isLoading;

  return (
    <Stack direction="column" gap={1} height="calc(100vh - 250px)">
      {/* Load Older Logs Button */}
      <TextField
        placeholder="Search"
        size="small"
        onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
          onSearch?.(e.target.value)
        }
        value={search}
        slotProps={{
          input: { endAdornment: <Search size={16} /> },
        }}
      />
      {isNoLogs && (
        <NoDataFound
          message="No logs found!"
          subtitle="Try changing the time range"
          icon={<FileText size={32} />}
        />
      )}
      {isLoading && (
        <Stack direction="column" gap={1}>
          <Skeleton variant="rounded" height={24} width="100%" />
          <Skeleton variant="rounded" height={24} width="100%" />
          <Skeleton variant="rounded" height={24} width="100%" />
          <Skeleton variant="rounded" height={24} width="100%" />
        </Stack>
      )}
      {isShowPanel && (
        <Paper sx={{ overflow: "auto", p: 2, flex: 1 }}>
          <Button
            variant="text"
            size="small"
            onClick={onLoadUp}
            disabled={isLoadingUp}
            startIcon={
              isLoadingUp ? (
                <CircularProgress size={16} />
              ) : (
                <ArrowUp size={16} />
              )
            }
          >
            {isLoadingUp ? "Loading logs..." : "Load more logs"}
          </Button>

          {logs.map((entry, idx) => (
            <Stack
              direction="row"
              gap={1}
              alignItems="center"
              width="fit-content"
              key={`${entry.timestamp}-${idx}`}
            >
              <Typography
                variant="caption"
                fontFamily="monospace"
                noWrap
                color="textDisabled"
              >
                {dayjs(entry.timestamp).format("DD/MM/YYYY HH:mm:ss")}
              </Typography>
              <Typography variant="caption" fontFamily="monospace" noWrap>
                {entry.log}
              </Typography>
            </Stack>
          ))}
          {/* Load Newer Logs Button */}

          <Button
            variant="text"
            size="small"
            onClick={onLoadDown}
            disabled={isLoadingDown}
            startIcon={
              isLoadingDown ? (
                <CircularProgress size={16} />
              ) : (
                <ArrowDown size={16} />
              )
            }
          >
            {isLoadingDown ? "Loading logs..." : "Load more logs"}
          </Button>
        </Paper>
      )}
    </Stack>
  );
};
