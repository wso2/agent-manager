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

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
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
    FileText,
    TextWrap,
} from "@wso2/oxygen-ui-icons-react";

export interface LogsPanelProps {
    logs?: LogEntry[];
    isLoading?: boolean;
    error?: unknown;
    isLoadingUp?: boolean;
    isLoadingDown?: boolean;
    hasMoreUp?: boolean;
    hasMoreDown?: boolean;
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

const getLogLevel = (logLevel: LogLevel | string): "info" | "warning" | "error" | "debug" | "unknown" => {
    if (logLevel === "ERROR") return "error";
    if (logLevel === "WARN" || logLevel === "WARNING") return "warning";
    if (logLevel === "INFO") return "info";
    if (logLevel === "DEBUG") return "debug";
    return "unknown";
};

const LEVEL_COLOR_TOKENS: Record<string, string> = {
    error: "error.main",
    warning: "warning.main",
    info: "info.main",
    debug: "text.disabled",
    unknown: "text.disabled",
};

const MONO_FONT = "'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'Consolas', monospace";

interface EditorLogsProps {
    entries: LogEntry[];
    wrap: boolean;
}

const EditorLogs = ({ entries, wrap }: EditorLogsProps) => {
    const lineCount = entries.length;
    const gutterWidth = String(lineCount).length;

    return (
        <Box
            component="pre"
            sx={{
                m: 0,
                pl: 2,
                pr: 0,
                pt: 0,
                pb: 0,
                fontFamily: MONO_FONT,
                fontSize: "0.8125rem",
                lineHeight: 2.2,
                whiteSpace: wrap ? "pre-wrap" : "pre",
                wordBreak: "normal",
                overflowWrap: wrap ? "anywhere" : "normal",
                userSelect: "text",
                cursor: "text",
            }}
        >
            {entries.map((entry, index) => {
                const level = getLogLevel(entry.logLevel);
                const levelColor = LEVEL_COLOR_TOKENS[level];
                const lineNum = String(index + 1).padStart(gutterWidth, " ");
                const timestamp = format(new Date(entry.timestamp), "dd/MM/yyyy HH:mm:ss");

                return (
                    <Box
                        key={`${entry.timestamp}-${index}`}
                        component="span"
                        sx={{
                            display: "block",
                            borderBottom: "1px solid",
                            borderColor: "divider",
                            "&:hover": { bgcolor: "action.hover" },
                        }}
                    >
                        <Box component="span" sx={{ color: "text.disabled", userSelect: "none", pr: 2 }}>
                            {lineNum}
                        </Box>
                        <Box component="span" sx={{ color: levelColor, pr: 2 }}>
                            {timestamp}
                        </Box>
                        <Box component="span" sx={{ color: "text.primary" }}>
                            {entry.log}
                        </Box>
                    </Box>
                );
            })}
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
    hasMoreUp,
    hasMoreDown,
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
    const hasInitializedRef = useRef(false);
    const [wrap, setWrap] = useState(true);

    useEffect(() => {
        if (!scrollContainerRef.current || !logs || logs.length === 0) return;
        if (hasInitializedRef.current) return;
        hasInitializedRef.current = true;
        scrollContainerRef.current.scrollTop = scrollContainerRef.current.scrollHeight;
    }, [logs]);

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
                {(showSearch || showPanel) && (
                    <Stack direction="row" p={1} px={2} spacing={2} alignItems="center" justifyContent="flex-end">
                        {showSearch && (
                            <Box display="flex" sx={{ flexGrow: 1, minWidth: 400 }}>
                                <SearchBar
                                    placeholder="Search logs..."
                                    size="small"
                                    fullWidth
                                    onChange={(event: React.ChangeEvent<HTMLInputElement>) =>
                                        onSearch?.(event.target.value)}
                                    value={search}
                                />
                            </Box>
                        )}
                        {showPanel && (
                            <Tooltip title={wrap ? "Disable line wrap" : "Enable line wrap"}>
                                <IconButton
                                    size="small"
                                    onClick={() => setWrap(v => !v)}
                                    color={wrap ? "primary" : "default"}
                                    aria-label={wrap ? "Disable line wrap" : "Enable line wrap"}
                                    aria-pressed={wrap}
                                >
                                    <TextWrap size={16} />
                                </IconButton>
                            </Tooltip>
                        )}
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
                    <>
                        {onLoadUp && (
                            <Box sx={{ p: 1, borderBottom: "1px solid", borderColor: "divider" }}>
                                <Button
                                    variant="text"
                                    size="small"
                                    fullWidth
                                    onClick={onLoadUp}
                                    disabled={isLoadingUp || hasMoreUp === false}
                                    startIcon={isLoadingUp ?
                                        <CircularProgress size={16} /> : <ArrowUp size={16} />}
                                >
                                    {isLoadingUp ? upLoadingLabel : upLabel}
                                </Button>
                            </Box>
                        )}
                        <Box
                            ref={scrollContainerRef}
                            sx={{
                                flex: 1,
                                overflow: "auto",
                                bgcolor: "background.default",
                            }}
                        >
                            <EditorLogs entries={reversedLogs} wrap={wrap} />
                        </Box>
                        {onLoadDown && (
                            <Box sx={{ p: 1, borderTop: "1px solid", borderColor: "divider" }}>
                                <Button
                                    variant="text"
                                    size="small"
                                    fullWidth
                                    onClick={onLoadDown}
                                    disabled={isLoadingDown || hasMoreDown === false}
                                    startIcon={isLoadingDown ?
                                        <CircularProgress size={16} /> : <ArrowDown size={16} />}
                                >
                                    {isLoadingDown ? downLoadingLabel : downLabel}
                                </Button>
                            </Box>
                        )}
                    </>
                )}
            </Paper>
        </Stack>
    );
}
