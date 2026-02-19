/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import {
    Alert,
    Box,
    Chip,
    Grid,
    Stack,
    StatCard,
    Typography,
} from "@wso2/oxygen-ui";
import { CheckCircle, Clock, Logs, Timer, Users } from "@wso2/oxygen-ui-icons-react";
import { DrawerContent, DrawerHeader, LogsPanel } from "@agent-management-platform/views";
import { useMonitorRunLogs } from "@agent-management-platform/api-client";
import { type MonitorRunResponse, type MonitorRunStatus } from "@agent-management-platform/types";



const RUN_STATUS_CHIP_COLOR_MAP: Record<MonitorRunStatus, "success" | "warning" | "default" | "error"> = {
    success: "success",
    running: "warning",
    pending: "default",
    failed: "error",
};

export interface MonitorRunDrawerProps {
    run: MonitorRunResponse;
    orgName: string;
    monitorName: string;
    monitorDisplayName?: string;
    onClose: () => void;
    formatDateTime: (value?: string) => string;
    traceWindowLabel: string;
    durationLabel: string;
}

export function MonitorRunDrawer({
    run,
    orgName,
    monitorName,
    monitorDisplayName,
    onClose,
    formatDateTime,
    traceWindowLabel,
    durationLabel,
}: MonitorRunDrawerProps) {

    const { data, isLoading, error } = useMonitorRunLogs({
        orgName,
        monitorName,
        runId: run.id ?? "",
    });

    const logs = data?.logs ?? [];
    const logsEmptyState = {
        title: "No logs yet",
        description: "Run logs will appear once this monitor produces output.",
        illustration: <Logs size={64} />,
    };
    const chipColor = RUN_STATUS_CHIP_COLOR_MAP[run.status] ?? "default";
    const evaluatorCount = run.evaluators?.length ?? 0;
    const statCards = [
        {
            label: "Started",
            value: formatDateTime(run.startedAt) || "—",
            icon: <Clock size={24} />,
            iconColor: "info" as const,
        },
        {
            label: "Completed",
            value: formatDateTime(run.completedAt) || "—",
            icon: <CheckCircle size={24} />,
            iconColor: "success" as const,
        },
        {
            label: "Duration",
            value: durationLabel || "—",
            icon: <Timer size={24} />,
            iconColor: "primary" as const,
        },
        {
            label: "Evaluators",
            value: evaluatorCount.toString(),
            icon: <Users size={24} />,
            iconColor: "secondary" as const,
        },
    ];

    return (
        <Stack direction="column" height="100%" maxWidth={900} width="100%">
            <DrawerHeader
                icon={<Logs size={24} />}
                title={monitorDisplayName ?? monitorName}
                onClose={onClose}
            />
            <DrawerContent>
                <Stack spacing={3} height="calc(100vh - 96px)">
                    <Stack spacing={0.5} alignItems="center" direction="row">
                        <Typography variant="h6">{traceWindowLabel}&nbsp;</Typography>
                        <Box>
                            <Chip size="small" variant="outlined" label={run.status.toUpperCase()} color={chipColor} />
                        </Box>
                    </Stack>

                    <Grid container spacing={2}>
                        {statCards.map((card) => (
                            <Grid key={card.label} size={{ xs: 12, sm: 6 }}>
                                <StatCard
                                    label={card.label}
                                    value={card.value}
                                    icon={card.icon}
                                    iconColor={card.iconColor}
                                />
                            </Grid>
                        ))}
                    </Grid>
                    {run.errorMessage && (
                        <Alert severity="warning">
                            {run.errorMessage}
                        </Alert>
                    )}
                    {error && (
                        <Alert severity="error">
                            {error instanceof Error ? error.message : "Failed to load logs. Please try again."}
                        </Alert>
                    )}
                    <Box>
                        <LogsPanel
                            logs={logs}
                            isLoading={isLoading}
                            error={error}
                            showSearch={false}
                            maxHeight="calc(100vh - 350px)"
                            emptyState={logsEmptyState}
                        />
                    </Box>
                </Stack>
            </DrawerContent >
        </Stack >
    );
}
