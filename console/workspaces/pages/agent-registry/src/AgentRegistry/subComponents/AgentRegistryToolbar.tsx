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

import React, { useState } from "react";
import {
  Box,
  Button,
  ButtonGroup,
  SearchBar,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  LayoutGrid,
  List as TableIcon,
  ListFilter,
  X as RemoveIcon,
} from "@wso2/oxygen-ui-icons-react";
import type { AgentRegistryRegistration } from "@agent-management-platform/types";
import {
  ALL,
  type AgentRegistryFilterOptions,
  type AgentRegistryFilters,
  EMPTY_FILTERS,
  type FilterChip,
  PANEL_FILTER_KEYS,
  type PanelFilters,
  filterChips,
  withRegistration,
} from "../agentRegistryFilters";
import { AgentRegistryFiltersPanel, FILTERS_PANEL_ID } from "./AgentRegistryFiltersPanel";

/** How the list shows agents. */
export type AgentRegistryView = "table" | "tiles";

function panelFiltersOf(filters: AgentRegistryFilters): PanelFilters {
  return {
    hosting: filters.hosting,
    interface: filters.interface,
    project: filters.project,
    owner: filters.owner,
    tag: filters.tag,
    environment: filters.environment,
    status: filters.status,
    llmProvider: filters.llmProvider,
    mcpServer: filters.mcpServer,
    foundOn: filters.foundOn,
  };
}

interface RegistrationToggleProps {
  value: AgentRegistryRegistration;
  counts: Record<AgentRegistryRegistration, number>;
  onChange: (value: AgentRegistryRegistration) => void;
}

/**
 * Registered agents, or shadow agents found outside Agent Manager. One at a time: the
 * two have almost no fields in common, so they don't share a table.
 */
function RegistrationToggle({ value, counts, onChange }: RegistrationToggleProps) {
  return (
    <ToggleButtonGroup
      exclusive
      size="small"
      value={value}
      aria-label="Registration"
      onChange={(_, next: AgentRegistryRegistration | null) => {
        if (next !== null) onChange(next);
      }}
      sx={{ "& .MuiToggleButton-root": { px: 1.75, textTransform: "none" } }}
    >
      <ToggleButton value="registered">Registered ({counts.registered})</ToggleButton>
      <Tooltip title="Shadow agents: found outside Agent Manager and not onboarded yet">
        <ToggleButton value="unregistered">Unregistered ({counts.unregistered})</ToggleButton>
      </Tooltip>
    </ToggleButtonGroup>
  );
}

interface ViewToggleProps {
  value: AgentRegistryView;
  onChange: (value: AgentRegistryView) => void;
}

function ViewToggle({ value, onChange }: ViewToggleProps) {
  return (
    <ToggleButtonGroup
      exclusive
      size="small"
      value={value}
      aria-label="View"
      onChange={(_, next: AgentRegistryView | null) => {
        if (next !== null) onChange(next);
      }}
    >
      <Tooltip title="Table">
        <ToggleButton value="table" aria-label="Table view">
          <TableIcon size={16} />
        </ToggleButton>
      </Tooltip>
      <Tooltip title="Tiles">
        <ToggleButton value="tiles" aria-label="Tile view">
          <LayoutGrid size={16} />
        </ToggleButton>
      </Tooltip>
    </ToggleButtonGroup>
  );
}

interface FilterChipButtonProps {
  chip: FilterChip;
  onOpen: () => void;
  onRemove: () => void;
}

/** The filter's name and value, which open the panel, and an × that removes it. */
function FilterChipButton({ chip, onOpen, onRemove }: FilterChipButtonProps) {
  return (
    <ButtonGroup
      size="small"
      variant="outlined"
      color="inherit"
      sx={{ maxWidth: 320, "& .MuiButton-root": { borderColor: "divider" } }}
    >
      <Button
        title={`${chip.name}: ${chip.value}`}
        onClick={onOpen}
        sx={{ textTransform: "none", minWidth: 0 }}
      >
        <Box component="span" sx={{ color: "text.secondary", whiteSpace: "nowrap" }}>
          {chip.name}
        </Box>
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
      <Button
        aria-label={`Remove ${chip.name} filter`}
        onClick={onRemove}
        sx={{ minWidth: 0, px: 0.75 }}
      >
        <RemoveIcon size={14} />
      </Button>
    </ButtonGroup>
  );
}

