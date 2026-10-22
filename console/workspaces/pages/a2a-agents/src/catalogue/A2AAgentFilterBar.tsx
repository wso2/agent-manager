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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useState } from 'react';
import {
  Box,
  Button,
  MenuItem,
  SearchBar,
  Select,
  Stack,
  Typography,
} from '@wso2/oxygen-ui';
import { ListFilter } from '@wso2/oxygen-ui-icons-react';
import {
  FilterBar,
  FilterChip,
  FilterPanel,
  FilterToggle,
} from '@agent-management-platform/views';
import type {
  Environment,
  ProjectResponse,
} from '@agent-management-platform/types';
import {
  ADVANCED_FILTER_KEYS,
  type AdvancedFilterKey,
  type CatalogueFilters,
} from './catalogueQuery';

export const A2A_FILTER_PANEL_ID = 'a2a-agents-filter-panel';

const PANEL_WIDTH = 640;

export type AdvancedFilters = Record<AdvancedFilterKey, string>;

const EMPTY_ADVANCED: AdvancedFilters = {
  environment: '',
  project: '',
  status: '',
};

interface FilterOption {
  value: string;
  label: string;
}

const KIND_OPTIONS: FilterOption[] = [
  { value: 'platform', label: 'Platform' },
  { value: 'external', label: 'External' },
  { value: 'proxied', label: 'Proxied' },
];

const STATUS_OPTIONS: FilterOption[] = [
  { value: 'fetched', label: 'Fetched' },
  { value: 'pending', label: 'Pending' },
  { value: 'failed', label: 'Failed' },
];

const FILTER_LABELS: Record<AdvancedFilterKey, string> = {
  environment: 'Environment',
  project: 'Project',
  status: 'Card status',
};

const advancedFrom = (filters: CatalogueFilters): AdvancedFilters => ({
  environment: filters.environment,
  project: filters.project,
  status: filters.status,
});

/** The option's label, or the raw value when it is not an option (e.g. a stale URL value). */
const optionLabel = (options: FilterOption[], value: string): string =>
  options.find((option) => option.value === value)?.label ?? value;

const selectId = (key: AdvancedFilterKey) => `a2a-agents-filter-${key}`;

const FilterSelect: React.FC<{
  filterKey: AdvancedFilterKey;
  options: FilterOption[];
  value: string;
  onChange: (value: string) => void;
}> = ({ filterKey, options, value, onChange }) => {
  const labelId = `${selectId(filterKey)}-label`;
  return (
    <Box sx={{ flex: 1, minWidth: 180 }}>
      <Typography id={labelId} variant="body2" color="text.secondary">
        {FILTER_LABELS[filterKey]}
      </Typography>
      <Select
        labelId={labelId}
        size="small"
        fullWidth
        displayEmpty
        value={value}
        onChange={(e) => onChange(String(e.target.value))}
      >
        <MenuItem value="">Any</MenuItem>
        {options.map((option) => (
          <MenuItem key={option.value} value={option.value}>
            {option.label}
          </MenuItem>
        ))}
      </Select>
    </Box>
  );
};

export interface A2AAgentFilterBarProps {
  search: string;
  onSearchChange: (value: string) => void;
  /** The applied filters (from the URL). */
  filters: CatalogueFilters;
  /** Applies immediately. */
  onKindChange: (kind: string | undefined) => void;
  /** Writes the advanced filters in one go; the applied kind is kept. */
  onApply: (advanced: AdvancedFilters) => void;
  onRemove: (key: AdvancedFilterKey) => void;
  onClearAll: () => void;
  environments: Environment[];
  projects: ProjectResponse[];
}

/** Owns the panel's draft and open flag; the applied values belong to the catalogue. */
export const A2AAgentFilterBar: React.FC<A2AAgentFilterBarProps> = ({
  search,
  onSearchChange,
  filters,
  onKindChange,
  onApply,
  onRemove,
  onClearAll,
  environments,
  projects,
}) => {
  const [draft, setDraft] = useState<AdvancedFilters>(EMPTY_ADVANCED);
  const [open, setOpen] = useState(false);
  const [filtersButton, setFiltersButton] = useState<HTMLButtonElement | null>(
    null,
  );

  const options: Record<AdvancedFilterKey, FilterOption[]> = {
    environment: environments.map((environment) => ({
      value: environment.name,
      label: environment.displayName ?? environment.name,
    })),
    project: projects.map((project) => ({
      value: project.name,
      label: project.displayName || project.name,
    })),
    status: STATUS_OPTIONS,
  };

  const activeKeys = ADVANCED_FILTER_KEYS.filter((key) => filters[key] !== '');
  const showClearAll = activeKeys.length + (filters.kind ? 1 : 0) >= 2;

  /** Opens on the applied values; an open panel keeps its draft. */
  const openPanel = () => {
    if (open) return;
    setDraft(advancedFrom(filters));
    setOpen(true);
  };
  const closePanel = () => setOpen(false);
  const apply = () => {
    onApply(draft);
    closePanel();
  };
  const remove = (key: AdvancedFilterKey) => {
    onRemove(key);
    setDraft((current) => ({ ...current, [key]: '' }));
  };
  const clearAll = () => {
    onClearAll();
    setDraft(EMPTY_ADVANCED);
  };

  return (
    <>
      <FilterBar
        actions={
          <>
            <SearchBar
              placeholder="Search A2A agents"
              size="small"
              value={search}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                onSearchChange(e.target.value)
              }
              sx={{ width: 300, maxWidth: '100%' }}
            />
            <Button
              ref={setFiltersButton}
              variant="outlined"
              color="inherit"
              startIcon={<ListFilter size={16} />}
              aria-expanded={open}
              aria-controls={A2A_FILTER_PANEL_ID}
              onClick={() => (open ? closePanel() : openPanel())}
              sx={{ textTransform: 'none', borderColor: 'divider' }}
            >
              Advanced Filters
            </Button>
          </>
        }
      >
        <FilterToggle
          label="Kind"
          value={filters.kind || undefined}
          options={KIND_OPTIONS}
          onChange={onKindChange}
        />
        {activeKeys.map((key) => {
          const name = FILTER_LABELS[key];
          const value = optionLabel(options[key], filters[key]);
          return (
            <FilterChip
              key={key}
              name={name}
              value={value}
              label={`${name}: ${value}`}
              onOpen={openPanel}
              onRemove={() => remove(key)}
            />
          );
        })}
        {showClearAll && (
          <Button size="small" variant="text" onClick={clearAll}>
            Clear all
          </Button>
        )}
      </FilterBar>
      {open && filtersButton && (
        <FilterPanel
          anchorEl={filtersButton}
          id={A2A_FILTER_PANEL_ID}
          title="Advanced Filters"
          width={PANEL_WIDTH}
          onReset={() => setDraft(EMPTY_ADVANCED)}
          onApply={apply}
          onClose={closePanel}
        >
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
            {ADVANCED_FILTER_KEYS.map((key) => (
              <FilterSelect
                key={key}
                filterKey={key}
                options={options[key]}
                value={draft[key]}
                onChange={(value) =>
                  setDraft((current) => ({ ...current, [key]: value }))
                }
              />
            ))}
          </Stack>
        </FilterPanel>
      )}
    </>
  );
};
