/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { useMemo, useCallback } from "react";
import { Typography, Chip, Button, useTheme, Box, Skeleton, ButtonBase, MenuItem, Select, InputAdornment } from "@mui/material";
import { DataListingTable, FadeIn, TableColumn } from "@agent-management-platform/views";
import { generatePath, Link, useParams } from "react-router-dom";
import { useGetAgent, useTraceList } from "@agent-management-platform/api-client";
import { absoluteRouteMap, Trace, TraceListResponse, TraceListTimeRange } from "@agent-management-platform/types";
import dayjs from "dayjs";
import { AccessTimeOutlined, RemoveRedEyeOutlined } from "@mui/icons-material";

interface TraceRow {
    id: string;
    traceId: string;
    rootSpanName: string;
    startTime: string;
    endTime: string;
    durationInNanos: number;
}

function TracesTableSkeleton() {
    const theme = useTheme();

    return (
        <Box
            sx={{
                display: 'flex',
                flexDirection: 'column',
                gap: theme.spacing(1)
            }}
        >
            <Skeleton variant="rectangular" width="100%" height={theme.spacing(7)} />
            {[...Array(10)].map((_, index) => (
                <Skeleton
                    key={index}
                    variant="rectangular"
                    width="100%"
                    height={theme.spacing(6)}
                />
            ))}
        </Box>
    );
}

const TIME_RANGE_OPTIONS = [
    { value: TraceListTimeRange.TEN_MINUTES, label: '10 Minutes' },
    { value: TraceListTimeRange.THIRTY_MINUTES, label: '30 Minutes' },
    { value: TraceListTimeRange.ONE_HOUR, label: '1 Hour' },
    { value: TraceListTimeRange.THREE_HOURS, label: '3 Hours' },
    { value: TraceListTimeRange.SIX_HOURS, label: '6 Hours' },
    { value: TraceListTimeRange.TWELVE_HOURS, label: '12 Hours' },
    { value: TraceListTimeRange.ONE_DAY, label: '1 Day' },
    { value: TraceListTimeRange.THREE_DAYS, label: '3 Days' },
    { value: TraceListTimeRange.SEVEN_DAYS, label: '7 Days' },
];

interface TracesTableProps {
    timeRange: TraceListTimeRange;
    setTimeRange: (timeRange: TraceListTimeRange) => void;
}

