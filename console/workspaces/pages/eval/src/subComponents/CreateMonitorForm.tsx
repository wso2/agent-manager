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

import { AdapterDateFns, Alert, Avatar, Box, CardContent, CardHeader, Chip, Collapse, DatePickers, Form, ListingTable, SearchBar, Skeleton, Slider, Stack, TextField, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Check, CircleIcon, History, Timer, Search as SearchIcon } from "@wso2/oxygen-ui-icons-react";
import type { MonitorType } from "@agent-management-platform/types";
import type { CreateMonitorFormValues } from "../form/schema";
import { useListEvaluators } from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";
import { useMemo, useState } from "react";


const toSlug = (value: string): string => value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 60);

interface CreateMonitorFormProps {
    formData: CreateMonitorFormValues;
    errors: Partial<Record<keyof CreateMonitorFormValues, string | undefined>>;
    onFieldChange: (field: keyof CreateMonitorFormValues, value: unknown) => void;
}

interface SelectPresetMonitorsProps {
    selectedEvaluators: string[];
    onToggleEvaluator: (value: string) => void;
    error?: string;
}

export function CreateMonitorForm({ formData, errors, onFieldChange }: CreateMonitorFormProps) {
    const handleTypeChange = (nextType: MonitorType) => {
        if (formData.type === nextType) {
            return;
        }
        onFieldChange("type", nextType);
        if (nextType === "future") {
            onFieldChange("traceStart", null);
            onFieldChange("traceEnd", null);
        }
    };

    return (
        <Form.Stack>
            <Form.Section>
                <Form.Header>
                    Basic Details
                </Form.Header>
                <Form.ElementWrapper name="displayName" label="Monitor Title" >
                    <TextField
                        id="displayName"
                        placeholder="Enter monitor name"
                        required
                        fullWidth
                        value={formData.displayName}
                        onChange={(event) => onFieldChange("displayName", event.target.value)}
                        error={!!errors.displayName}
                        helperText={errors.displayName ?? "Visible label shown in the monitors list"}
                    />
                </Form.ElementWrapper>
                <Form.ElementWrapper name="description" label="Description" >
                    <TextField
                        id="description"
                        placeholder="Enter monitor description"
                        fullWidth
                        multiline
                        minRows={3}
                        value={formData.description ?? ""}
                        onChange={(event) => onFieldChange("description", event.target.value)}
                        error={!!errors.description}
                        helperText={errors.description}
                    />
                </Form.ElementWrapper>
            </Form.Section>
            <Form.Section>
                <Form.Header>
                    Data Collection
                </Form.Header>
                <Form.Stack direction="row">
                    <Form.CardButton onClick={() => handleTypeChange("past")} selected={formData.type === "past"}>
                        <Form.CardHeader title={<Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                            <History size={24} />
                            <Form.Body>Past Traces</Form.Body>
                        </Form.Stack>} />
                    </Form.CardButton>
                    <Form.CardButton onClick={() => handleTypeChange("future")} selected={formData.type === "future"}>
                        <Form.CardHeader title={<Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                            <Timer size={24} />
                            <Form.Body>Future Traces</Form.Body>
                        </Form.Stack>} />
                    </Form.CardButton>
                </Form.Stack>
                <Collapse in={formData.type === "past"}>
                    <Form.Stack direction="row" maxWidth={600} justifyContent="space-around" alignItems="flex-start" >
                        <Form.ElementWrapper name="traceStart" label="Start Time" >
                            <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                                <DatePickers.DateTimePicker
                                    value={formData.traceStart ?? null}
                                    onChange={(value) => onFieldChange("traceStart", value)}
                                    slotProps={{
                                        textField: {
                                            fullWidth: true,
                                            error: !!errors.traceStart,
                                            helperText: errors.traceStart,
                                        },
                                    }}
                                />
                            </DatePickers.LocalizationProvider>
                        </Form.ElementWrapper>
                        <Form.ElementWrapper name="traceEnd" label="End Time" >
                            <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                                <DatePickers.DateTimePicker
                                    value={formData.traceEnd ?? null}
                                    onChange={(value) => onFieldChange("traceEnd", value)}
                                    slotProps={{
                                        textField: {
                                            fullWidth: true,
                                            error: !!errors.traceEnd,
                                            helperText: errors.traceEnd,
                                        },
                                    }}
                                />
                            </DatePickers.LocalizationProvider>
                        </Form.ElementWrapper>
                    </Form.Stack>
                </Collapse>
                <Form.Stack direction="column" spacing={2} maxWidth={600}>
                    <Form.ElementWrapper name="intervalMinutes" label="Run Every (minutes)">
                        <TextField
                            id="intervalMinutes"
                            type="number"
                            placeholder="60"
                            value={formData.intervalMinutes ?? ""}
                            onChange={(event) => onFieldChange("intervalMinutes", event.target.value)}
                            error={!!errors.intervalMinutes}
                            helperText={errors.intervalMinutes ?? "How often the monitor should execute"}
                            fullWidth
                        />
                    </Form.ElementWrapper>
                    <Form.ElementWrapper name="samplingRate" label={`Sampling (${formData.samplingRate ?? 0}%)`}>
                        <Stack spacing={1} sx={{ px: 1 }}>
                            <Slider
                                aria-label="Sampling rate"
                                min={0}
                                max={100}
                                step={1}
                                value={formData.samplingRate ?? 0}
                                valueLabelDisplay="auto"
                                onChange={(_, value) => {
                                    const nextValue = Array.isArray(value) ? value[0] : value;
                                    onFieldChange("samplingRate", nextValue);
                                }}
                            />
                            <Typography variant="caption" color={errors.samplingRate ? "error" : "text.secondary"}>
                                {errors.samplingRate ?? "Percentage of traces evaluated"}
                            </Typography>
                        </Stack>
                    </Form.ElementWrapper>
                </Form.Stack>
            </Form.Section>
        </Form.Stack>

    );
}

