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
  Checkbox,
  Divider,
  FormControlLabel,
  FormHelperText,
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
  type PanelDraft,
  type PanelFilterKey,
  registrationFromTicks,
  registrationTicks,
} from "../agentRegistryFilters";

export const FILTERS_PANEL_ID = "agent-registry-filters-panel";

// Wide enough for two columns of fields.
const PANEL_WIDTH = 640;

/** The option that filters nothing; "All", as in the header's "All environments". */
const ALL_LABELS: Record<PanelFilterKey, string> = {
  project: "All projects",
  owner: "All owners",
  tag: "All tags",
  llmProvider: "All LLM providers",
  mcpServer: "All MCP servers",
};

interface PanelSelectProps {
  filterKey: PanelFilterKey;
  draft: PanelDraft;
  options: AgentRegistryFilterOptions;
  onDraftChange: (draft: PanelDraft) => void;
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

interface RegistrationChecksProps {
  draft: PanelDraft;
  onDraftChange: (draft: PanelDraft) => void;
}

/** Registered and Unregistered (shadow) agents; Unregistered is unticked by default. */
function RegistrationChecks({ draft, onDraftChange }: RegistrationChecksProps) {
  // While both are unticked the draft holds null, which reads as neither ticked.
  const ticks = draft.registration
    ? registrationTicks(draft.registration)
    : { registered: false, unregistered: false };
  const setTicks = (registered: boolean, unregistered: boolean) =>
    onDraftChange({ ...draft, registration: registrationFromTicks(registered, unregistered) });

  return (
    <Stack spacing={0}>
      <FormControlLabel
        label="Registered"
        control={
          <Checkbox
            size="small"
            checked={ticks.registered}
            onChange={(e) => setTicks(e.target.checked, ticks.unregistered)}
          />
        }
        slotProps={{ typography: { variant: "body2" } }}
      />
      <FormControlLabel
        label="Unregistered (shadow agents found outside Agent Manager)"
        control={
          <Checkbox
            size="small"
            checked={ticks.unregistered}
            onChange={(e) => setTicks(ticks.registered, e.target.checked)}
          />
        }
        slotProps={{ typography: { variant: "body2" } }}
      />
      {draft.registration === null && (
        <FormHelperText error>Tick at least one.</FormHelperText>
      )}
    </Stack>
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
  draft: PanelDraft;
  options: AgentRegistryFilterOptions;
  /** Says where the Access filters look, e.g. "Production" or "any environment". */
  environmentLabel: string;
  onDraftChange: (draft: PanelDraft) => void;
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
  const isValid = draft.registration !== null;

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
              <Stack spacing={2.5} divider={<Divider flexItem />} sx={{ flex: 1, minWidth: 0 }}>
                <FilterSection title="Access">
                  <Typography variant="caption" color="text.secondary">
                    What the agent can reach in {environmentLabel}.
                  </Typography>
                  <PanelSelect filterKey="llmProvider" {...fieldProps} />
                  <PanelSelect filterKey="mcpServer" {...fieldProps} />
                </FilterSection>
                <FilterSection title="Registration">
                  <RegistrationChecks draft={draft} onDraftChange={onDraftChange} />
                </FilterSection>
              </Stack>
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
              <Button variant="contained" disabled={!isValid} onClick={onApply}>
                Filter
              </Button>
            </Stack>
          </Paper>
        </Grow>
      )}
    </Popper>
  );
}
