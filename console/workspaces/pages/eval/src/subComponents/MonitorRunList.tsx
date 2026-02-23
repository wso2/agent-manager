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

import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import {
    ListingTable,
    Chip,
    Stack,
    Typography,
    Tooltip,
    Skeleton,
    Alert,
    TablePagination,
    useTheme,
    CircularProgress,
    IconButton,
} from "@wso2/oxygen-ui";
import { Activity, AlertTriangle, CheckCircle, CircleAlert, RefreshCcw, Repeat } from "@wso2/oxygen-ui-icons-react";
import { useListMonitorRuns, useRerunMonitor } from "@agent-management-platform/api-client";
import { type MonitorRunResponse } from "@agent-management-platform/types";
import { DrawerWrapper } from "@agent-management-platform/views";
import { MonitorRunDrawer } from "./MonitorRunDrawer";

const formatDuration = (startedAt?: string, completedAt?: string) => {
    if (!startedAt) {
        return "-";
    }
    const startTime = Date.parse(startedAt);
    const endTime = completedAt ? Date.parse(completedAt) : Date.now();
    if (Number.isNaN(startTime) || Number.isNaN(endTime)) {
        return "-";
    }
    const diffMs = Math.max(endTime - startTime, 0);
    const totalSeconds = Math.floor(diffMs / 1000);
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;
    const parts: string[] = [];
    if (hours) {
        parts.push(`${hours}h`);
    }
    if (minutes || hours) {
        parts.push(`${minutes}m`);
    }
    parts.push(`${seconds}s`);
    return parts.join(" ");
};

const formatWithFormatter = (formatter: Intl.DateTimeFormat, value?: string) => {
    if (!value) {
        return undefined;
    }
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
        return undefined;
    }
    return formatter.format(date);
};

const formatTraceWindow = (formatter: Intl.DateTimeFormat, traceStart?:
    string, traceEnd?: string) => {
    const startLabel = formatWithFormatter(formatter, traceStart);
    const endLabel = formatWithFormatter(formatter, traceEnd);
    if (!startLabel && !endLabel) {
        return "-";
    }
    return `${startLabel ?? "-"} → ${endLabel ?? "-"}`;
};

const partitionEvaluators = (evaluators?: MonitorRunResponse["evaluators"]) => {
    const list = evaluators ?? [];
    const visible = list.slice(0, 3);
    const extraLabels = list
        .slice(3)
        .map((evaluator) => evaluator.displayName ?? evaluator.identifier)
        .filter((name): name is string => Boolean(name));
    return { visible, extraLabels };
};


