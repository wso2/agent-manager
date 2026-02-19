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
import { Button, Grid, Stack } from "@wso2/oxygen-ui";
import { Download, RefreshCcw } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Route, Routes, useParams } from "react-router-dom";
import { absoluteRouteMap, relativeRouteMap } from "@agent-management-platform/types";
import AgentPerformanceCard, { RadarDefinition, ScoreBadge } from "./subComponents/AgentPerformanceCard";
import EvaluationSummaryCard, { EvaluationSummaryItem } from "./subComponents/EvaluationSummaryCard";
import TopDegradingMetricsCard, { DegradingMetric } from "./subComponents/TopDegradingMetricsCard";
import PerformanceByEvaluatorCard, {
    AggregateStats,
    EvaluatorTrendPoint,
} from "./subComponents/PerformanceByEvaluatorCard";
import { useGetMonitor } from "@agent-management-platform/api-client";
import MonitorRunList from "./subComponents/MonitorRunList";

const radarMetrics = [
    "Tone",
    "Completeness",
    "Safety",
    "Coherence",
    "Believability",
    "Latency",
    "Accuracy",
    "Conciseness",
];

const radarSeries: Array<{ label: string; data: number[] }> = [
    {
        label: "v2.4.1",
        data: [0.92, 0.88, 0.96, 0.85, 0.87, 0.75, 0.91, 0.79],
    },
    {
        label: "v2.3.8",
        data: [0.88, 0.84, 0.93, 0.80, 0.82, 0.73, 0.89, 0.76],
    },
];

const normalizeSeriesKey = (label: string): string => label.toLowerCase().replace(/[^a-z0-9]/g, "_");

const radarSeriesWithKeys = radarSeries.map((series) => ({
    ...series,
    dataKey: normalizeSeriesKey(series.label),
}));

const radarChartData = radarMetrics.map((metric, metricIndex) => {
    const dataPoint: Record<string, string | number> = { metric };

    radarSeriesWithKeys.forEach((series) => {
        dataPoint[series.dataKey] = series.data[metricIndex];
    });

    return dataPoint;
});

const radars: RadarDefinition[] = radarSeriesWithKeys.map((series) => ({
    dataKey: series.dataKey,
    name: series.label,
    fillOpacity: 0.15,
    strokeWidth: 2,
}));

const evaluatorTrend: EvaluatorTrendPoint[] = [
    { label: "Day 1", score: 0.94 },
    { label: "Day 2", score: 0.91 },
    { label: "Day 3", score: 0.87 },
    { label: "Day 4", score: 0.92 },
    { label: "Day 5", score: 0.89 },
    { label: "Day 6", score: 0.93 },
    { label: "Day 7", score: 0.90 },
];

const topDegrading: DegradingMetric[] = [
    { label: "Latency", delta: -6, range: "0.83 → 0.78" },
    { label: "Conciseness", delta: -4, range: "0.82 → 0.79" },
    { label: "Relevance", delta: -1, range: "0.88 → 0.87" },
];

const evaluatorSummary: EvaluationSummaryItem[] = [
    { label: "Traces Evaluated", value: "1,247", helper: "5% sampled", trend: 12 },
    { label: "Total Produced", value: "24,940", helper: "↑ 12% vs prev", trend: 8 },
    { label: "Eval Failures", value: "23", helper: "1.8% failed", trend: -2 },
];

const scoreBadges: ScoreBadge[] = [
    { label: "Latency", score: "0.83", intent: "warning" },
    { label: "Trajectory", score: "0.94", intent: "success" },
    { label: "Safety", score: "0.96", intent: "success" },
];

const evaluationSummaryAverage = {
    value: "0.89",
    helper: "No change vs previous period",
    progress: 89,
};

export const ViewMonitorComponent: React.FC = () => {
    const { orgId, projectId, agentId, envId, monitorId } = useParams();
    const { data } = useGetMonitor({
        monitorName: monitorId ?? "",
        orgName: orgId ?? "",
        projName: projectId ?? "",
        agentName: agentId ?? "",
    });

    const aggregateStats = useMemo<AggregateStats>(() => {
        const scores = evaluatorTrend.map((point) => point.score);
        const avg = scores.reduce((acc, val) => acc + val, 0) / scores.length;

        return {
            mean: avg.toFixed(2),
            min: Math.min(...scores).toFixed(2),
            max: Math.max(...scores).toFixed(2),
        };
    }, []);

    return (

        <Routes>
            <Route
                path={relativeRouteMap.children.org.children
                    .projects.children.agents.children.evaluation
                    .children.monitor.children.view.children.runs.path}
                element={
                    <PageLayout
                        title={`Run History ${data?.displayName ? `(${data.displayName})` : ""}`}
                        description="View detailed results of each evaluation run, including trace-level insights and evaluator feedback."
                        disableIcon
                        backLabel={`Back to ${data?.displayName ? `${data.displayName}` : ""}`}
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
                            monitorDisplayName={data?.displayName ?? data?.name ?? monitorId ?? ""}
                        />
                    </PageLayout>
                }
            />
            <Route index element={
                <PageLayout
                    title="Quality Check"
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
                        <Stack direction="row" spacing={1}>
                            <Button variant="outlined" startIcon={<RefreshCcw size={16} />}>
                                Refresh
                            </Button>
                            <Button variant="contained" color="primary" startIcon={<Download size={16} />}>
                                Export Summary
                            </Button>
                        </Stack>
                    }
                >
                    <Stack spacing={3}>
                        <Grid container spacing={3}>
                            <Grid size={{ xs: 12, md: 6 }}>
                                <AgentPerformanceCard
                                    radarChartData={radarChartData}
                                    radars={radars}
                                    scoreBadges={scoreBadges}
                                />
                            </Grid>
                            <Grid size={{ xs: 12, md: 6 }}>
                                <Stack spacing={2}>
                                    <EvaluationSummaryCard
                                        items={evaluatorSummary}
                                        averageScoreValue={evaluationSummaryAverage.value}
                                        averageScoreHelper={evaluationSummaryAverage.helper}
                                        averageScoreProgress={evaluationSummaryAverage.progress}
                                    />
                                    <TopDegradingMetricsCard metrics={topDegrading} />
                                </Stack>
                            </Grid>
                        </Grid>

                        <PerformanceByEvaluatorCard
                            evaluatorTrend={evaluatorTrend}
                            aggregateStats={aggregateStats}
                        />
                    </Stack>
                </PageLayout>
            }
            />


        </Routes>

    );
};

export default ViewMonitorComponent;
