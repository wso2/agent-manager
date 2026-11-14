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
import { NESTED_CARD_SX } from "@agent-management-platform/shared-component";
import {
    Box,
    Card,
    CardContent,
    Grid,
    Skeleton,
    Typography,
    useTheme,
} from "@wso2/oxygen-ui";
import { AreaChart } from "@wso2/oxygen-ui-charts-react";
import {
    useListAgentDeployments,
    useTraceList,
} from "@agent-management-platform/api-client";
import {
    absoluteRouteMap,
    TraceListTimeRange,
} from "@agent-management-platform/types";
import { format } from "date-fns";
import { generatePath } from "react-router-dom";
import { DonutIcon, DonutColor, getDonutColorForPercent } from "./DonutIcon";
import { SectionHeader } from "./SectionHeader";

interface EnvObservabilitySectionProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    external?: boolean;
}

const formatTokens = (n: number): string =>
    n >= 1000 ? `${(n / 1000).toFixed(1)}k` : `${n}`;

const formatDuration = (nanos: number): string => {
    const ms = nanos / 1_000_000;
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
};

interface MetricCardProps {
    label: string;
    value: string;
    points: Array<{ time: string; value: number }>;
    color?: string;
    isLoading?: boolean;
}

const MetricCard: React.FC<MetricCardProps> = ({ label, value, points, color = "currentColor", isLoading }) => (
    <Card variant="outlined" sx={{ ...NESTED_CARD_SX, width: "100%" }}>
        <CardContent sx={{ py: 1, px: 1.5, "&:last-child": { pb: 1 }, display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1 }}>
            <Box>
                {isLoading
                    ? <Skeleton variant="text" width={48} height={28} />
                    : <Typography variant="h6" lineHeight={1.2}>{value}</Typography>
                }
                <Typography variant="caption" color="text.secondary">{label}</Typography>
            </Box>
            {isLoading
                ? <Skeleton variant="rounded" width={120} height={48} />
                : (
                    <AreaChart
                        data={points}
                        xAxisDataKey="time"
                        height={48}
                        width={120}
                        xAxis={{ show: false }}
                        yAxis={{ show: false }}
                        grid={{ show: false }}
                        tooltip={{ show: false }}
                        legend={{ show: false }}
                        margin={{ top: 4, right: 0, bottom: 0, left: 0 }}
                        areas={[{
                            dataKey: "value",
                            stroke: color,
                            fill: color,
                            fillOpacity: 0.15,
                            dot: false,
                            activeDot: false,
                            connectNulls: true,
                            isAnimationActive: false,
                            type: "monotone",
                        }]}
                    />
                )
            }
        </CardContent>
    </Card>
);

interface DonutMetricCardProps {
    label: string;
    value: string;
    percent: number;
    color: DonutColor;
    isLoading?: boolean;
}

const DonutMetricCard: React.FC<DonutMetricCardProps> =
    ({ label, value, percent, color, isLoading }) => (
        <Card variant="outlined" sx={{ ...NESTED_CARD_SX, width: "100%" }}>
            <CardContent sx={{ py: 1, px: 1.5, "&:last-child": { pb: 1 }, display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1 }}>
                <Box>
                    {isLoading
                        ? <Skeleton variant="text" width={48} height={28} />
                        : <Typography variant="h6" lineHeight={1.2}>{value}</Typography>
                    }
                    <Typography variant="caption" color="text.secondary">{label}</Typography>
                </Box>
                {isLoading
                    ? <Skeleton variant="circular" width={48} height={48} />
                    : <DonutIcon percent={percent} color={color} size={48} />
                }
            </CardContent>
        </Card>
    );