export default function MonitorRunList() {
    const { orgId, projectId, agentId, monitorId } = useParams<{
        orgId: string;
        projectId: string;
        agentId: string;
        monitorId: string;
    }>();
    const [searchValue, setSearchValue] = useState("");
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(5);
    const theme = useTheme();
    const [searchParams, setSearchParams] = useSearchParams();
    const {mutate: rerunMonitor, isPending: isRerunning} = useRerunMonitor();

    const dateFormatter = useMemo(
        () =>
            new Intl.DateTimeFormat(undefined, {
                dateStyle: "medium",
                timeStyle: "short",
            }),
        []
    );

    const { data, isLoading, error, refetch, isRefetching } = useListMonitorRuns({
        monitorName: monitorId ?? "",
        orgName: orgId ?? "",
        projName: projectId ?? "",
        agentName: agentId ?? "",
    });

    const runs = useMemo(() => data?.runs ?? [], [data]);

    const selectedRunId = searchParams.get("runId");

    const selectedRun = useMemo(() => {
        if (!selectedRunId) {
            return null;
        }
        return runs.find((run) => run.id === selectedRunId) ?? null;
    }, [runs, selectedRunId]);

    const handleRowClick = useCallback((run: MonitorRunResponse) => {
        const next = new URLSearchParams(searchParams);
        next.set("runId", run.id);
        setSearchParams(next, { replace: true });
    }, [searchParams, setSearchParams]);

    const handleDrawerClose = useCallback(() => {
        const next = new URLSearchParams(searchParams);
        next.delete("runId");
        setSearchParams(next, { replace: true });
    }, [searchParams, setSearchParams]);

    const drawerOpen = Boolean(selectedRun);

    const filteredRuns = useMemo(() => {
        const term = searchValue.trim().toLowerCase();
        if (!term) {
            return runs;
        }
        return runs.filter((run) => {
            const evaluatorNames = run.evaluators?.map((evaluator) => evaluator.displayName ?? evaluator.identifier ?? "").join(" ") ?? "";
            const haystack = [
                run.id,
                run.status,
                run.errorMessage ?? "",
                evaluatorNames,
                run.traceStart ?? "",
                run.traceEnd ?? "",
                run.startedAt ?? "",
                run.completedAt ?? "",
            ]
                .join(" ")
                .toLowerCase();
            return haystack.includes(term);
        });
    }, [runs, searchValue]);

    useEffect(() => {
        if (page !== 0 && page * rowsPerPage >= filteredRuns.length) {
            setPage(0);
        }
    }, [filteredRuns.length, page, rowsPerPage]);

    const formatDateTime = (value?: string) => formatWithFormatter(dateFormatter, value) ?? "-";

    const toolbar = (
        <ListingTable.Toolbar
            showSearch
            searchValue={searchValue}
            onSearchChange={setSearchValue}
            searchPlaceholder="Search runs..."
            actions={
                <IconButton  color="primary" onClick={() => refetch()} disabled={isLoading}>
                    {isRefetching ? <CircularProgress size={20} /> : <RefreshCcw size={20} />}
                </IconButton>
            }
        />
    );

    const palette = theme.vars?.palette;
    const statusIcons = useMemo(() => ({
        failed: <CircleAlert size={20} color={palette?.error.main} />,
        success: <CheckCircle size={20} color={palette?.success.main} />,
        running: <Activity size={20} color={palette?.warning.main} />,
        pending: <CircularProgress size={20}/>,
    }), [palette?.error.main, palette?.success.main, palette?.warning.main]);

    if (error) {
        return (
            <ListingTable.Container>
                {toolbar}
                <Alert severity="error" icon={<AlertTriangle size={18} />} sx={{ alignSelf: "stretch" }}>
                    {error instanceof Error ? error.message : "Failed to load monitor runs. Please try again."}
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

    if (!runs.length) {
        return (
            <ListingTable.Container>
                {toolbar}
                <ListingTable.EmptyState
                    illustration={<Activity size={64} />}
                    title="No runs yet"
                    description="Run this monitor to see evaluation history."
                />
            </ListingTable.Container>
        );
    }

    if (!filteredRuns.length) {
        return (
            <ListingTable.Container>
                {toolbar}
                <ListingTable.EmptyState
                    illustration={<Activity size={64} />}
                    title="No runs match your search"
                    description="Try a different keyword."
                />
            </ListingTable.Container>
        );
    }

    const paginatedRuns = filteredRuns.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);

    return (
        <>
            <ListingTable.Container>
                {toolbar}
                <ListingTable>
                    <ListingTable.Head>
                        <ListingTable.Row>
                            <ListingTable.Cell align="center">Status</ListingTable.Cell>
                            <ListingTable.Cell>Trace Window</ListingTable.Cell>
                            <ListingTable.Cell>Evaluators</ListingTable.Cell>
                            <ListingTable.Cell>Started</ListingTable.Cell>
                            <ListingTable.Cell align="right">Duration</ListingTable.Cell>
                            <ListingTable.Cell align="center">Actions</ListingTable.Cell>
                        </ListingTable.Row>
                    </ListingTable.Head>
                    <ListingTable.Body>
                        {paginatedRuns.map((run: MonitorRunResponse) => {
                            const { visible, extraLabels } = partitionEvaluators(run.evaluators);
                            return (
                                <ListingTable.Row
                                    key={run.id}
                                    hover
                                    clickable
                                    onClick={() => handleRowClick(run)}
                                >
                                    <ListingTable.Cell align="center">
                                        {statusIcons[run.status as keyof typeof statusIcons]
                                            ?? null}
                                    </ListingTable.Cell>
                                    <ListingTable.Cell>
                                        {formatTraceWindow(dateFormatter, run.traceStart,
                                            run.traceEnd)}
                                    </ListingTable.Cell>
                                    <ListingTable.Cell>
                                        <Stack direction="row" spacing={1} flexWrap="wrap">
                                            {visible.map((evaluator, index) => {
                                                const label = evaluator.displayName ?? evaluator.identifier ?? `Evaluator ${index + 1}`;
                                                return (
                                                    <Chip
                                                        key={`${run.id}-${label}-${index}`}
                                                        size="small"
                                                        label={label}
                                                    />
                                                );
                                            })}
                                            {extraLabels.length > 0 && (
                                                <Tooltip title={extraLabels.join(", ")}>
                                                    <Typography variant="caption" color="text.secondary">
                                                        {`+${extraLabels.length} more`}
                                                    </Typography>
                                                </Tooltip>
                                            )}
                                        </Stack>
                                    </ListingTable.Cell>
                                    <ListingTable.Cell>
                                        {formatDateTime(run.startedAt)}</ListingTable.Cell>
                                    <ListingTable.Cell align="right">
                                        {formatDuration(run.startedAt, run.completedAt)}
                                    </ListingTable.Cell>
                                    <ListingTable.Cell
                                        align="center"
                                        onClick={(e) => e.stopPropagation()}
                                    >
                                        <Tooltip title="Re-run">
                                            <span>
                                                <IconButton
                                                    size="small"
                                                    disabled={
                                                        isRerunning ||
                                                        run.status === "running" ||
                                                        run.status === "pending"
                                                    }
                                                    onClick={() =>
                                                        rerunMonitor({
                                                            monitorName: monitorId ?? "",
                                                            orgName: orgId ?? "",
                                                            projName: projectId ?? "",
                                                            agentName: agentId ?? "",
                                                            runId: run.id,
                                                        })
                                                    }
                                                >
                                                    <Repeat size={16} />
                                                </IconButton>
                                            </span>
                                        </Tooltip>
                                    </ListingTable.Cell>
                                </ListingTable.Row>
                            );
                        })}
                    </ListingTable.Body>
                </ListingTable>
                <TablePagination
                    component="div"
                    count={filteredRuns.length}
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
            <DrawerWrapper open={drawerOpen} onClose={handleDrawerClose} maxWidth={360}>
                {selectedRun && (
                    <MonitorRunDrawer
                        run={selectedRun}
                        onClose={handleDrawerClose}
                        orgName={orgId ?? ""}
                        projectName={projectId ?? ""}
                        agentName={agentId ?? ""}
                        monitorName={monitorId ?? ""}
                        traceWindowLabel={
                            formatTraceWindow(dateFormatter,
                                selectedRun.traceStart, selectedRun.traceEnd)}
                        durationLabel={
                            formatDuration(selectedRun.startedAt, selectedRun.completedAt)}
                    />
                )}
            </DrawerWrapper>
        </>
    );
}
