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

import type { EvaluatorConfigParam, EvaluatorResponse } from "@agent-management-platform/types";
import { DrawerWrapper, DrawerHeader, DrawerContent } from "@agent-management-platform/views";
import { Avatar, Box, Button, Chip, Form, IconButton, MenuItem, Slider, Stack, Switch, TextField, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { useCallback, useEffect, useMemo, useState } from "react";


interface EvaluatorDetailsDrawerProps {
    evaluator: EvaluatorResponse | null;
    open: boolean;
    onClose: () => void;
    isSelected: boolean;
    onAdd: (config: Record<string, unknown>) => void;
    onRemove: () => void;
    initialConfig?: Record<string, unknown>;
}

function keyToDisplay(key: string): string {
    return key
        .replace(/([A-Z])/g, " $1") // Add space before capital letters
        .replace(/[_-]+/g, " ") // Replace underscores and hyphens with space
        .replace(/\s+/g, " ") // Replace multiple spaces with a single space
        .trim() // Trim leading and trailing spaces
        .replace(/^./, (str) => str.toUpperCase()); // Capitalize the first letter
}

interface ConfigParamFieldProps {
    param: EvaluatorConfigParam;
    value: unknown;
    onChange: (value: unknown) => void;
}

function ConfigParamField({ param, value, onChange }: ConfigParamFieldProps) {
    const { description, key, required, type, enumValues, max, min } = param;
    const helperText = description || "No description provided.";
    const label = keyToDisplay(key);
    const labelWithRequired = required ? `* ${label}` : label;

    if (type === "enum" || (enumValues?.length ?? 0) > 0) {
        const selectValue = typeof value === "string" ? value : "";
        return (
            <Form.ElementWrapper label={labelWithRequired} name={key}>
                <TextField
                    select
                    value={selectValue}
                    required={required}
                    helperText={helperText}
                    onChange={(event) => onChange(event.target.value)}
                >
                    {!required && (
                        <MenuItem value="">
                            Select a value
                        </MenuItem>
                    )}
                    {(enumValues ?? []).map((option) => (
                        <MenuItem key={option} value={option}>
                            {option}
                        </MenuItem>
                    ))}
                </TextField>
            </Form.ElementWrapper>
        );
    }

    if (type === "boolean") {
        const checked = typeof value === "boolean" ? value : false;
        return (
            <Form.ElementWrapper label={labelWithRequired} name={key}>
                <Stack direction="row" alignItems="center" spacing={1}>
                    <Switch
                        checked={checked}
                        onChange={(_, nextChecked) => onChange(nextChecked)}
                    />
                    <Typography variant="caption" color="text.secondary">
                        {helperText}
                    </Typography>
                </Stack>
            </Form.ElementWrapper>
        );
    }

    if (type === "integer") {
        const numericValue = typeof value === "number" ? value : undefined;
        if (min !== undefined && max !== undefined) {
            const sliderValue = numericValue ?? min;
            return (
                <Form.ElementWrapper label={labelWithRequired} name={key}>
                    <Stack spacing={1}>
                        <Slider
                            value={sliderValue}
                            min={min}
                            max={max}
                            step={1}
                            valueLabelDisplay="auto"
                            onChange={(_, sliderValue_local) => {
                                const nextValue = Array.isArray(sliderValue_local)
                                    ? sliderValue_local[0]
                                    : sliderValue_local;
                                onChange(nextValue);
                            }}
                        />
                        <Typography variant="caption" color="text.secondary">
                            {helperText}
                        </Typography>
                    </Stack>
                </Form.ElementWrapper>
            );
        }
        return (
            <Form.ElementWrapper label={labelWithRequired} name={key}>
                <TextField
                    type="number"
                    value={numericValue ?? ""}
                    inputProps={{ min, max, step: 1 }}
                    required={required}
                    helperText={helperText}
                    onChange={(event) => {
                        const nextValue = event.target.value === ""
                            ? undefined
                            : Number(event.target.value);
                        onChange(nextValue);
                    }}
                />
            </Form.ElementWrapper>
        );
    }

    if (type === "float" || type === "number") {
        const numericValue = typeof value === "number" ? value : undefined;
        if (min !== undefined && max !== undefined) {
            const sliderStep = Math.max((max - min) / 100, 0.01);
            const sliderValue = numericValue ?? min ?? max ?? 0;
            return (
                <Form.ElementWrapper label={labelWithRequired} name={key}>
                    <Stack spacing={1}>
                        <Slider
                            value={sliderValue}
                            min={min}
                            max={max}
                            step={sliderStep}
                            valueLabelDisplay="auto"
                            onChange={(_, sliderValue_local) => {
                                const nextValue = Array.isArray(sliderValue_local)
                                    ? sliderValue_local[0]
                                    : sliderValue_local;
                                onChange(typeof nextValue === "number"
                                    ? nextValue
                                    : Number(nextValue));
                            }}
                        />
                        <Typography variant="caption" color="text.secondary">
                            {helperText}
                        </Typography>
                    </Stack>
                </Form.ElementWrapper>
            );
        }
        return (
            <Form.ElementWrapper label={labelWithRequired} name={key}>
                <TextField
                    type="number"
                    value={numericValue ?? ""}
                    inputProps={{ min, max, step: 0.01 }}
                    required={required}
                    helperText={helperText}
                    onChange={(event) => {
                        const nextValue = event.target.value === ""
                            ? undefined
                            : Number(event.target.value);
                        onChange(nextValue);
                    }}
                />
            </Form.ElementWrapper>
        );
    }

    if (type === "array") {
        const entries = Array.isArray(value) && (value as string[]).length
            ? (value as string[])
            : [""];
        const canRemove = entries.length > 1;
        return (
            <Form.ElementWrapper label={labelWithRequired} name={key}>
                <Stack spacing={2} pt={2}>
                    {entries.map((entryValue, index) => (
                        <Stack key={`${key}-${index}`} direction="row" spacing={1} alignItems="center">
                            <TextField
                                fullWidth
                                value={entryValue}
                                placeholder={`Value ${index + 1}`}
                                onChange={(event) => {
                                    const next = [...entries];
                                    next[index] = event.target.value;
                                    onChange(next);
                                }}
                            />
                            <Tooltip title={canRemove ? "Remove value" : "At least one value is required"}>
                                <span>
                                    <IconButton
                                        size="small"
                                        color="error"
                                        disabled={!canRemove}
                                        onClick={() => {
                                            if (!canRemove) {
                                                return;
                                            }
                                            const next = entries.filter((_, itemIndex) =>
                                                itemIndex !== index);
                                            onChange(next.length ? next : [""]);
                                        }}
                                    >
                                        <Trash size={16} />
                                    </IconButton>
                                </span>
                            </Tooltip>
                        </Stack>
                    ))}
                    <Box>
                        <Button
                            variant="outlined"
                            size="small"
                            startIcon={<Plus size={16} />}
                            onClick={() => onChange([...entries, ""])}
                        >
                            Add another value
                        </Button>
                    </Box>
                    <Typography variant="caption" color="text.secondary">
                        {helperText}
                    </Typography>
                </Stack>
            </Form.ElementWrapper>
        );
    }

    if (type === "string") {
        const textValue = typeof value === "string"
            ? value
            : value !== undefined && value !== null
                ? String(value)
                : "";
        return (
            <Form.ElementWrapper label={label} name={key}>
                <TextField
                    value={textValue}
                    required={required}
                    helperText={helperText}
                    onChange={(event) => onChange(event.target.value)}
                />
            </Form.ElementWrapper>
        );
    }

    return (
        <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 1.5 }}>
            <Typography variant="body2" fontWeight={600}>
                {label} ({type})
                {required && (
                    <Typography component="span" color="error" sx={{ ml: 0.5 }}>
                        *
                    </Typography>
                )}
            </Typography>
            <Typography variant="caption" color="text.secondary">
                {helperText}
            </Typography>
        </Box>
    );
}

