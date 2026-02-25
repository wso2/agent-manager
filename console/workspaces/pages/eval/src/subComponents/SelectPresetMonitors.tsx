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

import { Alert, Avatar, Box, Button, CardContent, CardHeader, Chip, Divider, Form, IconButton, ListingTable, MenuItem, SearchBar, Skeleton, Stack, TextField, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Check, CircleIcon, Plus, Search as SearchIcon, Trash } from "@wso2/oxygen-ui-icons-react";
import type { EvaluatorResponse, MonitorEvaluator, MonitorLLMProviderConfig } from "@agent-management-platform/types";
import { useListEvaluatorLLMProviders, useListEvaluators } from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";
import { useMemo, useState, useCallback } from "react";
import EvaluatorDetailsDrawer from "./EvaluatorDetailsDrawer";

const toSlug = (value: string): string => value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60);

/** A single entry in the local LLM pool — provider + optional model + config values. */
export interface LLMPoolEntry {
    /** Stable local key for React lists */
    id: string;
    llmProviderId: string;
    model?: string;
    configValues?: Record<string, string>;
}

interface SelectPresetMonitorsProps {
    selectedEvaluators: MonitorEvaluator[];
    onToggleEvaluator: (evaluator: EvaluatorResponse) => void;
    onSaveEvaluatorConfig: (evaluator: EvaluatorResponse, config: Record<string, unknown>) => void;
    /** Per-evaluator assignment (evaluatorIdentifier → pool entry id) */
    llmProviderConfigs: MonitorLLMProviderConfig[];
    onSaveLLMProviderConfig: (evaluatorIdentifier: string,
        llmProviderId: string | null, model?: string,
        configValues?: Record<string, string>) => void;
    /** The pool of LLM configs the user has added */
    llmPool: LLMPoolEntry[];
    onLLMPoolChange: (pool: LLMPoolEntry[]) => void;
    error?: string;
}

