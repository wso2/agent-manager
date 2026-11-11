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

import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  Typography,
  Tooltip,
  ListingTable,
  Box,
  Button,
  CircularProgress,
  IconButton,
  Link,
  Skeleton,
  Stack,
} from "@wso2/oxygen-ui";
import { FadeIn, scoreColor } from "@agent-management-platform/views";
import { copyToClipboard } from "@agent-management-platform/shared-component";
import {
  TraceOverview,
} from "@agent-management-platform/types";
import {
  ArrowDown,
  CheckCircle,
  Copy,
  Workflow,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";
import { DEFAULT_TRACE_COLUMNS, type TraceColumn } from "../traceColumns";
import { formatStartTime } from "../traceTime";

interface TracesTableProps {
  traces: TraceOverview[];
  onTraceSelect?: (traceId: string) => void;
  selectedTrace: string | null;
  isLoading?: boolean;
  isLoadingMore?: boolean;
  hasMore?: boolean;
  hasActiveFilters?: boolean;
  // Optional columns to show; the rest are always shown.
  visibleColumns?: TraceColumn[];
  lookedBackTo?: string;
  // The last loadMore failed; auto-loading stops until Retry.
  loadError?: Error | null;
  // Resolves to whether the page added rows, or undefined if the list moved on.
  onLoadMore?: () => Promise<boolean | undefined>;
  onConversationSelect?: (conversationId: string) => void;
  sortOrder?: "asc" | "desc";
  onSortOrderChange?: (sortOrder: "asc" | "desc") => void;
}

// How far below the visible area the end of the list is when the next page starts loading.
// The page scrolls inside PageContent, which clips the sentinel, so a viewport rootMargin
// would have no effect; the sentinel is this tall instead.
const PRELOAD_DISTANCE_PX = 200;

// Placeholder rows shown while the first page loads.
const SKELETON_ROWS = 8;

const toNStoSeconds = (ns: number) => {
  return ns / 1000_000_000;
};

const ellipsisSx = {
  display: "block",
  textOverflow: "ellipsis",
  overflow: "hidden",
  whiteSpace: "nowrap",
  maxWidth: "100%",
} as const;

// Header label, width and alignment per column, in display order.
const COLUMNS: {
  field: string;
  headerName: string;
  width: number;
  align: "left" | "center" | "right";
  optional?: TraceColumn;
}[] = [
  { field: "status", headerName: "Status", width: 4, align: "center" },
  { field: "traceId", headerName: "Trace ID", width: 11, align: "left", optional: "traceId" },
  { field: "conversation", headerName: "Conversation", width: 11, align: "left", optional: "conversation" },
  { field: "input", headerName: "Input", width: 21, align: "left", optional: "input" },
  { field: "output", headerName: "Output", width: 21, align: "left", optional: "output" },
  { field: "startTime", headerName: "Start Time", width: 10, align: "center" },
  { field: "duration", headerName: "Duration", width: 6, align: "right" },
  { field: "tokens", headerName: "Tokens", width: 6, align: "right" },
  { field: "spans", headerName: "Spans", width: 5, align: "right" },
  { field: "score", headerName: "Score", width: 5, align: "right" },
];

// Line-style sort arrow matching the console's other icons; MUI rotates it for asc.
function SortArrow({ className }: { className?: string }) {
  return <ArrowDown size={14} className={className} />;
}

// Trace ID, truncated, with a button that copies it.
function TraceIdCell({ traceId }: { traceId: string }) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timerRef.current), []);
  /** Copies the ID and shows "Copied!" briefly. */
  const copy = async (e: React.MouseEvent) => {
    // The row opens the trace drawer; this click only copies.
    e.stopPropagation();
    if (!(await copyToClipboard(traceId))) return;
    setCopied(true);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setCopied(false), 1500);
  };
  return (
    <Stack direction="row" alignItems="center" spacing={0.5}>
      <Tooltip title={traceId}>
        <Typography
          variant="caption"
          component="span"
          sx={{ ...ellipsisSx, fontFamily: "monospace" }}
        >
          {traceId}
        </Typography>
      </Tooltip>
      <Tooltip title={copied ? "Copied!" : "Copy trace ID"}>
        <IconButton size="small" aria-label={`Copy trace ID ${traceId}`} onClick={copy}>
          <Copy size={14} />
        </IconButton>
      </Tooltip>
    </Stack>
  );
}

