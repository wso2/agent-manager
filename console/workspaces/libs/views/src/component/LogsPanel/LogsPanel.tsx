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

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { format } from "date-fns";
import type { LogLevel, LogEntry } from "@agent-management-platform/types";
import {
    Alert,
    Box,
    CircularProgress,
    IconButton,
    ListingTable,
    Paper,
    SearchBar,
    Skeleton,
    Stack,
    Tooltip,
    alpha,
    useTheme,
} from "@wso2/oxygen-ui";
import {
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
    onSearch?: (search: string) => void;
    search?: string;
    showSearch?: boolean;
    showTimestamp?: boolean;
    showLogLevel?: boolean;
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
const MONO_FONT_SIZE = "0.75rem";


const ROW_HEIGHT = "2rem";
const TS_WIDTH = "11rem";
const LV_WIDTH = "6rem";

const HEADER_CELL_SX = {
    fontFamily: MONO_FONT,
    fontSize: MONO_FONT_SIZE,
    fontWeight: 600,
    color: "text.secondary",
    px: 2,
    height: "2rem",
    display: "flex",
    alignItems: "center",
    borderBottom: "2px solid",
    borderColor: "divider",
    bgcolor: "background.default",
    whiteSpace: "nowrap",
} as const;

const LogsPanelHeader = (
    { showTimestamp, showLogLevel }: { showTimestamp: boolean; showLogLevel: boolean }
) => (
    <Box sx={{ display: "flex", flexShrink: 0, borderBottom: "2px solid", borderColor: "divider", bgcolor: "background.default" }}>
        {showTimestamp && <Box sx={{ ...HEADER_CELL_SX, width: TS_WIDTH, flexShrink: 0, borderBottom: "none" }}>Timestamp</Box>}
        {showLogLevel && <Box sx={{ ...HEADER_CELL_SX, width: LV_WIDTH, flexShrink: 0, borderBottom: "none" }}>LogLevel</Box>}
        <Box sx={{ ...HEADER_CELL_SX, borderBottom: "none" }}>Log</Box>
    </Box>
);

interface LogsPanelRowsProps {
    entries: LogEntry[];
    wrap: boolean;
    showTimestamp: boolean;
    showLogLevel: boolean;
    isLoadingUp?: boolean;
    isLoadingDown?: boolean;
}

const LogsPanelRows = (
    { entries, wrap, showTimestamp, showLogLevel, isLoadingUp, isLoadingDown }: LogsPanelRowsProps
) => {
    const theme = useTheme();
    const gridCols = [showTimestamp && TS_WIDTH, showLogLevel && LV_WIDTH, "1fr"].filter(Boolean).join(" ");

    const loadingSpan = (key: string, label: string) => (
        <Box key={key} sx={{ gridColumn: "1 / -1", display: "flex", alignItems: "center", justifyContent: "center", gap: 1, py: 1, borderBottom: "1px solid", borderColor: "divider", fontFamily: MONO_FONT, fontSize: MONO_FONT_SIZE, color: "text.secondary" }}>
            <CircularProgress size={14} />
            {label}
        </Box>
    );

    return (
        <Box
            sx={{
                display: "grid",
                gridTemplateColumns: gridCols,
                minWidth: "100%",
                width: wrap ? "100%" : "max-content",
            }}
        >
                {isLoadingUp && loadingSpan("loading-up", "Loading older logs...")}
                {entries.map((entry, index) => {
                    const level = getLogLevel(entry.logLevel);
                    const levelColor = LEVEL_COLOR_TOKENS[level];
                    const timestamp = format(new Date(entry.timestamp), "dd/MM/yyyy HH:mm:ss");
                    const rowKey = `${entry.timestamp}-${index}`;
                    const isError = level === "error";
                    const bgBase = theme.palette.background.default;
                    const rowBg = isError
                        ? alpha(theme.palette.error.main, 0.08) : bgBase;
                    // Sticky cells must be fully opaque to avoid bleed-through
                    // when scrolling horizontally over the log column.
                    const stickyBg = isError
                        ? `color-mix(in srgb, ${theme.palette.error.main} 8%, ${bgBase})`
                        : bgBase;
                    const hoverBg = isError
                        ? alpha(theme.palette.error.main, 0.15) : theme.palette.action.hover;
                    const stickyHoverBg = isError
                        ? `color-mix(in srgb, ${theme.palette.error.main} 15%, ${bgBase})`
                        : `color-mix(in srgb, ${theme.palette.mode === 'light' ? 'rgb(0 0 0)' : 'rgb(255 255 255)'} ${Math.round(theme.palette.action.hoverOpacity * 100)}%, ${bgBase})`;
                    const cellBase = {
                        display: "flex",
                        alignItems: "flex-start",
                        pt: "0.45rem",
                        pb: "0.45rem",
                        fontFamily: MONO_FONT,
                        fontSize: MONO_FONT_SIZE,
                        borderBottom: "1px solid",
                        borderColor: "divider",
                        minHeight: ROW_HEIGHT,
                        bgcolor: rowBg,
                        ".log-row:hover &": { bgcolor: hoverBg },
                    };
                    return (
                        <Box
                            key={rowKey}
                            className="log-row"
                            sx={{ display: "contents" }}
                        >
                            {showTimestamp && (
                                <Box sx={{
                                    ...cellBase, px: 2, color: "text.disabled",
                                    whiteSpace: "nowrap", position: "sticky", left: 0,
                                    zIndex: 1, bgcolor: stickyBg,
                                    ".log-row:hover &": { bgcolor: stickyHoverBg },
                                }}>
                                    {timestamp}
                                </Box>
                            )}
                            {showLogLevel && (
                                <Box sx={{
                                    ...cellBase, px: 2, color: levelColor, fontWeight: 600,
                                    whiteSpace: "nowrap", position: "sticky",
                                    left: showTimestamp ? TS_WIDTH : 0,
                                    zIndex: 1, bgcolor: stickyBg,
                                    ".log-row:hover &": { bgcolor: stickyHoverBg },
                                }}>
                                    {entry.logLevel}
                                </Box>
                            )}
                            <Box
                                component="pre"
                                sx={{
                                    ...cellBase,
                                    m: 0,
                                    px: 2,
                                    color: "text.primary",
                                    whiteSpace: wrap ? "pre-wrap" : "pre",
                                    wordBreak: "normal",
                                    overflowWrap: wrap ? "anywhere" : "normal",
                                    alignItems: "flex-start",
                                }}
                            >
                                {entry.log}
                            </Box>
                        </Box>
                    );
                })}
                {isLoadingDown && loadingSpan("loading-down", "Loading newer logs...")}
        </Box>
    );
};

const defaultEmptyState = {
    title: "No logs found",
    description: "Try adjusting your search or time range",
    illustration: <FileText size={64} />,
};

const SCROLL_THRESHOLD = 80; // px from edge to trigger load


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
    onSearch,
    search,
    showSearch = Boolean(onSearch),
    showTimestamp = true,
    showLogLevel = true,
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

    const handleScroll = useCallback(() => {
        const el = scrollContainerRef.current;
        if (!el) return;
        if (onLoadUp && hasMoreUp !== false && !isLoadingUp && el.scrollTop <= SCROLL_THRESHOLD) {
            onLoadUp();
        }
        if (onLoadDown && hasMoreDown !== false && !isLoadingDown &&
            el.scrollHeight - el.scrollTop - el.clientHeight <= SCROLL_THRESHOLD) {
            onLoadDown();
        }
    }, [onLoadUp, onLoadDown, hasMoreUp, hasMoreDown, isLoadingUp, isLoadingDown]);

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
                        <LogsPanelHeader
                            showTimestamp={showTimestamp}
                            showLogLevel={showLogLevel}
                        />
                        <Box
                            ref={scrollContainerRef}
                            onScroll={handleScroll}
                            sx={{ flex: 1, overflow: "auto", bgcolor: "background.default" }}
                        >
                            <LogsPanelRows
                                entries={reversedLogs}
                                wrap={wrap}
                                showTimestamp={showTimestamp}
                                showLogLevel={showLogLevel}
                                isLoadingUp={isLoadingUp}
                                isLoadingDown={isLoadingDown}
                            />
                        </Box>
                    </>
                )}
            </Paper>
        </Stack>
    );
}
