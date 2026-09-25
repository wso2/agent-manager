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

import React, { useRef, useState } from "react";
import {
    Alert,
    Box,
    Button,
    Divider,
    FormControlLabel,
    IconButton,
    Stack,
    Switch,
    Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { INPUT_LIMITS } from "@agent-management-platform/types";
import {
    EnvFileUploadButton,
    MAX_FILE_SIZE,
    TextInput,
    parseEnvFileContent,
    type ParsedEnvEntry,
} from "@agent-management-platform/views";

const KEY_REGEX = /^[A-Za-z_][A-Za-z0-9_]*$/;
const KEY_MAX_LENGTH = 64;

// `maxLength` on the inputs only constrains typing. A .env upload or paste
// writes straight to row state, so oversized entries are caught here. They are
// rejected rather than truncated: a shortened value (often a secret) looks
// complete but fails at runtime, and a shortened key can collide with another.
const isEntryTooLong = (key: string, value: string) =>
    key.length > KEY_MAX_LENGTH || value.length > INPUT_LIMITS.VALUE;

const describeRejected = (keys: string[]) => {
    const shown = keys.map((key) => (key.length > 40 ? `${key.slice(0, 40)}…` : key));
    return `Not imported: ${shown.join(", ")}. Keys are limited to ${KEY_MAX_LENGTH} `
        + `characters and values to ${INPUT_LIMITS.VALUE.toLocaleString()}.`;
};

const getKeyError = (key: string, keyCounts: Map<string, number>): string | null => {
    const trimmed = key.trim();
    if (!trimmed) return "Key is required.";
    if (trimmed.length > KEY_MAX_LENGTH) return `Key must be at most ${KEY_MAX_LENGTH} characters.`;
    if (!KEY_REGEX.test(trimmed)) return "Key must start with a letter or underscore, and contain only letters, numbers, or underscores.";
    if ((keyCounts.get(trimmed) ?? 0) > 1) return "Key must be unique.";
    return null;
};

const stripQuotes = (value: string): string => {
    if (
        value.length >= 2 &&
        ((value.startsWith('"') && value.endsWith('"')) ||
            (value.startsWith("'") && value.endsWith("'")))
    ) {
        return value.slice(1, -1);
    }
    return value;
};

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
    isSecret: false,
    isMandatory: false,
    defaultValue: "",
    description: "",
    ...overrides,
});

export interface RuntimeConfigRow {
    id: string;
    key: string;
    isSecret: boolean;
    isMandatory?: boolean;
    defaultValue?: string;
    description?: string;
}

export interface RuntimeConfigEditorProps {
    rows: RuntimeConfigRow[];
    onChange: (rows: RuntimeConfigRow[]) => void;
    /** When true: key is shown as a read-only label, type selector and
     * add/remove buttons are hidden */
    readonlyKey?: boolean;
}

interface ConfigRowProps {
    row: RuntimeConfigRow;
    keyError: string | null;
    readonlyKey?: boolean;
    canRemove: boolean;
    onUpdate: <K extends keyof RuntimeConfigRow>(field: K, value: RuntimeConfigRow[K]) => void;
    onRemove: () => void;
    /**
     * Called instead of onUpdateMany when the pasted clipboard text contains
     * multiple "KEY=VALUE" lines (e.g. the full contents of a .env file), so
     * the caller can split it into multiple rows.
     */
    onBulkPaste: (entries: ParsedEnvEntry[]) => void;
    /** Called when a single "KEY=VALUE" is pasted, so the caller can reject it if too long. */
    onPasteEntry: (key: string, value: string) => void;
}

