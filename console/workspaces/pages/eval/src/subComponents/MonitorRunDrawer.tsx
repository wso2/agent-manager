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

import { type ReactNode } from "react";
import {
    Alert,
    Box,
    Chip,
    Skeleton,
    Stack,
    Typography,
} from "@wso2/oxygen-ui";
import { Logs } from "@wso2/oxygen-ui-icons-react";
import { DrawerContent, DrawerHeader, NoDataFound } from "@agent-management-platform/views";
import { useMonitorRunLogs } from "@agent-management-platform/api-client";
import { type MonitorRunResponse, type MonitorRunStatus } from "@agent-management-platform/types";

interface DrawerInfoItemProps {
    label: string;
    value: string | number | ReactNode;
}

const DrawerInfoItem = ({ label, value }: DrawerInfoItemProps) => (
    <Stack spacing={0.5} minWidth={180} flex={1}>
        <Typography variant="caption" color="text.secondary">
            {label}
        </Typography>
        {typeof value === "string" || typeof value === "number" ? (
            <Typography variant="body2">{value}</Typography>
        ) : (
            <Box>{value}</Box>
        )}
    </Stack>
);

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
    const chipColor = RUN_STATUS_CHIP_COLOR_MAP[run.status] ?? "default";

    return (
        <Stack direction="column" height="100%" maxWidth={900} width="100%">
            <DrawerHeader
                icon={<Logs size={24} />}
                title={monitorDisplayName ?? monitorName}
                onClose={onClose}
            />
            <DrawerContent>
                <Stack spacing={2} height="calc(100vh - 96px)">
                    <Stack spacing={0.5} alignItems="center" direction="row">
                        <Typography variant="h6">{traceWindowLabel}&nbsp;</Typography>
                         <Box>
                        <Chip size="small" variant="outlined" label={run.status.toUpperCase()} color={chipColor} />
                    </Box>
                    </Stack>
                   
                    <Stack direction={{ xs: "column", md: "row" }} spacing={2} flexWrap="wrap">
                        <DrawerInfoItem label="Started" value={formatDateTime(run.startedAt)} />
                        <DrawerInfoItem label="Completed" value={formatDateTime(run.completedAt)} />
                        <DrawerInfoItem label="Duration" value={durationLabel} />
                        <DrawerInfoItem label="Evaluators" value={`${run.evaluators?.length ?? 0}`} />
                    </Stack>
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
                    <Box
                        flex={1}
                        borderRadius={1}
                        border={1}
                        borderColor="divider"
                        p={2}
                        overflow="auto"
                        bgcolor="background.paper"
                    >
                        {isLoading ? (
                            <Stack spacing={1}>
                                <Skeleton variant="rounded" height={20} />
                                <Skeleton variant="rounded" height={20} />
                                <Skeleton variant="rounded" height={20} />
                            </Stack>
                        ) : logs.length ? (
                            <Stack spacing={1}>
                                {logs.map((log, index) => (
                                    <Typography
                                        key={`${log.timestamp}-${index}`}
                                        variant="body2"
                                        fontFamily="monospace"
                                        color="text.primary"
                                    >
                                        [{formatDateTime(log.timestamp)}] {log.log}
                                    </Typography>
                                ))}
                            </Stack>
                        ) : (
                            <NoDataFound
                                message="No logs yet"
                                subtitle="Run logs will appear once this monitor produces output."
                                disableBackground
                                iconElement={Logs}
                            />
                        )}
                    </Box>
                </Stack>
            </DrawerContent >
        </Stack >
    );
}
