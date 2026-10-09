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

import React, { useEffect, useRef } from "react";
import type { TraceFilters } from "@agent-management-platform/types";
import {
  Box,
  Button,
  Checkbox,
  Divider,
  FormControlLabel,
  IconButton,
  Paper,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { X as CloseIcon } from "@wso2/oxygen-ui-icons-react";
import { EMPTY_DRAFT, type TraceFilterDraft, isDraftValid } from "../traceFilters";
import {
  EvaluatorSelect,
  FilterSection,
  FilterTextField,
  ThresholdField,
} from "./TraceFilterControls";

export const TRACE_FILTERS_DRAWER_ID = "trace-filters-drawer";

// Fixed, so the drawer stays the same length however long the list is.
export const TRACE_FILTERS_DRAWER_HEIGHT = 560;

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

interface EvaluationSectionProps extends SectionProps {
  evaluators?: string[];
  evaluatorsLoading?: boolean;
  onEvaluatorsOpen?: () => void;
}

/** Evaluator and the score range. */
const EvaluationSection: React.FC<EvaluationSectionProps> = ({
  evaluators,
  evaluatorsLoading,
  onEvaluatorsOpen,
  ...props
}) => (
  <FilterSection title="Evaluation">
    <EvaluatorSelect
      value={props.draft.filters.evaluator}
      options={evaluators}
      loading={evaluatorsLoading}
      onOpen={onEvaluatorsOpen}
      onChange={(v) => props.onDraftChange(withFilter(props.draft, "evaluator", v))}
    />
    <ThresholdField thresholdKey="minScore" {...props} />
    <ThresholdField thresholdKey="maxScore" {...props} />
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

/** Conversation ID, in monospace. */
const ConversationSection: React.FC<SectionProps> = ({ draft, onDraftChange, onSubmit }) => (
  <FilterSection title="Conversation">
    <FilterTextField
      label="Conversation ID"
      placeholder="Paste a conversation ID"
      monospace
      value={draft.filters.conversationId}
      onChange={(v) => onDraftChange(withFilter(draft, "conversationId", v))}
      onSubmit={onSubmit}
    />
  </FilterSection>
);

export interface TraceFiltersDrawerProps {
  draft: TraceFilterDraft;
  onDraftChange: (draft: TraceFilterDraft) => void;
  onApply: () => void;
  onClose: () => void;
  evaluators?: string[];
  evaluatorsLoading?: boolean;
  onEvaluatorsOpen?: () => void;
}

/** Panel over the list that edits a draft of the filters; Filter applies it, Escape discards it. */
export const TraceFiltersDrawer: React.FC<TraceFiltersDrawerProps> = ({
  draft,
  onDraftChange,
  onApply,
  onClose,
  evaluators,
  evaluatorsLoading,
  onEvaluatorsOpen,
}) => {
  const paperRef = useRef<HTMLElement>(null);
  const valid = isDraftValid(draft);
  /** Applies the draft unless a custom value is invalid. */
  const submit = () => {
    if (valid) onApply();
  };
  const sectionProps = { draft, onDraftChange, onSubmit: submit };

  useEffect(() => {
    paperRef.current?.focus();
  }, []);

  // Escape anywhere closes the drawer; an open menu takes its own Escape first.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  return (
    <Paper
      ref={paperRef}
      id={TRACE_FILTERS_DRAWER_ID}
      component="aside"
      aria-label="Filters"
      tabIndex={-1}
      elevation={8}
      sx={{
        position: "absolute",
        top: 0,
        right: 0,
        height: TRACE_FILTERS_DRAWER_HEIGHT,
        // Full width once the list is narrower than the drawer.
        width: 380,
        maxWidth: "100%",
        zIndex: (theme) => theme.zIndex.mobileStepper,
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
        sx={{ pl: 2.5, pr: 1.5, py: 1.5, borderBottom: 1, borderColor: "divider" }}
      >
        <Typography variant="h6" component="h2">
          Filters
        </Typography>
        <IconButton size="small" aria-label="Close filters" onClick={onClose}>
          <CloseIcon size={18} />
        </IconButton>
      </Stack>
      <Box sx={{ flex: 1, overflowY: "auto", px: 2.5, py: 2 }}>
        <Stack spacing={2.5} divider={<Divider flexItem />}>
          <PerformanceSection {...sectionProps} />
          <EvaluationSection
            {...sectionProps}
            evaluators={evaluators}
            evaluatorsLoading={evaluatorsLoading}
            onEvaluatorsOpen={onEvaluatorsOpen}
          />
          <ModelToolsSection {...sectionProps} />
          <ConversationSection {...sectionProps} />
        </Stack>
      </Box>
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
  );
};
