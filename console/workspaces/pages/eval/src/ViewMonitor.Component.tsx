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

import React, { useCallback, useMemo } from "react";
import { PageLayout } from "@agent-management-platform/views";
import {
  Chip,
  CircularProgress,
  Grid,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { Clock, RefreshCcw, Timer } from "@wso2/oxygen-ui-icons-react";
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
  type EvaluationLevel,
  type EvaluatorScoreSummary,
  TraceListTimeRange,
} from "@agent-management-platform/types";
import AgentPerformanceCard, {
  RadarDefinition,
} from "./subComponents/AgentPerformanceCard";
import EvaluationSummaryCard, {
  type LevelSummary,
} from "./subComponents/EvaluationSummaryCard";
import RunSummaryCard from "./subComponents/RunSummaryCard";
import PerformanceByEvaluatorCard from "./subComponents/PerformanceByEvaluatorCard";
import ScoreBreakdownCard from "./subComponents/ScoreBreakdownCard";
import {
  useGetMonitor,
  useGroupedScores,
  useMonitorScores,
} from "@agent-management-platform/api-client";
import { useQueryClient } from "@tanstack/react-query";
import MonitorRunList from "./subComponents/MonitorRunList";

const MONITOR_TIME_RANGE_OPTIONS = [
  { value: TraceListTimeRange.ONE_DAY, label: "Last 1 Day" },
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
  const theme = useTheme();
  const palette = theme.vars?.palette;
  const queryClient = useQueryClient();

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

  const {
    data: monitorData,
    refetch: refetchMonitor,
    isLoading: isMonitorLoading,
    isRefetching: isMonitorRefetching,
  } = useGetMonitor(commonParams);

  const isPastMonitor = monitorData?.type === "past";

  const scoreQueryParams = useMemo(
    () =>
      isPastMonitor && monitorData?.traceStart && monitorData?.traceEnd
        ? { startTime: monitorData.traceStart, endTime: monitorData.traceEnd }
        : { timeRange },
    [isPastMonitor, monitorData?.traceStart, monitorData?.traceEnd, timeRange],
  );

  const {
    data: scoresMain,
    refetch: refetchMain,
    isLoading: isScoresMainLoading,
    isRefetching: isScoresMainRefetching,
  } = useMonitorScores(commonParams, scoreQueryParams);

  const handleRefresh = useCallback(() => {
    void refetchMonitor();
    void refetchMain();
    void queryClient.invalidateQueries({ queryKey: ["monitor-runs"] });
    void queryClient.invalidateQueries({ queryKey: ["monitor-scores-timeseries-batch"] });
  }, [refetchMonitor, refetchMain, queryClient]);

  const isLoading = isMonitorLoading || isScoresMainLoading;
  const isRefetching = isMonitorRefetching || isScoresMainRefetching;

  // ── raw evaluator arrays ─────────────────────────────────────────────────
  const evaluators = useMemo(() => scoresMain?.evaluators ?? [], [scoresMain]);

  // ── Determine which levels are present ────────────────────────────────────
  const levelsPresent = useMemo(() => {
    const s = new Set<EvaluationLevel>();
    evaluators.forEach((e) => s.add(e.level));
    return s;
  }, [evaluators]);

  const hasAgentLevel = levelsPresent.has("agent");
  const hasLlmLevel = levelsPresent.has("llm");

  // ── EvaluationSummaryCard — per-level breakdown ───────────────────────────
  const levelSummaries = useMemo<LevelSummary[]>(() => {
    const levelOrder: EvaluationLevel[] = ["trace", "agent", "llm"];
    return levelOrder
      .filter((lvl) => levelsPresent.has(lvl))
      .map((lvl) => {
        const group = evaluators.filter((e) => e.level === lvl);
        return {
          level: lvl,
          evaluatorCount: group.length,
          uniqueCount: Math.max(...group.map((e) => e.count), 0),
          totalEvaluations: group.reduce((s, e) => s + e.count, 0),
          skippedCount: group.reduce((s, e) => s + e.skippedCount, 0),
        };
      });
  }, [evaluators, levelsPresent]);

  const averageScore = useMemo(() => {
    const means = evaluators
      .map(getMean)
      .filter((m): m is number => m !== null);
    if (means.length === 0) {
      return null;
    }
    const sum = means.reduce((acc, m) => acc + m, 0);
    return sum / means.length;
  }, [evaluators]);

  // ── PerformanceByEvaluatorCard ───────────────────────────────────────────
  const evaluatorInfoList = useMemo(
    () => evaluators.map((e) => ({ name: e.evaluatorName, level: e.level })),
    [evaluators],
  );

  // ── AgentPerformanceCard (radar) ─────────────────────────────────────────
  const radarChartData = useMemo(
    () =>
      evaluators.map((e) => {
        const mean = getMean(e);
        const scoredCount = e.count - e.skippedCount;
        return {
          metric: e.evaluatorName,
          current: (mean ?? 0) * 100,
          _isNoData: mean === null,
          _scoredCount: scoredCount,
          _totalCount: e.count,
          _level: e.level,
        };
      }),
    [evaluators],
  );

  const radars = useMemo<RadarDefinition[]>(
    () => [
      {
        dataKey: "current",
        name: `Current (${isPastMonitor && traceWindowLabel ? traceWindowLabel : timeRangeLabel})`,
        stroke: palette?.primary.main,
        fill: palette?.primary.main,
        fillOpacity: 0.2,
        strokeWidth: 2,
        dot: (props: {
          cx?: number;
          cy?: number;
          payload?: { _isNoData?: boolean };
        }) => {
          const { cx = 0, cy = 0, payload } = props;
          if (payload?._isNoData) {
            return (
              <circle
                cx={cx}
                cy={cy}
                r={5}
                fill={palette?.background.paper}
                stroke={palette?.action.disabled}
                strokeWidth={1.5}
                strokeDasharray="3 2"
              />
            );
          }
          return (
            <circle
              cx={cx}
              cy={cy}
              r={4}
              fill={palette?.background.paper}
              stroke={palette?.action.disabled}
              strokeWidth={1.5}
            />
          );
        },
        activeDot: (props: { cx?: number; cy?: number }) => {
          const { cx = 0, cy = 0 } = props;
          return (
            <circle
              cx={cx}
              cy={cy}
              r={6}
              fill={palette?.primary.main}
              stroke={palette?.background.paper}
              strokeWidth={2}
              style={{
                filter: `drop-shadow(0 0 2px ${palette?.primary.main})`,
              }}
            />
          );
        },
      },
    ],
    [timeRangeLabel, palette],
  );

  // ── Grouped scores for breakdown tables ──────────────────────────────────
  const { data: agentGrouped, isLoading: isAgentGroupedLoading } =
    useGroupedScores(
      commonParams,
      { level: "agent", ...scoreQueryParams },
      { enabled: hasAgentLevel },
    );

  const { data: llmGrouped, isLoading: isLlmGroupedLoading } = useGroupedScores(
    commonParams,
    { level: "llm", ...scoreQueryParams },
    { enabled: hasLlmLevel },
  );

  const isFutureMonitor = monitorData?.type === "future";
  const hasNoData = evaluators.length === 0 && !monitorData?.latestRun;

  const nextRunLabel = useMemo(() => {
    if (!monitorData?.nextRunTime) return null;
    const next = new Date(monitorData.nextRunTime);
    return next.toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  }, [monitorData?.nextRunTime]);

  const traceWindowLabel = useMemo(() => {
    if (!isPastMonitor || !monitorData?.traceStart || !monitorData?.traceEnd)
      return null;
    const fmt = (iso: string) =>
      new Date(iso).toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    return `${fmt(monitorData.traceStart)} – ${fmt(monitorData.traceEnd)}`;
  }, [isPastMonitor, monitorData?.traceStart, monitorData?.traceEnd]);

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
            titleTail={
              monitorData?.type ? (
                <Stack direction="row" spacing={1} alignItems="center">
                  <Chip
                    label={isPastMonitor ? "Historical" : "Continuous"}
                    color="default"
                    size="small"
                    variant="outlined"
                  />
                  {isPastMonitor && (
                    <Typography variant="body2" color="text.secondary">
                      Based on latest run
                    </Typography>
                  )}
                </Stack>
              ) : undefined
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
                {isPastMonitor ? (
                  traceWindowLabel && (
                    <Stack direction="row" spacing={0.5} alignItems="center">
                      <Clock size={16} />
                      <Typography variant="caption" color="text.secondary">
                        {traceWindowLabel}
                      </Typography>
                    </Stack>
                  )
                ) : (
                  <Select
                    size="small"
                    variant="outlined"
                    value={timeRange}
                    onChange={(e) =>
                      handleTimeRangeChange(
                        e.target.value as TraceListTimeRange,
                      )
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
                )}
                <IconButton
                  size="small"
                  onClick={handleRefresh}
                  aria-label="Refresh"
                  disabled={isRefetching}
                >
                  {isRefetching ? (
                    <CircularProgress size={16} />
                  ) : (
                    <RefreshCcw size={16} />
                  )}
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
              ) : hasNoData ? (
                <Stack
                  alignItems="center"
                  justifyContent="center"
                  spacing={2}
                  sx={{
                    py: 10,
                    px: 4,
                    border: `1px solid ${palette?.divider}`,
                    borderRadius: 2,
                    backgroundColor: palette?.background.paper,
                  }}
                >
                  <Timer size={48} color={palette?.text.secondary} />
                  <Typography variant="h6" fontWeight={600}>
                    {isFutureMonitor
                      ? "Your monitor is scheduled"
                      : "No evaluation data yet"}
                  </Typography>
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    textAlign="center"
                    maxWidth={480}
                  >
                    {isFutureMonitor
                      ? `This monitor will automatically start evaluating incoming traces.${
                          nextRunLabel
                            ? ` The first run is scheduled for ${nextRunLabel}.`
                            : " The first run will begin shortly."
                        }`
                      : "Run this monitor to see evaluation results, performance trends, and score breakdowns."}
                  </Typography>
                  <IconButton
                    size="small"
                    onClick={handleRefresh}
                    aria-label="Refresh"
                    disabled={isRefetching}
                    sx={{ mt: 1 }}
                  >
                    {isRefetching ? (
                      <CircularProgress size={16} />
                    ) : (
                      <RefreshCcw size={16} />
                    )}
                  </IconButton>
                </Stack>
              ) : (
                <>
                  <Grid container spacing={3} sx={{ alignItems: "stretch" }}>
                    <Grid size={{ xs: 12, md: 6 }} sx={{ display: "flex" }}>
                      <AgentPerformanceCard
                        radarChartData={radarChartData}
                        radars={radars}
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 6 }}>
                      <Stack spacing={2} height="100%">
                        <EvaluationSummaryCard
                          levels={levelSummaries}
                          averageScore={averageScore}
                        />
                        <RunSummaryCard />
                      </Stack>
                    </Grid>
                  </Grid>
                  <PerformanceByEvaluatorCard
                    evaluators={evaluatorInfoList}
                    timeRange={timeRange}
                    environmentId={monitorData?.environmentName}
                    traceStart={isPastMonitor ? monitorData?.traceStart : undefined}
                    traceEnd={isPastMonitor ? monitorData?.traceEnd : undefined}
                  />
                  {(hasAgentLevel || hasLlmLevel) && (
                    <Grid container spacing={3}>
                      {hasAgentLevel && (
                        <Grid size={{ xs: 12, md: hasLlmLevel ? 6 : 12 }}>
                          <ScoreBreakdownCard
                            level="agent"
                            data={agentGrouped}
                            isLoading={isAgentGroupedLoading}
                          />
                        </Grid>
                      )}
                      {hasLlmLevel && (
                        <Grid size={{ xs: 12, md: hasAgentLevel ? 6 : 12 }}>
                          <ScoreBreakdownCard
                            level="llm"
                            data={llmGrouped}
                            isLoading={isLlmGroupedLoading}
                          />
                        </Grid>
                      )}
                    </Grid>
                  )}
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
