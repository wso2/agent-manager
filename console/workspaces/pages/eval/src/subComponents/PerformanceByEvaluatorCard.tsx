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
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useMonitorScoresTimeSeries } from "@agent-management-platform/api-client";
import MetricsTooltip from "./MetricsTooltip";

/** Stable palette – one colour per evaluator slot */
const LINE_COLOURS = [
    "#3f8cff", "#22c55e", "#f59e0b", "#ef4444",
    "#a855f7", "#06b6d4", "#f97316", "#ec4899",
];

interface PerformanceByEvaluatorCardProps {
    /** Evaluator identifier strings from the scores summary */
    evaluatorNames: string[];
    /** ISO start of the window (same used by parent) */
    startTime: string;
    /** ISO end of the window */
    endTime: string;
    environmentId?: string;
}

/** Inner component that calls the hook for a single evaluator */
function EvaluatorSeriesFetcher({
    commonParams,
    evaluatorName,
    startTime,
    endTime,
    onData,
    onLoading,
}: {
    commonParams: {
        orgName: string; projName: string;
        agentName: string; monitorName: string;
    };
    evaluatorName: string;
    startTime: string;
    endTime: string;
    onData: (name: string, points: Array<{ timestamp: string; mean: number }>) => void;
    onLoading: (name: string, loading: boolean) => void;
}) {
    const { data, isLoading } = useMonitorScoresTimeSeries(
        commonParams,
        { startTime, endTime, evaluator: evaluatorName, granularity: "hour" },
    );
    React.useEffect(() => {
        onLoading(evaluatorName, isLoading);
        return () => {
            // Remove from loadingSet on unmount so isFetching doesn't stick true
            onLoading(evaluatorName, false);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [isLoading]);
    React.useEffect(() => {
        if (!data) return;
        const pts = data.points.map((p) => ({
            timestamp: p.timestamp,
            mean: typeof p.aggregations?.["mean"] === "number"
                ? (p.aggregations["mean"] as number) * 100
                : 0,
        }));
        onData(evaluatorName, pts);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [data]);
    return null;
}

const PerformanceByEvaluatorCard:
    React.FC<PerformanceByEvaluatorCardProps> = ({
        evaluatorNames,
        startTime,
        endTime,
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

        /**
         * Collect per-evaluator series as they resolve.
         * Key = evaluatorName, value = [{timestamp, mean}]
         */
        const [seriesMap, setSeriesMap] = React.useState<
            Record<string, Array<{ timestamp: string; mean: number }>>
        >({});



        /** Track how many fetchers are still loading */
        const [loadingSet, setLoadingSet] = React.useState<Set<string>>(new Set());
        const isFetching = loadingSet.size > 0;

        /** Clear stale data whenever the time window or evaluator set changes */
        React.useEffect(() => {
            setSeriesMap({});
            setLoadingSet(new Set());
        }, [startTime, endTime, evaluatorNames]);

        const handleLoading = React.useCallback(
            (name: string, loading: boolean) => {
                setLoadingSet((prev) => {
                    const next = new Set(prev);
                    if (loading) { next.add(name); } else { next.delete(name); }
                    return next;
                });
            }, []);

        const handleData = React.useCallback(
            (name: string, pts: Array<{ timestamp: string; mean: number }>) => {
                setSeriesMap((prev) => ({ ...prev, [name]: pts }));
            }, []);

        /**
         * Merge all series into a unified list keyed by timestamp.
         * Shape: [{ xLabel, [evaluatorName]: mean, ... }]
         */
        const chartData = useMemo(() => {
            const allTimestamps = Array.from(
                new Set(
                    Object.values(seriesMap).flatMap((pts) =>
                        pts.map((p) => p.timestamp)
                    )
                )
            ).sort();

            return allTimestamps.map((ts) => {
                const date = new Date(ts);
                const label = date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
                const row: Record<string, string | number> = { xLabel: label };
                evaluatorNames.forEach((name) => {
                    const pt = seriesMap[name]?.find((p) => p.timestamp === ts);
                    if (pt !== undefined) row[name] = pt.mean;
                });
                return row;
            });
        }, [seriesMap, evaluatorNames]);

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

                    {/* Hidden fetcher components – one per evaluator */}
                    {evaluatorNames.map((name) => (
                        <EvaluatorSeriesFetcher
                            key={name}
                            commonParams={commonParams}
                            evaluatorName={name}
                            startTime={startTime}
                            endTime={endTime}
                            onData={handleData}
                            onLoading={handleLoading}
                        />
                    ))}

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