export const EnvObservabilitySection: React.FC<EnvObservabilitySectionProps> = ({
    orgId, projectId, agentId, envId, external = false,
}) => {
    const theme = useTheme();
    const { data: deployments } = useListAgentDeployments(
        { orgName: orgId, projName: projectId, agentName: agentId },
        { enabled: !external },
    );
    const deploymentStatus = deployments?.[envId]?.status;

    // Observability is only meaningful once a deployment exists in a running,
    // failed, or suspended state — there are no metrics or traces to show while
    // an internal agent is not-deployed or still deploying. External agents have
    // no deployment lifecycle, so they are always observable.
    const hasObservableDeployment =
        deploymentStatus === "active" ||
        deploymentStatus === "error" ||
        deploymentStatus === "failed" ||
        deploymentStatus === "suspended";
    const hideObservability = !external && !hasObservableDeployment;

    const { traceList, isLoading: isTracesLoading } = useTraceList(
        orgId, projectId, agentId, envId, TraceListTimeRange.ONE_DAY, 10, "desc",
        undefined, undefined, { enableAutoRefresh: true, enabled: !hideObservability },
    );

    const traces = useMemo(() => traceList?.traces ?? [], [traceList]);

    const latencyPoints = useMemo(() =>
        [...traces].reverse().map((t) => ({
            time: format(new Date(t.startTime), "HH:mm"),
            value: Math.round(t.durationInNanos / 1_000_000),
        })),
        [traces],
    );

    const avgLatencyNanos = traces.length
        ? traces.reduce((sum, t) => sum + t.durationInNanos, 0) / traces.length
        : null;

    const tokenPoints = useMemo(() =>
        [...traces].reverse()
            .filter((t) => t.tokenUsage?.totalTokens != null)
            .map((t) => ({
                time: format(new Date(t.startTime), "HH:mm"),
                value: t.tokenUsage!.totalTokens!,
            })),
        [traces],
    );

    const avgTokens = (() => {
        const withTokens = traces.filter((t) => t.tokenUsage?.totalTokens != null);
        return withTokens.length
            ? Math.round(withTokens.reduce((sum, t) => sum +
                (t.tokenUsage?.totalTokens ?? 0), 0) / withTokens.length)
            : null;
    })();

    const scorePoints = useMemo(() =>
        [...traces].reverse()
            .filter((t) => t.score?.score != null)
            .map((t) => ({
                time: format(new Date(t.startTime), "HH:mm"),
                value: Math.round(t.score!.score! * 100),
            })),
        [traces],
    );

    const avgScore = (() => {
        const withScore = traces.filter((t) => t.score?.score != null);
        return withScore.length
            ? withScore.reduce((sum, t) => sum + (t.score?.score ?? 0), 0) / withScore.length
            : null;
    })();

    const successRate = traces.length
        ? (traces.filter((t) => (t.status?.errorCount ?? 0) === 0).length / traces.length) * 100
        : null;
    const successRateColor: DonutColor = getDonutColorForPercent(successRate);

    // The list is capped at 10 (see the useTraceList call below) — once the
    // environment has more than that in the last 24h, the tooltip should say
    // so instead of implying these stats cover everything in the window.
    const isTraceListTruncated = (traceList?.totalCount ?? 0) > traces.length;
    const tracesInfoTooltip = isTraceListTruncated
        ? "Showing the last 10 traces from the last 24 hours."
        : "Showing traces from the last 24 hours.";

    const tracesHref = generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.observability.children.traces.path,
        { orgId, projectId, agentId, envId },
    );

    // Nothing to surface for a not-deployed / deploying internal environment.
    if (hideObservability) {
        return null;
    }

    return (
        <>
            <SectionHeader title="Recent Traces" titleInfo={tracesInfoTooltip} viewAllHref={tracesHref} />
            <Grid container spacing={1.5}>
                <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
                    <MetricCard
                        label="Average Tokens"
                        value={avgTokens !== null ? formatTokens(avgTokens) : "—"}
                        points={tokenPoints}
                        color={theme.vars?.palette?.warning?.main}
                        isLoading={isTracesLoading}
                    />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
                    <DonutMetricCard
                        label="Success Rate"
                        value={successRate !== null ? `${successRate.toFixed(1)}%` : "—"}
                        percent={successRate ?? 0}
                        color={successRateColor}
                        isLoading={isTracesLoading}
                    />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
                    <MetricCard
                        label="Average Latency"
                        value={avgLatencyNanos !== null ? formatDuration(avgLatencyNanos) : "—"}
                        points={latencyPoints}
                        color={theme.vars?.palette?.info?.main}
                        isLoading={isTracesLoading}
                    />
                </Grid>
                {avgScore !== null && (
                    <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
                        <MetricCard
                            label="Average Score"
                            value={`${(avgScore * 100).toFixed(1)}%`}
                            points={scorePoints}
                            color={theme.vars?.palette?.success?.main}
                            isLoading={isTracesLoading}
                        />
                    </Grid>
                )}
            </Grid>
        </>
    );
};