export function SelectPresetMonitors(
    { selectedEvaluators, onToggleEvaluator, error }: SelectPresetMonitorsProps) {
    const { orgId } = useParams<{ orgId: string }>();
    const { data, isPending, error: evaluatorsError } = useListEvaluators({
        orgName: orgId,
    });
    const evaluators = useMemo(() => data?.evaluators ?? [], [data]);
    const [search, setSearch] = useState("");

    const filteredEvaluators = useMemo(() => {
        const term = search.trim().toLowerCase();
        if (!term) {
            return evaluators;
        }
        return evaluators.filter((evaluator) => {
            const haystack = [
                evaluator.displayName,
                evaluator.identifier,
                evaluator.description,
                ...(evaluator.tags ?? []),
            ]
                .filter(Boolean)
                .map((value) => value?.toLowerCase() ?? "");
            return haystack.some((value) => value.includes(term));
        });
    }, [evaluators, search]);

    const selectedFullEval = evaluators.filter(evaluator =>
        selectedEvaluators.includes(evaluator.identifier ?? toSlug(evaluator.displayName)));
    return (
        <Form.Stack>
            <Form.Section>
                <Form.Header   >
                    <Stack direction="row" spacing={1} alignItems="start" justifyContent="space-between">
                        Available Evaluators and Metrics
                        <SearchBar
                            placeholder="Search evaluators"
                            size="small"
                            value={search}
                            onChange={(event) => setSearch(event.target.value)}
                            disabled={!evaluators.length}
                        />
                    </Stack>
                </Form.Header>
                <Form.Section>
                    <Stack direction="row" spacing={2} flexWrap="wrap" alignItems="center">
                        {
                            selectedFullEval.map((evaluator) => {
                                const identifier = evaluator.identifier ??
                                    toSlug(evaluator.displayName);
                                return (
                                    <Box py={0.25} key={evaluator.id}>
                                        <Chip
                                            key={evaluator.id}
                                            label={evaluator.displayName}
                                            onDelete={() => onToggleEvaluator(identifier)}
                                            color="primary"
                                        />
                                    </Box>
                                );
                            })
                        }
                        {
                            selectedFullEval.length === 0 && (
                                <Typography variant="body2" color="text.secondary">
                                    No evaluators selected yet. Click on the cards below to select.
                                </Typography>
                            )
                        }
                    </Stack>
                </Form.Section>
                {!orgId && (
                    <Alert severity="warning" sx={{ mt: 2 }}>
                        Unable to determine organization.
                        Navigate from the project context to load evaluators.
                    </Alert>
                )}
                {evaluatorsError && (
                    <Alert severity="error" sx={{ mt: 2 }}>
                        {evaluatorsError instanceof Error ? evaluatorsError.message : "Failed to load evaluators"}
                    </Alert>
                )}
                {isPending && (
                    <Stack direction="row" gap={1} p={2}>
                        <Skeleton variant="rounded" height={160} width="100%" />
                        <Skeleton variant="rounded" height={160} width="100%" />
                        <Skeleton variant="rounded" height={160} width="100%" />
                        <Skeleton variant="rounded" height={160} width="100%" />
                    </Stack>
                )}
                {!isPending && orgId && !evaluatorsError && evaluators.length === 0 && (
                    <ListingTable.Container sx={{ my: 3 }}>
                        <ListingTable.EmptyState
                            illustration={<CircleIcon size={64} />}
                            title="No evaluators yet"
                            description="Connect evaluator providers or import custom evaluators to see them here."
                        />
                    </ListingTable.Container>
                )}
                {evaluators.length > 0 && filteredEvaluators.length === 0 && (
                    <ListingTable.Container sx={{ my: 3 }}>
                        <ListingTable.EmptyState
                            illustration={<SearchIcon size={64} />}
                            title="No evaluators match your search"
                            description="Try a different keyword or clear the search filter."
                        />
                    </ListingTable.Container>
                )}
                {filteredEvaluators.length > 0 && (
                    <Box
                        sx={{
                            display: "grid",
                            gridTemplateColumns: {
                                xs: "repeat(auto-fill, minmax(260px, 1fr))",
                                md: "repeat(auto-fill, minmax(300px, 1fr))",
                            },
                            gap: 2,
                        }}
                    >
                        {filteredEvaluators.map((monitor) => {
                            const identifier = monitor.identifier ?? toSlug(monitor.displayName);
                            const isSelected = selectedEvaluators.includes(identifier);
                            return (
                                <Form.CardButton
                                    key={monitor.id}
                                    sx={{ width: "100%" }}
                                    selected={isSelected}
                                    onClick={() => onToggleEvaluator(identifier)}
                                >
                                    <CardHeader title={<Stack direction="row" spacing={1} alignItems="center">

                                        <Stack direction="column" spacing={2}>
                                            <Stack direction="row" spacing={2} alignItems="center">
                                                <Avatar sx={{ bgcolor: isSelected ? "primary.main" : "default", width: 40, height: 40 }}>
                                                    {
                                                        isSelected ? <Check size={20} /> :
                                                            <CircleIcon size={20} />
                                                    }
                                                </Avatar>
                                                <Typography variant="h5" textOverflow="ellipsis" overflow="hidden" whiteSpace="nowrap" maxWidth="90%">
                                                    {monitor.displayName}
                                                </Typography>
                                            </Stack>
                                            <Stack direction="row" spacing={1} alignItems="center">
                                                {(monitor.tags.slice(0, 2) ?? []).map(tag => (
                                                    <Chip key={tag} size="small" label={tag} variant="outlined" />
                                                ))}
                                                {monitor.tags.length > 2 && (
                                                    <Tooltip title={monitor.tags.join(", ")} placement="top">
                                                        <Typography variant="caption" color="text.secondary">
                                                            {`+${monitor.tags.length - 2} more`}
                                                        </Typography>
                                                    </Tooltip>
                                                )}
                                            </Stack>
                                        </Stack>
                                    </Stack>} />
                                    <CardContent>
                                        <Stack spacing={1}>
                                            <Typography variant="caption">
                                                {monitor.description}
                                            </Typography>
                                        </Stack>
                                    </CardContent>

                                </Form.CardButton>
                            );
                        })}
                    </Box>
                )}
                {error && (
                    <Typography variant="caption" color="error" sx={{ mt: 1 }}>
                        {error}
                    </Typography>
                )}
            </Form.Section>
        </Form.Stack>
    );
}

