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

import React, { useCallback, useEffect } from "react";
import {
  Button,
  Divider,
  Grow,
  IconButton,
  MenuItem,
  Paper,
  Popper,
  type PopperProps,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import type { AgentRegistryManagement } from "@agent-management-platform/types";
import { TextInput } from "@agent-management-platform/views";
import {
  ALL,
  ALL_LABELS,
  type AgentRegistryFilterOptions,
  PANEL_FILTER_NAMES,
  type PanelFilterKey,
  type PanelFilters,
} from "../agentRegistryFilters";

export const FILTERS_PANEL_ID = "agent-registry-filters-panel";

// Two columns of fields for managed agents; one short column for unmanaged ones.
const PANEL_WIDTH: Record<AgentRegistryManagement, number> = {
  managed: 680,
  unmanaged: 360,
};

// Space between the Filters button and the panel.
const PANEL_GAP = 6;

// Caps the panel's height at the space left below the button (16px margin), as the
// traces filter panel does, so Filter stays on screen. Popper reruns it on resize.
const PANEL_MODIFIERS: PopperProps["modifiers"] = [
  { name: "offset", options: { offset: [0, PANEL_GAP] } },
  {
    name: "fitBelowAnchor",
    enabled: true,
    phase: "beforeWrite",
    fn: ({ state }) => {
      const anchorBottom = state.elements.reference.getBoundingClientRect().bottom;
      state.elements.popper.style.setProperty(
        "--registry-filters-max-height",
        `${window.innerHeight - anchorBottom - PANEL_GAP - 16}px`,
      );
    },
  },
];

interface PanelSelectProps {
  filterKey: PanelFilterKey;
  draft: PanelFilters;
  options: AgentRegistryFilterOptions;
  onDraftChange: (draft: PanelFilters) => void;
}

/** One field of the panel: a select with "All …" and the values agents have. */
function PanelSelect({ filterKey, draft, options, onDraftChange }: PanelSelectProps) {
  return (
    <TextInput
      select
      size="small"
      fullWidth
      label={PANEL_FILTER_NAMES[filterKey]}
      value={draft[filterKey]}
      onChange={(e) => onDraftChange({ ...draft, [filterKey]: e.target.value })}
    >
      <MenuItem value={ALL}>{ALL_LABELS[filterKey]}</MenuItem>
      {options[filterKey].map((option) => (
        <MenuItem key={option.value} value={option.value}>
          {option.label}
        </MenuItem>
      ))}
    </TextInput>
  );
}

/** A titled group of panel fields. */
function FilterSection({
  title,
  hint,
  children,
}: {
  title: string;
  /** One line under the title saying how the fields apply. */
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <Stack component="section" spacing={1.5}>
      <Typography variant="subtitle2" component="h3" sx={{ fontWeight: 600 }}>
        {title}
      </Typography>
      {hint && (
        <Typography variant="caption" color="text.secondary">
          {hint}
        </Typography>
      )}
      {children}
    </Stack>
  );
}

interface FieldsProps {
  draft: PanelFilters;
  options: AgentRegistryFilterOptions;
  onDraftChange: (draft: PanelFilters) => void;
}

/** Managed agents: what they are and who has them on the left; where they run on the right. */
function ManagedFields(fieldProps: FieldsProps) {
  const column = { flex: 1, minWidth: 0 };
  return (
    <Stack
      direction={{ xs: "column", sm: "row" }}
      spacing={3}
      divider={<Divider orientation="vertical" flexItem />}
    >
      <Stack spacing={2.5} divider={<Divider flexItem />} sx={column}>
        <FilterSection title="Agent">
          <PanelSelect filterKey="hosting" {...fieldProps} />
          <PanelSelect filterKey="interface" {...fieldProps} />
        </FilterSection>
        <FilterSection title="Ownership">
          <PanelSelect filterKey="project" {...fieldProps} />
          <PanelSelect filterKey="owner" {...fieldProps} />
          <PanelSelect filterKey="tag" {...fieldProps} />
        </FilterSection>
      </Stack>
      <Stack spacing={2.5} divider={<Divider flexItem />} sx={column}>
        <FilterSection
          title="Deployment"
          hint="Status, LLM provider and MCP server look only at the chosen environment."
        >
          <PanelSelect filterKey="environment" {...fieldProps} />
          <PanelSelect filterKey="status" {...fieldProps} />
        </FilterSection>
        <FilterSection title="Access">
          <PanelSelect filterKey="llmProvider" {...fieldProps} />
          <PanelSelect filterKey="mcpServer" {...fieldProps} />
        </FilterSection>
      </Stack>
    </Stack>
  );
}

/** Unmanaged agents carry where Shadow Agent Discovery found them, and by which connection. */
function UnmanagedFields(fieldProps: FieldsProps) {
  return (
    <FilterSection title="Discovery">
      <PanelSelect filterKey="foundOn" {...fieldProps} />
      <PanelSelect filterKey="connection" {...fieldProps} />
    </FilterSection>
  );
}

export interface AgentRegistryFiltersPanelProps {
  /** The Filters button the panel hangs from. */
  anchorEl: HTMLElement;
  management: AgentRegistryManagement;
  draft: PanelFilters;
  options: AgentRegistryFilterOptions;
  onDraftChange: (draft: PanelFilters) => void;
  onReset: () => void;
  onApply: () => void;
  onClose: () => void;
}

/** Panel under the Filters button that edits a draft; Filter applies it, Escape discards it. */
export function AgentRegistryFiltersPanel({
  anchorEl,
  management,
  draft,
  options,
  onDraftChange,
  onReset,
  onApply,
  onClose,
}: AgentRegistryFiltersPanelProps) {
  const fieldProps = { draft, options, onDraftChange };

  // A callback ref, as the panel mounts in the portal after this component.
  const focusOnMount = useCallback(
    (el: HTMLElement | null) => el?.focus({ preventScroll: true }),
    [],
  );

  // Escape closes the panel; an open select menu takes its own Escape first.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  return (
    <Popper
      open
      anchorEl={anchorEl}
      placement="bottom-end"
      transition
      role="presentation"
      modifiers={PANEL_MODIFIERS}
      sx={{ zIndex: (theme) => theme.zIndex.mobileStepper }}
    >
      {({ TransitionProps }) => (
        <Grow {...TransitionProps} style={{ transformOrigin: "right top" }}>
          <Paper
            ref={focusOnMount}
            id={FILTERS_PANEL_ID}
            component="aside"
            aria-label="Filters"
            tabIndex={-1}
            elevation={8}
            sx={{
              width: PANEL_WIDTH[management],
              maxWidth: "calc(100vw - 32px)",
              // Scrolls inside on short windows, so Filter stays reachable.
              maxHeight: "var(--registry-filters-max-height)",
              display: "flex",
              flexDirection: "column",
              border: 1,
              borderColor: "divider",
              borderRadius: 1.5,
              overflow: "hidden",
              outline: "none",
              // Opaque, so the list doesn't show through.
              bgcolor: "background.default",
              backgroundImage: "none",
            }}
          >
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
              sx={{ pl: 2.5, pr: 1.5, py: 1.5, borderBottom: 1, borderColor: "divider" }}
            >
              <Typography variant="h6" component="h2">
                Filters
              </Typography>
              <IconButton size="small" aria-label="Close filters" onClick={onClose}>
                <CloseIcon size={18} />
              </IconButton>
            </Stack>
            <Stack sx={{ flex: 1, minHeight: 0, overflowY: "auto", px: 2.5, py: 2 }}>
              {management === "managed" ? (
                <ManagedFields {...fieldProps} />
              ) : (
                <UnmanagedFields {...fieldProps} />
              )}
            </Stack>
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
              sx={{ px: 2.5, py: 1.5, borderTop: 1, borderColor: "divider" }}
            >
              <Button variant="text" onClick={onReset}>
                Reset
              </Button>
              <Button variant="contained" onClick={onApply}>
                Filter
              </Button>
            </Stack>
          </Paper>
        </Grow>
      )}
    </Popper>
  );
}
