/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { useState } from "react";
import {
  BuildDetailsResponse,
  BuildStep,
  BuildStepStatus,
} from "@agent-management-platform/types";
import {
  alpha,
  Box,
  Card,
  CircularProgress,
  Collapse,
  IconButton,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { Check, ChevronDown, Clock, X } from "@wso2/oxygen-ui-icons-react";
import { NESTED_CARD_SX } from "./OverviewSectionCard";

const PIP = 20;
/** Steps fade in one after another rather than all at once. */
const STAGGER_MS = 70;
/** The pipeline's fixed length, used to shape the placeholder. */
const STEP_COUNT = 5;
/** Fixed so the spacer opposite it can balance the strip exactly. */
const TOGGLE = 28;
/** The drawer surface is translucent, so the card opts out of stacking it. */
const CARD_SX = { ...NESTED_CARD_SX, px: 2, py: 1.5 } as const;

type PaletteKey = "success" | "error" | "warning" | "primary";

const STATUS_COLOR: Record<BuildStepStatus, PaletteKey> = {
  Succeeded: "success",
  Failed: "error",
  Running: "warning",
  Pending: "primary",
};

function colorFor(status: BuildStepStatus): PaletteKey {
  return STATUS_COLOR[status] ?? "primary";
}

function iconFor(status: BuildStepStatus) {
  switch (status) {
    case "Succeeded":
      return <Check size={12} />;
    case "Failed":
      return <X size={12} />;
    case "Running":
      return <CircularProgress size={12} color="inherit" />;
    default:
      return <Clock size={12} />;
  }
}

/**
 * The wire `type` is the status reached at a step, not the stage itself
 * (slot 3 is `BuildSucceeded` even when it failed), so label by stage.
 */
const STEP_LABEL: Record<string, string> = {
  BuildInitiated: "Initiated",
  BuildTriggered: "Triggered",
  BuildRunning: "Build execution",
  BuildSucceeded: "Build workflow",
  BuildCompleted: "Workload update",
  WorkloadUpdated: "Workload update",
  BuildFailed: "Build workflow",
};

function labelFor(type: string): string {
  return STEP_LABEL[type] ?? type;
}

/** Shared by the strip and list connectors; callers add their own geometry. */
function connectorSx(status: BuildStepStatus, reached: boolean) {
  return {
    borderRadius: 1,
    bgcolor: reached ? `${colorFor(status)}.main` : "divider",
    opacity: reached ? 0.5 : 1,
  };
}

/** "4s", "1m 20s" — blank until a step has both ends of its window. */
function formatElapsed(startedAt?: string, finishedAt?: string): string | undefined {
  if (!startedAt || !finishedAt) return undefined;
  const ms = new Date(finishedAt).getTime() - new Date(startedAt).getTime();
  if (!Number.isFinite(ms) || ms < 0) return undefined;
  const totalSeconds = Math.round(ms / 1000);
  if (totalSeconds < 60) return `${totalSeconds}s`;
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
}

/** The status disc, shared by both the collapsed strip and the list. */
function StepPip(props: { status: BuildStepStatus }) {
  const { status } = props;
  const theme = useTheme();
  const palette = colorFor(status);
  const color = theme.palette[palette].main;

  return (
    <Box
      sx={{
        width: PIP,
        height: PIP,
        flexShrink: 0,
        borderRadius: "50%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        color,
        bgcolor: alpha(color, 0.12),
        border: "1px solid",
        borderColor: status === "Pending" ? "divider" : color,
        ...(status === "Running" && {
          animation: "buildPipPulse 1.8s ease-in-out infinite",
          "@keyframes buildPipPulse": {
            "0%, 100%": { boxShadow: `0 0 0 0 ${alpha(color, 0)}` },
            "50%": { boxShadow: `0 0 0 4px ${alpha(color, 0.2)}` },
          },
          "@media (prefers-reduced-motion: reduce)": { animation: "none" },
        }),
      }}
    >
      {iconFor(status)}
    </Box>
  );
}

export interface BuildStepsProps {
  build: BuildDetailsResponse;
}

/** Collapsed view: one labelled column per step, sized to fit any width. */
function StepStrip(props: { steps: BuildStep[] }) {
  const { steps } = props;

  return (
    <Stack direction="row" alignItems="flex-start" sx={{ flex: 1, minWidth: 0 }}>
      {steps.map((step, index) => {
        const reached = step.status !== "Pending";
        const elapsed = formatElapsed(step.startedAt, step.finishedAt);
        const tip = [labelFor(step.type), elapsed ?? step.status, step.message]
          .filter(Boolean)
          .join(" · ");
        return (
          <Stack
            key={`${step.type}-${index}`}
            alignItems="center"
            sx={{ flex: 1, minWidth: 0, position: "relative" }}
          >
            {/* Sits in the gap between pips, never under one. */}
            {index > 0 && (
              <Box
                sx={{
                  position: "absolute",
                  top: PIP / 2 - 1,
                  right: `calc(50% + ${PIP / 2}px)`,
                  width: `calc(100% - ${PIP}px)`,
                  height: "2px",
                  ...connectorSx(steps[index - 1].status, reached),
                }}
              />
            )}
            <Tooltip title={tip} placement="top">
              <Box sx={{ display: "flex" }}>
                <StepPip status={step.status} />
              </Box>
            </Tooltip>
            <Typography
              variant="caption"
              color={reached ? "text.secondary" : "text.disabled"}
              noWrap
              sx={{ mt: 0.5, maxWidth: "100%", px: 0.5 }}
            >
              {labelFor(step.type)}
            </Typography>
          </Stack>
        );
      })}
    </Stack>
  );
}

/** Expanded view: every step's status, duration and message, top to bottom. */
function StepList(props: { steps: BuildStep[] }) {
  const { steps } = props;

  return (
    <Stack component="ol" sx={{ listStyle: "none", m: 0, p: 0 }}>
      {steps.map((step, index) => {
        const isLast = index === steps.length - 1;
        const reached = step.status !== "Pending";
        const elapsed = formatElapsed(step.startedAt, step.finishedAt);
        return (
          <Stack
            component="li"
            direction="row"
            gap={1.5}
            key={`${step.type}-${index}`}
            sx={{
              opacity: 0,
              animation: `buildStepIn 240ms ease-out ${index * STAGGER_MS}ms forwards`,
              "@keyframes buildStepIn": {
                from: { opacity: 0, transform: "translateY(4px)" },
                to: { opacity: 1, transform: "none" },
              },
              "@media (prefers-reduced-motion: reduce)": { opacity: 1, animation: "none" },
            }}
          >
            <Stack alignItems="center" sx={{ flexShrink: 0 }}>
              <StepPip status={step.status} />
              {!isLast && (
                <Box
                  sx={{
                    width: "2px",
                    flex: 1,
                    minHeight: 12,
                    my: 0.25,
                    ...connectorSx(step.status, reached),
                  }}
                />
              )}
            </Stack>

            <Box sx={{ flex: 1, minWidth: 0, pb: isLast ? 0 : 1.5, pt: 0.25 }}>
              <Stack direction="row" gap={1} alignItems="baseline">
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 500, flex: 1, minWidth: 0 }}
                  noWrap
                >
                  {labelFor(step.type)}
                </Typography>
                <Typography variant="caption" color="text.disabled" sx={{ flexShrink: 0 }}>
                  {[step.status, elapsed].filter(Boolean).join(" · ")}
                </Typography>
              </Stack>
              {step.message && (
                <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
                  {step.message}
                </Typography>
              )}
            </Box>
          </Stack>
        );
      })}
    </Stack>
  );
}

