/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { useEffect, useMemo, useRef, type ReactNode } from "react";
import { format } from "date-fns";
import type { LogLevel, LogEntry } from "@agent-management-platform/types";
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    IconButton,
    ListingTable,
    Paper,
    SearchBar,
    Skeleton,
    Stack,
    Tooltip,
} from "@wso2/oxygen-ui";
import {
    ArrowDown,
    ArrowUp,
    Copy,
    FileText,
} from "@wso2/oxygen-ui-icons-react";

export interface LogsPanelProps {
    logs?: LogEntry[];
    isLoading?: boolean;
    error?: unknown;
    isLoadingUp?: boolean;
    isLoadingDown?: boolean;
    onLoadUp?: () => void;
    onLoadDown?: () => void;
    sortOrder?: "asc" | "desc";
    onSearch?: (search: string) => void;
    search?: string;
    showSearch?: boolean;
    maxHeight?: string | number;
    emptyState?: {
        title: string;
        description?: string;
        illustration?: ReactNode;
    };
}

interface LogEntryItemProps {
    entry: LogEntry;
}

const getLogLevel = (logLevel: LogLevel | string): "info" | "warning" | "error" | "debug" | "unknown" => {

    if (logLevel === "ERROR") {
        return "error";
    }
    if (logLevel === "WARN" || logLevel === "WARNING") {
        return "warning";
    }
    if (logLevel === "INFO") {
        return "info";
    }
    if (logLevel === "DEBUG") {
        return "debug";
    }
    return "unknown";
};

const getLevelColor = (level: string) => {
    switch (level) {
        case "info":
            return "info";
        case "warning":
            return "warning";
        case "error":
            return "error";
        case "debug":
            return "secondary";
        case "unknown":
            return "secondary";
        default:
            return "info";
    }
};

const LogEntryItem = ({ entry }: LogEntryItemProps) => {
    const level = getLogLevel(entry.logLevel);

    const handleCopy = async (event: React.MouseEvent) => {
        event.stopPropagation();
        try {
            await navigator.clipboard.writeText(entry.log);
        } catch (copyError) {
            // eslint-disable-next-line no-console
            console.error("Failed to copy log", copyError);
        }
    };

    return (
        <Box sx={{
            display: "flex",
            flexDirection: "row",
            alignItems: "center",
            fontFamily: "monospace",
            fontSize: "0.8125rem",
            lineHeight: 1.5,
            whiteSpace: "nowrap",
            color: "text.primary",
            "& .action": { opacity: 0, transition: "opacity 0.2s" },
            "& .time-stamp": {
                bgcolor: "background.default",
            },
            "&:hover": {
                bgcolor: "background.paper",
                "& .time-stamp": {
                    bgcolor: "background.paper",

                },
                "& .action": { opacity: 1 },
            },
        }}>
            <Box component="span" className="time-stamp" sx={{
                left: 0,
                zIndex: 1,
                color: `${getLevelColor(level)}.main`,
                fontFamily: "monospace",
                whiteSpace: "nowrap",
                pr: 1,
            }}>
                {format(new Date(entry.timestamp), "dd/MM/yyyy")}
            </Box>
            <Box component="span" className="time-stamp" sx={{
                position: "sticky",
                left: 0,
                zIndex: 1,
                color: `${getLevelColor(level)}.main`,
                fontFamily: "monospace",
                whiteSpace: "nowrap",
                pr: 1,
            }}>
                {format(new Date(entry.timestamp), " HH:mm:ss")}
            </Box>
            <Box component="span" sx={{ color: "text.primary", fontFamily: "monospace", whiteSpace: "nowrap" }}>
                {entry.log}
            </Box>
            <Box component="span" className="action" sx={{ display: "inline-flex", ml: 1 }}>
                <IconButton size="small" sx={{ p: 0 }} onClick={handleCopy} aria-label="Copy log">
                    <Tooltip title="Copy log line">
                        <Copy size={12} />
                    </Tooltip>
                </IconButton>
            </Box>
        </Box>
    );
};

const defaultEmptyState = {
    title: "No logs found",
    description: "Try adjusting your search or time range",
    illustration: <FileText size={64} />,
};

const LABEL_LOAD_OLDER = "Load older logs";
const LABEL_LOAD_NEWER = "Load newer logs";
const LABEL_LOADING_OLDER = "Loading older logs...";
const LABEL_LOADING_NEWER = "Loading newer logs...";

