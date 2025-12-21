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
  Box,
  TableContainer,
  TableHead,
  TableRow,
  TableBody,
  TableCell,
  Paper,
  Tooltip,
  TablePagination,
} from "@wso2/oxygen-ui";
import { FadeIn, NoDataFound } from "@agent-management-platform/views";
import { TraceOverview } from "@agent-management-platform/types";
import { BadgeCent, CheckCircle, Workflow, XCircle } from "@wso2/oxygen-ui-icons-react";
import dayjs from "dayjs";

interface TracesTableProps {
  traces: TraceOverview[];
  onTraceSelect?: (traceId: string) => void;
  count: number;
  page: number;
  rowsPerPage: number;
  onPageChange: (page: number) => void;
  onRowsPerPageChange: (rowsPerPage: number) => void;
  selectedTrace: string | null;
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
}: TracesTableProps) {
  return (
    <FadeIn>
      {traces.length > 0 && (
        <Box sx={{ borderRadius: 1, backgroundColor: "background.paper" }}>
          <TableContainer component={Paper}>
            <TableHead>
              <TableRow>
                <TableCell align="center" sx={{ width: "10%", maxWidth: 20 }}>
                  Status
                </TableCell>
                <TableCell align="left" sx={{ width: "10%" }}>
                  Name
                </TableCell>
                <TableCell align="left" sx={{ width: "20%" }}>
                  Input
                </TableCell>
                <TableCell align="left" sx={{ width: "20%" }}>
                  Output
                </TableCell>
                <TableCell align="center" sx={{ width: "10%" }}>
                  Start Time
                </TableCell>
                <TableCell align="right" sx={{ width: "10%" }}>
                  Duration
                </TableCell>
                <TableCell align="right" sx={{ width: "10%" }}>
                  Spans
                </TableCell>
                <TableCell align="right" sx={{ width: "10%" }}>
                  Tokens
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody sx={{ width: "100%" }}>
              {traces.map((trace) => (
                <TableRow
                  hover
                  selected={selectedTrace === trace.traceId}
                  key={trace.traceId}
                  sx={{
                    "&:last-child td, &:last-child th": { border: 0 },
                    cursor: "pointer",
                  }}
                  onClick={() => onTraceSelect?.(trace.traceId)}
                >
                  <TableCell
                    align="center"
                    sx={{
                      color: (theme) =>
                        trace.status?.errorCount && trace.status.errorCount > 0
                          ? theme.palette.error.main
                          : theme.palette.success.main,
                      maxWidth: 20,
                    }}
                  >
                    {trace.status?.errorCount && trace.status.errorCount > 0 ? (
                      <XCircle size={16} />
                    ) : (
                      <CheckCircle size={16} />
                    )}
                  </TableCell>
                  <TableCell align="left" sx={{ width: "10%" }}>
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
                  </TableCell>
                  <TableCell align="left" sx={{ width: "20%", maxWidth: 200 }}>
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
                  </TableCell>
                  <TableCell align="left" sx={{ width: "25%", maxWidth: 200 }}>
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
                  </TableCell>
                  <TableCell align="center" sx={{ width: "10%" }}>
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
                      {dayjs(trace.startTime).format("YYYY-MM-DD HH:mm:ss")}
                    </Typography>
                  </TableCell>
                  <TableCell align="right" sx={{ width: "10%" }}>
                    <Typography variant="caption" component="span">
                      {toNStoSeconds(trace.durationInNanos).toFixed(2)}s
                    </Typography>
                  </TableCell>
                  <TableCell align="right" sx={{ width: "10%" }}>
                    <Typography variant="caption" component="span">
                      {trace.spanCount}
                    </Typography>
                  </TableCell>
                  <TableCell align="right" sx={{ width: "10%" }}>
                    <Typography variant="caption" component="span">
                      {trace.tokenUsage?.totalTokens}&nbsp;
                    </Typography>
                    {trace.tokenUsage?.totalTokens && <BadgeCent size={10} />}
                    {!trace.tokenUsage?.totalTokens && "-"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
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
          </TableContainer>
        </Box>
      )}
      {traces.length === 0 && (
        <NoDataFound
          message="No traces found!"
          icon={<Workflow size={32} />}
          subtitle="Try changing the time range"
        />
      )}
    </FadeIn>
  );
}
