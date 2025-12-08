import { useMemo, useCallback } from "react";
import { Box, Button, CircularProgress, Typography, useTheme } from "@wso2/oxygen-ui";
import { DataListingTable, TableColumn, renderStatusChip, InitialState, DrawerWrapper } from "@agent-management-platform/views";
import { Rocket } from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import { BuildLogs, DeploymentConfig } from "@agent-management-platform/shared-component";
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
                <Typography noWrap variant="body2">
                    {`${value} : ${row.commit}`}
                </Typography>
            
            ),
        },
        {
            id: "title",
            label: "Build Name",
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
                <Box display="flex" justifyContent="flex-end" gap={1}>
                    <Button
                        variant="text"
                        color="primary"
                        onClick={() => handleBuildClick(row.title, 'logs')}
                        size="small"
                    >
                       Build Logs
                    </Button>
                    <Button
                        variant="outlined"
                        color="primary"
                        disabled={row.status === "BuildInProgress" || row.status === "BuildFailed"}
                        onClick={() => handleBuildClick(row.title, 'deploy')}
                        size="small"
                        startIcon={
                            row.status === "BuildInProgress" ?
                                <CircularProgress color="inherit" size={14} /> :
                                <Rocket size={16} />
                        }
                    >
                        {row.status === "BuildInProgress" ? "Building" : "Deploy"}
                    </Button>
                </Box>
            )
        },
    ], [theme, handleBuildClick]);

    // Define initial state for sorting - most recent builds first
    const tableInitialState: InitialState<BuildRow> = useMemo(() => ({
        sorting: {
            sortModel: [{
                field: 'startedAt',
                sort: 'desc'
            }]
        }
    }), []);

    return (
        (
            <Box display="flex" borderRadius={1} flexDirection="column" bgcolor={"background.paper"}>
                <DataListingTable
                    data={rows.map(row => ({
                        ...row,
                        actions: row.id
                    }))}
                    columns={columns}
                    pagination
                    pageSize={5}
                    maxRows={rows.length}
                    initialState={tableInitialState}
                />
                <DrawerWrapper open={!!selectedBuildName} onClose={clearSelectedBuild}>
                    {selectedPanel === 'deploy' && (
                        <DeploymentConfig
                            onClose={clearSelectedBuild}
                            imageId={rows.find(row =>
                                row.id === selectedBuildName)?.imageId || 'busybox'}
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
                </DrawerWrapper>
            </Box>
        )

    );
}
