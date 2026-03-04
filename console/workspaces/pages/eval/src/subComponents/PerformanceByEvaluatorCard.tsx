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
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Skeleton,
  Stack,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { ChartTooltip, LineChart } from "@wso2/oxygen-ui-charts-react";
import { Activity, Workflow } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type EvaluationLevel,
  type TimeSeriesResponse,
  TraceListTimeRange,
} from "@agent-management-platform/types";
import { useMonitorScoresTimeSeriesForEvaluators } from "@agent-management-platform/api-client";
import MetricsTooltip from "./MetricsTooltip";
import { LEVEL_CONFIG, levelChipSx } from "./levelConfig";

/** Stable palette – one colour per evaluator slot */
const LINE_COLOURS = [
  "#3f8cff",
  "#22c55e",
  "#f59e0b",
  "#ef4444",
  "#a855f7",
  "#06b6d4",
  "#f97316",
  "#ec4899",
];

export interface EvaluatorInfo {
  name: string;
  level: EvaluationLevel;
}

interface PerformanceByEvaluatorCardProps {
  evaluators: EvaluatorInfo[];
  timeRange: TraceListTimeRange;
  environmentId?: string;
}

const PerformanceByEvaluatorCard: React.FC<PerformanceByEvaluatorCardProps> = ({
  evaluators,
  timeRange,
  environmentId,
}) => {
  const theme = useTheme();
  const isDark = theme.palette.mode === "dark";
  const { orgId, projectId, agentId, envId, monitorId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    monitorId: string;
  }>();

  const commonParams = useMemo(
    () => ({
      orgName: orgId ?? "",
      projName: projectId ?? "",
      agentName: agentId ?? "",
      monitorName: monitorId ?? "",
    }),
    [orgId, projectId, agentId, monitorId],
  );

  const evaluatorNames = useMemo(
    () => evaluators.map((e) => e.name),
    [evaluators],
  );

  const { data: timeSeriesByEvaluator, isLoading: isFetching } =
    useMonitorScoresTimeSeriesForEvaluators(commonParams, {
      timeRange,
      granularity: "hour",
      evaluators: evaluatorNames,
    });

  const chartData = useMemo(() => {
    if (!timeSeriesByEvaluator) {
      return [];
    }

    const seriesMap: Record<
      string,
      Array<{ timestamp: string; mean: number | null }>
    > = {};

    Object.entries(
      timeSeriesByEvaluator as Record<string, TimeSeriesResponse>,
    ).forEach(([name, resp]) => {
      seriesMap[name] = resp.points.map((p) => ({
        timestamp: p.timestamp,
        mean:
          typeof p.aggregations?.["mean"] === "number"
            ? (p.aggregations["mean"] as number) * 100
            : null,
      }));
    });

    const allTimestamps = Array.from(
      new Set(
        Object.values(seriesMap).flatMap((pts) => pts.map((p) => p.timestamp)),
      ),
    ).sort();

    return allTimestamps.map((ts) => {
      const date = new Date(ts);
      const label = date.toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      });
      const row: Record<string, string | number> = { xLabel: label };
      evaluatorNames.forEach((name) => {
        const pt = seriesMap[name]?.find((p) => p.timestamp === ts);
        if (pt !== undefined && pt.mean !== null) row[name] = pt.mean;
      });
      return row;
    });
  }, [timeSeriesByEvaluator, evaluatorNames]);

  const [hiddenSeries, setHiddenSeries] = React.useState<Set<string>>(
    new Set(),
  );

  const toggleSeries = React.useCallback((name: string) => {
    setHiddenSeries((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }, []);

  const levelMap = useMemo(() => {
    const m = new Map<string, EvaluationLevel>();
    evaluators.forEach((e) => m.set(e.name, e.level));
    return m;
  }, [evaluators]);

  const allLines = useMemo(
    () =>
      evaluatorNames.map((name, i) => ({
        dataKey: name,
        name,
        stroke: LINE_COLOURS[i % LINE_COLOURS.length],
        strokeWidth: 2,
        dot: false,
      })),
    [evaluatorNames],
  );

  const visibleLines = useMemo(
    () => allLines.filter((l) => !hiddenSeries.has(l.dataKey)),
    [allLines, hiddenSeries],
  );

  const hasData = chartData.length > 0;

  return (
    <Card variant="outlined">
      <CardContent>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          mb={2}
        >
          <Stack spacing={0.5}>
            <Typography variant="subtitle1">
              Performance by Evaluator
            </Typography>
          </Stack>
          <Button
            size="small"
            variant="text"
            component={Link}
            to={generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .path,
              {
                orgId: orgId ?? "",
                projectId: projectId ?? "",
                agentId: agentId ?? "",
                envId: environmentId ?? envId ?? "",
              },
            )}
            startIcon={<Workflow size={16} />}
          >
            View all traces
          </Button>
        </Stack>

        {isFetching ? (
          <Skeleton variant="rounded" height={320} />
        ) : !hasData ? (
          <Box
            display="flex"
            flexDirection="column"
            alignItems="center"
            justifyContent="center"
            py={6}
            gap={1}
          >
            <Activity size={36} />
            <Typography variant="body2" fontWeight={500}>
              No trend data
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              textAlign="center"
            >
              Evaluator scores will appear here after runs complete.
            </Typography>
          </Box>
        ) : (
          <>
            <LineChart
              height={320}
              data={chartData}
              xAxisDataKey="xLabel"
              lines={visibleLines}
              legend={{ show: false }}
              grid={{ show: true, strokeDasharray: "3 3" }}
              tooltip={{ show: false }}
            >
              <ChartTooltip
                content={
                  <MetricsTooltip formatter={(v) => `${v.toFixed(1)}%`} />
                }
              />
            </LineChart>

            {/* Custom clickable legend — each item is a contained unit with
                                color swatch + name + level Chip grouped tightly together,
                                separated from neighbors by a visible gap */}
            {evaluatorNames.length > 0 && (
              <Stack
                direction="row"
                flexWrap="wrap"
                justifyContent="center"
                gap={2}
                mt={1.5}
              >
                {allLines.map((line) => {
                  const isHidden = hiddenSeries.has(line.dataKey);
                  const level = levelMap.get(line.dataKey);
                  const cfg = level ? LEVEL_CONFIG[level] : null;
                  return (
                    <Stack
                      key={line.dataKey}
                      direction="row"
                      alignItems="center"
                      spacing={0.5}
                      onClick={() => toggleSeries(line.dataKey)}
                      sx={{
                        cursor: "pointer",
                        opacity: isHidden ? 0.35 : 1,
                        userSelect: "none",
                        transition: "opacity 0.15s",
                        border: "1px solid",
                        borderColor: "divider",
                        borderRadius: 1,
                        px: 1,
                        py: 0.25,
                      }}
                    >
                      <Box
                        sx={{
                          width: 10,
                          height: 10,
                          borderRadius: "2px",
                          backgroundColor: line.stroke,
                          flexShrink: 0,
                        }}
                      />
                      <Typography
                        variant="caption"
                        sx={{
                          textDecoration: isHidden ? "line-through" : "none",
                          color: "text.secondary",
                          fontWeight: 500,
                        }}
                      >
                        {line.name}
                      </Typography>
                      {cfg && (
                        <Chip
                          label={cfg.label}
                          size="small"
                          sx={levelChipSx(cfg, isDark)}
                        />
                      )}
                    </Stack>
                  );
                })}
              </Stack>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
};

export default PerformanceByEvaluatorCard;
