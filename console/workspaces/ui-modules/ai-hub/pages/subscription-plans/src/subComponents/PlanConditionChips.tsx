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

import { Fragment, useLayoutEffect, useRef, useState } from "react";
import { Box, Chip, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";

import { isBuiltInType, type ConditionLimits, type PlanCondition } from "./planConditions";

/** The plans table shows a plan's first three condition windows as chips; a "+N" chip lists the
 * rest. */
export const MAX_CONDITION_CHIPS = 3;

/** What one chip shows, and its tooltip. */
export type ConditionChip = {
  label: string;
  first: string;
  more: number;
  details: string[];
  custom: boolean;
};

const LABELS: Record<string, string> = {
  completion: "Completion tokens",
  prompt: "Prompt tokens",
  requests: "Requests",
  toolCalls: "Tool calls",
  total: "Tokens",
};

const LABEL_KEYS: Record<string, string> = {
  "request-count": "requests",
  "tool-call-count": "toolCalls",
};

const UNITS: Record<string, string> = { DAY: "day", HOUR: "hr", MINUTE: "min", MONTH: "mo" };

/** One limit of a condition, e.g. "1,000 / min" or "5,000 / 2 hr". */
const windowText = (count: number, amount: number, unit: string) =>
  `${count.toLocaleString()} / ${amount === 1 ? "" : `${amount} `}${UNITS[unit] ?? ""}`;

/** One chip per condition window (a custom condition: one chip, its values for the tooltip). */
export const conditionChips = (conditions: PlanCondition[]): ConditionChip[] =>
  conditions.flatMap((condition): ConditionChip[] => {
    const params = (condition.params ?? {}) as Record<string, unknown>;
    if (!isBuiltInType(condition.type)) {
      const values = Object.entries(params).map(
        ([name, value]) => `${name} = ${typeof value === "string" ? value : JSON.stringify(value)}`,
      );
      return [
        {
          custom: true,
          details: values.length > 0 ? values : ["No values"],
          first: condition.id ?? "",
          label: "Custom",
          more: 0,
        },
      ];
    }
    const limits = params as Partial<ConditionLimits>;
    const labelKey =
      condition.type === "token-count" ? (limits.tokenType ?? "total") : LABEL_KEYS[condition.type];
    const windows = (limits.limits ?? []).map((window) =>
      windowText(window.count, window.timeAmount ?? 1, window.timeUnit),
    );
    const label = LABELS[labelKey] ?? "Custom";
    const allowed = limits.onExceed === "allow" ? ["Allowed past the limit"] : [];
    if (windows.length === 0) {
      return [{ custom: false, details: ["No limit"], first: "No limit", label, more: 0 }];
    }
    // Each window is its own chip: two request windows read as two Requests chips.
    return windows.map((window) => ({
      custom: false,
      details: [window, ...allowed],
      first: window,
      label,
      more: 0,
    }));
  });

const chipSx = {
  "& .MuiChip-label": { alignItems: "center", display: "flex", gap: 0.75 },
  height: 24,
  typography: "caption",
} as const;

const chipTitle = (chip: ConditionChip) =>
  chip.custom ? `${chip.label} · ${chip.first}` : chip.label;

/** The tooltip surface: a solid card with a border, not the default grey. */
const tipSlotProps = {
  tooltip: {
    sx: {
      // The theme's paper is translucent; the page background is solid, so rows don't show through.
      bgcolor: "background.default",
      border: 1,
      borderColor: "divider",
      boxShadow: 3,
      color: "text.primary",
      maxWidth: 360,
      px: 1.5,
      py: 1,
    },
  },
} as const;

/** Conditions as a compact two-column list: the condition, then its limits (or values). */
const ConditionTip = ({ chips }: { chips: ConditionChip[] }) => (
  <Box
    sx={{
      alignItems: "baseline",
      columnGap: 1.5,
      display: "grid",
      gridTemplateColumns: "auto 1fr",
      rowGap: 0.5,
    }}
  >
    {chips.map((chip, index) => (
      <Fragment key={index}>
        <Typography component="div" noWrap sx={{ fontWeight: 600 }} variant="caption">
          {chipTitle(chip)}
        </Typography>
        <Typography
          color="text.secondary"
          component="div"
          sx={{ fontFamily: "monospace" }}
          variant="caption"
        >
          {/* A custom condition's pairs one under another; a limit's on one line. */}
          {chip.custom
            ? chip.details.map((line, i) => <div key={i}>{line}</div>)
            : chip.details.join(", ")}
        </Typography>
      </Fragment>
    ))}
  </Box>
);

/** The Conditions cell of a plans table: one chip per condition window (a condition with two
 * windows shows two chips), at most `max` (three by default), and a "+N" chip whose tooltip lists
 * the rest; a plan with none is Unlimited. */
const chipGapPx = 6; // gap={0.75}
const moreChipPx = 44; // room kept for the "+N" chip

/** One condition chip: what it counts in bold, then its limit in monospace. */
const ConditionChipView = ({ chip, shrink = false }: { chip: ConditionChip; shrink?: boolean }) => (
  <Chip
    label={
      <>
        <Box component="span" sx={{ fontWeight: 600 }}>
          {chip.label}
        </Box>
        <Box component="span" sx={{ fontFamily: "monospace" }}>
          {chip.first}
        </Box>
      </>
    }
    size="small"
    sx={{ ...chipSx, flexShrink: shrink ? 1 : 0, minWidth: shrink ? 0 : undefined }}
    variant="outlined"
  />
);

/** How many of the chips fit, whole, in `width` pixels, keeping room for "+N" when some don't. */
const chipsThatFit = (widths: number[], width: number, max: number): number => {
  const all = Math.min(widths.length, max);
  let used = 0;
  for (let i = 0; i < all; i += 1) {
    used += widths[i] + (i > 0 ? chipGapPx : 0);
    const room = i + 1 < widths.length ? width - moreChipPx - chipGapPx : width;
    // The first chip always shows, cut short when even it doesn't fit.
    if (used > room) return Math.max(i, 1);
  }
  return all;
};

/** The Conditions cell of a plans table: one chip per condition window (a condition with two
 * windows shows two chips), at most `max` (three by default), and a "+N" chip whose tooltip lists
 * the rest; a plan with none is Unlimited. On one line, only chips that fit whole are shown: the
 * others move into "+N", so no chip is cut. */
export function PlanConditionChips({
  conditions,
  max = MAX_CONDITION_CHIPS,
  singleLine = false,
}: {
  conditions: PlanCondition[] | undefined;
  max?: number;
  /** Keep the chips on one line (a fixed-height table row), never wrapping. */
  singleLine?: boolean;
}) {
  const chips = conditionChips(conditions ?? []);
  const boxRef = useRef<HTMLDivElement>(null);
  const measureRef = useRef<HTMLDivElement>(null);
  const [fit, setFit] = useState(max);

  useLayoutEffect(() => {
    if (!singleLine || !boxRef.current || !measureRef.current) return undefined;
    const box = boxRef.current;
    const measure = () => {
      const widths = Array.from(measureRef.current?.children ?? []).map(
        (el) => (el as HTMLElement).offsetWidth,
      );
      setFit(chipsThatFit(widths, box.clientWidth, max));
    };
    measure();
    // The hidden copy changes size when the chips' fonts load (the limits are monospace), so it is
    // watched as well as the cell: measured before the font arrives, the chips read too narrow.
    const observer = new ResizeObserver(measure);
    observer.observe(box);
    observer.observe(measureRef.current);
    let live = true;
    document.fonts?.ready.then(() => {
      if (live) measure();
    });
    return () => {
      live = false;
      observer.disconnect();
    };
  }, [singleLine, max, conditions]);

  if (chips.length === 0) {
    return (
      <Typography color="text.secondary" variant="body2">
        Unlimited
      </Typography>
    );
  }
  const count = singleLine ? Math.min(fit, max) : max;
  const shown = chips.slice(0, count);
  const rest = chips.slice(count);
  return (
    <Box ref={boxRef} sx={{ minWidth: 0, position: "relative", width: "100%" }}>
      {singleLine && (
        // A hidden copy of the chips, to measure each one. Its wrapper has no height and clips,
        // so the copy never widens the row.
        <Box
          aria-hidden
          sx={{
            height: 0,
            left: 0,
            overflow: "hidden",
            position: "absolute",
            top: 0,
            width: "100%",
          }}
        >
          <Box
            ref={measureRef}
            sx={{
              display: "flex",
              gap: 0.75,
              pointerEvents: "none",
              visibility: "hidden",
              whiteSpace: "nowrap",
              width: "max-content",
            }}
          >
            {chips.slice(0, max).map((chip, index) => (
              <ConditionChipView chip={chip} key={index} />
            ))}
          </Box>
        </Box>
      )}
      <Stack direction="row" flexWrap={singleLine ? "nowrap" : "wrap"} gap={0.75}>
        {shown.map((chip, index) => (
          // The chip says it all; only "+N" has a tooltip, for what it hides.
          // Only a lone chip may be cut short, when even it doesn't fit the cell.
          <ConditionChipView chip={chip} key={index} shrink={shown.length === 1} />
        ))}
        {rest.length > 0 && (
          <Tooltip slotProps={tipSlotProps} title={<ConditionTip chips={rest} />}>
            <Chip
              aria-label={`${rest.length} more ${rest.length === 1 ? "condition" : "conditions"}`}
              label={`+${rest.length}`}
              size="small"
              sx={{ ...chipSx, flexShrink: 0 }}
              tabIndex={0}
              variant="outlined"
            />
          </Tooltip>
        )}
      </Stack>
    </Box>
  );
}