const ConfigRow: React.FC<ConfigRowProps> = ({
    row,
    keyError,
    readonlyKey,
    canRemove,
    onUpdate,
    onRemove,
    onBulkPaste,
    onPasteEntry,
}) => (
    <Stack key={row.id} spacing={0.5}>
        <Stack direction="row" spacing={1} alignItems="top" justifyContent="flex-start">
            <Box sx={{ width: 180 }}>
                {readonlyKey ? (
                    <Typography variant="body2" fontWeight={600}>{row.key}</Typography>
                ) : (
                    <>
                        <TextInput
                            maxLength={KEY_MAX_LENGTH}
                            placeholder="Key"
                            value={row.key}
                            onChange={(e) => onUpdate("key", e.target.value.replace(/\s/g, "_"))}
                            onPaste={(e) => {
                                const pasted = e.clipboardData.getData("text");

                                // A paste this large is almost certainly the wrong clipboard
                                // contents (e.g. an entire log file), not a real .env list —
                                // let the browser's default paste handle it instead of
                                // splitting it into countless rows.
                                if (pasted.length > MAX_FILE_SIZE) return;

                                // Pasting one or more "KEY=VALUE" lines (including the full
                                // contents of a .env file) parses through the same helper as
                                // the upload path, so a single entry with a leading/trailing
                                // comment line isn't corrupted by the raw-text fallback below.
                                const bulkEntries = parseEnvFileContent(pasted);
                                if (bulkEntries.length > 1) {
                                    e.preventDefault();
                                    onBulkPaste(bulkEntries);
                                    return;
                                }
                                if (bulkEntries.length === 1) {
                                    e.preventDefault();
                                    onPasteEntry(bulkEntries[0].key.replace(/\s/g, "_"), bulkEntries[0].value);
                                    return;
                                }

                                const equalsIdx = pasted.indexOf("=");
                                if (equalsIdx === -1) return;
                                const pastedKey = pasted.slice(0, equalsIdx).trim();
                                const pastedValue = stripQuotes(pasted.slice(equalsIdx + 1).trim());
                                if (!pastedKey) return;
                                e.preventDefault();
                                onPasteEntry(pastedKey.replace(/\s/g, "_"), pastedValue);
                            }}
                            fullWidth
                            size="small"
                            error={!!keyError}
                        />
                        {keyError && (
                            <Typography variant="caption" color="error.main">
                                {keyError}
                            </Typography>
                        )}
                    </>
                )}
            </Box>

            <Box sx={{ width: 180 }}>
                {/* readonlyKey means this row came from an already-published version: its
                 * defaultValue is whatever the backend returned, which for a secret item is
                 * a placeholder that only signals whether a default exists, never the real
                 * value. A fresh "Create new version" row (readonlyKey unset) still gets a
                 * normal, fully-editable field so authors can type a real secret default. */}
                <TextInput
                    maxLength={INPUT_LIMITS.VALUE}
                    placeholder={
                        readonlyKey && row.isSecret
                            ? (row.defaultValue ? "•••••••• (hidden)" : "Not set")
                            : "Default value"
                    }
                    value={readonlyKey && row.isSecret ? "" : (row.defaultValue ?? "")}
                    onChange={(e) => onUpdate("defaultValue", e.target.value)}
                    fullWidth
                    size="small"
                    disabled={readonlyKey && row.isSecret}
                    type={row.isSecret && !readonlyKey ? "password" : "text"}
                    showPasswordToggle={row.isSecret && !readonlyKey}
                />
            </Box>
            <Box display="flex" flexDirection="row" flexGrow={1} alignItems="start" pl={2} pt={0.5} gap={1}>
                <FormControlLabel
                    control={
                        <Switch
                            size="small"
                            checked={row.isMandatory ?? false}
                            onChange={(_, checked) => onUpdate("isMandatory", checked)}
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
                            onChange={(_, checked) => onUpdate("isSecret", checked)}
                        />
                    }
                    label="Secret"
                    sx={{ mr: 0, minWidth: 80 }}
                />
                {!readonlyKey && (
                    <IconButton
                        size="small"
                        onClick={onRemove}
                        disabled={!canRemove}
                        aria-label="Remove row"
                        color="error"
                    >
                        <Trash size={16} />
                    </IconButton>
                )}
            </Box>
        </Stack>
        <TextInput
            maxLength={INPUT_LIMITS.DESCRIPTION}
            label="Description"
            placeholder="Optional"
            value={row.description ?? ""}
            onChange={(e) => onUpdate("description", e.target.value)}
            fullWidth
            size="small"
        />
    </Stack>
);

