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

import React, { useMemo, useState } from "react";
import { PageLayout } from "@agent-management-platform/views";
import {
  Grid,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Skeleton,
  Stack,
} from "@wso2/oxygen-ui";
import { Clock, RefreshCcw } from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  Route,
  Routes,
  useParams,
  useSearchParams,
} from "react-router-dom";
import {
  absoluteRouteMap,
  relativeRouteMap,
  type EvaluatorScoreSummary,
  TraceListTimeRange,
  getTimeRange,
} from "@agent-management-platform/types";
import AgentPerformanceCard, {
  RadarDefinition,
} from "./subComponents/AgentPerformanceCard";
import EvaluationSummaryCard, {
  EvaluationSummaryItem,
} from "./subComponents/EvaluationSummaryCard";
import RunSummaryCard from "./subComponents/RunSummaryCard";
import PerformanceByEvaluatorCard from "./subComponents/PerformanceByEvaluatorCard";
import {
  useGetMonitor,
  useMonitorScores,
} from "@agent-management-platform/api-client";
import MonitorRunList from "./subComponents/MonitorRunList";

const MONITOR_TIME_RANGE_OPTIONS = [
  { value: TraceListTimeRange.ONE_DAY, label: "Last 24 Hours" },
  { value: TraceListTimeRange.THREE_DAYS, label: "Last 3 Days" },
  { value: TraceListTimeRange.SEVEN_DAYS, label: "Last 7 Days" },
  { value: TraceListTimeRange.THIRTY_DAYS, label: "Last 30 Days" },
];

/** Extract the numeric mean from an evaluator's aggregations map. */
const getMean = (e: EvaluatorScoreSummary): number | null => {
  const v = e.aggregations?.["mean"];
  return typeof v === "number" ? v : null;
};