const LOG_LOAD_LABELS = {
    asc: {
        up: LABEL_LOAD_NEWER,
        upLoading: LABEL_LOADING_NEWER,
        down: LABEL_LOAD_OLDER,
        downLoading: LABEL_LOADING_OLDER,
    },
    desc: {
        up: LABEL_LOAD_OLDER,
        upLoading: LABEL_LOADING_OLDER,
        down: LABEL_LOAD_NEWER,
        downLoading: LABEL_LOADING_NEWER,
    },
} as const;

export function LogsPanel({
    logs,
    isLoading,
    error,
    isLoadingUp,
    isLoadingDown,
    onLoadUp,
    onLoadDown,
    sortOrder = "desc",
    onSearch,
    search,
    showSearch = Boolean(onSearch),
    maxHeight = "calc(100vh - 340px)",
    emptyState,
}: LogsPanelProps) {
    const scrollContainerRef = useRef<HTMLDivElement>(null);
    const resolvedEmptyState = emptyState ?? defaultEmptyState;

    useEffect(() => {
        if (scrollContainerRef.current && logs && logs.length > 0 && !isLoading) {
            scrollContainerRef.current.scrollTop = scrollContainerRef.current.scrollHeight;
        }
    }, [logs, isLoading]);

    const reversedLogs = useMemo(() => (logs ? [...logs].reverse() : []), [logs]);
    const isNoLogs = !isLoading && (logs?.length ?? 0) === 0;
    const showPanel = reversedLogs.length > 0 && !isLoading;

    const {
        up: upLabel,
        upLoading: upLoadingLabel,
        down: downLabel,
        downLoading: downLoadingLabel
    } = LOG_LOAD_LABELS[sortOrder];

    if (error) {
        return (
            <Alert severity="error">
                {error instanceof Error ? error.message : "Failed to load logs"}
            </Alert>
        );
    }

    return (
        <Stack direction="column" gap={2} maxHeight={maxHeight} height={maxHeight}>
            <Paper
                variant="outlined"
                sx={{
                    flex: 1,
                    display: "flex",
                    flexDirection: "column",
                    overflow: "hidden",
                    bgcolor: "background.default",
                }}
            >
                {showSearch && (
                    <Stack direction="row" p={2} spacing={2} alignItems="center" flexWrap="wrap">
                        <Box alignItems="center" justifyContent="flex-start" display="flex" sx={{ flexGrow: 1, minWidth: 250 }}>
                            <SearchBar
                                placeholder="Search logs..."
                                size="small"
                                onChange={(event:
                                    React.ChangeEvent<HTMLInputElement>) =>
                                    onSearch?.(event.target.value)}
                                value={search}
                            />
                        </Box>
                    </Stack>
                )}
                {isLoading && (
                    <Stack direction="column" gap={1} p={2}>
                        {Array.from({ length: 5 }).map((_, index) => (
                            <Skeleton key={index} variant="rounded" height={60} width="100%" />
                        ))}
                    </Stack>
                )}
                {!isLoading && isNoLogs && (
                    <Box sx={{
                        height: "100%", display: "flex",
                        justifyContent: "center", alignItems: "center"
                    }} >
                        <ListingTable.EmptyState
                            illustration={resolvedEmptyState.illustration}
                            title={resolvedEmptyState.title}
                            description={resolvedEmptyState.description}

                        />
                    </Box>
                )}
                {showPanel && (
                    <Box ref={scrollContainerRef} sx={{ flex: 1, overflow: "auto", mx: 2, }}>
                        {onLoadUp && (
                            <Box sx={{ p: 1.5 }}>
                                <Button
                                    variant="text"
                                    size="small"
                                    fullWidth
                                    onClick={onLoadUp}
                                    disabled={isLoadingUp}
                                    startIcon={isLoadingUp ?
                                        <CircularProgress size={16} /> : <ArrowUp size={16} />}
                                >
                                    {isLoadingUp ? upLoadingLabel : upLabel}
                                </Button>
                            </Box>
                        )}
                        {reversedLogs.map((entry, index) => (
                            <LogEntryItem key={`${entry.timestamp}-${index}`} entry={entry} />
                        ))}
                        {onLoadDown && (
                            <Box sx={{ p: 1.5 }}>
                                <Button
                                    variant="text"
                                    size="small"
                                    fullWidth
                                    onClick={onLoadDown}
                                    disabled={isLoadingDown}
                                    startIcon={isLoadingDown ?
                                        <CircularProgress size={16} /> : <ArrowDown size={16} />}
                                >
                                    {isLoadingDown ? downLoadingLabel : downLabel}
                                </Button>
                            </Box>
                        )}
                    </Box>
                )}
            </Paper>
        </Stack>
    );
}
