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
import { FilterBar, FilterChip, FilterToggle } from "@agent-management-platform/views";
import { Button } from "@wso2/oxygen-ui";
import { ListFilter } from "@wso2/oxygen-ui-icons-react";
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
import { TRACE_FILTERS_DRAWER_ID, TraceFiltersDrawer } from "./TraceFiltersDrawer";

export interface TraceFilterBarProps {
  filters: TraceFilters;
  onChange: (filters: TraceFilters) => void;
  /** Whether the Filters drawer is open. */
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Opens one trace by ID; the field shows only when this is set. */
  onTraceSearch?: (traceId: string) => void;
  /** The list below the bar. */
  children?: React.ReactNode;
}

const STATUS_OPTIONS = [
  { value: "error", label: "Errors" },
  { value: "ok", label: "Success" },
];

/** Status toggle and chips on the left, trace ID search and Filters fixed on the right. */
export const TraceFilterBar: React.FC<TraceFilterBarProps> = ({
  filters,
  onChange,
  open,
  onOpenChange,
  onTraceSearch,
  children,
}) => {
  const [draft, setDraft] = useState<TraceFilterDraft>(() => draftFrom(filters));
  const [filtersButton, setFiltersButton] = useState<HTMLButtonElement | null>(null);
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
    <>
      <FilterBar
        actions={
          <>
            {onTraceSearch && <TraceIdSearch onSearch={onTraceSearch} />}
            <Button
              ref={setFiltersButton}
              variant="outlined"
              color="inherit"
              startIcon={<ListFilter size={16} />}
              aria-expanded={open}
              aria-controls={TRACE_FILTERS_DRAWER_ID}
              onClick={() => (open ? onOpenChange(false) : openDrawer())}
              sx={{ textTransform: "none", borderColor: "divider" }}
            >
              Advanced Filters
            </Button>
          </>
        }
      >
        <FilterToggle
          label="Status"
          value={filters.status}
          options={STATUS_OPTIONS}
          onChange={(status) =>
            onChange({
              ...filters,
              status: status === "error" || status === "ok" ? status : undefined,
            })}
        />
        {chips.map((chip) => (
          <FilterChip
            key={chip.key}
            name={chip.name}
            value={chip.value}
            label={chip.label}
            onOpen={openDrawer}
            onRemove={() => remove(chip)}
          />
        ))}
        {chips.length + (filters.status ? 1 : 0) >= 2 && (
          <Button size="small" variant="text" onClick={clearAll}>
            Clear all
          </Button>
        )}
      </FilterBar>
      {children}
      {open && filtersButton && (
        <TraceFiltersDrawer
          anchorEl={filtersButton}
          draft={draft}
          onDraftChange={setDraft}
          onApply={apply}
          onClose={() => onOpenChange(false)}
        />
      )}
    </>
  );
};