export const ViewMonitorComponent: React.FC = () => {
  const { orgId, projectId, agentId, envId, monitorId } = useParams();

  const [searchParams, setSearchParams] = useSearchParams();

  const timeRange = useMemo(
    () =>
      (searchParams.get("timeRange") as TraceListTimeRange) ||
      TraceListTimeRange.SEVEN_DAYS,
    [searchParams],
  );

  const handleTimeRangeChange = React.useCallback(
    (value: TraceListTimeRange) => {
      const next = new URLSearchParams(searchParams);
      next.set("timeRange", value);
      setSearchParams(next, { replace: true });
    },
    [searchParams, setSearchParams],
  );
  const timeRangeLabel = useMemo(
    () =>
      MONITOR_TIME_RANGE_OPTIONS.find((o) => o.value === timeRange)?.label ??
      "Selected period",
    [timeRange],
  );
  const commonParams = useMemo(
    () => ({
      monitorName: monitorId ?? "",
      orgName: orgId ?? "",
      projName: projectId ?? "",
      agentName: agentId ?? "",
    }),
    [monitorId, orgId, projectId, agentId],
  );

  const [now, setNow] = useState(() => new Date());
  const defaultStartFallback = useMemo(
    () => new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000),
    [now],
  );
  const mainStartTime = useMemo(() => {
    const range = getTimeRange(timeRange);
    return range?.startTime ?? defaultStartFallback.toISOString();
  }, [timeRange, defaultStartFallback]);

  const {
    data: monitorData,
    refetch: refetchMonitor,
    isLoading: isMonitorLoading,
  } = useGetMonitor(commonParams);

  const {
    data: scoresMain,
    refetch: refetchMain,
    isLoading: isScoresMainLoading,
  } = useMonitorScores(commonParams, {
    startTime: mainStartTime,
    endTime: now.toISOString(),
  });

  const handleRefresh = () => {
    setNow(new Date());
    void refetchMonitor();
    void refetchMain();
  };

  const isLoading = isMonitorLoading || isScoresMainLoading;

  // ── raw evaluator arrays ─────────────────────────────────────────────────
  const evaluators = useMemo(() => scoresMain?.evaluators ?? [], [scoresMain]);

  // ── EvaluationSummaryCard ────────────────────────────────────────────────
  const evaluatorSummary = useMemo<EvaluationSummaryItem[]>(() => {
    const totalCount = evaluators.reduce((s, e) => s + e.count, 0);
    const totalFailed = evaluators.reduce((s, e) => s + e.skippedCount, 0);

    return [
      {
        label: "Traces Evaluated",
        value: totalCount.toLocaleString(),
        helper: timeRangeLabel,
      },
      {
        label: "Skipped Traces",
        value: totalFailed.toLocaleString(),
        helper: "Skipped traces/Total evaluator failures",
        rate: totalCount > 0 ? (totalFailed / totalCount) * 100 : 0 as number,
      },
    ];
  }, [evaluators, timeRangeLabel]);

  // Weighted average — weight each evaluator's mean by its trace count so
  // high-volume evaluators contribute proportionally.
  const averageScore = useMemo(() => {
    let weightedSum = 0;
    let totalWeight = 0;
    for (const e of evaluators) {
      const mean = getMean(e);
      if (mean !== null && e.count > 0) {
        weightedSum += mean * e.count;
        totalWeight += e.count;
      }
    }
    return totalWeight > 0 ? weightedSum / totalWeight : null;
  }, [evaluators]);

  const evaluationSummaryAverage = useMemo(() => {
    if (averageScore === null)
      return { value: "–", helper: "No data yet", progress: 0 };
    const scorePct = averageScore * 100;
    return {
      value: `${scorePct.toFixed(1)}%`,
      progress: Math.round(scorePct),
    };
  }, [averageScore]);

  // ── PerformanceByEvaluatorCard ───────────────────────────────────────────
  const evaluatorNames = useMemo(
    () => evaluators.map((e) => e.evaluatorName),
    [evaluators],
  );

  // ── AgentPerformanceCard (radar) ─────────────────────────────────────────
  const radarChartData = useMemo(
    () =>
      evaluators.map((e) => ({
        metric: e.evaluatorName,
        current: (getMean(e) ?? 0) * 100,
      })),
    [evaluators],
  );

  const radars = useMemo<RadarDefinition[]>(
    () => [
      {
        dataKey: "current",
        name: `Current (${timeRangeLabel})`,
        fillOpacity: 0.2,
        strokeWidth: 2,
      },
    ],
    [timeRangeLabel],
  );

  return (
    <Routes>
      <Route
        path={
          relativeRouteMap.children.org.children.projects.children.agents
            .children.evaluation.children.monitor.children.view.children.runs
            .path
        }
        element={
          <PageLayout
            title={`Run History ${monitorData?.displayName ? `(${monitorData.displayName})` : ""}`}
            disableIcon
            backLabel={`Back to ${monitorData?.displayName ?? "Monitor"}`}
            backHref={generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.evaluation.children.monitor.children.view.path,
              {
                orgId: orgId,
                projectId: projectId,
                monitorId: monitorId,
                agentId: agentId,
                envId: envId,
              },
            )}
          >
            <MonitorRunList />
          </PageLayout>
        }
      />
      <Route
        index
        element={
          <PageLayout
            title={
              monitorData?.displayName ?? monitorData?.name ?? "Monitor Details"
            }
            disableIcon
            backLabel="Back to Monitors"
            backHref={generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.evaluation.children.monitor.path,
              {
                orgId: orgId,
                projectId: projectId,
                agentId: agentId,
                envId: envId,
              },
            )}
            actions={
              <Stack direction="row" spacing={1} alignItems="center">
                <Select
                  size="small"
                  variant="outlined"
                  value={timeRange}
                  onChange={(e) =>
                    handleTimeRangeChange(e.target.value as TraceListTimeRange)
                  }
                  startAdornment={
                    <InputAdornment position="start">
                      <Clock size={16} />
                    </InputAdornment>
                  }
                  sx={{ minWidth: 140 }}
                >
                  {MONITOR_TIME_RANGE_OPTIONS.map((opt) => (
                    <MenuItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </MenuItem>
                  ))}
                </Select>
                <IconButton
                  size="small"
                  onClick={handleRefresh}
                  aria-label="Refresh"
                >
                  <RefreshCcw size={16} />
                </IconButton>
              </Stack>
            }
          >
            <Stack spacing={3}>
              {isLoading ? (
                <>
                  <Grid container spacing={3}>
                    <Grid size={{ xs: 12, md: 6 }}>
                      <Skeleton variant="rounded" height={480} />
                    </Grid>
                    <Grid size={{ xs: 12, md: 6 }}>
                      <Stack spacing={2}>
                        <Skeleton variant="rounded" height={280} />
                        <Skeleton variant="rounded" height={180} />
                      </Stack>
                    </Grid>
                  </Grid>
                  <Skeleton variant="rounded" height={360} />
                </>
              ) : (
                <>
                  <Grid container spacing={3}>
                    <Grid size={{ xs: 12, md: 6 }}>
                      <AgentPerformanceCard
                        radarChartData={radarChartData}
                        radars={radars}
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 6 }}>
                      <Stack spacing={2}>
                        <EvaluationSummaryCard
                          items={evaluatorSummary}
                          averageScoreValue={evaluationSummaryAverage.value}
                          averageScoreProgress={
                            evaluationSummaryAverage.progress
                          }
                        />
                        <RunSummaryCard />
                      </Stack>
                    </Grid>
                  </Grid>
                  <PerformanceByEvaluatorCard
                    evaluatorNames={evaluatorNames}
                    startTime={mainStartTime}
                    endTime={now.toISOString()}
                    environmentId={monitorData?.environmentName}
                  />
                </>
              )}
            </Stack>
          </PageLayout>
        }
      />
    </Routes>
  );
};

export default ViewMonitorComponent;
