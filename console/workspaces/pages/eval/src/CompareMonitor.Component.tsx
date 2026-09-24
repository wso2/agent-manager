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
import { formatTraceWindow, PageLayout } from "@agent-management-platform/views";
import {
  Alert,
  Card,
  CardContent,
  Chip,
  Grid,
  Menu,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { ChevronDown } from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import {
  absoluteRouteMap,
  type EvaluationLevel,
  type EvaluatorScoreSummary,
  type MonitorResponse,
  TraceListTimeRange,
} from "@agent-management-platform/types";
import {
  useGetMonitor,
  useListMonitors,
  useMonitorScores,
} from "@agent-management-platform/api-client";
import AgentPerformanceCard, {
  type RadarDataPoint,
  type RadarDefinition,
} from "./subComponents/AgentPerformanceCard";
import EvaluationSummaryCard from "./subComponents/EvaluationSummaryCard";
import ScoreChip from "./subComponents/ScoreChip";
import { LEVEL_CONFIG, levelChipSx } from "./subComponents/levelConfig";
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

/** Guard a raw query-param value before treating it as a TraceListTimeRange. */
function isValidTimeRange(value: string | null): value is TraceListTimeRange {
  return (
    value !== null &&
    (Object.values(TraceListTimeRange) as string[]).includes(value)
  );
}

interface CompareRadarPoint extends RadarDataPoint {
  source: number | null;
  target: number | null;
  _isNoDataSource: boolean;
  _isNoDataTarget: boolean;
  _scoredCountSource: number;
  _totalCountSource: number;
  _scoredCountTarget: number;
  _totalCountTarget: number;
}

/** Per-monitor score query params: past monitors use their own fixed trace window. */
function buildScoreQueryParams(
  monitor: MonitorResponse | undefined,
  timeRange: TraceListTimeRange,
) {
  if (monitor?.type === "past" && monitor.traceStart && monitor.traceEnd) {
    return { startTime: monitor.traceStart, endTime: monitor.traceEnd };
  }
  return { timeRange };
}

/**
 * Shows each monitor's actual comparison window inline next to its name —
 * a fixed date range for past monitors, or its own time-range picker for
 * future monitors — so a past-vs-future comparison never hides that the two
 * sides are looking at different time semantics.
 */
function MonitorTimeBadge({
  monitor,
  timeRange,
  onTimeRangeChange,
}: {
  monitor: MonitorResponse | undefined;
  timeRange: TraceListTimeRange;
  onTimeRangeChange: (value: TraceListTimeRange) => void;
}) {
  if (monitor?.type === "past" && monitor.traceStart && monitor.traceEnd) {
    return (
      <Typography variant="caption" color="text.secondary">
        {formatTraceWindow(monitor.traceStart, monitor.traceEnd)}
      </Typography>
    );
  }

  return (
    <Select
      size="small"
      variant="standard"
      value={timeRange}
      onChange={(e) => onTimeRangeChange(e.target.value as TraceListTimeRange)}
      sx={{ fontSize: "0.75rem" }}
    >
      {MONITOR_TIME_RANGE_OPTIONS.map((opt) => (
        <MenuItem key={opt.value} value={opt.value}>
          {opt.label}
        </MenuItem>
      ))}
    </Select>
  );
}

export const CompareMonitorComponent: React.FC = () => {
  const { orgId, projectId, agentId, envId, monitorId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    monitorId: string;
  }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const targetMonitorId = searchParams.get("with") ?? "";
  const theme = useTheme();
  const palette = theme.vars?.palette;

  const sourceTimeRange = useMemo(() => {
    const raw = searchParams.get("sourceTimeRange");
    return isValidTimeRange(raw) ? raw : TraceListTimeRange.SEVEN_DAYS;
  }, [searchParams]);
  const targetTimeRange = useMemo(() => {
    const raw = searchParams.get("targetTimeRange");
    return isValidTimeRange(raw) ? raw : TraceListTimeRange.SEVEN_DAYS;
  }, [searchParams]);

  const handleSourceTimeRangeChange = (value: TraceListTimeRange) => {
    const next = new URLSearchParams(searchParams);
    next.set("sourceTimeRange", value);
    setSearchParams(next, { replace: true });
  };
  const handleTargetTimeRangeChange = (value: TraceListTimeRange) => {
    const next = new URLSearchParams(searchParams);
    next.set("targetTimeRange", value);
    setSearchParams(next, { replace: true });
  };

  // ── Target monitor picker — change who we're comparing against ──────────
  const [targetAnchorEl, setTargetAnchorEl] = useState<HTMLElement | null>(
    null,
  );
  const { data: candidateMonitors } = useListMonitors(
    { orgName: orgId ?? "", projName: projectId ?? "", agentName: agentId ?? "" },
    { environmentName: envId },
  );
  const otherMonitors = useMemo(
    () =>
      (candidateMonitors?.monitors ?? []).filter(
        (m) => m.name !== monitorId && m.name !== targetMonitorId,
      ),
    [candidateMonitors, monitorId, targetMonitorId],
  );
  const handleOpenTargetMenu = (event: React.MouseEvent<HTMLElement>) => {
    setTargetAnchorEl(event.currentTarget);
  };
  const handleCloseTargetMenu = () => {
    setTargetAnchorEl(null);
  };
  const handleChangeTarget = (nextTargetName: string) => {
    handleCloseTargetMenu();
    const next = new URLSearchParams(searchParams);
    next.set("with", nextTargetName);
    setSearchParams(next, { replace: true });
  };

  // ── Source monitor picker — source lives in the URL path, not a query
  // param, so swapping it means navigating to a new compare URL instead of
  // just updating search params. Shares the same candidate list as target.
  const [sourceAnchorEl, setSourceAnchorEl] = useState<HTMLElement | null>(
    null,
  );
  const handleOpenSourceMenu = (event: React.MouseEvent<HTMLElement>) => {
    setSourceAnchorEl(event.currentTarget);
  };
  const handleCloseSourceMenu = () => {
    setSourceAnchorEl(null);
  };
  const handleChangeSource = (nextSourceName: string) => {
    handleCloseSourceMenu();
    if (!orgId || !projectId || !agentId || !envId) {
      return;
    }
    navigate({
      pathname: generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
          .children.environment.children.evaluation.children.monitor.children
          .compare.path,
        { orgId, projectId, agentId, envId, monitorId: nextSourceName },
      ),
      search: searchParams.toString(),
    });
  };

  const sourceParams = useMemo(
    () => ({
      monitorName: monitorId ?? "",
      orgName: orgId ?? "",
      projName: projectId ?? "",
      agentName: agentId ?? "",
    }),
    [monitorId, orgId, projectId, agentId],
  );

  const targetParams = useMemo(
    () => ({
      monitorName: targetMonitorId,
      orgName: orgId ?? "",
      projName: projectId ?? "",
      agentName: agentId ?? "",
    }),
    [targetMonitorId, orgId, projectId, agentId],
  );

  const { data: sourceMonitor, isLoading: isSourceMonitorLoading } =
    useGetMonitor(sourceParams);
  const { data: targetMonitor, isLoading: isTargetMonitorLoading } =
    useGetMonitor(targetParams);

  const sourceScoreQuery = useMemo(
    () => buildScoreQueryParams(sourceMonitor, sourceTimeRange),
    [sourceMonitor, sourceTimeRange],
  );
  const targetScoreQuery = useMemo(
    () => buildScoreQueryParams(targetMonitor, targetTimeRange),
    [targetMonitor, targetTimeRange],
  );

  const { data: sourceScores, isLoading: isSourceScoresLoading } =
    useMonitorScores(sourceParams, sourceScoreQuery);
  const { data: targetScores, isLoading: isTargetScoresLoading } =
    useMonitorScores(targetParams, targetScoreQuery);

  const isLoading =
    isSourceMonitorLoading ||
    isTargetMonitorLoading ||
    isSourceScoresLoading ||
    isTargetScoresLoading;

  const sourceEvaluators = useMemo(
    () => sourceScores?.evaluators ?? [],
    [sourceScores],
  );
  const targetEvaluators = useMemo(
    () => targetScores?.evaluators ?? [],
    [targetScores],
  );

  const sourceName = sourceMonitor?.displayName ?? sourceMonitor?.name ?? "Monitor A";
  const targetName = targetMonitor?.displayName ?? targetMonitor?.name ?? "Monitor B";

  const sourceColor = palette?.primary.main ?? "#3f8cff";
  // theme.palette.secondary is a neutral grey in this design system (not a
  // visible accent), so use the warning token (amber) as the high-contrast
  // comparison series, falling back to a fixed hex if the token is unavailable.
  const targetColor = palette?.warning?.main ?? "#f59e0b";

  // ── Union radar dataset: every evaluator either monitor has ──────────────
  const radarChartData = useMemo<CompareRadarPoint[]>(() => {
    const byNameSource = new Map<string, EvaluatorScoreSummary>(
      sourceEvaluators.map((e) => [e.evaluatorName, e]),
    );
    const byNameTarget = new Map<string, EvaluatorScoreSummary>(
      targetEvaluators.map((e) => [e.evaluatorName, e]),
    );
    const names = Array.from(
      new Set([...byNameSource.keys(), ...byNameTarget.keys()]),
    );

    return names.map((name) => {
      const a = byNameSource.get(name);
      const b = byNameTarget.get(name);
      const meanA = a ? getMean(a) : undefined;
      const meanB = b ? getMean(b) : undefined;
      // Keep null (not 0) when an evaluator is absent or fully skipped, so the
      // radar bridges the axis (connectNulls) instead of plotting a misleading
      // 0 score. The _isNoData* flags below still distinguish "absent" from
      // "all skipped" for the tooltip.
      const sourceValue = meanA != null ? meanA * 100 : null;
      const targetValue = meanB != null ? meanB * 100 : null;
      const level: EvaluationLevel = (a ?? b)?.level ?? "trace";

      return {
        metric: name,
        current: sourceValue ?? targetValue ?? 0,
        _isNoData: a ? meanA === null : true,
        _scoredCount: a ? a.count - a.skippedCount : 0,
        _totalCount: a ? a.count : 0,
        _level: level,
        source: sourceValue,
        target: targetValue,
        _isNoDataSource: a ? meanA === null : false,
        _isNoDataTarget: b ? meanB === null : false,
        _scoredCountSource: a ? a.count - a.skippedCount : 0,
        _totalCountSource: a ? a.count : 0,
        _scoredCountTarget: b ? b.count - b.skippedCount : 0,
        _totalCountTarget: b ? b.count : 0,
      };
    });
  }, [sourceEvaluators, targetEvaluators]);

  const radars = useMemo<RadarDefinition[]>(
    () => [
      {
        dataKey: "source",
        name: sourceName,
        stroke: sourceColor,
        fill: sourceColor,
        fillOpacity: 0.2,
        strokeWidth: 2,
        connectNulls: true,
      },
      {
        dataKey: "target",
        name: targetName,
        stroke: targetColor,
        fill: targetColor,
        fillOpacity: 0.15,
        strokeWidth: 2,
        connectNulls: true,
      },
    ],
    [sourceName, targetName, sourceColor, targetColor],
  );

  const sourceLevelSummaries = useMemo(
    () => computeLevelSummaries(sourceEvaluators),
    [sourceEvaluators],
  );
  const targetLevelSummaries = useMemo(
    () => computeLevelSummaries(targetEvaluators),
    [targetEvaluators],
  );
  const sourceAverageScore = useMemo(
    () => computeAverageScore(sourceEvaluators),
    [sourceEvaluators],
  );
  const targetAverageScore = useMemo(
    () => computeAverageScore(targetEvaluators),
    [targetEvaluators],
  );

  const backHref = useMemo(() => {
    if (!orgId || !projectId || !agentId || !envId || !monitorId) {
      return "#";
    }
    return generatePath(
      absoluteRouteMap.children.org.children.projects.children.agents.children
        .environment.children.evaluation.children.monitor.children.view.path,
      { orgId, projectId, agentId, envId, monitorId },
    );
  }, [orgId, projectId, agentId, envId, monitorId]);

  if (!targetMonitorId) {
    return (
      <PageLayout docs={["evaluation", "evaluationMonitors", "customEvaluators"]}
        title="Compare Monitors"
        disableIcon
        backLabel="Back to Monitor"
        backHref={backHref}
      >
        <Alert severity="error">
          No comparison monitor selected. Go back and choose a monitor to
          compare against.
        </Alert>
      </PageLayout>
    );
  }

  return (
    <PageLayout docs={["evaluation", "evaluationMonitors", "customEvaluators"]}
      title="Compare Monitors"
      disableIcon
      backLabel={`Back to ${sourceName}`}
      backHref={backHref}
      actions={
        <Stack direction="row" spacing={1.5} alignItems="center">
          <Stack direction="row" spacing={1} alignItems="center">
            <Chip
              size="small"
              label={sourceName}
              deleteIcon={<ChevronDown size={14} />}
              onDelete={handleOpenSourceMenu}
              onClick={handleOpenSourceMenu}
              sx={{ fontWeight: 600 }}
            />
            <Menu
              anchorEl={sourceAnchorEl}
              open={!!sourceAnchorEl}
              onClose={handleCloseSourceMenu}
            >
              {otherMonitors.length === 0 ? (
                <MenuItem disabled>No other monitors to compare</MenuItem>
              ) : (
                otherMonitors.map((m) => (
                  <MenuItem key={m.name} onClick={() => handleChangeSource(m.name)}>
                    {m.displayName ?? m.name}
                  </MenuItem>
                ))
              )}
            </Menu>
            <MonitorTimeBadge
              monitor={sourceMonitor}
              timeRange={sourceTimeRange}
              onTimeRangeChange={handleSourceTimeRangeChange}
            />
          </Stack>
          <Typography variant="caption" color="text.secondary">
            vs
          </Typography>
          <Stack direction="row" spacing={1} alignItems="center">
            <Chip
              size="small"
              label={targetName}
              deleteIcon={<ChevronDown size={14} />}
              onDelete={handleOpenTargetMenu}
              onClick={handleOpenTargetMenu}
              sx={{ fontWeight: 600 }}
            />
            <Menu
              anchorEl={targetAnchorEl}
              open={!!targetAnchorEl}
              onClose={handleCloseTargetMenu}
            >
              {otherMonitors.length === 0 ? (
                <MenuItem disabled>No other monitors to compare</MenuItem>
              ) : (
                otherMonitors.map((m) => (
                  <MenuItem key={m.name} onClick={() => handleChangeTarget(m.name)}>
                    {m.displayName ?? m.name}
                  </MenuItem>
                ))
              )}
            </Menu>
            <MonitorTimeBadge
              monitor={targetMonitor}
              timeRange={targetTimeRange}
              onTimeRangeChange={handleTargetTimeRangeChange}
            />
          </Stack>
        </Stack>
      }
    >
      <Stack spacing={3}>
        {isLoading ? (
          <Grid container spacing={3}>
            <Grid size={{ xs: 12, md: 6 }}>
              <Skeleton variant="rounded" height={460} />
            </Grid>
            <Grid size={{ xs: 12, md: 6 }}>
              <Stack spacing={2}>
                <Skeleton variant="rounded" height={222} />
                <Skeleton variant="rounded" height={222} />
              </Stack>
            </Grid>
          </Grid>
        ) : (
          <>
            <Grid container spacing={3} sx={{ alignItems: "stretch" }}>
              <Grid size={{ xs: 12, md: 6 }} sx={{ display: "flex" }}>
                <AgentPerformanceCard
                  title="Agent Performance Comparison"
                  radarChartData={radarChartData}
                  radars={radars}
                  renderTooltipContent={({ active, payload }) => {
                    if (!active || !payload?.length) return null;
                    const dataPoint = payload[0]?.payload as
                      | CompareRadarPoint
                      | undefined;
                    if (!dataPoint) return null;
                    const cfg = LEVEL_CONFIG[dataPoint._level];

                    const rows = [
                      {
                        label: sourceName,
                        color: sourceColor,
                        value: dataPoint.source,
                        isNoData: dataPoint._isNoDataSource,
                        scored: dataPoint._scoredCountSource,
                        total: dataPoint._totalCountSource,
                      },
                      {
                        label: targetName,
                        color: targetColor,
                        value: dataPoint.target,
                        isNoData: dataPoint._isNoDataTarget,
                        scored: dataPoint._scoredCountTarget,
                        total: dataPoint._totalCountTarget,
                      },
                      // Keep a row when the monitor has the evaluator: either it has
                      // a score (value set) or it ran but fully skipped (isNoData).
                      // Drop only evaluators the monitor doesn't have at all.
                    ].filter((row) => row.value !== null || row.isNoData);

                    return (
                      <Card variant="outlined">
                        <CardContent
                          sx={{ py: 1, px: 1.5, "&:last-child": { pb: 1 } }}
                        >
                          <Stack
                            direction="row"
                            alignItems="center"
                            spacing={0.75}
                            mb={0.5}
                          >
                            <Chip
                              label={cfg.label}
                              size="small"
                              sx={levelChipSx(cfg)}
                            />
                            <Typography variant="body2" fontWeight={600} noWrap>
                              {dataPoint.metric}
                            </Typography>
                          </Stack>
                          <Stack spacing={0.5}>
                            {rows.map((row) => (
                              <Stack
                                key={row.label}
                                direction="row"
                                alignItems="center"
                                spacing={0.75}
                              >
                                <Stack
                                  width={8}
                                  height={8}
                                  borderRadius="50%"
                                  sx={{ backgroundColor: row.color }}
                                />
                                <Typography
                                  variant="caption"
                                  color="text.secondary"
                                  sx={{ minWidth: 90 }}
                                  noWrap
                                >
                                  {row.label}
                                </Typography>
                                {row.isNoData ? (
                                  <Typography variant="caption">–</Typography>
                                ) : (
                                  <ScoreChip
                                    score={(row.value ?? 0) / 100}
                                    variant="text"
                                  />
                                )}
                                <Typography
                                  variant="caption"
                                  color="text.secondary"
                                >
                                  {row.isNoData
                                    ? "all skipped"
                                    : `(${row.scored}/${row.total} ${cfg.unit})`}
                                </Typography>
                              </Stack>
                            ))}
                          </Stack>
                        </CardContent>
                      </Card>
                    );
                  }}
                />
              </Grid>
              <Grid size={{ xs: 12, md: 6 }}>
                <Stack spacing={2} height="100%">
                  <EvaluationSummaryCard
                    title={`${sourceName} — Summary`}
                    levels={sourceLevelSummaries}
                    averageScore={sourceAverageScore}
                  />
                  <EvaluationSummaryCard
                    title={`${targetName} — Summary`}
                    levels={targetLevelSummaries}
                    averageScore={targetAverageScore}
                  />
                </Stack>
              </Grid>
            </Grid>
          </>
        )}
      </Stack>
    </PageLayout>
  );
};

export default CompareMonitorComponent;
