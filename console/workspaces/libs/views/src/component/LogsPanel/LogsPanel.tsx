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

import { ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { format } from "date-fns";
import { LogLevel, type LogEntry } from "@agent-management-platform/types";
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Collapse,
    Divider,
    IconButton,
    ListingTable,
    Paper,
    SearchBar,
    Skeleton,
    Stack,
    Typography,
} from "@wso2/oxygen-ui";
import {
    AlertCircle,
    AlertTriangle,
    ArrowDown,
    ArrowUp,
    CheckCircle,
    ChevronDown,
    ChevronRight,
    Copy,
    FileText,
    Info,
} from "@wso2/oxygen-ui-icons-react";

export interface LogsPanelProps {
    logs?: LogEntry[];
    isLoading?: boolean;
    error?: unknown;
    hasMoreUp?: boolean;
    hasMoreDown?: boolean;
    isLoadingUp?: boolean;
    isLoadingDown?: boolean;
    onLoadUp?: () => void;
    onLoadDown?: () => void;
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

const getLogLevel = (logLevel: LogLevel): "info" | "warning" | "error" | "success" => {

    if (logLevel === "ERROR") {
        return "error";
    }
    if (logLevel === "WARN") {
        return "warning";
    }
    if (logLevel === "INFO") {
        return "success";
    }
    if (logLevel === "DEBUG") {
        return "warning";
    }
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

const LogEntryItem = ({ entry }: LogEntryItemProps) => {
    const [expanded, setExpanded] = useState(false);
    const level = getLogLevel(entry.logLevel);
    const hasDetails = entry.log.length > 100;

    const handleCopy = async (event: React.MouseEvent) => {
        event.stopPropagation();
        try {
            await navigator.clipboard.writeText(entry.log);
        } catch (copyError) {
            console.error("Failed to copy log", copyError);
        }
    };

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
                onClick={() => hasDetails && setExpanded((prev) => !prev)}
            >
                <Stack direction="row" spacing={1.5} alignItems="flex-start">
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
                    <Box sx={{ flexGrow: 1, minWidth: 0 }}>
                        <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 0.5 }}>
                            <Typography
                                variant="caption"
                                color="text.secondary"
                                sx={{ fontFamily: "monospace", whiteSpace: "nowrap" }}
                            >
                                {format(new Date(entry.timestamp), "dd/MM/yyyy HH:mm:ss")}
                            </Typography>
                        </Stack>
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
                            {(!hasDetails || !expanded) && `${entry.log.slice(0, 100)}${hasDetails ? "..." : ""}`}
                            <Collapse in={hasDetails && expanded} timeout="auto" unmountOnExit>
                                <Typography variant="caption" sx={{ fontFamily: "monospace" }}>
                                    {entry.log}
                                </Typography>
                            </Collapse>
                        </Typography>
                    </Box>
                    <Stack direction="row" spacing={0.5}>
                        <IconButton size="small" onClick={handleCopy} aria-label="Copy log">
                            <Copy size={16} />
                        </IconButton>
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

const defaultEmptyState = {
    title: "No logs found",
    description: "Try adjusting your search or time range",
    illustration: <FileText size={64} />,
};

export function LogsPanel({
    logs,
    isLoading,
    error,
    isLoadingUp,
    isLoadingDown,
    onLoadUp,
    onLoadDown,
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
                    <ListingTable.EmptyState
                        illustration={resolvedEmptyState.illustration}
                        title={resolvedEmptyState.title}
                        description={resolvedEmptyState.description}
                    />
                )}
                {showPanel && (
                    <Box ref={scrollContainerRef} sx={{ flex: 1, overflow: "auto" }}>
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
                                    {isLoadingUp ? "Loading more logs..." : "Load more logs"}
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
                                    {isLoadingDown ? "Loading more logs..." : "Load more logs"}
                                </Button>
                            </Box>
                        )}
                    </Box>
                )}
            </Paper>
        </Stack>
    );
}
