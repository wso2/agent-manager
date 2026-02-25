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
import {
  useListMonitorRuns,
  useMonitorRunScores,
} from "@agent-management-platform/api-client";
import {
  type MonitorRunResponse,
  absoluteRouteMap,
} from "@agent-management-platform/types";

const formatDuration = (startedAt?: string, completedAt?: string) => {
  if (!startedAt) {
    return "-";
  }
  const startTime = Date.parse(startedAt);
  const endTime = completedAt ? Date.parse(completedAt) : Date.now();
  if (Number.isNaN(startTime) || Number.isNaN(endTime)) {
    return "-";
  }
  const diffMs = Math.max(endTime - startTime, 0);
  const totalSeconds = Math.floor(diffMs / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
};

const RunScore = (props: { runId: string }) => {
  const { runId } = props;
  const { orgId, projectId, agentId, monitorId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    monitorId: string;
  }>();
  const { data, isLoading } = useMonitorRunScores({
    monitorName: monitorId ?? "",
    orgName: orgId ?? "",
    projName: projectId ?? "",
    agentName: agentId ?? "",
    runId: runId,
  });
  const { averageScore, tooltipContent } = useMemo(() => {
    if (!data || !data.evaluators || data.evaluators.length === 0) {
      return { averageScore: 0 };
    }
    const total = data.evaluators.reduce(
      (acc, evaluator) =>
        acc + ((evaluator.aggregations?.["mean"] as number) ?? 0),
      0,
    );

    const tooltipContentText = data.evaluators
      .map((evaluator) => {
        return `${evaluator.evaluatorName}: ${(((evaluator.aggregations?.["mean"] as number) ?? 0) * 100).toFixed(2)}`;
      })
      .join("\n");

    return {
      averageScore: (total * 100) / data.evaluators.length,
      tooltipContent: tooltipContentText,
    };
  }, [data]);

  return (
    <Typography variant="body2">
      {isLoading ? (
        <Skeleton variant="text" width={50} height={20} />
      ) : (
        <Tooltip title={tooltipContent}>
          <Typography variant="body2">{averageScore.toFixed(2)}%</Typography>
        </Tooltip>
      )}
    </Typography>
  );
};
export default function RunSummaryCard() {
  const { orgId, projectId, agentId, monitorId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    monitorId: string;
  }>();
  const theme = useTheme();

  const { data, isLoading, error } = useListMonitorRuns({
    monitorName: monitorId ?? "",
    orgName: orgId ?? "",
    projName: projectId ?? "",
    agentName: agentId ?? "",
  });

  const runs = useMemo(() => data?.runs ?? [], [data]);

  const latestRuns: MonitorRunResponse[] = useMemo(() => {
    const sorted = [...runs].sort((a, b) => {
      const aTime = a.startedAt ? Date.parse(a.startedAt) : 0;
      const bTime = b.startedAt ? Date.parse(b.startedAt) : 0;
      return bTime - aTime;
    });
    return sorted.slice(0, 5);
  }, [runs]);

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
          icon: <Activity size={14} color={palette?.warning.main} />,
          label: "Running",
        },
        pending: {
          icon: <Activity size={14} />,
          label: "Pending",
        },
      }),
      [palette?.error.main, palette?.success.main, palette?.warning.main],
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
    <Card variant="outlined" sx={{ height: 280 }}>
      <CardContent>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          mb={1}
        >
          <Typography variant="subtitle2">Run Summary</Typography>
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
            <Activity size={32} />
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
                  <Typography variant="caption">Started</Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="caption">Duration</Typography>
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
                const started = run.startedAt
                  ? new Date(run.startedAt).toLocaleString()
                  : "-";
                return (
                  <TableRow key={run.id}>
                    <TableCell sx={{ width: 16 }}>{status.icon}</TableCell>
                    <TableCell>
                      <Typography variant="caption" noWrap>
                        {started}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="caption">
                        {formatDuration(run.startedAt, run.completedAt)}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <RunScore runId={run.id} />
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