// Conversation ID, truncated, that sets the conversation filter on click.
function ConversationCell({
  conversationId,
  onSelect,
}: {
  conversationId?: string;
  onSelect?: (conversationId: string) => void;
}) {
  if (!conversationId) return null;
  return (
    <Tooltip title={conversationId}>
      {onSelect ? (
        <Link
          component="button"
          variant="caption"
          underline="hover"
          aria-label={`Filter by conversation ${conversationId}`}
          onClick={(e: React.MouseEvent) => {
            // The row opens the trace drawer; this click filters instead.
            e.stopPropagation();
            onSelect(conversationId);
          }}
          sx={{ ...ellipsisSx, textAlign: "left" }}
        >
          {conversationId}
        </Link>
      ) : (
        <Typography variant="caption" component="span" sx={ellipsisSx}>
          {conversationId}
        </Typography>
      )}
    </Tooltip>
  );
}
/** Trace list table with optional columns; loads the next page as its end scrolls into view. */
export function TracesTable({
  traces,
  onTraceSelect,
  selectedTrace,
  isLoading = false,
  isLoadingMore = false,
  hasMore = false,
  hasActiveFilters = false,
  visibleColumns = DEFAULT_TRACE_COLUMNS,
  lookedBackTo,
  loadError,
  onLoadMore,
  onConversationSelect,
  sortOrder = "desc",
  onSortOrderChange,
}: TracesTableProps) {
  const columns = COLUMNS.filter((c) => !c.optional || visibleColumns.includes(c.optional));
  const showTraceId = visibleColumns.includes("traceId");
  const showConversation = visibleColumns.includes("conversation");
  const showInput = visibleColumns.includes("input");
  const showOutput = visibleColumns.includes("output");

  const [sentinel, setSentinel] = useState<HTMLElement | null>(null);
  // Set when a load added no rows; auto-loading waits for a click.
  const [paused, setPaused] = useState(false);
  // A new object per load result, so the render that brings the load's rows can be told apart.
  const [loadResult, setLoadResult] = useState<{ added: boolean } | null>(null);
  const prevResultRef = useRef(loadResult);
  const prevTracesRef = useRef(traces);

  // A load from a list that has since been reset resolves undefined and is ignored.
  const loadMore = useCallback(async () => {
    const added = await onLoadMore?.();
    if (added !== undefined) setLoadResult({ added });
  }, [onLoadMore]);

  // Pause when a finished load added no rows, resume when one did, and reset on a new list.
  useEffect(() => {
    if (loadResult !== prevResultRef.current) {
      setPaused(!loadResult?.added);
    } else if (traces !== prevTracesRef.current && !isLoadingMore) {
      setPaused(false);
    }
    prevResultRef.current = loadResult;
    prevTracesRef.current = traces;
  }, [traces, isLoadingMore, loadResult]);

  const autoLoad = hasMore && !isLoadingMore && !paused && !loadError;

  // Loads once when the sentinel is in view. A new observer after each load checks again.
  useEffect(() => {
    if (!sentinel || !autoLoad) return;
    const observer = new IntersectionObserver(([entry]) => {
      if (!entry?.isIntersecting) return;
      observer.disconnect();
      loadMore();
    });
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [sentinel, autoLoad, loadMore]);

  const showMoreRow = hasMore || isLoadingMore || (hasActiveFilters && !!lookedBackTo);

  // Spinner, Retry after an error, or a Load More button when auto-loading is off.
  const moreStatus = (manual: boolean) => (
    <Stack direction="row" spacing={1} alignItems="center" justifyContent="center" minHeight={32}>
      {isLoadingMore ? (
        <CircularProgress size={16} />
      ) : loadError && hasMore ? (
        <>
          <Typography variant="caption" color="error">
            Couldn&apos;t load more traces.
          </Typography>
          <Button size="small" variant="text" onClick={loadMore}>
            Retry
          </Button>
        </>
      ) : manual && hasMore ? (
        <Button size="small" variant="text" onClick={loadMore} startIcon={<ArrowDown size={16} />}>
          Load more traces
        </Button>
      ) : null}
      {hasActiveFilters && lookedBackTo && (
        <Typography variant="caption" color="text.secondary">
          Searched as far as {formatStartTime(lookedBackTo)}
        </Typography>
      )}
    </Stack>
  );

  // Start Time sorts the list; clicking it flips the order.
  const head = (
    <ListingTable.Head>
      <ListingTable.Row>
        {columns.map((c) => (
          <ListingTable.Cell
            key={c.field}
            align={c.align}
            width={`${c.width}%`}
            sx={{ whiteSpace: "nowrap" }}
          >
            {c.field === "startTime" ? (
              <ListingTable.SortLabel
                active
                direction={sortOrder}
                IconComponent={SortArrow}
                onClick={() => onSortOrderChange?.(sortOrder === "desc" ? "asc" : "desc")}
              >
                {c.headerName}
              </ListingTable.SortLabel>
            ) : (
              c.headerName
            )}
          </ListingTable.Cell>
        ))}
      </ListingTable.Row>
    </ListingTable.Head>
  );

  return (
    <FadeIn>
      {isLoading ? (
        <ListingTable.Container>
          <ListingTable>
            {head}
            <ListingTable.Body>
              {Array.from({ length: SKELETON_ROWS }, (_, i) => (
                <ListingTable.Row key={i}>
                  {columns.map((c) => (
                    <ListingTable.Cell key={c.field}>
                      <Skeleton variant="text" />
                    </ListingTable.Cell>
                  ))}
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      ) : traces.length > 0 ? (
        <ListingTable.Container>
          <ListingTable>
            {head}
            <ListingTable.Body>
              {traces.map((trace) => (
                <ListingTable.Row
                  key={trace.traceId}
                  hover
                  selected={selectedTrace === trace.traceId}
                  clickable
                  onClick={() => onTraceSelect?.(trace.traceId)}
                >
                  <ListingTable.Cell
                    align="center"
                    sx={{
                      color: (theme) =>
                        trace.status?.errorCount && trace.status.errorCount > 0
                          ? theme.palette.error.main
                          : theme.palette.success.main,
                      maxWidth: 20,
                    }}
                  >
                    <Tooltip
                      title={`${trace.status?.errorCount} errors found`}
                      disableHoverListener={
                        !trace.status?.errorCount ||
                        trace.status?.errorCount === 0
                      }
                    >
                      {trace.status?.errorCount &&
                      trace.status.errorCount > 0 ? (
                        <XCircle size={16} />
                      ) : (
                        <CheckCircle size={16} />
                      )}
                    </Tooltip>
                  </ListingTable.Cell>
                  {showTraceId && (
                    <ListingTable.Cell align="left" sx={{ maxWidth: 160 }}>
                      <TraceIdCell traceId={trace.traceId} />
                    </ListingTable.Cell>
                  )}
                  {showConversation && (
                    <ListingTable.Cell align="left" sx={{ maxWidth: 160 }}>
                      <ConversationCell
                        conversationId={trace.conversationId}
                        onSelect={onConversationSelect}
                      />
                    </ListingTable.Cell>
                  )}
                  {showInput && (
                    <ListingTable.Cell align="left" sx={{ maxWidth: 200 }}>
                      <Tooltip
                        title="Preview only. Open the trace for the full input."
                        disableHoverListener={!trace.input}
                      >
                        <Typography
                          variant="caption"
                          component="span"
                          sx={{
                            display: "block",
                            textOverflow: "ellipsis",
                            overflow: "hidden",
                            whiteSpace: "nowrap",
                            maxWidth: "100%",
                          }}
                        >
                          {trace.input}
                        </Typography>
                      </Tooltip>
                    </ListingTable.Cell>
                  )}
                  {showOutput && (
                    <ListingTable.Cell align="left" sx={{ maxWidth: 200 }}>
                      <Tooltip
                        title="Preview only. Open the trace for the full output."
                        disableHoverListener={!trace.output}
                      >
                        <Typography
                          variant="caption"
                          component="span"
                          sx={{
                            display: "block",
                            textOverflow: "ellipsis",
                            overflow: "hidden",
                            whiteSpace: "nowrap",
                            maxWidth: "100%",
                          }}
                        >
                          {trace.output}
                        </Typography>
                      </Tooltip>
                    </ListingTable.Cell>
                  )}
                  <ListingTable.Cell align="center">
                    <Typography variant="caption" component="span" sx={ellipsisSx}>
                      {formatStartTime(trace.startTime)}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    <Typography variant="caption" component="span">
                      {toNStoSeconds(trace.durationInNanos).toFixed(2)}s
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    {(() => {
                      const tu = trace.tokenUsage;
                      // null-check rather than truthy: a legitimate 0-token
                      // trace (e.g. error path) should still render "0", not "-".
                      const hasTotal = tu?.totalTokens != null;
                      // partial=true means the trace had more LLM leaves than
                      // the list view aggregates; render an approximate marker
                      // and an explanatory tooltip.
                      const tooltip = hasTotal
                        ? tu?.partial
                          ? "Approximate total. This trace has more LLM spans than the list view aggregates. Open the trace for the exact total."
                          : `${tu?.inputTokens} input tokens, ${tu?.outputTokens} output tokens`
                        : "";
                      return (
                        <Tooltip
                          disableHoverListener={!hasTotal}
                          title={tooltip}
                        >
                          <Typography variant="caption" component="span">
                            {hasTotal ? (
                              <>
                                {tu?.totalTokens}
                                {tu?.partial ? "+" : null}
                              </>
                            ) : (
                              "-"
                            )}
                          </Typography>
                        </Tooltip>
                      );
                    })()}
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    <Typography variant="caption" component="span">
                      {trace.spanCount}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    {(() => {
                      const scoreSummary = trace.score;
                      if (!scoreSummary || scoreSummary.score == null) {
                        return (
                          <Typography variant="caption" component="span">
                            -
                          </Typography>
                        );
                      }
                      return (
                        <Tooltip
                          title={`${scoreSummary.totalCount} evaluations, ${scoreSummary.skippedCount} skipped`}
                        >
                          <Typography
                            variant="caption"
                            component="span"
                            sx={{
                              color: scoreColor(scoreSummary.score),
                              fontWeight: 600,
                            }}
                          >
                            {(scoreSummary.score * 100).toFixed(1)}%
                          </Typography>
                        </Tooltip>
                      );
                    })()}
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
              {showMoreRow && (
                <ListingTable.Row>
                  <ListingTable.Cell
                    colSpan={columns.length}
                    align="center"
                    sx={{ position: "relative" }}
                  >
                    <Box
                      ref={setSentinel}
                      data-testid="traces-sentinel"
                      aria-hidden
                      sx={{
                        position: "absolute",
                        left: 0,
                        right: 0,
                        bottom: 0,
                        height: PRELOAD_DISTANCE_PX,
                        pointerEvents: "none",
                      }}
                    />
                    {moreStatus(paused)}
                  </ListingTable.Cell>
                </ListingTable.Row>
              )}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      ) : (
        <ListingTable.Container>
          {/* The header stays so the sort order can still change. */}
          <ListingTable>{head}</ListingTable>
          <ListingTable.EmptyState
            illustration={<Workflow size={64} />}
            title="No traces found!"
            description={
              hasActiveFilters
                ? "Try changing the filters or the time range"
                : "Try changing the time range"
            }
          />
          {/* A filtered page can be empty while later pages hold matches; load those on click. */}
          {showMoreRow && <Box sx={{ pb: 2 }}>{moreStatus(true)}</Box>}
        </ListingTable.Container>
      )}
    </FadeIn>
  );
}
