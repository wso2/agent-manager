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

import React, { useId } from "react";
import {
  FormControl,
  InputAdornment,
  MenuItem,
  Select,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from "@wso2/oxygen-ui";
import {
  MAX_TEXT_FILTER_LENGTH,
  THRESHOLDS,
  type ThresholdKey,
  type TraceFilterDraft,
  crossesOtherBound,
  customError,
} from "../traceFilters";

const ANY = "any";
const CUSTOM = "custom";

export interface FilterSectionProps {
  title: string;
  children: React.ReactNode;
}

/** A titled group of drawer fields. */
export const FilterSection: React.FC<FilterSectionProps> = ({ title, children }) => (
  <Stack component="section" spacing={1.5}>
    <Typography variant="subtitle2" component="h3" sx={{ fontWeight: 600 }}>
      {title}
    </Typography>
    {children}
  </Stack>
);

export interface ThresholdFieldProps {
  thresholdKey: ThresholdKey;
  draft: TraceFilterDraft;
  onDraftChange: (draft: TraceFilterDraft) => void;
  onSubmit: () => void;
}

/** Pills for Any, the presets and Custom; Custom opens a number field under them. */
export const ThresholdField: React.FC<ThresholdFieldProps> = ({
  thresholdKey: key,
  draft,
  onDraftChange,
  onSubmit,
}) => {
  const labelId = useId();
  const spec = THRESHOLDS[key];
  const value = draft.filters[key];
  const customText = draft.custom[key];
  const error = customError(draft, key);
  // A non-preset value, from the URL or an applied custom value, gets its own pill.
  const options = value === undefined || spec.presets.includes(value)
    ? spec.presets
    : [...spec.presets, value].sort((a, b) => a - b);
  const selected = customText !== undefined ? CUSTOM : value === undefined ? ANY : String(value);

  /** Picks Any or a value and leaves Custom. */
  const pick = (next?: number) => {
    const custom = { ...draft.custom };
    delete custom[key];
    onDraftChange({ filters: { ...draft.filters, [key]: next }, custom });
  };
  /** Sets the custom field's text. */
  const setCustom = (text: string) =>
    onDraftChange({ ...draft, custom: { ...draft.custom, [key]: text } });

  return (
    <Stack spacing={0.75}>
      <Typography id={labelId} variant="body2" color="text.secondary">
        {spec.label}
      </Typography>
      <ToggleButtonGroup
        exclusive
        size="small"
        color="primary"
        value={selected}
        aria-labelledby={labelId}
        onChange={(_, next: string | null) => {
          if (next === null) return;
          if (next === CUSTOM) setCustom(value === undefined ? "" : spec.inputValue(value));
          else pick(next === ANY ? undefined : Number(next));
        }}
        sx={{
          flexWrap: "wrap",
          gap: 0.75,
          "& .MuiToggleButton-root": {
            px: 1.5,
            py: 0.25,
            ml: 0,
            border: 1,
            borderColor: "divider",
            borderRadius: 999,
            textTransform: "none",
          },
          "& .MuiToggleButton-root.Mui-selected": { borderColor: "primary.main" },
        }}
      >
        <ToggleButton value={ANY}>Any</ToggleButton>
        {options.map((n) => (
          <ToggleButton key={n} value={String(n)} disabled={crossesOtherBound(draft, key, n)}>
            {spec.format(n)}
          </ToggleButton>
        ))}
        <ToggleButton value={CUSTOM}>Custom</ToggleButton>
      </ToggleButtonGroup>
      {customText !== undefined && (
        <TextField
          size="small"
          autoFocus
          value={customText}
          error={!!error}
          helperText={error}
          onChange={(e) => setCustom(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") onSubmit();
          }}
          slotProps={{
            htmlInput: {
              "aria-label": `Custom ${spec.label.toLowerCase()}`,
              inputMode: spec.unit ? "decimal" : "numeric",
            },
            input: spec.unit
              ? { endAdornment: <InputAdornment position="end">{spec.unit}</InputAdornment> }
              : undefined,
          }}
          sx={{ maxWidth: 160 }}
        />
      )}
    </Stack>
  );
};

export interface FilterTextFieldProps {
  label: string;
  placeholder: string;
  value?: string;
  monospace?: boolean;
  onChange: (value: string) => void;
  onSubmit: () => void;
}

/** A labelled text field; Enter submits the drawer. */
export const FilterTextField: React.FC<FilterTextFieldProps> = ({
  label,
  placeholder,
  value,
  monospace = false,
  onChange,
  onSubmit,
}) => {
  const id = useId();
  return (
    <Stack spacing={0.75}>
      <Typography component="label" htmlFor={id} variant="body2" color="text.secondary">
        {label}
      </Typography>
      <TextField
        id={id}
        size="small"
        fullWidth
        placeholder={placeholder}
        value={value ?? ""}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") onSubmit();
        }}
        slotProps={{ htmlInput: { maxLength: MAX_TEXT_FILTER_LENGTH } }}
        sx={monospace ? { "& input": { fontFamily: "monospace" } } : undefined}
      />
    </Stack>
  );
};

export interface EvaluatorSelectProps {
  value?: string;
  // Evaluator names to offer; undefined until loaded.
  options?: string[];
  loading?: boolean;
  // Called on each open, so the options can load on first use.
  onOpen?: () => void;
  onChange: (value?: string) => void;
}

/** Evaluator select; a value from a pasted URL shows before the options load. */
export const EvaluatorSelect: React.FC<EvaluatorSelectProps> = ({
  value,
  options,
  loading = false,
  onOpen,
  onChange,
}) => {
  const names = value && !options?.includes(value) ? [value, ...(options ?? [])] : (options ?? []);
  return (
    <Stack spacing={0.75}>
      <Typography variant="body2" color="text.secondary">
        Evaluator
      </Typography>
      <FormControl size="small" fullWidth>
        <Select
          value={value ?? ""}
          displayEmpty
          inputProps={{ "aria-label": "Evaluator" }}
          onOpen={onOpen}
          onChange={(e) => onChange((e.target.value as string) || undefined)}
          renderValue={(v) => (v ? v : "Any evaluator")}
        >
          <MenuItem value="">Any evaluator</MenuItem>
          {names.map((name) => (
            <MenuItem key={name} value={name}>
              {name}
            </MenuItem>
          ))}
          {loading && <MenuItem disabled>Loading evaluators…</MenuItem>}
          {!loading && options?.length === 0 && <MenuItem disabled>No evaluators</MenuItem>}
        </Select>
      </FormControl>
    </Stack>
  );
};