export function SelectPresetMonitors({
    selectedEvaluators,
    onToggleEvaluator,
    onSaveEvaluatorConfig,
    llmProviderConfigs,
    onSaveLLMProviderConfig,
    llmPool,
    onLLMPoolChange,
    error,
}: SelectPresetMonitorsProps) {
    const { orgId } = useParams<{ orgId: string }>();
    const { data, isPending, error: evaluatorsError } = useListEvaluators({
        orgName: orgId,
    });
    const evaluators = useMemo(() => data?.evaluators ?? [], [data]);
    const { data: llmProvidersData, isPending: isLLMProvidersPending } =
        useListEvaluatorLLMProviders({
            orgName: orgId,
        });

    const llmProviders = useMemo(() => llmProvidersData?.list ?? [], [llmProvidersData]);

    // Draft state for the "add a new LLM entry" row
    const [draftProviderId, setDraftProviderId] = useState("");
    const [draftModel, setDraftModel] = useState("");
    const [draftConfigValues, setDraftConfigValues] = useState<Record<string, string>>({});

    const draftProvider = useMemo(
        () => llmProviders.find((p) => p.name === draftProviderId) ?? null,
        [llmProviders, draftProviderId]
    );

    const handleAddPoolEntry = useCallback(() => {
        if (!draftProviderId) return;
        const entry: LLMPoolEntry = {
            id: `${draftProviderId}:${draftModel}:${Date.now()}`,
            llmProviderId: draftProviderId,
            model: draftModel || undefined,
            configValues: Object.keys(draftConfigValues).length > 0
                ? { ...draftConfigValues } : undefined,
        };
        onLLMPoolChange([...llmPool, entry]);
        setDraftProviderId("");
        setDraftModel("");
        setDraftConfigValues({});
    }, [draftProviderId, draftModel, draftConfigValues, llmPool, onLLMPoolChange]);

    const handleRemovePoolEntry = useCallback((entryId: string) => {
        onLLMPoolChange(llmPool.filter((e) => e.id !== entryId));
        // Clear any evaluator assignment that pointed at this entry
        // (find by matching llmProviderId+model)
        const removed = llmPool.find((e) => e.id === entryId);
        if (removed) {
            llmProviderConfigs
                .filter((c) => c.llmProviderId ===
                    removed.llmProviderId && c.model === removed.model)
                .forEach((c) => onSaveLLMProviderConfig(c.evaluatorIdentifier, null));
        }
    }, [llmPool, llmProviderConfigs, onLLMPoolChange, onSaveLLMProviderConfig]);

    const [search, setSearch] = useState("");
    const [drawerEvaluator, setDrawerEvaluator] = useState<EvaluatorResponse | null>(null);

    const selectedEvaluatorNames = useMemo(
        () => selectedEvaluators.map((item) => item.identifier),
        [selectedEvaluators]
    );

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

    const selectedFullEval = evaluators.filter((evaluator) =>
        selectedEvaluatorNames.includes(evaluator.identifier ?? toSlug(evaluator.displayName))
    );

    const handleOpenDrawer = useCallback((evaluator: EvaluatorResponse) => {
        setDrawerEvaluator(evaluator);
    }, []);

    const handleCloseDrawer = useCallback(() => {
        setDrawerEvaluator(null);
    }, []);

    const drawerIdentifier = drawerEvaluator
        ? drawerEvaluator.identifier ?? toSlug(drawerEvaluator.displayName)
        : "";
    const drawerEvaluatorAlreadySelected = drawerIdentifier
        ? selectedEvaluatorNames.includes(drawerIdentifier)
        : false;

    const drawerInitialConfig = useMemo(() => {
        if (!drawerIdentifier) {
            return undefined;
        }
        return selectedEvaluators.find((item) => item.identifier === drawerIdentifier)?.config;
    }, [drawerIdentifier, selectedEvaluators]);

    const drawerLLMProviderConfig = useMemo(() => {
        if (!drawerIdentifier) return null;
        return llmProviderConfigs.find((c) => c.evaluatorIdentifier === drawerIdentifier) ?? null;
    }, [drawerIdentifier, llmProviderConfigs]);

    // Find which pool entry is currently assigned to the drawer's evaluator
    const assignedLLMEntryId = useMemo(() => {
        if (!drawerLLMProviderConfig) return null;
        const assigned = llmPool.find(
            (e) =>
                e.llmProviderId === drawerLLMProviderConfig.llmProviderId &&
                e.model === drawerLLMProviderConfig.model
        );
        return assigned?.id ?? null;
    }, [drawerLLMProviderConfig, llmPool]);

    const handleAssignLLMEntry = useCallback(
        (entryId: string | null) => {
            if (!drawerIdentifier) return;
            if (!entryId) {
                onSaveLLMProviderConfig(drawerIdentifier, null);
                return;
            }
            const entry = llmPool.find((e) => e.id === entryId);
            if (entry) {
                onSaveLLMProviderConfig(drawerIdentifier,
                    entry.llmProviderId, entry.model, entry.configValues);
            }
        },
        [drawerIdentifier, llmPool, onSaveLLMProviderConfig]
    );

    const handleConfirmEvaluator = useCallback((config: Record<string, unknown>) => {
        if (!drawerEvaluator || !drawerIdentifier) {
            return;
        }
        onSaveEvaluatorConfig(drawerEvaluator, config);
        handleCloseDrawer();
    }, [drawerEvaluator, drawerIdentifier, handleCloseDrawer, onSaveEvaluatorConfig]);

    const handleRemoveEvaluator = useCallback(() => {
        if (!drawerEvaluator || !drawerIdentifier) {
            return;
        }
        if (selectedEvaluatorNames.includes(drawerIdentifier)) {
            onToggleEvaluator(drawerEvaluator);
        }
        handleCloseDrawer();
    }, [drawerEvaluator, drawerIdentifier,
        handleCloseDrawer, onToggleEvaluator, selectedEvaluatorNames]);

    return (
        <Form.Stack>
            <Form.Section>
                <Form.Header>LLM Provider Configurations</Form.Header>
                {isLLMProvidersPending ? (
                    <Skeleton variant="rounded" height={56} />
                ) : (
                    <Stack spacing={2}>
                        {/* Existing pool entries */}
                        {llmPool.map((entry) => {
                            const provider = llmProviders.find((p) =>
                                p.name === entry.llmProviderId);
                            return (
                                <Stack
                                    key={entry.id}
                                    direction="row"
                                    spacing={2}
                                    alignItems="center"
                                    sx={{
                                        p: 1.5,
                                        border: 1,
                                        borderColor: "divider",
                                        borderRadius: 1,
                                    }}
                                >
                                    <Stack direction="row" spacing={1} flexGrow={1} alignItems="center">
                                        <Chip
                                            size="small"
                                            label={provider?.displayName ?? entry.llmProviderId}
                                            color="primary"
                                            variant="outlined"
                                        />
                                        {entry.model && (
                                            <Chip size="small" label={entry.model} variant="outlined" />
                                        )}
                                    </Stack>
                                    <Tooltip title="Remove">
                                        <IconButton
                                            size="small"
                                            color="error"
                                            onClick={() => handleRemovePoolEntry(entry.id)}
                                        >
                                            <Trash size={16} />
                                        </IconButton>
                                    </Tooltip>
                                </Stack>
                            );
                        })}

                        {llmPool.length > 0 && <Divider />}

                        {/* Add new entry row */}
                        {llmProviders.length === 0 ? (
                            <Typography variant="body2" color="text.secondary">
                                No LLM providers available for this organization.
                            </Typography>
                        ) : (
                            <Stack spacing={2}>
                                <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
                                    <Form.ElementWrapper label="LLM Provider" name="draftProvider">
                                        <TextField
                                            select
                                            size="small"
                                            value={draftProviderId}

                                            onChange={(e) => {
                                                setDraftProviderId(e.target.value);
                                                setDraftModel("");
                                                setDraftConfigValues({});
                                            }}
                                        >
                                            <MenuItem value="">Select a provider</MenuItem>
                                            {llmProviders.map((p) => (
                                                <MenuItem key={p.name} value={p.name}>
                                                    {p.displayName}
                                                </MenuItem>
                                            ))}
                                        </TextField>
                                    </Form.ElementWrapper>
                                    {draftProvider && (draftProvider.models?.length ?? 0) > 0 && (
                                        <Form.ElementWrapper label="Model" name="draftModel">
                                            <TextField
                                                select
                                                size="small"
                                                value={draftModel}

                                                onChange={(e) => setDraftModel(e.target.value)}
                                            >
                                                <MenuItem value="">Default model</MenuItem>
                                                {draftProvider.models!.map((m) => (
                                                    <MenuItem key={m} value={m}>{m}</MenuItem>
                                                ))}
                                            </TextField>
                                        </Form.ElementWrapper>
                                    )}
                                </Stack>
                                {(draftProvider?.configFields?.length ?? 0) > 0 && (
                                    <Form.Stack>
                                        {draftProvider!.configFields!.map((field) => (
                                            <Form.Section key={field.key}>
                                                <Form.ElementWrapper
                                                    label={field.required ? `* ${field.label}` : field.label}
                                                    name={field.key}
                                                >
                                                    <TextField
                                                        size="small"
                                                        required={field.required}
                                                        type={field.fieldType === "password" ? "password" : "text"}
                                                        value={draftConfigValues[field.key] ?? ""}
                                                        fullWidth
                                                        onChange={(e) =>
                                                            setDraftConfigValues((prev) => ({
                                                                ...prev,
                                                                [field.key]: e.target.value,
                                                            }))
                                                        }
                                                    />
                                                </Form.ElementWrapper>
                                            </Form.Section>
                                        ))}
                                    </Form.Stack>
                                )}
                                <Box>
                                    <Button
                                        variant="outlined"
                                        size="small"
                                        startIcon={<Plus size={16} />}
                                        disabled={
                                            !draftProviderId ||
                                            (draftProvider?.configFields ?? [])
                                                .filter((f) => f.required)
                                                .some((f) => !draftConfigValues[f.key])
                                        }
                                        onClick={handleAddPoolEntry}
                                    >
                                        Add
                                    </Button>
                                </Box>
                            </Stack>
                        )}
                    </Stack>
                )}
            </Form.Section>
            <Form.Section>
                <Form.Header>
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
                        {selectedFullEval.map((evaluator) => {
                            return (
                                <Box py={0.25} key={evaluator.id}>
                                    <Chip
                                        label={evaluator.displayName}
                                        onDelete={() => onToggleEvaluator(evaluator)}
                                        color="primary"
                                    />
                                </Box>
                            );
                        })}
                        {selectedFullEval.length === 0 && (
                            <Typography variant="body2" color="text.secondary">
                                No evaluators selected yet. Click on the cards below to select.
                            </Typography>
                        )}
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
                            const isSelected = selectedEvaluators.some(
                                (item) => item.identifier === identifier);
                            return (
                                <Form.CardButton
                                    key={monitor.id}
                                    sx={{ width: "100%" }}
                                    selected={isSelected}
                                    onClick={() => handleOpenDrawer(monitor)}
                                >
                                    <CardHeader
                                        title={
                                            <Stack direction="row" spacing={1} alignItems="center">
                                                <Stack direction="column" spacing={2}>
                                                    <Stack direction="row" spacing={2} alignItems="center">
                                                        <Avatar sx={{ bgcolor: isSelected ? "primary.main" : "default", width: 40, height: 40 }}>
                                                            {isSelected ? <Check size={20} />
                                                                : <CircleIcon size={20} />}
                                                        </Avatar>
                                                        <Typography
                                                            variant="h5"
                                                            textOverflow="ellipsis"
                                                            overflow="hidden"
                                                            whiteSpace="nowrap"
                                                            maxWidth="90%"
                                                        >
                                                            {monitor.displayName}
                                                        </Typography>
                                                    </Stack>
                                                    <Stack direction="row" spacing={1} alignItems="center">
                                                        {(monitor.tags.slice(0, 2)
                                                            ?? []).map((tag) => (
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
                                            </Stack>
                                        }
                                    />
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
            <EvaluatorDetailsDrawer
                evaluator={drawerEvaluator}
                open={Boolean(drawerEvaluator)}
                onClose={handleCloseDrawer}
                isSelected={drawerEvaluatorAlreadySelected}
                initialConfig={drawerInitialConfig}
                llmPool={llmPool}
                llmProviders={llmProvidersData?.list}
                assignedLLMEntryId={assignedLLMEntryId}
                onAssignLLMEntry={handleAssignLLMEntry}
                onAdd={handleConfirmEvaluator}
                onRemove={handleRemoveEvaluator}
            />
        </Form.Stack>
    );
}

export default SelectPresetMonitors;
