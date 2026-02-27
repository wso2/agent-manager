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
    Box, Button, Card, CardContent,
    Skeleton, Stack, Typography,
} from "@wso2/oxygen-ui";
import { ChartTooltip, LineChart } from "@wso2/oxygen-ui-charts-react";
import { Activity, Workflow } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import { absoluteRouteMap, type TimeSeriesResponse, TraceListTimeRange } from "@agent-management-platform/types";
import { useMonitorScoresTimeSeriesForEvaluators } from "@agent-management-platform/api-client";
import MetricsTooltip from "./MetricsTooltip";

/** Stable palette – one colour per evaluator slot */
const LINE_COLOURS = [
    "#3f8cff", "#22c55e", "#f59e0b", "#ef4444",
    "#a855f7", "#06b6d4", "#f97316", "#ec4899",
];

interface PerformanceByEvaluatorCardProps {
    /** Evaluator identifier strings from the scores summary */
    evaluatorNames: string[];
    /** Logical time range window (e.g., last 7 days) */
    timeRange: TraceListTimeRange;
    environmentId?: string;
}

const PerformanceByEvaluatorCard:
    React.FC<PerformanceByEvaluatorCardProps> = ({
        evaluatorNames,
        timeRange,
        environmentId
    }) => {
        const { orgId, projectId, agentId, envId, monitorId } = useParams<{
            orgId: string; projectId: string;
            agentId: string; envId: string; monitorId: string;
        }>();

        const commonParams = useMemo(() => ({
            orgName: orgId ?? "",
            projName: projectId ?? "",
            agentName: agentId ?? "",
            monitorName: monitorId ?? "",
        }), [orgId, projectId, agentId, monitorId]);

        const { data: timeSeriesByEvaluator, isLoading: isFetching } =
            useMonitorScoresTimeSeriesForEvaluators(commonParams, {
                // startTime/endTime are derived from `timeRange` inside the hook.
                startTime: "",
                endTime: "",
                timeRange,
                granularity: "hour",
                evaluators: evaluatorNames,
            });

        /**
         * Merge all series into a unified list keyed by timestamp.
         * Shape: [{ xLabel, [evaluatorName]: mean, ... }]
         */
        const chartData = useMemo(() => {
            if (!timeSeriesByEvaluator) {
                return [];
            }

            const seriesMap: Record<string, Array<{ timestamp: string; mean: number }>> = {};

            Object.entries(timeSeriesByEvaluator as Record<string, TimeSeriesResponse>).forEach(
                ([name, resp]) => {
                    seriesMap[name] = resp.points.map((p) => ({
                        timestamp: p.timestamp,
                        mean:
                            typeof p.aggregations?.["mean"] === "number"
                                ? (p.aggregations["mean"] as number) * 100
                                : 0,
                    }));
                }
            );

            const allTimestamps = Array.from(
                new Set(
                    Object.values(seriesMap).flatMap((pts) =>
                        pts.map((p) => p.timestamp)
                    )
                )
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
                    if (pt !== undefined) row[name] = pt.mean;
                });
                return row;
            });
        }, [timeSeriesByEvaluator, evaluatorNames]);

        /** Track which evaluator lines are toggled off */
        const [hiddenSeries, setHiddenSeries] = React.useState<Set<string>>(new Set());

        const toggleSeries = React.useCallback((name: string) => {
            setHiddenSeries((prev) => {
                const next = new Set(prev);
                if (next.has(name)) { next.delete(name); } else { next.add(name); }
                return next;
            });
        }, []);

        /** All lines (for legend colours), filtered lines (for chart) */
        const allLines = useMemo(() =>
            evaluatorNames.map((name, i) => ({
                dataKey: name,
                name,
                stroke: LINE_COLOURS[i % LINE_COLOURS.length],
                strokeWidth: 2,
                dot: false,
            })),
            [evaluatorNames]);

        const visibleLines = useMemo(
            () => allLines.filter((l) => !hiddenSeries.has(l.dataKey)),
            [allLines, hiddenSeries]
        );

        const hasData = chartData.length > 0;

        return (
            <Card variant="outlined">
                <CardContent>
                    <Stack direction="row" justifyContent="space-between"
                        alignItems="center" mb={2}>
                        <Stack spacing={0.5}>
                            <Typography variant="subtitle1">
                                Performance by Evaluator
                            </Typography>
                        </Stack>
                        <Button
                            size="small" variant="text"
                            component={Link}
                            to={generatePath(
                                absoluteRouteMap.children.org
                                    .children.projects.children.agents
                                    .children.environment
                                    .children.observability.children.traces.path,
                                {
                                    orgId: orgId ?? "",
                                    projectId: projectId ?? "",
                                    agentId: agentId ?? "",
                                    envId: environmentId ?? envId ?? "",
                                }
                            )}
                            startIcon={<Workflow size={16} />}
                        >
                            View All Traces
                        </Button>
                    </Stack>

                    {isFetching ? (
                        <Skeleton variant="rounded" height={320} />
                    ) : !hasData ? (
                        <Box
                            display="flex" flexDirection="column"
                            alignItems="center" justifyContent="center"
                            py={6} gap={1}
                        >
                            <Activity size={48} />
                            <Typography variant="body2" fontWeight={500}>
                                No trend data
                            </Typography>
                            <Typography variant="caption" color="text.secondary"
                                textAlign="center">
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
                                        <MetricsTooltip
                                            formatter={(v) => `${v.toFixed(1)}%`}
                                        />
                                    }
                                />
                            </LineChart>

                            {/* Custom clickable legend */}
                            {evaluatorNames.length > 0 && (
                                <Stack
                                    direction="row" flexWrap="wrap"
                                    justifyContent="center" gap={1.5} mt={1}
                                >
                                    {allLines.map((line) => {
                                        const isHidden = hiddenSeries.has(line.dataKey);
                                        return (
                                            <Box
                                                key={line.dataKey}
                                                onClick={() => toggleSeries(line.dataKey)}
                                                sx={{
                                                    display: "flex",
                                                    alignItems: "center",
                                                    gap: 0.75,
                                                    cursor: "pointer",
                                                    opacity: isHidden ? 0.35 : 1,
                                                    userSelect: "none",
                                                    transition: "opacity 0.15s",
                                                }}
                                            >
                                                <Box
                                                    sx={{
                                                        width: 12,
                                                        height: 12,
                                                        borderRadius: "2px",
                                                        backgroundColor: line.stroke,
                                                        flexShrink: 0,
                                                    }}
                                                />
                                                <Typography
                                                    variant="caption"
                                                    sx={{
                                                        textDecoration: isHidden
                                                            ? "line-through" : "none",
                                                        color: "text.secondary",
                                                    }}
                                                >
                                                    {line.name}
                                                </Typography>
                                            </Box>
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
