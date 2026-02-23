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
import { PageLayout } from "@agent-management-platform/views";
import { Grid, IconButton, InputAdornment, MenuItem, Select, Skeleton, Stack } from "@wso2/oxygen-ui";
import { Clock, RefreshCcw } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Route, Routes, useParams, useSearchParams } from "react-router-dom";
import {
    absoluteRouteMap,
    relativeRouteMap,
    type EvaluatorScoreSummary,
    TraceListTimeRange,
    getTimeRange,
} from "@agent-management-platform/types";
import AgentPerformanceCard, { RadarDefinition } from "./subComponents/AgentPerformanceCard";
import EvaluationSummaryCard, { EvaluationSummaryItem } from "./subComponents/EvaluationSummaryCard";
import TopDegradingMetricsCard, { DegradingMetric } from "./subComponents/TopDegradingMetricsCard";
import PerformanceByEvaluatorCard from "./subComponents/PerformanceByEvaluatorCard";
import { useGetMonitor, useMonitorScores } from "@agent-management-platform/api-client";
import MonitorRunList from "./subComponents/MonitorRunList";

const MONITOR_TIME_RANGE_OPTIONS = [
    { value: TraceListTimeRange.ONE_DAY,     label: "Last 24 Hours" },
    { value: TraceListTimeRange.THREE_DAYS,  label: "Last 3 Days"   },
    { value: TraceListTimeRange.SEVEN_DAYS,  label: "Last 7 Days"   },
    { value: TraceListTimeRange.THIRTY_DAYS, label: "Last 30 Days"  },
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
        () => (searchParams.get("timeRange") as TraceListTimeRange) || TraceListTimeRange.SEVEN_DAYS,
        [searchParams]
    );

    const handleTimeRangeChange = React.useCallback(
        (value: TraceListTimeRange) => {
            const next = new URLSearchParams(searchParams);
            next.set("timeRange", value);
            setSearchParams(next, { replace: true });
        },
        [searchParams, setSearchParams]
    );
    const timeRangeLabel = useMemo(
        () => MONITOR_TIME_RANGE_OPTIONS.find((o) => o.value === timeRange)?.label ?? "Selected period",
        [timeRange]
    );
    const commonParams = useMemo(() => ({
        monitorName: monitorId ?? "",
        orgName: orgId ?? "",
        projName: projectId ?? "",
        agentName: agentId ?? "",
    }), [monitorId, orgId, projectId, agentId]);

    const now = useMemo(() => new Date(), []);
    const sevenDaysAgo  = useMemo(() => new Date(now.getTime() - 7  * 24 * 60 * 60 * 1000), [now]);
    const thirtyDaysAgo = useMemo(() => new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000), [now]);

    const mainStartTime = useMemo(() => {
        const range = getTimeRange(timeRange);
        return range?.startTime ?? sevenDaysAgo.toISOString();
    }, [timeRange, sevenDaysAgo]);

    const { data: monitorData, refetch: refetchMonitor, isLoading: isMonitorLoading } =
        useGetMonitor(commonParams);

    const { data: scores7d,  refetch: refetch7d,  isLoading: isScores7dLoading  } =
        useMonitorScores(commonParams, {
            startTime: mainStartTime,
            endTime:   now.toISOString(),
        });

    const { data: scores30d, refetch: refetch30d, isLoading: isScores30dLoading } =
        useMonitorScores(commonParams, {
            startTime: thirtyDaysAgo.toISOString(),
            endTime:   now.toISOString(),
        });

    const handleRefresh = () => {
        void refetchMonitor();
        void refetch7d();
        void refetch30d();
    };

    const isLoading = isMonitorLoading || isScores7dLoading || isScores30dLoading;

    // ── raw evaluator arrays ─────────────────────────────────────────────────
    const evaluators7d  = useMemo(() => scores7d?.evaluators  ?? [], [scores7d]);
    const evaluators30d = useMemo(() => scores30d?.evaluators ?? [], [scores30d]);

    // ── EvaluationSummaryCard ────────────────────────────────────────────────
    const evaluatorSummary = useMemo<EvaluationSummaryItem[]>(() => {
        const totalCount  = evaluators7d.reduce((s, e) => s + e.count,      0);
        const totalErrors = evaluators7d.reduce((s, e) => s + e.errorCount, 0);
        const failureRate = totalCount > 0 ? (totalErrors / totalCount) * 100 : 0;

        const totalCount30  = evaluators30d.reduce((s, e) => s + e.count,      0);
        const totalErrors30 = evaluators30d.reduce((s, e) => s + e.errorCount, 0);
        const countTrend = totalCount30 > 0
            ? Math.round(((totalCount - totalCount30) / totalCount30) * 100)
            : 0;
        const errorTrend = totalErrors30 > 0
            ? Math.round(((totalErrors - totalErrors30) / totalErrors30) * 100)
            : 0;

        return [
            {
                label: "Traces Evaluated",
                value: totalCount.toLocaleString(),
                helper: totalCount30 > 0 ? `${countTrend >= 0 ? "↑" : "↓"} vs prev 30 days` : timeRangeLabel,
                trend: countTrend,
            },
            {
                label: "Eval Failures",
                value: totalErrors.toLocaleString(),
                helper: totalCount > 0 ? `${failureRate.toFixed(1)}% failure rate` : "No data",
                trend: -errorTrend,
            },
            {
                label: "Evaluators Active",
                value: String(evaluators7d.length),
                helper: "configured evaluators",
                trend: 0,
            },
        ];
    }, [evaluators7d, evaluators30d, timeRangeLabel]);

    const averageScore7d = useMemo(() => {
        const means = evaluators7d.map(getMean).filter((v): v is number => v !== null);
        if (means.length === 0) return null;
        return means.reduce((a, b) => a + b, 0) / means.length;
    }, [evaluators7d]);

    const averageScore30d = useMemo(() => {
        const means = evaluators30d.map(getMean).filter((v): v is number => v !== null);
        if (means.length === 0) return null;
        return means.reduce((a, b) => a + b, 0) / means.length;
    }, [evaluators30d]);

    const evaluationSummaryAverage = useMemo(() => {
        if (averageScore7d === null) return { value: "–", helper: "No data yet", progress: 0 };
        const delta = averageScore30d !== null
            ? ` (${averageScore7d >= averageScore30d ? "↑" : "↓"} vs 30-day avg ${(averageScore30d * 100).toFixed(1)}%)`
            : "";
        return {
            value: `${(averageScore7d * 100).toFixed(1)}%`,
            helper: `${timeRangeLabel}${delta}`,
            progress: Math.round(averageScore7d * 100),
        };
    }, [averageScore7d, averageScore30d, timeRangeLabel]);

    // ── PerformanceByEvaluatorCard ───────────────────────────────────────────
    const evaluatorNames = useMemo(
        () => evaluators7d.map((e) => e.evaluatorName),
        [evaluators7d]
    );

    // ── AgentPerformanceCard (radar) ─────────────────────────────────────────
    const radarChartData = useMemo(() =>
        evaluators7d.map((e) => ({ metric: e.evaluatorName, current: (getMean(e) ?? 0) * 100 })),
    [evaluators7d]);

    const radars = useMemo<RadarDefinition[]>(() => [
        { dataKey: "current", name: "Current (7d)", fillOpacity: 0.2, strokeWidth: 2 },
    ], []);

    // ── TopDegradingMetricsCard (7d vs 30d) ─────────────────────────────────
    const topDegrading = useMemo<DegradingMetric[]>(() => {
        const map30d = new Map(evaluators30d.map((e) => [e.evaluatorName, getMean(e)]));
        return evaluators7d
            .map((e) => {
                const m7  = getMean(e);
                const m30 = map30d.get(e.evaluatorName) ?? null;
                if (m7 === null || m30 === null) return null;
                const delta = Math.round((m7 - m30) * 100);
                return { label: e.evaluatorName, delta, range: `${(m30 * 100).toFixed(1)} → ${(m7 * 100).toFixed(1)}` };
            })
            .filter((m): m is DegradingMetric => m !== null && m.delta < 0)
            .sort((a, b) => a.delta - b.delta)
            .slice(0, 5);
    }, [evaluators7d, evaluators30d]);

    return (

        <Routes>
            <Route
                path={relativeRouteMap.children.org.children
                    .projects.children.agents.children.evaluation
                    .children.monitor.children.view.children.runs.path}
                element={
                    <PageLayout
                        title={`Run History ${monitorData?.displayName ? `(${monitorData.displayName})` : ""}`}
                        description="View detailed results of each evaluation run, including trace-level insights and evaluator feedback."
                        disableIcon
                        backLabel={`Back to ${monitorData?.displayName ?? "Monitor"}`}
                        backHref={
                            generatePath(
                                absoluteRouteMap.children.org.children.projects
                                    .children.agents.children.evaluation.children.monitor
                                    .children.view.path,
                                {
                                    orgId: orgId, projectId: projectId,
                                    monitorId: monitorId,
                                    agentId: agentId, envId: envId
                                }
                            )
                        }
                    >
                        <MonitorRunList
                            monitorDisplayName={monitorData?.displayName ?? monitorData?.name ?? monitorId ?? ""}
                        />
                    </PageLayout>
                }
            />
            <Route index element={
                <PageLayout
                    title={monitorData?.displayName ?? monitorData?.name ?? "Monitor Details"}
                    description="Monitor active agent performance, compare builds, and export evaluator summaries."
                    disableIcon
                    backLabel="Back to Monitors"
                    backHref={
                        generatePath(
                            absoluteRouteMap.children.org.children.projects
                                .children.agents.children.evaluation.children.monitor.path,
                            { orgId: orgId, projectId: projectId, agentId: agentId, envId: envId }
                        )
                    }
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
                                        averageScoreHelper={evaluationSummaryAverage.helper}
                                        averageScoreProgress={evaluationSummaryAverage.progress}
                                        timeRangeLabel={timeRangeLabel}
                                    />
                                    <TopDegradingMetricsCard metrics={topDegrading} />
                                </Stack>
                            </Grid>
                        </Grid>
                        <PerformanceByEvaluatorCard
                            evaluatorNames={evaluatorNames}
                            startTime={mainStartTime}
                            endTime={now.toISOString()}
                            environmentId={monitorData?.environmentName}
                            timeRangeLabel={timeRangeLabel}
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
