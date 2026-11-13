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
    IconButton,
    Skeleton,
    Typography,
} from "@wso2/oxygen-ui";
import { ExternalLink } from "@wso2/oxygen-ui-icons-react";
import {
    useListMonitors,
    useMonitorScores,
} from "@agent-management-platform/api-client";
import {
    absoluteRouteMap,
    TraceListTimeRange,
    type EvaluatorScoreSummary,
    type MonitorResponse,
} from "@agent-management-platform/types";
import { formatTraceWindow } from "@agent-management-platform/views";
import { generatePath, Link } from "react-router-dom";
import { DonutIcon, getDonutColorForPercent } from "./DonutIcon";
import { SectionHeader } from "./SectionHeader";

interface EnvMonitorsSectionProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
}

const getMean = (evaluators: EvaluatorScoreSummary[]): number | null => {
    const means = evaluators
        .map((e) => e.aggregations?.["mean"])
        .filter((v): v is number => typeof v === "number");
    if (means.length === 0) return null;
    return means.reduce((a, b) => a + b, 0) / means.length;
};

/** Human-friendly run cadence for continuous monitors, e.g. "Runs every 10 mins". */
const formatRunInterval = (minutes?: number): string | null => {
    if (!minutes || minutes <= 0) return null;
    if (minutes < 60) return `Runs every ${minutes} min${minutes === 1 ? "" : "s"}`;
    if (minutes === 60) return "Runs hourly";
    if (minutes === 1440) return "Runs daily";
    if (minutes % 1440 === 0) return `Runs every ${minutes / 1440} days`;
    if (minutes % 60 === 0) return `Runs every ${minutes / 60} hours`;
    const hours = Math.floor(minutes / 60);
    return `Runs every ${hours}h ${minutes % 60}m`;
};

interface MonitorTileProps {
    monitor: MonitorResponse;
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
}

const MonitorTile: React.FC<MonitorTileProps> = ({ monitor, orgId, projectId, agentId, envId }) => {
    const { data: scoresData, isLoading } = useMonitorScores(
        { orgName: orgId, projName: projectId, agentName: agentId, monitorName: monitor.name },
        { timeRange: TraceListTimeRange.SEVEN_DAYS },
    );

    const scorePercent = useMemo(() => {
        if (!scoresData) return null;
        const mean = getMean(scoresData.evaluators);
        return mean !== null ? (mean * 100) : null;
    }, [scoresData]);

    const monitorHref = generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.evaluation.children.monitor.children.view.path,
        { orgId, projectId, agentId, envId, monitorId: monitor.name },
    );

    const evaluatorNames = monitor.evaluators.map((e) => e.displayName).join(" · ");
    const color = getDonutColorForPercent(scorePercent);

    // Surface a low-priority schedule hint so users understand the score's
    // cadence: a fixed window for historical monitors, an interval for
    // continuous (future) ones.
    const scheduleLabel = useMemo(() => {
        if (monitor.type === "past") {
            if (!monitor.traceStart || !monitor.traceEnd) return null;
            return formatTraceWindow(monitor.traceStart, monitor.traceEnd);
        }
        return formatRunInterval(monitor.intervalMinutes);
    }, [monitor.type, monitor.traceStart, monitor.traceEnd, monitor.intervalMinutes]);

    return (
        <Card variant="outlined" sx={NESTED_CARD_SX}>
            <CardContent sx={{ display: "flex", alignItems: "center", gap: 2, "&:last-child": { pb: 1.5 } }}>
                {isLoading ? (
                    <Skeleton variant="circular" width={52} height={52} />
                ) : (
                    <DonutIcon percent={scorePercent ?? 0} color={color} size={72} />
                )}
                <Box minWidth={0} flex={1} overflow="hidden">
                    {isLoading ? (
                        <Skeleton variant="text" width={48} />
                    ) : (
                        <Typography variant="h6" lineHeight={1.2}>
                            {scorePercent !== null ? `${scorePercent.toFixed(2)}%` : "—"}
                        </Typography>
                    )}
                    <Box display="flex" alignItems="center" gap={0.5} minWidth={0}>
                        <Typography variant="body2" noWrap fontWeight={500}>
                            {monitor.displayName || monitor.name}
                        </Typography>
                        <IconButton
                            size="small"
                            component={Link}
                            to={monitorHref}
                            sx={{ p: 0.25, flexShrink: 0 }}
                        >
                            <ExternalLink size={12} />
                        </IconButton>
                    </Box>
                    {evaluatorNames && (
                        <Typography variant="caption" color="text.secondary" noWrap display="block" title={evaluatorNames}>
                            {evaluatorNames}
                        </Typography>
                    )}
                    {scheduleLabel && (
                        <Typography variant="caption" color="text.disabled" noWrap display="block" title={scheduleLabel}>
                            {scheduleLabel}
                        </Typography>
                    )}
                </Box>
            </CardContent>
        </Card>
    );
};

/**
 * Per-environment "Agent Performance" monitors, rendered as a section inside an
 * EnvironmentCard (alongside EnvObservabilitySection). Lists only the monitors
 * that belong to this environment. Renders nothing when the env has no monitors.
 */
export const EnvMonitorsSection: React.FC<EnvMonitorsSectionProps> = ({
    orgId, projectId, agentId, envId,
}) => {
    const { data: monitorsList, isLoading } = useListMonitors(
        {
            orgName: orgId,
            projName: projectId,
            agentName: agentId,
        },
        { environmentName: envId },
    );

    const monitors = monitorsList?.monitors ?? [];

    if (!isLoading && monitors.length === 0) {
        return null;
    }

    const allMonitorsHref = generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.evaluation.children.monitor.path,
        { orgId, projectId, agentId, envId },
    );

    const gridSx = {
        display: "grid",
        gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
        gap: 1.5,
    };

    return (
        <>
            <SectionHeader title="Evaluations" viewAllHref={allMonitorsHref} />
            {isLoading ? (
                <Box sx={gridSx}>
                    {[1, 2, 3].map((i) => <Skeleton key={i} variant="rounded" height={96} />)}
                </Box>
            ) : (
                <Box sx={gridSx}>
                    {monitors.map((monitor) => (
                        <MonitorTile
                            key={monitor.name}
                            monitor={monitor}
                            orgId={orgId}
                            projectId={projectId}
                            agentId={agentId}
                            envId={envId}
                        />
                    ))}
                </Box>
            )}
        </>
    );
};
