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

import React, { useCallback, useMemo, useState } from "react";
import {
  formatTraceWindow,
  PageLayout,
  TimeRangeSelector,
  useTimeRangeParams,
} from "@agent-management-platform/views";
import {
  Button,
  Chip,
  CircularProgress,
  Grid,
  IconButton,
  Menu,
  MenuItem,
  Skeleton,
  Stack,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import {
  ChevronDown,
  Clock,
  GitCompare,
  RefreshCcw,
  Timer,
} from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  Route,
  Routes,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import {
  absoluteRouteMap,
  relativeRouteMap,
  type EvaluationLevel,
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
  useListMonitors,
  useMonitorScores,
} from "@agent-management-platform/api-client";
import { useQueryClient } from "@tanstack/react-query";
import MonitorRunList from "./subComponents/MonitorRunList";
import {
  computeAverageScore,
  computeLevelSummaries,
  getMean,
} from "./utils/monitorScoreUtils";

const MONITOR_TIME_RANGE_OPTIONS = [
  { value: TraceListTimeRange.ONE_DAY, label: "Last 1 Day" },
  { value: TraceListTimeRange.THREE_DAYS, label: "Last 3 Days" },
  { value: TraceListTimeRange.SEVEN_DAYS, label: "Last 7 Days" },
  { value: TraceListTimeRange.THIRTY_DAYS, label: "Last 30 Days" },
];

export const ViewMonitorComponent: React.FC = () => {
  const { orgId, projectId, agentId, envId, monitorId } = useParams();
  const theme = useTheme();
  const palette = theme.vars?.palette;
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const [searchParams, setSearchParams] = useSearchParams();

  // ── Compare button / monitor picker ───────────────────────────────────
  const [compareAnchorEl, setCompareAnchorEl] =
    useState<HTMLElement | null>(null);
  const { data: compareCandidates } = useListMonitors(
    { orgName: orgId ?? "", projName: projectId ?? "", agentName: agentId ?? "" },
    { environmentName: envId },
  );
  const otherMonitors = useMemo(
    () => (compareCandidates?.monitors ?? []).filter((m) => m.name !== monitorId),
    [compareCandidates, monitorId],
  );
  const handleOpenCompareMenu = useCallback(
    (event: React.MouseEvent<HTMLElement>) => {
      setCompareAnchorEl(event.currentTarget);
    },
    [],
  );
  const handleCloseCompareMenu = useCallback(() => {
    setCompareAnchorEl(null);
  }, []);
  const handleCompareWith = useCallback(
    (targetMonitorName: string) => {
      handleCloseCompareMenu();
      if (!orgId || !projectId || !agentId || !envId || !monitorId) {
        return;
      }
      navigate({
        pathname: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.evaluation.children.monitor.children
            .compare.path,
          { orgId, projectId, agentId, envId, monitorId },
        ),
        search: `?with=${encodeURIComponent(targetMonitorName)}`,
      });
    },
    [agentId, envId, handleCloseCompareMenu, monitorId, navigate, orgId, projectId],
  );

  const {
    customStartTime,
    customEndTime,
    hasCustomRange,
    handleCustomRangeApply,
  } = useTimeRangeParams(searchParams, setSearchParams);

  const timeRange = useMemo(
    () =>
      (searchParams.get("timeRange") as TraceListTimeRange) ||
      TraceListTimeRange.SEVEN_DAYS,
    [searchParams],
  );

  const handleTimeRangeChange = React.useCallback(
    (value: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("timeRange", value);
      next.delete("startTime");
      next.delete("endTime");
      setSearchParams(next, { replace: true });
    },
    [searchParams, setSearchParams],
  );
  const timeRangeLabel = useMemo(
    () =>
      hasCustomRange && customStartTime && customEndTime
        ? formatTraceWindow(customStartTime, customEndTime)
        : (MONITOR_TIME_RANGE_OPTIONS.find((o) => o.value === timeRange)?.label ??
          "Selected period"),
    [hasCustomRange, customStartTime, customEndTime, timeRange],
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

  const scoreQueryParams = useMemo(() => {
    if (isPastMonitor && monitorData?.traceStart && monitorData?.traceEnd) {
      return { startTime: monitorData.traceStart, endTime: monitorData.traceEnd };
    }
    if (hasCustomRange && customStartTime && customEndTime) {
      return { startTime: customStartTime, endTime: customEndTime };
    }
    return { timeRange };
  }, [
    isPastMonitor,
    monitorData?.traceStart,
    monitorData?.traceEnd,
    hasCustomRange,
    customStartTime,
    customEndTime,
    timeRange,
  ]);

  const {
    data: scoresMain,
    refetch: refetchMain,
    isLoading: isScoresMainLoading,
    isRefetching: isScoresMainRefetching,
  } = useMonitorScores(commonParams, scoreQueryParams);

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
  const levelSummaries = useMemo<LevelSummary[]>(
    () => computeLevelSummaries(evaluators),
    [evaluators],
  );

  const averageScore = useMemo(
    () => computeAverageScore(evaluators),
    [evaluators],
  );

  // ── PerformanceByEvaluatorCard ───────────────────────────────────────────
  const evaluatorInfoList = useMemo(
    () => evaluators.map((e) => ({ name: e.evaluatorName, level: e.level })),
    [evaluators],
  );

  const traceWindowLabel = useMemo(() => {
    if (!isPastMonitor || !monitorData?.traceStart || !monitorData?.traceEnd)
      return null;
    return formatTraceWindow(monitorData.traceStart, monitorData.traceEnd);
  }, [isPastMonitor, monitorData?.traceStart, monitorData?.traceEnd]);

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
    [isPastMonitor, traceWindowLabel, timeRangeLabel, palette],
  );

  // ── Grouped scores for breakdown tables ──────────────────────────────────
  const {
    data: agentGrouped,
    isLoading: isAgentGroupedLoading,
    refetch: refetchAgentGrouped,
  } = useGroupedScores(
    commonParams,
    { level: "agent", ...scoreQueryParams },
    { enabled: hasAgentLevel },
  );

  const {
    data: llmGrouped,
    isLoading: isLlmGroupedLoading,
    refetch: refetchLlmGrouped,
  } = useGroupedScores(
    commonParams,
    { level: "llm", ...scoreQueryParams },
    { enabled: hasLlmLevel },
  );

  const handleRefresh = useCallback(() => {
    void refetchMonitor();
    void refetchMain();
    void refetchAgentGrouped();
    void refetchLlmGrouped();
    void queryClient.invalidateQueries({ queryKey: ["monitor-runs"] });
    void queryClient.invalidateQueries({
      queryKey: ["monitor-scores-timeseries-batch"],
    });
  }, [refetchMonitor, refetchMain, refetchAgentGrouped, refetchLlmGrouped, queryClient]);

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

  return (
    <Routes>
      <Route
        path={
          relativeRouteMap.children.org.children.projects.children.agents
            .children.environment.children.evaluation.children.monitor.children
            .view.children.runs.path
        }
        element={
          <PageLayout docs={["evaluationMonitors", "evaluation", "customEvaluators"]}
            title={`Run History ${monitorData?.displayName ? `(${monitorData.displayName})` : ""}`}
            disableIcon
            backLabel={`Back to ${monitorData?.displayName ?? "Monitor"}`}
            backHref={generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor
                .children.view.path,
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
          <PageLayout docs={["evaluationMonitors", "evaluation", "customEvaluators"]}
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
                .children.environment.children.evaluation.children.monitor.path,
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
                  <TimeRangeSelector
                    preset={timeRange}
                    customStart={customStartTime}
                    customEnd={customEndTime}
                    options={MONITOR_TIME_RANGE_OPTIONS}
                    onPresetChange={handleTimeRangeChange}
                    onCustomRangeApply={handleCustomRangeApply}
                  />
                )}
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<GitCompare size={16} />}
                  endIcon={<ChevronDown size={14} />}
                  onClick={handleOpenCompareMenu}
                >
                  Compare
                </Button>
                <Menu
                  anchorEl={compareAnchorEl}
                  open={!!compareAnchorEl}
                  onClose={handleCloseCompareMenu}
                >
                  {otherMonitors.length === 0 ? (
                    <MenuItem disabled>
                      No other monitors in this environment
                    </MenuItem>
                  ) : (
                    otherMonitors.map((m) => (
                      <MenuItem
                        key={m.name}
                        onClick={() => handleCompareWith(m.name)}
                      >
                        {m.displayName ?? m.name}
                      </MenuItem>
                    ))
                  )}
                </Menu>
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
                    traceStart={
                      isPastMonitor
                        ? monitorData?.traceStart
                        : hasCustomRange
                          ? customStartTime
                          : undefined
                    }
                    traceEnd={
                      isPastMonitor
                        ? monitorData?.traceEnd
                        : hasCustomRange
                          ? customEndTime
                          : undefined
                    }
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
