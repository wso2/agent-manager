import { useEffect, useMemo, useState } from "react";
import {
    ListingTable,
    Chip,
    Stack,
    Typography,
    IconButton,
    TablePagination,
    Tooltip,
    Skeleton,
    Alert,
} from "@wso2/oxygen-ui";
import { Trash, Activity, AlertTriangle, Edit } from "@wso2/oxygen-ui-icons-react";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap, MonitorStatus, type MonitorResponse } from "@agent-management-platform/types";
import { useDeleteMonitor, useListEnvironments, useListMonitors } from "@agent-management-platform/api-client";
import { MonitorStartStopButton } from "./MonitorStartStopButton";

const getStatusColor = (status: MonitorStatus) => {
    switch (status) {
        case "Active":
            return "success";
        case "Suspended":
            return "default";
        default:
            return "error";
    }
};

export function MonitorTable() {
    const navigate = useNavigate();
    const [searchValue, setSearchValue] = useState("");
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(5);
    const { addConfirmation } = useConfirmationDialog();
    const { agentId, orgId, projectId, envId } = useParams<{
        orgId: string;
        projectId: string;
        agentId: string;
        envId: string;
    }>();
    const { data: monitorsList, isLoading, error } = useListMonitors({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
    });

    const { mutate: deleteMonitor } = useDeleteMonitor();

    const { data: environmentsList } = useListEnvironments({
        orgName: orgId ?? "",
    });

    const environmentDisplayNameMap = useMemo(() => {
        if (!environmentsList) {
            return {} as Record<string, string>;
        }
        return environmentsList.reduce<Record<string, string>>((acc, environment) => {
            const label = environment.displayName ?? environment.name;
            acc[environment.name] = label;
            if (environment.uuid) {
                acc[environment.uuid] = label;
            }
            return acc;
        }, {});
    }, [environmentsList]);

    const monitors = useMemo(() => {
        return (monitorsList?.monitors ?? []).map((monitor: MonitorResponse) => ({
            id: monitor.id,
            displayName: monitor.displayName,
            name: monitor.name,
            environment:
                environmentDisplayNameMap[monitor.environmentId ?? ""] ??
                environmentDisplayNameMap[monitor.environmentName ?? ""] ??
                monitor.environmentName ??
                "-",
            dataSource: monitor.type === "future" ? "Continuous" : "Historical",
            evaluators:
                monitor.evaluators?.map((evaluator) => evaluator.name).filter(
                    (name): name is string => Boolean(name)) ?? [],
            type: monitor.type,
            status: monitor.status ?? "Unknown",
        }));
    }, [environmentDisplayNameMap, monitorsList?.monitors]);
    const filteredMonitors = useMemo(() => {
        const term = searchValue.trim().toLowerCase();
        if (!term) {
            return monitors;
        }
        return monitors.filter((monitor) => {
            const haystack = [
                monitor.name,
                monitor.environment,
                monitor.dataSource,
                ...monitor.evaluators,
                monitor.status,
            ]
                .join(" ")
                .toLowerCase();
            return haystack.includes(term);
        });
    }, [monitors, searchValue]);

    useEffect(() => {
        if (page !== 0 && page * rowsPerPage >= filteredMonitors.length) {
            setPage(0);
        }
    }, [filteredMonitors.length, page, rowsPerPage]);

    const toolbar = (
        <ListingTable.Toolbar
            showSearch
            searchValue={searchValue}
            onSearchChange={setSearchValue}
            searchPlaceholder="Search monitors..."
        />
    );

    if (error) {
        return (
            <ListingTable.Container>
                {toolbar}
                <Alert
                    severity="error"
                    icon={<AlertTriangle size={18} />}
                    sx={{ alignSelf: "stretch" }}
                >
                    {error instanceof Error ? error.message : "Failed to load monitors. Please try again."}
                </Alert>
            </ListingTable.Container>
        );
    }

    if (isLoading) {
        return (
            <ListingTable.Container>
                {toolbar}
                <Stack spacing={1} m={2}>
                    <Skeleton variant="rounded" height={60} />
                    <Skeleton variant="rounded" height={60} />
                    <Skeleton variant="rounded" height={60} />
                    <Skeleton variant="rounded" height={60} />
                </Stack>
            </ListingTable.Container>
        );
    }

    if (!monitors.length) {
        return (
            <ListingTable.Container>
                {toolbar}
                <ListingTable.EmptyState
                    illustration={<Activity size={64} />}
                    title="No monitors yet"
                    description="Create a monitor to start tracking your evaluations."
                />
            </ListingTable.Container>
        );
    }

    if (!filteredMonitors.length) {
        return (
            <ListingTable.Container>
                {toolbar}
                <ListingTable.EmptyState
                    illustration={<Activity size={64} />}
                    title="No monitors match your search"
                    description="Try adjusting your search keywords."
                />
            </ListingTable.Container>
        );
    }

    const paginatedMonitors = filteredMonitors.slice(
        page * rowsPerPage,
        page * rowsPerPage + rowsPerPage
    );

    return (
        <ListingTable.Container>
            {toolbar}
            <ListingTable>
                <ListingTable.Head>
                    <ListingTable.Row>
                        <ListingTable.Cell>Name</ListingTable.Cell>
                        <ListingTable.Cell align="center">Status</ListingTable.Cell>
                        <ListingTable.Cell>Data Source</ListingTable.Cell>
                        <ListingTable.Cell>Evaluators</ListingTable.Cell>
                        <ListingTable.Cell>Actions</ListingTable.Cell>
                    </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                    {paginatedMonitors.map((monitor) => (
                        <ListingTable.Row
                            key={monitor.id}
                            hover
                            sx={{ cursor: "pointer" }}
                            onClick={() => navigate(
                                generatePath(
                                    absoluteRouteMap.children
                                        .org.children.projects.children.agents
                                        .children.evaluation.children.monitor
                                        .children.view.path,
                                    {
                                        agentId, orgId, projectId, envId, monitorId: monitor.name
                                    }))}>
                            <ListingTable.Cell>
                                <Stack spacing={0.5}>
                                    <Typography variant="body1">{monitor.displayName}

                                    </Typography>
                                </Stack>
                            </ListingTable.Cell>
                            <ListingTable.Cell align="center">
                                <Chip
                                    size="small"
                                    label={monitor.status}
                                    variant="outlined"
                                    color={getStatusColor(monitor.status)}
                                    sx={{ ml: 1 }}
                                />
                            </ListingTable.Cell>
                            <ListingTable.Cell>{monitor.dataSource}</ListingTable.Cell>
                            <ListingTable.Cell>
                                <Stack direction="row" spacing={1} flexWrap="wrap">
                                    {monitor.evaluators.slice(0, 2).map((evaluator) => (
                                        <Chip key={evaluator} size="small" label={evaluator} />
                                    ))}
                                    {monitor.evaluators.length > 2 && (
                                        <Tooltip title={monitor.evaluators.join(", ")}>
                                            <Typography variant="caption" color="text.secondary">
                                                {`+${monitor.evaluators.length - 2} more..`}
                                            </Typography>
                                        </Tooltip>
                                    )}
                                </Stack>
                            </ListingTable.Cell>
                            <ListingTable.Cell onClick={(e) => e.stopPropagation()}>
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <MonitorStartStopButton
                                        monitorName={monitor.name}
                                        monitorType={monitor.type}
                                        monitorStatus={monitor.status}
                                        orgId={orgId}
                                        projectId={projectId}
                                        agentId={agentId}
                                    />
                                    <IconButton onClick={() => navigate(
                                        generatePath(
                                            absoluteRouteMap.children
                                                .org.children.projects.children.agents
                                                .children.evaluation.children.monitor
                                                .children.edit.path,
                                            {
                                                agentId, orgId, projectId,
                                                envId, monitorId: monitor.name
                                            }))}>
                                        <Edit size={16} />
                                    </IconButton>
                                    <Tooltip title="Delete Monitor">
                                        <IconButton color="error">
                                            <Trash size={16} onClick={() => addConfirmation(
                                                {
                                                    title: "Delete Monitor",
                                                    description: "Are you sure you want to delete this monitor? This action cannot be undone.",
                                                    confirmButtonText: "Delete",
                                                    onConfirm: () => {
                                                        //delete action
                                                        deleteMonitor({
                                                            monitorName: monitor.name,
                                                            orgName: orgId,
                                                            projName: projectId,
                                                            agentName: agentId,
                                                        }, {

                                                        });

                                                    }
                                                }
                                            )} />
                                        </IconButton>
                                    </Tooltip>
                                </Stack>
                            </ListingTable.Cell>
                        </ListingTable.Row>
                    ))}
                </ListingTable.Body>
            </ListingTable>
            <TablePagination
                component="div"
                count={filteredMonitors.length}
                page={page}
                rowsPerPage={rowsPerPage}
                onPageChange={(_event, newPage) => setPage(newPage)}
                onRowsPerPageChange={(event) => {
                    setRowsPerPage(parseInt(event.target.value, 10));
                    setPage(0);
                }}
                rowsPerPageOptions={[5, 10, 25]}
            />
        </ListingTable.Container>
    );
}