export function TracesTable({ timeRange, setTimeRange }: TracesTableProps) {
    const theme = useTheme();
    const { orgId = "default", projectId = "default", agentId = "default", envId = "default" } = useParams();
    const { data: agentData } = useGetAgent(
        { orgName: orgId, projName: projectId, agentName: agentId });
    const isExternalAgent = agentData?.provisioning?.type === 'external';

    const { data: traceData, isLoading } = useTraceList(
        orgId,
        projectId,
        agentId,
        envId,
        timeRange
    );

    const traceListResponse = traceData as unknown as TraceListResponse;

    const rows = useMemo(() => traceListResponse?.traces?.map((trace: Trace) => {
        const start = new Date(trace.startTime).getTime();
        const end = new Date(trace.endTime).getTime();
        const durationInNanos = (end - start) / 1000;

        return {
            id: trace.traceId,
            traceId: trace.traceId,
            rootSpanName: trace.rootSpanName,
            startTime: trace.startTime,
            endTime: trace.endTime,
            durationInNanos: durationInNanos,
        } as TraceRow;
    }) ?? [], [traceListResponse?.traces]);

    const getDurationColor = useCallback((durationInNanos: number) => {
        if (durationInNanos < 2) return "success";
        if (durationInNanos < 5) return "warning";
        return "error";
    }, []);

    const columns: TableColumn<TraceRow>[] = useMemo(() => [
        {
            id: "rootSpanName",
            label: "Name",
            width: "20%",
            render: (value, row) => (
                <ButtonBase
                    component={Link}
                    to={
                        isExternalAgent ?
                            generatePath(absoluteRouteMap.children.org.children.projects
                                .children.agents.children.traces.path,
                                { orgId: orgId ?? '', projectId: projectId ?? '', agentId: agentId ?? '', traceId: row.traceId as string })
                            :
                            generatePath(absoluteRouteMap.children.org
                                .children.projects.children.agents.children.environment
                                .children.observability.children.traces.path,
                                { orgId: orgId ?? '', projectId: projectId ?? '', agentId: agentId ?? '', envId: envId ?? '', traceId: row.traceId as string })
                    }
                >
                    <Typography
                        noWrap
                        variant="body2"
                        sx={{ color: theme.palette.text.primary }}
                    >
                        {value}
                    </Typography>
                </ButtonBase>
            ),
        },
        {
            id: "traceId",
            label: "Trace ID",
            width: "25%",
            render: (value) => (
                <ButtonBase
                    component={Link}
                    to={
                        isExternalAgent ?
                            generatePath(absoluteRouteMap.children.org.children.projects
                                .children.agents.children.traces.path,
                                { orgId: orgId ?? '', projectId: projectId ?? '', agentId: agentId ?? '', traceId: value as string })
                            :
                            generatePath(absoluteRouteMap.children.org
                                .children.projects.children.agents.children.environment
                                .children.observability.children.traces.path,
                                { orgId: orgId ?? '', projectId: projectId ?? '', agentId: agentId ?? '', envId: envId ?? '', traceId: value as string })
                    }
                >
                    <Typography
                        noWrap
                        variant="body2"
                        color="text.secondary"
                    >
                        {(value as string).substring(0, 8)}.....{(value as string)
                            .substring((value as string).length - 8)}
                    </Typography>
                </ButtonBase>
            ),
        },
        {
            id: "startTime",
            label: "Start Time",
            width: "20%",
            render: (value) => (
                <Typography
                    noWrap
                    variant="body2"
                    sx={{ color: theme.palette.text.secondary }}
                >
                    {dayjs(value as string).format('DD/MM/YYYY HH:mm:ss')}
                </Typography>
            ),
        },
        {
            id: "durationInNanos",
            label: "Duration",
            width: "15%",
            render: (value) => (
                <Chip
                    label={`${(value as number).toFixed(2)}s`}
                    size="small"
                    color={getDurationColor(value as number)}
                    variant="outlined"
                />
            ),
        },
        {
            id: "actions",
            label: "",
            width: "10%",
            align: "center",
            render: (_value, row) => (
                <Button
                    variant="text"
                    size="small"
                    component={Link}
                    startIcon={<RemoveRedEyeOutlined fontSize="inherit" />}
                    to={
                        isExternalAgent ?
                            generatePath(absoluteRouteMap.children.org.children.projects
                                .children.agents.children.traces.path,
                                { orgId: orgId ?? '', projectId: projectId ?? '', agentId: agentId ?? '', traceId: row.traceId as string })
                            :
                            generatePath(absoluteRouteMap.children.org
                                .children.projects.children.agents.children.environment
                                .children.observability.children.traces.path,
                                { orgId: orgId ?? '', projectId: projectId ?? '', agentId: agentId ?? '', envId: envId ?? '', traceId: row.traceId as string })
                    }
                >
                    Expand
                </Button>
            ),
        }
    ], [
        orgId,
        projectId,
        agentId,
        envId,
        getDurationColor,
        theme.palette.text.primary,
        theme.palette.text.secondary
    ]);

    if (isLoading) {
        return <TracesTableSkeleton />;
    }

    return (
        <FadeIn>
            <Box display="flex" justifyContent="flex-end">
                <Select
                    size="small"
                    variant="outlined"
                    value={timeRange}
                    startAdornment={<InputAdornment position="start"><AccessTimeOutlined fontSize="inherit" /></InputAdornment>}
                    onChange={(e) => setTimeRange(e.target.value as TraceListTimeRange)}
                >
                    {TIME_RANGE_OPTIONS.map((option) => (
                        <MenuItem key={option.value} value={option.value}>
                            {option.label}
                        </MenuItem>
                    ))}
                </Select>
            </Box>
            <DataListingTable
                data={rows}
                columns={columns}
                pagination
                pageSize={10}
                maxRows={rows.length}
                defaultSortBy="startTime"
                defaultSortDirection="desc"
            />
        </FadeIn>
    );
}