export const RuntimeConfigEditor: React.FC<RuntimeConfigEditorProps> = ({
    rows,
    onChange,
    readonlyKey,
}) => {
    const normalizedKeys = rows.map((row) => row.key.trim());
    const keyCounts = normalizedKeys.reduce<Map<string, number>>((acc, key) => {
        if (!key) return acc;
        acc.set(key, (acc.get(key) ?? 0) + 1);
        return acc;
    }, new Map());
    const isInvalid = !readonlyKey && rows.some((row) => getKeyError(row.key, keyCounts) !== null);

    const updateRow = <K extends keyof RuntimeConfigRow>(
        index: number,
        field: K,
        value: RuntimeConfigRow[K],
    ) => {
        const next = [...rows];
        next[index] = { ...next[index], [field]: value };
        onChange(next);
    };

    const updateRowMany = (index: number, updates: Partial<RuntimeConfigRow>) => {
        const next = [...rows];
        next[index] = { ...next[index], ...updates };
        onChange(next);
    };

    const addRow = () => {
        if (isInvalid) {
            return;
        }
        onChange([...rows, createRuntimeConfigRow()]);
    };

    const removeRow = (index: number) => onChange(rows.filter((_, i) => i !== index));

    // FileReader resolves asynchronously, so by the time it fires, `rows` closed
    // over at render time may be behind the latest edits — read from this ref instead.
    const rowsRef = useRef(rows);
    rowsRef.current = rows;
    const [importError, setImportError] = useState<string | null>(null);

    const handlePasteEntry = (index: number, key: string, value: string) => {
        if (isEntryTooLong(key, value)) {
            setImportError(describeRejected([key]));
            return;
        }
        setImportError(null);
        updateRowMany(index, { key, defaultValue: value });
    };

    const handleEnvFileParsed = (entries: ParsedEnvEntry[]) => {
        const next = [...rowsRef.current];
        const indexByKey = new Map<string, number>();
        next.forEach((row, i) => {
            const trimmedKey = row.key.trim();
            if (trimmedKey) indexByKey.set(trimmedKey, i);
        });

        const rejected: string[] = [];
        for (const rawEntry of entries) {
            const key = rawEntry.key.replace(/\s/g, "_");
            const value = rawEntry.value;
            if (isEntryTooLong(key, value)) {
                rejected.push(key);
                continue;
            }
            const existingIndex = indexByKey.get(key);
            if (existingIndex !== undefined) {
                next[existingIndex] = { ...next[existingIndex], defaultValue: value };
            } else {
                indexByKey.set(key, next.length);
                next.push(createRuntimeConfigRow({ key, defaultValue: value }));
            }
        }
        setImportError(rejected.length > 0 ? describeRejected(rejected) : null);
        const withoutBlankRow = next.filter((row) => row.key.trim() !== "" || row.defaultValue?.trim());
        onChange(withoutBlankRow.length > 0 ? withoutBlankRow : next);
    };

    return (
        <Stack spacing={1.5} pt={1} divider={<Divider />}>
            {rows.map((row, i) => (
                <ConfigRow
                    key={row.id}
                    row={row}
                    keyError={readonlyKey ? null : getKeyError(row.key, keyCounts)}
                    readonlyKey={readonlyKey}
                    canRemove={rows.length > 1}
                    onUpdate={(field, value) => updateRow(i, field, value)}
                    onRemove={() => removeRow(i)}
                    onBulkPaste={handleEnvFileParsed}
                    onPasteEntry={(key, value) => handlePasteEntry(i, key, value)}
                />
            ))}
            {importError && (
                <Alert severity="warning" onClose={() => setImportError(null)}>
                    {importError}
                </Alert>
            )}
            {!readonlyKey && (
                <Box display="flex" flexDirection="row" gap={1.5} alignItems="center" flexWrap="wrap">
                    <Button size="small" variant="outlined" startIcon={<Plus />} onClick={addRow} disabled={isInvalid}>
                        Add Runtime Key
                    </Button>
                    <Box display="flex" flexDirection="column" alignItems="flex-start">
                        <EnvFileUploadButton onParsed={handleEnvFileParsed} label="Upload .env file" />
                    </Box>
                    <Typography variant="caption" color="text.secondary">
                        or paste .env text into a Key field above
                    </Typography>
                </Box>
            )}
        </Stack>
    );
};

export default RuntimeConfigEditor;
