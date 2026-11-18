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
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "@agent-management-platform/views";
import {
  ALL,
  type AgentRegistryFilterOptions,
  PANEL_FILTER_NAMES,
  type PanelFilterKey,
  type PanelFilters,
} from "../agentRegistryFilters";

export const FILTERS_PANEL_ID = "agent-registry-filters-panel";

// Wide enough for two columns of fields.
const PANEL_WIDTH = 640;

/** Placeholder of each field when nothing is chosen. */
const ANY_LABELS: Record<PanelFilterKey, string> = {
  project: "Any project",
  owner: "Any owner",
  tag: "Any tag",
  llmProvider: "Any LLM provider",
  mcpServer: "Any MCP server",
};

interface PanelSelectProps {
  filterKey: PanelFilterKey;
  draft: PanelFilters;
  options: AgentRegistryFilterOptions;
  onDraftChange: (draft: PanelFilters) => void;
}

/** One field of the panel: a select with "Any …" and the values agents have. */
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
      <MenuItem value={ALL}>{ANY_LABELS[filterKey]}</MenuItem>
      {options[filterKey].map((option) => (
        <MenuItem key={option.value} value={option.value}>
          {option.label}
        </MenuItem>
      ))}
    </TextInput>
  );
}

/** A titled group of panel fields. */
function FilterSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Stack component="section" spacing={1.5} sx={{ flex: 1, minWidth: 0 }}>
      <Typography variant="subtitle2" component="h3" sx={{ fontWeight: 600 }}>
        {title}
      </Typography>
      {children}
    </Stack>
  );
}

export interface AgentRegistryFiltersPanelProps {
  /** The Filters button the panel hangs from. */
  anchorEl: HTMLElement;
  draft: PanelFilters;
  options: AgentRegistryFilterOptions;
  /** Says where the Access filters look, e.g. "Production" or "any environment". */
  environmentLabel: string;
  onDraftChange: (draft: PanelFilters) => void;
  onReset: () => void;
  onApply: () => void;
  onClose: () => void;
}

/** Panel under the Filters button that edits a draft; Filter applies it, Escape discards it. */
export function AgentRegistryFiltersPanel({
  anchorEl,
  draft,
  options,
  environmentLabel,
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
      modifiers={[{ name: "offset", options: { offset: [0, 6] } }]}
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
              width: PANEL_WIDTH,
              maxWidth: "calc(100vw - 32px)",
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
            {/* Two columns; they stack on narrow windows. */}
            <Stack
              direction={{ xs: "column", sm: "row" }}
              spacing={3}
              divider={<Divider orientation="vertical" flexItem />}
              sx={{ px: 2.5, py: 2 }}
            >
              <FilterSection title="Ownership">
                <PanelSelect filterKey="project" {...fieldProps} />
                <PanelSelect filterKey="owner" {...fieldProps} />
                <PanelSelect filterKey="tag" {...fieldProps} />
              </FilterSection>
              <FilterSection title="Access">
                <Typography variant="caption" color="text.secondary">
                  What the agent can reach in {environmentLabel}.
                </Typography>
                <PanelSelect filterKey="llmProvider" {...fieldProps} />
                <PanelSelect filterKey="mcpServer" {...fieldProps} />
              </FilterSection>
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
