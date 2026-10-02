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

import React from "react";
import {
  Typography,
  Tooltip,
  ListingTable,
  DataGrid,
  Button,
  Chip,
  CircularProgress,
  Link,
  Stack,
} from "@wso2/oxygen-ui";
import { FadeIn, scoreColor } from "@agent-management-platform/views";

const { DataGrid: DataGridComponent } = DataGrid;
import {
  TraceOverview,
} from "@agent-management-platform/types";
import {
  ArrowDown,
  ArrowUp,
  CheckCircle,
  Workflow,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";
import { format } from "date-fns";
import { DEFAULT_TRACE_COLUMNS, type TraceColumn } from "../traceColumns";

interface TracesTableProps {
  traces: TraceOverview[];
  onTraceSelect?: (traceId: string) => void;
  sortOrder?: "asc" | "desc";
  selectedTrace: string | null;
  isLoading?: boolean;
  isLoadingOlder?: boolean;
  isLoadingNewer?: boolean;
  hasOlder?: boolean;
  hasActiveFilters?: boolean;
  // Optional columns to show; the rest are always shown.
  visibleColumns?: TraceColumn[];
  lookedBackTo?: string;
  onLoadOlder?: () => void;
  onLoadNewer?: () => void;
  onConversationSelect?: (conversationId: string) => void;
}

const toNStoSeconds = (ns: number) => {
  return ns / 1000_000_000;
};

const formatStartTime = (time: string) => format(new Date(time), "yyyy-MM-dd HH:mm:ss");

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
  { field: "name", headerName: "Name", width: 10, align: "left" },
  { field: "input", headerName: "Input", width: 17, align: "left" },
  { field: "output", headerName: "Output", width: 17, align: "left" },
  { field: "conversation", headerName: "Conversation", width: 10, align: "left", optional: "conversation" },
  { field: "model", headerName: "Model", width: 9, align: "left", optional: "model" },
  { field: "startTime", headerName: "Start Time", width: 11, align: "center" },
  { field: "duration", headerName: "Duration", width: 6, align: "right" },
  { field: "tokens", headerName: "Tokens", width: 6, align: "right" },
  { field: "spans", headerName: "Spans", width: 5, align: "right" },
  { field: "score", headerName: "Score", width: 5, align: "right" },
];

// One model plainly; two or more as the first plus a "+N" chip listing all.
function ModelsCell({ models }: { models?: string[] }) {
  if (!models?.length) return null;
  const [first, ...rest] = models;
  return (
    <Stack direction="row" spacing={0.5} alignItems="center" sx={{ minWidth: 0 }}>
      <Typography variant="caption" component="span" title={first} sx={{ ...ellipsisSx, minWidth: 0 }}>
        {first}
      </Typography>
      {rest.length > 0 && (
        <Tooltip title={models.join(", ")}>
          <Chip label={`+${rest.length}`} size="small" variant="outlined" sx={{ flexShrink: 0 }} />
        </Tooltip>
      )}
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
export function TracesTable({
  traces,
  onTraceSelect,
  sortOrder = "desc",
  selectedTrace,
  isLoading = false,
  isLoadingOlder = false,
  isLoadingNewer = false,
  hasOlder = false,
  hasActiveFilters = false,
  visibleColumns = DEFAULT_TRACE_COLUMNS,
  lookedBackTo,
  onLoadOlder,
  onLoadNewer,
  onConversationSelect,
}: TracesTableProps) {
  const columns = COLUMNS.filter((c) => !c.optional || visibleColumns.includes(c.optional));
  const showConversation = visibleColumns.includes("conversation");
  const showModel = visibleColumns.includes("model");
  const isDesc = sortOrder === "desc";

  // Load older, shown only while the server has an older page, plus how far a filtered list looked.
  const olderControl = (hasOlder && onLoadOlder) || (hasActiveFilters && lookedBackTo) ? (
    <Stack direction="row" spacing={1} alignItems="center" justifyContent="center">
      {hasOlder && onLoadOlder && (
        <Button
          size="small"
          variant="text"
          disabled={isLoadingOlder}
          onClick={onLoadOlder}
          startIcon={
            isLoadingOlder ? (
              <CircularProgress size={16} />
            ) : isDesc ? (
              <ArrowDown size={16} />
            ) : (
              <ArrowUp size={16} />
            )
          }
        >
          {isLoadingOlder ? "Loading..." : "Load Older Traces"}
        </Button>
      )}
      {hasActiveFilters && lookedBackTo && (
        <Typography variant="caption" color="text.secondary">
          Looked back to {formatStartTime(lookedBackTo)}
        </Typography>
      )}
    </Stack>
  ) : null;

  const newerControl = (
    <Button
      size="small"
      variant="text"
      disabled={!onLoadNewer || isLoadingNewer}
      onClick={onLoadNewer}
      startIcon={
        isLoadingNewer ? (
          <CircularProgress size={16} />
        ) : isDesc ? (
          <ArrowUp size={16} />
        ) : (
          <ArrowDown size={16} />
        )
      }
    >
      {isLoadingNewer ? "Loading..." : "Load Newer Traces"}
    </Button>
  );

  const topControl = isDesc ? newerControl : olderControl;
  const bottomControl = isDesc ? olderControl : newerControl;
  return (
    <FadeIn>
      {isLoading ? (
        <DataGridComponent
          rows={[]}
          columns={columns.map(({ field, headerName, width }) => ({
            field,
            headerName,
            flex: width,
          }))}
          loading
          hideFooter
        />
      ) : traces.length > 0 ? (
        <ListingTable.Container>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                {columns.map((c) => (
                  <ListingTable.Cell
                    key={c.field}
                    align={c.align}
                    width={`${c.width}%`}
                    sx={c.field === "status" ? { maxWidth: 20 } : undefined}
                  >
                    {c.headerName}
                  </ListingTable.Cell>
                ))}
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {topControl && (
                <ListingTable.Row>
                  <ListingTable.Cell colSpan={columns.length} align="center">
                    {topControl}
                  </ListingTable.Cell>
                </ListingTable.Row>
              )}
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
                  <ListingTable.Cell align="left">
                    <Typography
                      variant="caption"
                      component="span"
                      sx={{
                        display: "block",
                        textOverflow: "ellipsis",
                        overflow: "hidden",
                        whiteSpace: "nowrap",
                        maxWidth: "300px",
                      }}
                    >
                      {trace.rootSpanName}
                    </Typography>
                  </ListingTable.Cell>
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
                  {showConversation && (
                    <ListingTable.Cell align="left" sx={{ maxWidth: 160 }}>
                      <ConversationCell
                        conversationId={trace.conversationId}
                        onSelect={onConversationSelect}
                      />
                    </ListingTable.Cell>
                  )}
                  {showModel && (
                    <ListingTable.Cell align="left" sx={{ maxWidth: 160 }}>
                      <ModelsCell models={trace.models} />
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
              {bottomControl && (
                <ListingTable.Row>
                  <ListingTable.Cell colSpan={columns.length} align="center">
                    {bottomControl}
                  </ListingTable.Cell>
                </ListingTable.Row>
              )}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      ) : (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Workflow size={64} />}
            title="No traces found!"
            description={
              hasActiveFilters
                ? "Try changing the filters or the time range"
                : "Try changing the time range"
            }
          />
          {/* A filtered page can be empty while older pages still hold matches. */}
          {olderControl && <Stack sx={{ pb: 2 }}>{olderControl}</Stack>}
        </ListingTable.Container>
      )}
    </FadeIn>
  );
}
