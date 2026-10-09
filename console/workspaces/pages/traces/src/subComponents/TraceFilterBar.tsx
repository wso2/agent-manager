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
  Box,
  Button,
  ButtonGroup,
  Divider,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
} from "@wso2/oxygen-ui";
import { ListFilter, X as RemoveIcon } from "@wso2/oxygen-ui-icons-react";
import {
  EMPTY_DRAFT,
  type TraceFilterChip,
  type TraceFilterDraft,
  appliedFilters,
  draftFrom,
  draftWithout,
  traceFilterChips,
} from "../traceFilters";
import { TraceIdSearch } from "./TraceIdSearch";
import {
  TRACE_FILTERS_DRAWER_HEIGHT,
  TRACE_FILTERS_DRAWER_ID,
  TraceFiltersDrawer,
} from "./TraceFiltersDrawer";

export interface TraceFilterBarProps {
  filters: TraceFilters;
  onChange: (filters: TraceFilters) => void;
  /** Whether the Filters drawer is open. */
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Opens one trace by ID; the field shows only when this is set. */
  onTraceSearch?: (traceId: string) => void;
  /** Evaluator names for the Evaluator select; undefined until loaded. */
  evaluators?: string[];
  evaluatorsLoading?: boolean;
  /** Called when the Evaluator select opens, so its options load only once someone looks. */
  onEvaluatorsOpen?: () => void;
  /** The list the drawer sits over. */
  children?: React.ReactNode;
}

interface StatusToggleProps {
  value?: TraceFilters["status"];
  onChange: (value?: TraceFilters["status"]) => void;
}

/** All, Errors or OK; applies straight away. */
const StatusToggle: React.FC<StatusToggleProps> = ({ value, onChange }) => (
  <ToggleButtonGroup
    exclusive
    size="small"
    value={value ?? "all"}
    aria-label="Status"
    onChange={(_, next: string | null) => {
      if (next !== null) onChange(next === "error" || next === "ok" ? next : undefined);
    }}
    sx={{ "& .MuiToggleButton-root": { px: 1.75, textTransform: "none" } }}
  >
    <ToggleButton value="all">All</ToggleButton>
    <ToggleButton value="error">Errors</ToggleButton>
    <ToggleButton value="ok">OK</ToggleButton>
  </ToggleButtonGroup>
);

interface FilterChipProps {
  chip: TraceFilterChip;
  onOpen: () => void;
  onRemove: () => void;
}

/** The filter's name and value, which open the drawer, and an × that removes it. */
const FilterChip: React.FC<FilterChipProps> = ({ chip, onOpen, onRemove }) => (
  <ButtonGroup
    size="small"
    variant="outlined"
    color="inherit"
    sx={{ maxWidth: 320, "& .MuiButton-root": { borderColor: "divider" } }}
  >
    <Button title={chip.label} onClick={onOpen} sx={{ textTransform: "none", minWidth: 0 }}>
      <Box component="span" sx={{ color: "text.secondary", whiteSpace: "nowrap" }}>
        {chip.name}
      </Box>{" "}
      <Box
        component="span"
        sx={{
          ml: 0.75,
          fontWeight: 600,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
      >
        {chip.value}
      </Box>
    </Button>
    <Button aria-label={`Remove ${chip.label}`} onClick={onRemove} sx={{ minWidth: 0, px: 0.75 }}>
      <RemoveIcon size={14} />
    </Button>
  </ButtonGroup>
);

/** One row: the status toggle, the Filters drawer button, the chips and the trace ID search. */
export const TraceFilterBar: React.FC<TraceFilterBarProps> = ({
  filters,
  onChange,
  open,
  onOpenChange,
  onTraceSearch,
  evaluators,
  evaluatorsLoading,
  onEvaluatorsOpen,
  children,
}) => {
  const [draft, setDraft] = useState<TraceFilterDraft>(() => draftFrom(filters));
  // Status has the toggle, so it gets no chip.
  const chips = traceFilterChips(filters).filter((c) => c.key !== "status");

  /** Opens the drawer on the URL's filters; an open drawer keeps its draft. */
  const openDrawer = () => {
    if (open) return;
    setDraft(draftFrom(filters));
    onOpenChange(true);
  };
  /** Clears the chip's filters, in the URL and in an open drawer's draft. */
  const remove = (chip: TraceFilterChip) => {
    const keys = [chip.key, ...(chip.alsoClears ?? [])];
    const next = { ...filters };
    for (const key of keys) delete next[key];
    onChange(next);
    setDraft((d) => draftWithout(d, keys));
  };
  /** Clears every filter, status included, and the draft. */
  const clearAll = () => {
    onChange({});
    setDraft(EMPTY_DRAFT);
  };
  /** Applies the draft in one change and closes the drawer. */
  const apply = () => {
    onChange(appliedFilters(draft, filters.status));
    onOpenChange(false);
  };

  return (
    // Keeps room for the drawer over a short list.
    <Box sx={{ position: "relative", minHeight: open ? TRACE_FILTERS_DRAWER_HEIGHT : undefined }}>
      <Stack
        direction="row"
        spacing={1}
        useFlexGap
        flexWrap="wrap"
        alignItems="center"
        sx={{ mb: 2 }}
      >
        <StatusToggle
          value={filters.status}
          onChange={(status) => onChange({ ...filters, status })}
        />
        <Button
          variant="outlined"
          color="inherit"
          startIcon={<ListFilter size={16} />}
          aria-expanded={open}
          aria-controls={TRACE_FILTERS_DRAWER_ID}
          onClick={() => (open ? onOpenChange(false) : openDrawer())}
          sx={{ textTransform: "none", borderColor: "divider" }}
        >
          Filters
        </Button>
        {chips.length > 0 && (
          <Divider orientation="vertical" sx={{ height: 20, alignSelf: "center", mx: 0.5 }} />
        )}
        {chips.map((chip) => (
          <FilterChip
            key={chip.key}
            chip={chip}
            onOpen={openDrawer}
            onRemove={() => remove(chip)}
          />
        ))}
        {chips.length + (filters.status ? 1 : 0) >= 2 && (
          <Button size="small" variant="text" onClick={clearAll}>
            Clear all
          </Button>
        )}
        {onTraceSearch && <TraceIdSearch onSearch={onTraceSearch} />}
      </Stack>
      {children}
      {open && (
        <TraceFiltersDrawer
          draft={draft}
          onDraftChange={setDraft}
          onApply={apply}
          onClose={() => onOpenChange(false)}
          evaluators={evaluators}
          evaluatorsLoading={evaluatorsLoading}
          onEvaluatorsOpen={onEvaluatorsOpen}
        />
      )}
    </Box>
  );
};
