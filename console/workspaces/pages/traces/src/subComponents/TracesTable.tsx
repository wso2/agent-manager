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
  TablePagination,
  ListingTable,
  DataGrid,
} from "@wso2/oxygen-ui";
import { FadeIn } from "@agent-management-platform/views";

const { DataGrid: DataGridComponent } = DataGrid;
import { TraceOverview } from "@agent-management-platform/types";
import { CheckCircle, Workflow, XCircle } from "@wso2/oxygen-ui-icons-react";
import { format } from "date-fns";

interface TracesTableProps {
  traces: TraceOverview[];
  onTraceSelect?: (traceId: string) => void;
  count: number;
  page: number;
  rowsPerPage: number;
  onPageChange: (page: number) => void;
  onRowsPerPageChange: (rowsPerPage: number) => void;
  selectedTrace: string | null;
  isLoading?: boolean;
}

const toNStoSeconds = (ns: number) => {
  return ns / 1000_000_000;
};
export function TracesTable({
  traces,
  onTraceSelect,
  count,
  page,
  rowsPerPage,
  onPageChange,
  onRowsPerPageChange,
  selectedTrace,
  isLoading = false,
}: TracesTableProps) {
  return (
    <FadeIn>
      {isLoading ? (
        <DataGridComponent
          rows={[]}
          columns={[
            { field: 'status', headerName: 'Status', flex: 0.5 },
            { field: 'name', headerName: 'Name', flex: 1 },
            { field: 'input', headerName: 'Input', flex: 2 },
            { field: 'output', headerName: 'Output', flex: 2 },
            { field: 'startTime', headerName: 'Start Time', flex: 1 },
            { field: 'duration', headerName: 'Duration', flex: 1 },
            { field: 'tokens', headerName: 'Tokens', flex: 1 },
            { field: 'spans', headerName: 'Spans', flex: 1 },
          ]}
          loading
          hideFooter
        />
      ) : traces.length > 0 ? (
        <ListingTable.Container>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell align="center" width="10%" sx={{ maxWidth: 20 }}>
                  Status
                </ListingTable.Cell>
                <ListingTable.Cell align="left" width="10%">
                  Name
                </ListingTable.Cell>
                <ListingTable.Cell align="left" width="20%">
                  Input
                </ListingTable.Cell>
                <ListingTable.Cell align="left" width="20%">
                  Output
                </ListingTable.Cell>
                <ListingTable.Cell align="center" width="10%">
                  Start Time
                </ListingTable.Cell>
                <ListingTable.Cell
                  align="right"
                  width="10%"
                  sx={{ maxWidth: 100, minWidth: 80 }}
                >
                  Duration
                </ListingTable.Cell>
                <ListingTable.Cell
                  align="right"
                  width="10%"
                  sx={{ maxWidth: 100, minWidth: 80 }}
                >
                  Tokens
                </ListingTable.Cell>
                <ListingTable.Cell
                  align="right"
                  width="10%"
                  sx={{ maxWidth: 100, minWidth: 80 }}
                >
                  Spans
                </ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
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
                  <ListingTable.Cell align="left" sx={{ width: "10%" }}>
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
                      {trace.rootSpanName}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell align="left" sx={{ width: "20%", maxWidth: 200 }}>
                    <Tooltip title={trace.input}>
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
                  <ListingTable.Cell align="left" sx={{ width: "25%", maxWidth: 200 }}>
                    <Tooltip title={trace.output}>
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
                  <ListingTable.Cell align="center" sx={{ width: "10%" }}>
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
                  <ListingTable.Cell
                    align="right"
                    sx={{ width: "10%", maxWidth: 100, minWidth: 80 }}
                  >
                    <Typography variant="caption" component="span">
                      {toNStoSeconds(trace.durationInNanos).toFixed(2)}s
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell
                    align="right"
                    sx={{ width: "10%", maxWidth: 100, minWidth: 80 }}
                  >
                    <Tooltip
                      disableHoverListener={
                        !trace.tokenUsage?.totalTokens ||
                        trace.tokenUsage.totalTokens === 0
                      }
                      title={`${trace.tokenUsage?.inputTokens} input tokens, ${trace.tokenUsage?.outputTokens} output tokens`}
                    >
                      <Typography variant="caption" component="span">
                        {trace.tokenUsage?.totalTokens ? (
                          <>{trace.tokenUsage.totalTokens}</>
                        ) : (
                          "-"
                        )}
                      </Typography>
                    </Tooltip>
                  </ListingTable.Cell>
                  <ListingTable.Cell
                    align="right"
                    sx={{ width: "10%", maxWidth: 100, minWidth: 80 }}
                  >
                    <Typography variant="caption" component="span">
                      {trace.spanCount}
                    </Typography>
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
          <TablePagination
            rowsPerPageOptions={[5, 10, 25, 50]}
            component="div"
            count={count}
            rowsPerPage={rowsPerPage}
            page={page}
            onPageChange={(_event, newPage) => onPageChange(newPage)}
            onRowsPerPageChange={(event) =>
              onRowsPerPageChange(parseInt(event.target.value, 10))
            }
          />
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
