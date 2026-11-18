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
import {
  ALL,
  type AgentRegistryFilterOptions,
  type AgentRegistryFilters,
  EMPTY_FILTERS,
  type FilterChip,
  type PanelDraft,
  filterChips,
} from "../agentRegistryFilters";
import { AgentRegistryFiltersPanel, FILTERS_PANEL_ID } from "./AgentRegistryFiltersPanel";

/** How the list shows agents. */
export type AgentRegistryView = "table" | "tiles";

function panelDraftOf(filters: AgentRegistryFilters): PanelDraft {
  return {
    registration: filters.registration,
    project: filters.project,
    owner: filters.owner,
    tag: filters.tag,
    llmProvider: filters.llmProvider,
    mcpServer: filters.mcpServer,
  };
}

const EMPTY_PANEL_DRAFT = panelDraftOf(EMPTY_FILTERS);

interface LifecycleToggleProps {
  value: string;
  options: AgentRegistryFilterOptions["lifecycleStates"];
  onChange: (value: string) => void;
}

/** All, or one lifecycle state; applies straight away. */
function LifecycleToggle({ value, options, onChange }: LifecycleToggleProps) {
  return (
    <ToggleButtonGroup
      exclusive
      size="small"
      value={value}
      aria-label="Lifecycle state"
      onChange={(_, next: string | null) => {
        if (next !== null) onChange(next);
      }}
      sx={{ "& .MuiToggleButton-root": { px: 1.75, textTransform: "none" } }}
    >
      <ToggleButton value={ALL}>All</ToggleButton>
      {options.map((option) => (
        <ToggleButton key={option.value} value={option.value}>
          {option.label}
        </ToggleButton>
      ))}
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
  shownCount: number;
  totalCount: number;
  /** Unregistered agents the default registration filter hides; 0 when none are hidden. */
  hiddenUnregisteredCount: number;
}

/**
 * First row: search and the lifecycle toggle on the left, Filters and the view toggle
 * on the right. Second row, when anything is filtered: the applied filters as chips.
 */
export function AgentRegistryToolbar({
  filters,
  options,
  onChange,
  view,
  onViewChange,
  shownCount,
  totalCount,
  hiddenUnregisteredCount,
}: AgentRegistryToolbarProps) {
  const [panelOpen, setPanelOpen] = useState(false);
  const [draft, setDraft] = useState<PanelDraft>(() => panelDraftOf(filters));
  const [filtersButton, setFiltersButton] = useState<HTMLButtonElement | null>(null);

  const chips = filterChips(filters, options);
  const environmentLabel =
    filters.environment === ALL
      ? "any environment"
      : (options.environments.find((e) => e.value === filters.environment)?.label ??
        filters.environment);
  const isAnythingFiltered =
    chips.length > 0 || filters.lifecycleState !== ALL || filters.search.trim() !== "";

  /** Opens the panel on the applied filters; an open panel keeps its draft. */
  const openPanel = () => {
    if (panelOpen) return;
    setDraft(panelDraftOf(filters));
    setPanelOpen(true);
  };
  /** Applies the draft; the panel's Filter button is disabled while registration is null. */
  const applyDraft = () => {
    if (draft.registration === null) return;
    onChange({ ...filters, ...draft, registration: draft.registration });
    setPanelOpen(false);
  };
  /** Puts the chip's filter back to its default: ALL, or registered agents only. */
  const removeChip = (chip: FilterChip) => {
    const defaultValue = EMPTY_FILTERS[chip.key];
    onChange({ ...filters, [chip.key]: defaultValue });
    setDraft((d) => ({ ...d, [chip.key]: defaultValue }));
  };
  const showUnregistered = () => {
    onChange({ ...filters, registration: "both" });
    setDraft((d) => ({ ...d, registration: "both" }));
  };
  /** Clears search, lifecycle and panel filters. The environment is the page scope, so it stays. */
  const clearAll = () => {
    onChange({ ...EMPTY_FILTERS, environment: filters.environment });
    setDraft(EMPTY_PANEL_DRAFT);
  };

  const panelFilterCount = chips.length;

  return (
    <Stack spacing={1.5}>
      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
        <SearchBar
          placeholder="Search by name, project, owner or tag"
          size="small"
          width="standard"
          value={filters.search}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
            onChange({ ...filters, search: e.target.value })
          }
        />
        <LifecycleToggle
          value={filters.lifecycleState}
          options={options.lifecycleStates}
          onChange={(lifecycleState) => onChange({ ...filters, lifecycleState })}
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
          {panelFilterCount > 0 ? `Filters (${panelFilterCount})` : "Filters"}
        </Button>
        <ViewToggle value={view} onChange={onViewChange} />
      </Stack>

      {/* Hidden by default, so say so: otherwise people take the list as complete. */}
      {hiddenUnregisteredCount > 0 && (
        <Typography variant="body2" color="text.secondary">
          {hiddenUnregisteredCount === 1
            ? "1 unregistered agent found outside Agent Manager is not shown."
            : `${hiddenUnregisteredCount} unregistered agents found outside Agent Manager are not shown.`}{" "}
          <Button
            variant="text"
            size="small"
            onClick={showUnregistered}
            sx={{ p: 0, minWidth: 0, verticalAlign: "baseline" }}
          >
            Show
          </Button>
        </Typography>
      )}

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
            Showing {shownCount} of {totalCount} agents
          </Typography>
        </Stack>
      )}

      {panelOpen && filtersButton && (
        <AgentRegistryFiltersPanel
          anchorEl={filtersButton}
          draft={draft}
          options={options}
          environmentLabel={environmentLabel}
          onDraftChange={setDraft}
          onReset={() => setDraft(EMPTY_PANEL_DRAFT)}
          onApply={applyDraft}
          onClose={() => setPanelOpen(false)}
        />
      )}
    </Stack>
  );
}
