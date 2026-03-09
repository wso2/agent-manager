/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import React, { useMemo } from "react";
import { generatePath, Link, useParams } from "react-router-dom";
import {
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import {
  Activity,
  AlertTriangle,
  CheckCircle,
  CircleAlert,
  History,
} from "@wso2/oxygen-ui-icons-react";
import { useListMonitorRuns } from "@agent-management-platform/api-client";
import {
  type EvaluatorScoreSummary,
  type MonitorRunResponse,
  absoluteRouteMap,
} from "@agent-management-platform/types";
import ScoreChip from "./ScoreChip";


const getRunScoreDisplay = (scores?: EvaluatorScoreSummary[]) => {
  if (!scores || scores.length === 0) {
    return { averageScore: 0, tooltipContent: undefined };
  }
  const total = scores.reduce(
    (acc, evaluator) =>
      acc + ((evaluator.aggregations?.["mean"] as number) ?? 0),
    0,
  );
  const tooltipContent = scores
    .map(
      (evaluator) =>
        `${evaluator.evaluatorName}: ${(((evaluator.aggregations?.["mean"] as number) ?? 0) * 100).toFixed(2)}%`,
    )
    .join("\n");
  return {
    averageScore: (total * 100) / scores.length,
    tooltipContent,
  };
};

export default function RunSummaryCard() {
  const { orgId, projectId, agentId, monitorId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    monitorId: string;
  }>();
  const theme = useTheme();

  const { data, isLoading, error } = useListMonitorRuns(
    {
      monitorName: monitorId ?? "",
      orgName: orgId ?? "",
      projName: projectId ?? "",
      agentName: agentId ?? "",
    },
    { limit: 5, includeScores: true },
  );

  const latestRuns: MonitorRunResponse[] = useMemo(
    () => data?.runs ?? [],
    [data],
  );

  const palette = theme.vars?.palette;

  const statusColors: Record<string, { icon: React.ReactNode; label: string }> =
    useMemo(
      () => ({
        failed: {
          icon: <CircleAlert size={14} color={palette?.error.main} />,
          label: "Failed",
        },
        success: {
          icon: <CheckCircle size={14} color={palette?.success.main} />,
          label: "Success",
        },
        running: {
          icon: <CircularProgress size={14} />,
          label: "Running",
        },
        pending: {
          icon: <CircularProgress size={14} />,
          label: "Pending",
        },
      }),
      [palette?.error.main, palette?.success.main],
    );

  const runHistoryHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children
      .evaluation.children.monitor.children.view.children.runs.path,
    {
      orgId: orgId ?? "",
      projectId: projectId ?? "",
      agentId: agentId ?? "",
      monitorId: monitorId ?? "",
    },
  );

  return (
    <Card variant="outlined" sx={{ flex: 1, minHeight: 0 }}>
      <CardContent>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          mb={1}
        >
          <Typography variant="subtitle1">Run Summary</Typography>
          <Button
            size="small"
            variant="text"
            startIcon={<History size={14} />}
            component={Link}
            to={runHistoryHref}
          >
            View all runs
          </Button>
        </Stack>
        {isLoading ? (
          <Stack spacing={1.5} mt={1.5}>
            <Skeleton variant="rounded" height={40} />
            <Skeleton variant="rounded" height={40} />
            <Skeleton variant="rounded" height={40} />
          </Stack>
        ) : error ? (
          <Stack spacing={1.5} mt={1.5} alignItems="center">
            <AlertTriangle size={24} color={palette?.error.main} />
            <Typography
              variant="caption"
              color="text.secondary"
              textAlign="center"
            >
              Failed to load latest runs.
            </Typography>
          </Stack>
        ) : latestRuns.length === 0 ? (
          <Box
            display="flex"
            flexDirection="column"
            alignItems="center"
            justifyContent="center"
            py={4}
            gap={1}
          >
            <Activity size={36} />
            <Typography variant="body2" fontWeight={500}>
              No runs yet
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              textAlign="center"
            >
              Run this monitor to see recent activity.
            </Typography>
          </Box>
        ) : (
          <Table size="small" sx={{ mt: 1 }}>
            <TableHead>
              <TableRow>
                <TableCell sx={{ width: 16 }} />
                <TableCell>
                  <Typography variant="caption">Window Start</Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="caption">Run Score</Typography>
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {latestRuns.map((run) => {
                const statusKey = run.status ?? "pending";
                const status = statusColors[statusKey] ?? statusColors.pending;
                const traceStart = run.traceStart
                  ? new Date(run.traceStart).toLocaleString()
                  : "-";
                const isInProgress =
                  statusKey === "running" || statusKey === "pending";
                const { averageScore, tooltipContent } = isInProgress
                  ? { averageScore: 0, tooltipContent: undefined }
                  : getRunScoreDisplay(run.scores);
                return (
                  <TableRow key={run.id}>
                    <TableCell sx={{ width: 16 }}>{status.icon}</TableCell>
                    <TableCell>
                      <Typography variant="caption" noWrap>
                        {traceStart}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      {isInProgress ? (
                        "--"
                      ) : (
                        <Tooltip title={tooltipContent}>
                          <span>
                            <ScoreChip
                              score={averageScore / 100}
                              variant="text"
                              decimals={2}
                            />
                          </span>
                        </Tooltip>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
