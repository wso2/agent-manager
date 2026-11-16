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

import React, { useCallback, useEffect } from "react";
import type { TraceFilters } from "@agent-management-platform/types";
import {
  Button,
  Checkbox,
  Divider,
  FormControlLabel,
  Grow,
  IconButton,
  Paper,
  Popper,
  type PopperProps,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import { EMPTY_DRAFT, type TraceFilterDraft, isDraftValid } from "../traceFilters";
import { FilterSection, FilterTextField, ThresholdField } from "./TraceFilterControls";

export const TRACE_FILTERS_DRAWER_ID = "trace-filters-drawer";

// Wide enough for two columns of fields.
const TRACE_FILTERS_DRAWER_WIDTH = 760;

// Space between the Filters button and the panel.
const PANEL_GAP = 6;

// Places the panel below the button and caps its height at the space left above the
// window's bottom (16px margin). Popper reruns this on resize and scroll.
const PANEL_MODIFIERS: PopperProps["modifiers"] = [
  { name: "offset", options: { offset: [0, PANEL_GAP] } },
  {
    name: "fitBelowAnchor",
    enabled: true,
    phase: "beforeWrite",
    fn: ({ state }) => {
      const anchorBottom = state.elements.reference.getBoundingClientRect().bottom;
      state.elements.popper.style.setProperty(
        "--trace-filters-max-height",
        `${window.innerHeight - anchorBottom - PANEL_GAP - 16}px`,
      );
    },
  },
];

interface SectionProps {
  draft: TraceFilterDraft;
  onDraftChange: (draft: TraceFilterDraft) => void;
  onSubmit: () => void;
}

/** Sets one filter in the draft. */
const withFilter = <K extends keyof TraceFilters>(
  draft: TraceFilterDraft,
  key: K,
  value: TraceFilters[K],
): TraceFilterDraft => ({ ...draft, filters: { ...draft.filters, [key]: value } });

/** Latency, tokens and steps thresholds. */
const PerformanceSection: React.FC<SectionProps> = (props) => (
  <FilterSection title="Performance">
    <ThresholdField thresholdKey="minDurationMs" {...props} />
    <ThresholdField thresholdKey="minTokens" {...props} />
    <ThresholdField thresholdKey="minSpanCount" {...props} />
  </FilterSection>
);

/** Model, tool, tool failure and MCP server. */
const ModelToolsSection: React.FC<SectionProps> = ({ draft, onDraftChange, onSubmit }) => (
  <FilterSection title="Model and tools">
    <FilterTextField
      label="Model"
      placeholder="e.g. gpt-4o"
      value={draft.filters.model}
      onChange={(v) => onDraftChange(withFilter(draft, "model", v))}
      onSubmit={onSubmit}
    />
    <FilterTextField
      label="Tool called"
      placeholder="e.g. search_web"
      value={draft.filters.tool}
      onChange={(v) => onDraftChange(withFilter(draft, "tool", v))}
      onSubmit={onSubmit}
    />
    <FormControlLabel
      label="Only traces where a tool call failed"
      control={
        <Checkbox
          size="small"
          checked={!!draft.filters.toolError}
          onChange={(e) => onDraftChange(withFilter(draft, "toolError", e.target.checked))}
        />
      }
      slotProps={{ typography: { variant: "body2" } }}
    />
    <FilterTextField
      label="MCP server"
      placeholder="e.g. github"
      value={draft.filters.mcpServer}
      onChange={(v) => onDraftChange(withFilter(draft, "mcpServer", v))}
      onSubmit={onSubmit}
    />
  </FilterSection>
);

/** Conversation ID, in monospace, or no conversation ID at all. */
const ConversationSection: React.FC<SectionProps> = ({ draft, onDraftChange, onSubmit }) => {
  const missing = !!draft.filters.missing?.includes("conversationId");
  return (
    <FilterSection title="Conversation">
      <FilterTextField
        label="Conversation ID"
        placeholder=""
        monospace
        disabled={missing}
        value={missing ? "" : draft.filters.conversationId}
        onChange={(v) => onDraftChange(withFilter(draft, "conversationId", v))}
        onSubmit={onSubmit}
      />
      <FormControlLabel
        label="Only traces without a conversation ID"
        control={
          <Checkbox
            size="small"
            checked={missing}
            onChange={(e) =>
              onDraftChange(withFilter(draft, "missing", e.target.checked ? ["conversationId"] : []))}
          />
        }
        slotProps={{ typography: { variant: "body2" } }}
      />
    </FilterSection>
  );
};

export interface TraceFiltersDrawerProps {
  /** The Filters button the panel hangs from. */
  anchorEl: HTMLElement;
  draft: TraceFilterDraft;
  onDraftChange: (draft: TraceFilterDraft) => void;
  onApply: () => void;
  onClose: () => void;
}

/** Panel under the Filters button that edits a draft; Filter applies it, Escape discards it. */
export const TraceFiltersDrawer: React.FC<TraceFiltersDrawerProps> = ({
  anchorEl,
  draft,
  onDraftChange,
  onApply,
  onClose,
}) => {
  const valid = isDraftValid(draft);
  /** Applies the draft unless a custom value is invalid. */
  const submit = () => {
    if (valid) onApply();
  };
  const sectionProps = { draft, onDraftChange, onSubmit: submit };

  // A callback ref, as the panel mounts in the portal after this component.
  const focusOnMount = useCallback(
    (el: HTMLElement | null) => el?.focus({ preventScroll: true }),
    [],
  );

  // Escape anywhere closes the drawer; an open menu takes its own Escape first.
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
      // Not a tooltip; the panel inside is the landmark.
      role="presentation"
      modifiers={PANEL_MODIFIERS}
      sx={{ zIndex: (theme) => theme.zIndex.mobileStepper }}
    >
      {({ TransitionProps, placement }) => (
        <Grow
          {...TransitionProps}
          style={{
            transformOrigin: placement.startsWith("top") ? "right bottom" : "right top",
          }}
        >
          <Paper
            ref={focusOnMount}
            id={TRACE_FILTERS_DRAWER_ID}
            component="aside"
            aria-label="Advanced Filters"
            tabIndex={-1}
            elevation={8}
            sx={{
              maxHeight: "var(--trace-filters-max-height)",
              width: TRACE_FILTERS_DRAWER_WIDTH,
              maxWidth: "calc(100vw - 32px)",
              display: "flex",
              flexDirection: "column",
              border: 1,
              borderColor: "divider",
              borderRadius: 1.5,
              overflow: "hidden",
              outline: "none",
              // Opaque, as the trace details drawer is, so the list doesn't show through.
              bgcolor: "background.default",
              backgroundImage: "none",
              backdropFilter: "none",
            }}
          >
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
              sx={{
                pl: 2.5,
                pr: 1.5,
                py: 1.5,
                borderBottom: 1,
                borderColor: "divider",
              }}
            >
              <Typography variant="h6" component="h2">
                Advanced Filters
              </Typography>
              <IconButton size="small" aria-label="Close advanced filters" onClick={onClose}>
                <CloseIcon size={18} />
              </IconButton>
            </Stack>
            {/* Two columns; stacks on narrow windows, and scrolls only if the window is short. */}
            <Stack
              direction={{ xs: "column", sm: "row" }}
              spacing={3}
              divider={<Divider orientation="vertical" flexItem />}
              sx={{ flex: 1, minHeight: 0, overflowY: "auto", px: 2.5, py: 2 }}
            >
              <Stack spacing={2.5} divider={<Divider flexItem />} sx={{ flex: 1, minWidth: 0 }}>
                <PerformanceSection {...sectionProps} />
                <ConversationSection {...sectionProps} />
              </Stack>
              <Stack spacing={2.5} divider={<Divider flexItem />} sx={{ flex: 1, minWidth: 0 }}>
                <ModelToolsSection {...sectionProps} />
              </Stack>
            </Stack>
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
              sx={{ px: 2.5, py: 1.5, borderTop: 1, borderColor: "divider" }}
            >
              <Button variant="text" onClick={() => onDraftChange(EMPTY_DRAFT)}>
                Reset
              </Button>
              <Button variant="contained" disabled={!valid} onClick={submit}>
                Filter
              </Button>
            </Stack>
          </Paper>
        </Grow>
      )}
    </Popper>
  );
};
