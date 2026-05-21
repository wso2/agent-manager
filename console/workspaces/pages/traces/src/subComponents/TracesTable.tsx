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

import {
  Typography,
  Tooltip,
  ListingTable,
  DataGrid,
  Button,
  CircularProgress,
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

interface TracesTableProps {
  traces: TraceOverview[];
  onTraceSelect?: (traceId: string) => void;
  sortOrder?: "asc" | "desc";
  selectedTrace: string | null;
  isLoading?: boolean;
  isLoadingOlder?: boolean;
  isLoadingNewer?: boolean;
  onLoadOlder?: () => void;
  onLoadNewer?: () => void;
}

const toNStoSeconds = (ns: number) => {
  return ns / 1000_000_000;
};
export function TracesTable({
  traces,
  onTraceSelect,
  sortOrder = "desc",
  selectedTrace,
  isLoading = false,
  isLoadingOlder = false,
  isLoadingNewer = false,
  onLoadOlder,
  onLoadNewer,
}: TracesTableProps) {
  const isDesc = sortOrder === "desc";
  const topLabel = isDesc ? "Load Newer Traces" : "Load Older Traces";
  const topOnClick = isDesc ? onLoadNewer : onLoadOlder;
  const topDisabled = isDesc ? (!onLoadNewer || isLoadingNewer) : (!onLoadOlder || isLoadingOlder);
  const topLoading = isDesc ? isLoadingNewer : isLoadingOlder;

  const bottomLabel = isDesc ? "Load Older Traces" : "Load Newer Traces";
  const bottomOnClick = isDesc ? onLoadOlder : onLoadNewer;
  const bottomDisabled = isDesc
    ? !onLoadOlder || isLoadingOlder
    : !onLoadNewer || isLoadingNewer;
  const bottomLoading = isDesc ? isLoadingOlder : isLoadingNewer;
  return (
    <FadeIn>
      {isLoading ? (
        <DataGridComponent
          rows={[]}
          columns={[
            { field: "status", headerName: "Status", flex: 5 },
            { field: "name", headerName: "Name", flex: 10 },
            { field: "input", headerName: "Input", flex: 18 },
            { field: "output", headerName: "Output", flex: 18 },
            { field: "startTime", headerName: "Start Time", flex: 12 },
            { field: "duration", headerName: "Duration", flex: 8 },
            { field: "tokens", headerName: "Tokens", flex: 8 },
            { field: "spans", headerName: "Spans", flex: 8 },
            { field: "score", headerName: "Score", flex: 8 },
          ]}
          loading
          hideFooter
        />
      ) : traces.length > 0 ? (
        <ListingTable.Container>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell
                  align="center"
                  width="5%"
                  sx={{ maxWidth: 20 }}
                >
                  Status
                </ListingTable.Cell>
                <ListingTable.Cell align="left" width="10%">
                  Name
                </ListingTable.Cell>
                <ListingTable.Cell align="left" width="18%">
                  Input
                </ListingTable.Cell>
                <ListingTable.Cell align="left" width="18%">
                  Output
                </ListingTable.Cell>
                <ListingTable.Cell align="center" width="12%">
                  Start Time
                </ListingTable.Cell>
                <ListingTable.Cell align="right" width="8%">
                  Duration
                </ListingTable.Cell>
                <ListingTable.Cell align="right" width="8%">
                  Tokens
                </ListingTable.Cell>
                <ListingTable.Cell align="right" width="8%">
                  Spans
                </ListingTable.Cell>
                <ListingTable.Cell align="right" width="8%">
                  Score
                </ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              <ListingTable.Row>
                <ListingTable.Cell colSpan={9} align="center">
                  <Button
                    size="small"
                    variant="text"
                    disabled={topDisabled}
                    onClick={topOnClick}
                    startIcon={
                      topLoading ? (
                        <CircularProgress size={16} />
                      ) : (
                        <ArrowUp size={16} />
                      )
                    }
                  >
                    {topLoading ? "Loading..." : topLabel}
                  </Button>
                </ListingTable.Cell>
              </ListingTable.Row>
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
                  <ListingTable.Cell align="center">
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
                      {format(new Date(trace.startTime), "yyyy-MM-dd HH:mm:ss")}
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
              <ListingTable.Row>
                <ListingTable.Cell colSpan={9} align="center">
                  <Button
                    size="small"
                    variant="text"
                    disabled={bottomDisabled}
                    onClick={bottomOnClick}
                    startIcon={
                      bottomLoading ? (
                        <CircularProgress size={16} />
                      ) : (
                        <ArrowDown size={16} />
                      )
                    }
                  >
                    {bottomLoading ? "Loading..." : bottomLabel}
                  </Button>
                </ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      ) : (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Workflow size={64} />}
            title="No traces found!"
            description="Try changing the time range"
          />
        </ListingTable.Container>
      )}
    </FadeIn>
  );
}