export interface AgentRegistryToolbarProps {
  filters: AgentRegistryFilters;
  options: AgentRegistryFilterOptions;
  onChange: (filters: AgentRegistryFilters) => void;
  view: AgentRegistryView;
  onViewChange: (view: AgentRegistryView) => void;
  counts: Record<AgentRegistryRegistration, number>;
  shownCount: number;
}

/**
 * First row: search and the registration toggle on the left, Filters and the view
 * toggle on the right. Second row, when anything is filtered: the applied filters as chips.
 */
export function AgentRegistryToolbar({
  filters,
  options,
  onChange,
  view,
  onViewChange,
  counts,
  shownCount,
}: AgentRegistryToolbarProps) {
  const [panelOpen, setPanelOpen] = useState(false);
  const [draft, setDraft] = useState<PanelFilters>(() => panelFiltersOf(filters));
  const [filtersButton, setFiltersButton] = useState<HTMLButtonElement | null>(null);

  const chips = filterChips(filters, options);
  const isAnythingFiltered = chips.length > 0 || filters.search.trim() !== "";

  /** Opens the panel on the applied filters; an open panel keeps its draft. */
  const openPanel = () => {
    if (panelOpen) return;
    setDraft(panelFiltersOf(filters));
    setPanelOpen(true);
  };
  const applyDraft = () => {
    onChange({ ...filters, ...draft });
    setPanelOpen(false);
  };
  const removeChip = (chip: FilterChip) => {
    onChange({ ...filters, [chip.key]: ALL });
    setDraft((d) => ({ ...d, [chip.key]: ALL }));
  };
  /** Sets this registration's panel filters back to ALL in the draft; Filter applies it. */
  const resetDraft = () => {
    const next = { ...draft };
    for (const key of PANEL_FILTER_KEYS[filters.registration]) next[key] = ALL;
    setDraft(next);
  };
  /** Clears search and the panel filters; the registration toggle stays. */
  const clearAll = () => {
    const cleared = { ...EMPTY_FILTERS, registration: filters.registration };
    onChange(cleared);
    setDraft(panelFiltersOf(cleared));
  };
  /** Switching drops the other registration's filters, which mean nothing here. */
  const switchRegistration = (registration: AgentRegistryRegistration) => {
    const next = withRegistration(filters, registration);
    onChange(next);
    setDraft(panelFiltersOf(next));
    setPanelOpen(false);
  };

  const total = counts[filters.registration];

  return (
    <Stack spacing={1.5}>
      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
        <SearchBar
          placeholder={
            filters.registration === "registered"
              ? "Search by name, project, owner or tag"
              : "Search by name, platform or owner hint"
          }
          size="small"
          width="standard"
          value={filters.search}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
            onChange({ ...filters, search: e.target.value })
          }
        />
        <RegistrationToggle
          value={filters.registration}
          counts={counts}
          onChange={switchRegistration}
        />
        <Box sx={{ flexGrow: 1 }} />
        <Button
          ref={setFiltersButton}
          variant="outlined"
          color="inherit"
          startIcon={<ListFilter size={16} />}
          aria-expanded={panelOpen}
          aria-controls={FILTERS_PANEL_ID}
          onClick={() => (panelOpen ? setPanelOpen(false) : openPanel())}
          sx={{ textTransform: "none", borderColor: "divider" }}
        >
          {chips.length > 0 ? `Filters (${chips.length})` : "Filters"}
        </Button>
        <ViewToggle value={view} onChange={onViewChange} />
      </Stack>

      {isAnythingFiltered && (
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
          {chips.map((chip) => (
            <FilterChipButton
              key={chip.key}
              chip={chip}
              onOpen={openPanel}
              onRemove={() => removeChip(chip)}
            />
          ))}
          <Button size="small" variant="text" onClick={clearAll}>
            Clear all
          </Button>
          <Box sx={{ flexGrow: 1 }} />
          <Typography variant="body2" color="text.secondary">
            Showing {shownCount} of {total} agents
          </Typography>
        </Stack>
      )}

      {panelOpen && filtersButton && (
        <AgentRegistryFiltersPanel
          anchorEl={filtersButton}
          registration={filters.registration}
          draft={draft}
          options={options}
          onDraftChange={setDraft}
          onReset={resetDraft}
          onApply={applyDraft}
          onClose={() => setPanelOpen(false)}
        />
      )}
    </Stack>
  );
}
