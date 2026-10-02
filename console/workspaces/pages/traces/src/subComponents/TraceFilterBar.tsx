/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useState } from "react";
import type { TraceFilters } from "@agent-management-platform/types";
import {
  Button,
  Chip,
  FormControl,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { X as RemoveIcon } from "@wso2/oxygen-ui-icons-react";
import {
  LATENCY_PRESETS_MS,
  STEP_PRESETS,
  TOKEN_PRESETS,
  type TraceFilterKey,
  formatLatency,
  formatTokens,
  traceFilterChips,
} from "../traceFilters";

const ANY = "";

export interface TraceFilterBarProps {
  filters: TraceFilters;
  onChange: (filters: TraceFilters) => void;
}

interface ThresholdSelectProps {
  label: string;
  value?: number;
  presets: number[];
  format: (n: number) => string;
  onChange: (value?: number) => void;
}

// Preset select; a non-preset value from a pasted URL is listed so it still shows.
function ThresholdSelect({ label, value, presets, format, onChange }: ThresholdSelectProps) {
  const options = value === undefined || presets.includes(value)
    ? presets
    : [...presets, value].sort((a, b) => a - b);
  return (
    <FormControl size="small" sx={{ minWidth: 140 }}>
      <Select
        value={value === undefined ? ANY : String(value)}
        displayEmpty
        inputProps={{ "aria-label": label }}
        onChange={(e) => {
          const raw = e.target.value as string;
          onChange(raw === ANY ? undefined : Number(raw));
        }}
        renderValue={(v) => `${label} ${v === ANY ? "Any" : format(Number(v))}`}
      >
        <MenuItem value={ANY}>Any</MenuItem>
        {options.map((n) => (
          <MenuItem key={n} value={String(n)}>
            {format(n)}
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );
}

interface CommitTextFieldProps {
  label: string;
  value?: string;
  onCommit: (value?: string) => void;
}

// Text filter that commits on Enter or blur; remount it (via key) to reset the draft.
function CommitTextField({ label, value, onCommit }: CommitTextFieldProps) {
  const [draft, setDraft] = useState(value ?? "");
  const commit = () => {
    const trimmed = draft.trim();
    if (trimmed !== (value ?? "")) onCommit(trimmed || undefined);
  };
  return (
    <TextField
      size="small"
      placeholder={label}
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") commit();
      }}
      slotProps={{ htmlInput: { "aria-label": label } }}
      sx={{ minWidth: 180 }}
    />
  );
}

export const TraceFilterBar: React.FC<TraceFilterBarProps> = ({ filters, onChange }) => {
  const set = <K extends TraceFilterKey>(key: K, value: TraceFilters[K]) =>
    onChange({ ...filters, [key]: value });
  const remove = (key: TraceFilterKey) => {
    const next = { ...filters };
    delete next[key];
    onChange(next);
  };
  const chips = traceFilterChips(filters);

  return (
    <Stack spacing={1.5} sx={{ mb: 2 }}>
      <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" alignItems="center">
        <FormControl size="small" sx={{ minWidth: 140 }}>
          <Select
            value={filters.status ?? ANY}
            displayEmpty
            inputProps={{ "aria-label": "Status" }}
            onChange={(e) => {
              const raw = e.target.value as string;
              set("status", raw === "error" || raw === "ok" ? raw : undefined);
            }}
            renderValue={(v) =>
              `Status: ${v === "error" ? "Error" : v === "ok" ? "OK" : "Any"}`
            }
          >
            <MenuItem value={ANY}>Any</MenuItem>
            <MenuItem value="error">Error</MenuItem>
            <MenuItem value="ok">OK</MenuItem>
          </Select>
        </FormControl>
        <ThresholdSelect
          label="Latency ≥"
          value={filters.minDurationMs}
          presets={LATENCY_PRESETS_MS}
          format={formatLatency}
          onChange={(v) => set("minDurationMs", v)}
        />
        <ThresholdSelect
          label="Tokens ≥"
          value={filters.minTokens}
          presets={TOKEN_PRESETS}
          format={formatTokens}
          onChange={(v) => set("minTokens", v)}
        />
        <ThresholdSelect
          label="Steps ≥"
          value={filters.minSpanCount}
          presets={STEP_PRESETS}
          format={String}
          onChange={(v) => set("minSpanCount", v)}
        />
        <CommitTextField
          key={`model:${filters.model ?? ""}`}
          label="Model"
          value={filters.model}
          onCommit={(v) => set("model", v)}
        />
        <CommitTextField
          key={`conversationId:${filters.conversationId ?? ""}`}
          label="Conversation ID"
          value={filters.conversationId}
          onCommit={(v) => set("conversationId", v)}
        />
      </Stack>
      {chips.length > 0 && (
        <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" alignItems="center">
          <Typography variant="body2" color="text.secondary">
            Active:
          </Typography>
          {chips.map((chip) => (
            <Chip
              key={chip.key}
              label={chip.label}
              title={chip.label}
              size="small"
              variant="outlined"
              onDelete={() => remove(chip.key)}
              deleteIcon={<RemoveIcon size={14} aria-label={`Remove ${chip.label}`} />}
              sx={{ maxWidth: 320 }}
            />
          ))}
          {chips.length >= 2 && (
            <Button size="small" variant="text" onClick={() => onChange({})}>
              Clear all
            </Button>
          )}
        </Stack>
      )}
    </Stack>
  );
};
