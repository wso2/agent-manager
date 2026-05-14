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

import React from "react";
import {
    Box,
    Button,
    FormControlLabel,
    IconButton,
    MenuItem,
    Select,
    Stack,
    Switch,
    Typography,
} from "@wso2/oxygen-ui";
import { Plus, X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "@agent-management-platform/views";

export type RuntimeConfigTypeOption = "string" | "number" | "boolean";

const createRowId = (): string => {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
        return crypto.randomUUID();
    }
    return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
};

export const createRuntimeConfigRow = (
    overrides: Partial<RuntimeConfigRow> = {},
): RuntimeConfigRow => ({
    id: createRowId(),
    key: "",
    type: "string",
    isSecret: false,
    isMandatory: false,
    defaultValue: "",
    ...overrides,
});

export interface RuntimeConfigRow {
    id: string;
    key: string;
    type: RuntimeConfigTypeOption;
    isSecret: boolean;
    isMandatory?: boolean;
    defaultValue?: string;
}

export interface RuntimeConfigEditorProps {
    rows: RuntimeConfigRow[];
    onChange: (rows: RuntimeConfigRow[]) => void;
    /** When true: key is shown as a read-only label, type selector and
     * add/remove buttons are hidden */
    readonlyKey?: boolean;
}

export const RuntimeConfigEditor: React.FC<RuntimeConfigEditorProps> = ({
  rows,
  onChange,
  readonlyKey,
}) => {
    const normalizedKeys = rows.map((row) => row.key.trim());
    const hasEmptyKeys = !readonlyKey && normalizedKeys.some((key) => !key);
    const nonEmptyKeys = normalizedKeys.filter(Boolean);
    const hasDuplicateKeys = !readonlyKey && nonEmptyKeys.length !== new Set(nonEmptyKeys).size;
    const isInvalid = hasEmptyKeys || hasDuplicateKeys;
    const keyCounts = normalizedKeys.reduce<Map<string, number>>((acc, key) => {
        if (!key) {
            return acc;
        }
        acc.set(key, (acc.get(key) ?? 0) + 1);
        return acc;
    }, new Map());

    const updateRow = <K extends keyof RuntimeConfigRow>(
      index: number,
      field: K,
      value: RuntimeConfigRow[K],
    ) => {
        const next = [...rows];
        next[index] = { ...next[index], [field]: value };
        onChange(next);
    };

    const addRow = () => {
        if (isInvalid) {
            return;
        }
        onChange([...rows, createRuntimeConfigRow()]);
    };

    const removeRow = (index: number) => onChange(rows.filter((_, i) => i !== index));

    return (
        <Stack spacing={1}>
            {rows.map((row, i) => (
                <Stack key={row.id} direction="row" spacing={1} alignItems="center">
                    <Box sx={{ width: 160 }}>
                        {readonlyKey ? (
                            <Typography variant="body2" fontWeight={600}>{row.key}</Typography>
                        ) : (
                            <>
                                <TextInput
                                    placeholder="Key"
                                    value={row.key}
                                    onChange={(e) => updateRow(i, "key", e.target.value)}
                                    fullWidth
                                    size="small"
                                />
                                {!row.key.trim() ? (
                                    <Typography variant="caption" color="error.main">
                                        Key is required.
                                    </Typography>
                                ) : (keyCounts.get(row.key.trim()) ?? 0) > 1 ? (
                                    <Typography variant="caption" color="error.main">
                                        Key must be unique.
                                    </Typography>
                                ) : null}
                            </>
                        )}
                    </Box>
                    {!readonlyKey && (
                        <Select
                            size="small"
                            value={row.type}
                            onChange={(e) => updateRow(i, "type", e.target.value as RuntimeConfigTypeOption)}
                            sx={{ maxWidth: 110, width: 110 }}
                        >
                            <MenuItem value="string">string</MenuItem>
                            <MenuItem value="number">number</MenuItem>
                            <MenuItem value="boolean">boolean</MenuItem>
                        </Select>
                    )}
                    <Box sx={{ width: 130 }}>
                        <TextInput
                            placeholder="Default value"
                            value={row.defaultValue ?? ""}
                            onChange={(e) => updateRow(i, "defaultValue", e.target.value)}
                            fullWidth
                            size="small"
                        />
                    </Box>
                    <FormControlLabel
                        control={
                            <Switch
                                size="small"
                                checked={row.isMandatory ?? false}
                                onChange={(_, checked) => updateRow(i, "isMandatory", checked)}
                            />
                        }
                        label="Mandatory"
                        sx={{ mr: 0, minWidth: 105 }}
                    />
                    <FormControlLabel
                        control={
                            <Switch
                                size="small"
                                checked={row.isSecret}
                                onChange={(_, checked) => updateRow(i, "isSecret", checked)}
                            />
                        }
                        label="Secret"
                        sx={{ mr: 0, minWidth: 80 }}
                    />
                    {!readonlyKey && (
                        <IconButton
                            size="small"
                            onClick={() => removeRow(i)}
                            disabled={rows.length === 1}
                            aria-label="Remove row"
                        >
                            <CloseIcon size={16} />
                        </IconButton>
                    )}
                </Stack>
            ))}
            {!readonlyKey && (
                <Box>
                    <Button size="small" variant="outlined" startIcon={<Plus />} onClick={addRow} disabled={isInvalid}>
                        Add Runtime Key
                    </Button>
                </Box>
            )}
        </Stack>
    );
};

export default RuntimeConfigEditor;
