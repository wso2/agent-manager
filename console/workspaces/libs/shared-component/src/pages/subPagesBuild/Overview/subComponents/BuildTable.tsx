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
import { Box, Button, Chip, CircularProgress, Drawer, Typography, useTheme } from "@mui/material";
import { DataListingTable, TableColumn, renderStatusChip } from "@agent-management-platform/views";
import { DataArray, Rocket } from "@mui/icons-material";
import { useParams, useSearchParams } from "react-router-dom";
import { DeploymentConfig } from "../../../../components/DeploymentConfig";
import { BuildLogs } from "../../../../components/BuildLogs";
import { useGetAgentBuilds } from "@agent-management-platform/api-client";
import { BuildStatus } from "@agent-management-platform/types";
import dayjs from "dayjs";

interface BuildRow {
    id: string;
    branch: string;
    status: BuildStatus;
    title: string;
    commit: string;
    duration: number;
    actions: string;
    startedAt: string;
    imageId: string;
}

export function BuildTable() {
    const theme = useTheme();
    const [searchParams, setSearchParams] = useSearchParams();
    const selectedBuildName = searchParams.get('selectedBuild');
    const selectedPanel = searchParams.get('panel'); // 'logs' | 'deploy'
    const { orgId, projectId, agentId } = useParams();
    const { data: builds } = useGetAgentBuilds({ orgName: orgId ?? 'default', projName: projectId ?? 'default', agentName: agentId ?? '' });
    const orderedBuilds = useMemo(() =>
        builds?.builds.sort(
            (a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()),
        [builds]);

    const rows = useMemo(() => orderedBuilds?.map(build => ({
        id: build.buildName,
        actions: build.buildName,
        branch: build.branch,
        commit: build.commitId,
        duration: 20,
        startedAt: build.startedAt,
        status: build.status as BuildStatus,
        title: build.buildName,
        imageId: build.imageId ?? 'busybox',
    } as BuildRow)) ?? [], [orderedBuilds]);

    const handleBuildClick = useCallback((buildName: string, panel: 'logs' | 'deploy') => {
        const next = new URLSearchParams(searchParams);
        next.set('selectedBuild', buildName);
        next.set('panel', panel);
        setSearchParams(next);
    }, [searchParams, setSearchParams]);

    const clearSelectedBuild = useCallback(() => {
        const next = new URLSearchParams(searchParams);
        next.delete('selectedBuild');
        next.delete('panel');
        setSearchParams(next);
    }, [searchParams, setSearchParams]);


    const getStatusColor = (status: BuildStatus) => {
        switch (status) {
            case "Completed":
                return "success";
            case "BuildTriggered":
                return "warning";
            case "BuildInProgress":
                return "warning";
            case "BuildFailed":
                return "error";
            default:
                return "default";
        }
    }
    const columns: TableColumn<BuildRow>[] = useMemo(() => [
        {
            id: "branch",
            label: "Branch",
            width: "15%",
            render: (value, row) => (
                <Chip
                    label={`${value} : ${row.commit}`}
                    size="small"
                    variant="outlined"
                    sx={{
                        borderColor: theme.palette.divider,
                        color: theme.palette.text.secondary,
                        backgroundColor: theme.palette.background.default,
                    }}
                />
            ),
        },
        {
            id: "title",
            label: "Title",
            width: "15%",
            render: (value) => (
                <Typography noWrap variant="body2" color="text.primary">
                    {value}
                </Typography>
            ),
        },
        {
            id: "startedAt",
            label: "Started At",
            width: "15%",
            render: (value) => (
                <Typography noWrap variant="body2" color="text.secondary">
                    {dayjs(value as string).format('DD/MM/YYYY HH:mm:ss')}
                </Typography>
            ),
        },
        {
            id: "status",
            label: "Status",
            width: "12%",
            render: (value) =>
                renderStatusChip(
                    {
                        color: getStatusColor(value as BuildStatus),
                        label: value as string,
                    },
                    theme
                ),
        },
        {
            id: "actions", label: "", width: "10%", render: (_value, row) => (
                <Box display="flex" justifyContent="flex-end">
                    <Button
                        variant="text"
                        color="secondary"
                        onClick={() => handleBuildClick(row.title, 'logs')}
                        size="small"
                        startIcon={<DataArray fontSize="small" />}
                    >
                        Logs
                    </Button>
                    <Button
                        variant="contained"
                        color="primary"
                        disabled={row.status === "BuildInProgress" || row.status === "BuildFailed"}
                        onClick={() => handleBuildClick(row.title, 'deploy')}
                        size="small"
                        startIcon={
                            row.status === "BuildInProgress" ?
                                <CircularProgress color="inherit" size={14} /> :
                                <Rocket fontSize="small" />
                        }
                    >
                        {row.status === "BuildInProgress" ? "Building..." : "Deploy"}
                    </Button>
                </Box>
            )
        },
    ], [theme, handleBuildClick]);

    return (
        (
            <>
                <DataListingTable
                    data={rows.map(row => ({
                        ...row,
                        actions: row.id
                    }))}
                    columns={columns}
                    pagination
                    pageSize={5}
                    maxRows={rows.length}
                    defaultSortBy="startedAt"
                    defaultSortDirection="desc"
                />
                <Drawer
                    anchor="right"
                    open={!!selectedBuildName}
                    onClose={clearSelectedBuild}
                    sx={{
                        zIndex: 1300,
                    }}
                >
                    <Box
                        width={theme.spacing(100)}
                        p={2}
                        height="100%"
                        display="flex"
                        flexDirection="column"
                        gap={2}
                        bgcolor={theme.palette.background.paper}
                    >
                        {selectedPanel === 'deploy' && (
                            <DeploymentConfig
                                onClose={clearSelectedBuild}
                                imageId={rows.find(row => row.id === selectedBuildName)?.imageId || 'busybox'}
                                to="development"
                                orgName={orgId || ''}
                                projName={projectId || ''}
                                agentName={agentId || ''}
                            />
                        )}
                        {selectedPanel === 'logs' && selectedBuildName && (
                            <BuildLogs
                                onClose={clearSelectedBuild}
                                orgName={orgId || ''}
                                projName={projectId || ''}
                                agentName={agentId || ''}
                                buildName={selectedBuildName}
                            />
                        )}
                    </Box>
                </Drawer>
            </>
        )

    );
}