/**
 * Build progress. Collapsed it is a labelled strip that fits any drawer width;
 * expanded it swaps to a vertical list carrying each step's status, duration
 * and message — the two are alternatives, so neither crowds the other.
 */
export function BuildSteps(props: BuildStepsProps) {
  const { build } = props;
  const [expanded, setExpanded] = useState(false);
  const steps = build.steps ?? [];
  if (!steps.length) return null;

  return (
    <Card variant="outlined" sx={CARD_SX}>
      <Stack direction="row" alignItems="center" gap={1}>
        {expanded ? (
          <Typography variant="body2" sx={{ fontWeight: 500, flex: 1 }}>
            Build steps
          </Typography>
        ) : (
          <>
            {/* Balances the toggle so the pips stay centred. */}
            <Box sx={{ width: TOGGLE, flexShrink: 0 }} />
            <StepStrip steps={steps} />
          </>
        )}
        <Tooltip title={expanded ? "Hide details" : "Show details"}>
          <IconButton
            size="small"
            onClick={() => setExpanded((open) => !open)}
            aria-expanded={expanded}
            aria-label={expanded ? "Hide build step details" : "Show build step details"}
            sx={{
              flexShrink: 0,
              width: TOGGLE,
              height: TOGGLE,
              transition: "transform 180ms ease",
              transform: expanded ? "rotate(180deg)" : "none",
              "@media (prefers-reduced-motion: reduce)": { transition: "none" },
            }}
          >
            <ChevronDown size={16} />
          </IconButton>
        </Tooltip>
      </Stack>

      <Collapse in={expanded} unmountOnExit>
        <Box sx={{ mt: 1.5 }}>
          <StepList steps={steps} />
        </Box>
      </Collapse>
    </Card>
  );
}

/**
 * Placeholder for {@link BuildSteps}. Lives beside it so the two shapes stay
 * in step — the collapsed card's geometry is the only thing it has to match.
 */
export function BuildStepsSkeleton() {
  return (
    <Card variant="outlined" sx={CARD_SX} aria-hidden="true">
      <Stack direction="row" alignItems="center">
        <Box sx={{ width: TOGGLE, flexShrink: 0 }} />
        {Array.from({ length: STEP_COUNT }).map((_, index) => (
          <Stack key={index} alignItems="center" sx={{ flex: 1, minWidth: 0 }}>
            <Skeleton variant="circular" width={PIP} height={PIP} />
            <Skeleton variant="text" width="70%" height={14} sx={{ mt: 0.5 }} />
          </Stack>
        ))}
        <Box sx={{ width: TOGGLE, flexShrink: 0 }} />
      </Stack>
    </Card>
  );
}