function getInitialValue(param: EvaluatorConfigParam, existing?: unknown): unknown {
    if (existing !== undefined) {
        return existing;
    }
    if (param.default !== undefined) {
        return param.default;
    }
    switch (param.type) {
        case "boolean":
            return false;
        case "integer":
        case "float":
        case "number":
            if (param.min !== undefined) {
                return param.min;
            }
            if (param.max !== undefined) {
                return param.max;
            }
            return 0;
        case "array":
            return [];
        default:
            return "";
    }
}

export function EvaluatorDetailsDrawer({
    evaluator,
    open,
    onClose,
    isSelected,
    onAdd,
    onRemove,
    initialConfig,
}: EvaluatorDetailsDrawerProps) {
    const [configValues, setConfigValues] = useState<Record<string, unknown>>({});

    useEffect(() => {
        if (!evaluator) {
            setConfigValues({});
            return;
        }
        const nextConfig: Record<string, unknown> = {};
        (evaluator.configSchema ?? []).forEach((param) => {
            nextConfig[param.key] = getInitialValue(param, initialConfig?.[param.key]);
        });
        setConfigValues(nextConfig);
    }, [evaluator?.id, open, JSON.stringify(initialConfig ?? {})]);

    const handleConfigChange = useCallback((key: string, value: unknown) => {
        setConfigValues((prev) => ({ ...prev, [key]: value }));
    }, []);

    const handleConfirmSelection = useCallback(() => {
        onAdd({ ...configValues });
    }, [configValues, onAdd]);

    const configSchema = useMemo(() => evaluator?.configSchema ?? [], [evaluator]);

    return (
        <DrawerWrapper open={open} onClose={onClose} maxWidth={520}>
            <DrawerHeader
                title={evaluator?.displayName ?? "Evaluator details"}
                onClose={onClose}
                icon={
                    <Avatar>
                        {evaluator?.displayName ? evaluator.displayName.charAt(0).toUpperCase() : "E"}
                    </Avatar>
                }
            />
            <DrawerContent>
                {evaluator && (
                    <Stack spacing={3}>
                        <Stack spacing={1}>
                            <Typography variant="body2" color="text.secondary">
                                Version {evaluator.version ?? "n/a"}
                            </Typography>
                            <Typography variant="body1">
                                {evaluator.description}
                            </Typography>
                        </Stack>

                        <Stack direction="row" spacing={1} flexWrap="wrap">
                            {(evaluator.tags ?? []).map((tag) => (
                                <Chip key={tag} size="small" label={tag} variant="outlined" />
                            ))}
                            {(!evaluator.tags || evaluator.tags.length === 0) && (
                                <Typography variant="caption" color="text.secondary">
                                    No tags provided for this evaluator.
                                </Typography>
                            )}
                        </Stack>

                        <Stack spacing={1}>
                            <Typography variant="subtitle2">Configuration Parameters</Typography>
                            {configSchema.length ? (
                                <Form.Stack>
                                    {configSchema.map((param) => (
                                        <Form.Section key={param.key}>
                                            <ConfigParamField
                                                param={param}
                                                value={configValues[param.key]}
                                                onChange={(nextValue) =>
                                                    handleConfigChange(param.key, nextValue)}
                                            />
                                        </Form.Section>
                                    ))}
                                </Form.Stack>
                            ) : (
                                <Typography variant="caption" color="text.secondary">
                                    This evaluator does not require additional configuration.
                                </Typography>
                            )}
                        </Stack>

                        <Stack direction="row" justifyContent="flex-end" spacing={1}>
                            <Button variant="text" onClick={onClose}>
                                Close
                            </Button>
                            {isSelected && (
                                <Button variant="outlined" color="error" onClick={onRemove}>
                                    Remove
                                </Button>
                            )}
                            <Button variant="contained" color="primary" onClick={handleConfirmSelection}>
                                {isSelected ? "Save Changes" : "Add Evaluator"}
                            </Button>
                        </Stack>
                    </Stack>
                )}
            </DrawerContent>
        </DrawerWrapper>
    );
}

export default EvaluatorDetailsDrawer;
