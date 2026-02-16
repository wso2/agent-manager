import { useMemo, useState } from "react";
import { ListingTable, Chip, Stack, Typography, IconButton, TablePagination, Tooltip } from "@wso2/oxygen-ui";
import { Trash, Edit } from "@wso2/oxygen-ui-icons-react";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

const mock_monitors = [
    {
        name: "Latency Guard",
        id: "1",
        environment: "Production",
        dataSource: "continuous",
        evaluators: ["LatencyEval", "SLOCheck", "New", "custom-eval-1", "custom-eval-2"],
        status: "Active",
    },
    {
        name: "Error Spike",
        id: "2",
        environment: "Staging",
        dataSource: "batch",
        evaluators: ["ErrorRate", "CanaryEval"],
        status: "Degraded",
    },
    {
        name: "Token Watch",
        id: "3",
        environment: "Production",
        dataSource: "continuous",
        evaluators: ["CostGuard"],
        status: "Active",
    },
    {
        name: "Drift Detector",
        id: "4",
        environment: "Dev",
        dataSource: "adhoc",
        evaluators: ["EvalGPT", "Regression"],
        status: "Paused",
    },
];

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

    const filteredMonitors = useMemo(() => {
        const term = searchValue.trim().toLowerCase();
        if (!term) {
            return mock_monitors;
        }
        return mock_monitors.filter((monitor) => {
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
    }, [searchValue]);

    return (
        <ListingTable.Container>
            <ListingTable.Toolbar
                showSearch
                searchValue={searchValue}
                onSearchChange={setSearchValue}
                searchPlaceholder="Search monitors..."
            />
            <ListingTable>
                <ListingTable.Head>
                    <ListingTable.Row>
                        <ListingTable.Cell>Name</ListingTable.Cell>
                        <ListingTable.Cell>Environment</ListingTable.Cell>
                        <ListingTable.Cell>Data Source</ListingTable.Cell>
                        <ListingTable.Cell>Evaluators</ListingTable.Cell>
                        <ListingTable.Cell>Actions</ListingTable.Cell>
                    </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                    {filteredMonitors
                        .slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage)
                        .map((monitor) => (
                            <ListingTable.Row
                                key={monitor.id}
                                hover
                                sx={{ cursor: "pointer" }}
                                onClick={() => navigate(
                                    generatePath(
                                        absoluteRouteMap.children
                                            .org.children.projects.children.agents
                                            .children.environment
                                            .children.evaluation.children.monitor
                                            .children.view.path,
                                        {
                                            agentId, orgId, projectId, envId, monitorId: monitor.id
                                        }))}>
                                <ListingTable.Cell>
                                    <Stack spacing={0.5}>
                                        <Typography variant="body1">{monitor.name}</Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            ID: {monitor.id}
                                        </Typography>
                                    </Stack>
                                </ListingTable.Cell>
                                <ListingTable.Cell>{monitor.environment}</ListingTable.Cell>
                                <ListingTable.Cell>{monitor.dataSource}</ListingTable.Cell>
                                <ListingTable.Cell>
                                    <Stack direction="row" spacing={1} flexWrap="wrap">
                                        {monitor.evaluators.slice(0, 2).map((evaluator) => (
                                            <Chip key={evaluator} size="small" label={evaluator} />
                                        ))}
                                        {monitor.evaluators.length > 2 && (
                                            <Typography variant="caption" color="text.secondary">
                                                {`+${monitor.evaluators.length - 2} more..`}
                                            </Typography>
                                        )}
                                    </Stack>
                                </ListingTable.Cell>
                                <ListingTable.Cell onClick={(e) => e.stopPropagation()}>
                                    <Stack direction="row" spacing={1} alignItems="center">
                                        <Tooltip title="Edit Monitor">
                                            <IconButton color="default">
                                                <Edit size={16} />
                                            </IconButton>
                                        </Tooltip>
                                        <Tooltip title="Delete Monitor">
                                            <IconButton color="error">
                                                <Trash size={16} onClick={() => addConfirmation(
                                                    {
                                                        title: "Delete Monitor",
                                                        description: "Are you sure you want to delete this monitor? This action cannot be undone.",
                                                        confirmButtonText: "Delete",
                                                        onConfirm: () => {
                                                            //delete action
                                                            console.log("Deleted monitor with id: ", monitor.id);
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

