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

import React, { useCallback, useEffect, useRef } from "react";
import { Link as RouterLink } from "react-router-dom";
import { alpha, Box, Card, Divider, Skeleton, Stack, Typography } from "@wso2/oxygen-ui";
import { ArrowRight } from "@wso2/oxygen-ui-icons-react";
import { IconTile } from "@agent-management-platform/views";
import { useCountUp } from "../hooks/useCountUp";
import { CARD_STAGGER_MS, REDUCED, SWAP_OUT_MS } from "../motion";

/** The number beside a section title, set off by a divider so it reads as a count. */
export function ResourceCount({ count, animate = true }: { count?: number; animate?: boolean }) {
  const animated = useCountUp(animate ? count : undefined);
  const shown = animate ? animated : count;
  if (shown === undefined) return <Skeleton variant="text" width={20} />;
  return (
    <Typography variant="body1" color="text.secondary" sx={{ minWidth: "1ch" }}>
      {shown}
    </Typography>
  );
}

export interface ResourceCardData {
  icon: React.ReactNode;
  title: string;
  count?: number;
  description: string;
  actionLabel: string;
  to: string;
  /** For resources with no single org-wide total (agent identity is per environment). */
  countless?: boolean;
  /** Stays on the overview with no items, so there is always a way to add one. */
  alwaysShow?: boolean;
}

interface ResourceCardProps {
  card: ResourceCardData;
  /** Position in the grid, which sets when the entrance starts. */
  index: number;
  /** The content is lifting away to make room for the next card. */
  leaving: boolean;
  onPause: (paused: boolean) => void;
}
/** A platform resource at a glance: what it is, how many exist, and where to add more. */
export function ResourceCard({ card, index, leaving, onPause }: ResourceCardProps) {
  // Each hold is sent once, and released on unmount (no mouseleave/blur fires then),
  // so a card removed while hovered or focused can't leave rotation paused for good.
  const holds = useRef({ hover: false, focus: false });
  const hold = useCallback(
    (kind: "hover" | "focus", on: boolean) => {
      if (holds.current[kind] === on) return;
      holds.current[kind] = on;
      onPause(on);
    },
    [onPause],
  );
  useEffect(() => {
    const current = holds.current;
    return () => {
      if (current.hover) onPause(false);
      if (current.focus) onPause(false);
      current.hover = false;
      current.focus = false;
    };
  }, [onPause]);

  const swapSx = {
    animation: leaving
      ? `overviewSwapOut ${SWAP_OUT_MS}ms ease-in both`
      : `overviewSwapIn 420ms cubic-bezier(0.2, 0.7, 0.2, 1) both`,
    "@keyframes overviewSwapOut": {
      to: { opacity: 0, transform: "translateY(-10px)" },
    },
    "@keyframes overviewSwapIn": {
      from: { opacity: 0, transform: "translateY(12px)" },
      to: { opacity: 1, transform: "none" },
    },
    [REDUCED]: { animation: "none" },
  } as const;

  return (
    <Card
      variant="outlined"
      onMouseEnter={() => hold("hover", true)}
      onMouseLeave={() => hold("hover", false)}
      onFocus={() => hold("focus", true)}
      onBlur={() => hold("focus", false)}
      sx={{
        display: "flex",
        flexDirection: "column",
        minHeight: { xs: 0, md: 170 },
        // The cards arrive in a short cascade; nothing else on the page moves.
        animation: `overviewCardIn 480ms cubic-bezier(0.2, 0.7, 0.2, 1) ${index * CARD_STAGGER_MS}ms both`,
        "@keyframes overviewCardIn": {
          from: { opacity: 0, transform: "translateY(14px)" },
          to: { opacity: 1, transform: "none" },
        },
        [REDUCED]: { animation: "none" },
      }}
    >
      <Stack direction="row" spacing={2} sx={{ p: { xs: 2, md: 2.5 }, flex: 1, ...swapSx }}>
        <IconTile selected size={52}>
          {card.icon}
        </IconTile>
        <Box sx={{ minWidth: 0 }}>
          <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mb: 0.5 }}>
            <Typography variant="h6" component="h2" sx={{ fontWeight: 600 }}>
              {card.title}
            </Typography>
            {!card.countless && !!card.count && (
              <>
                <Divider orientation="vertical" flexItem />
                <ResourceCount count={card.count} animate={false} />
              </>
            )}
          </Stack>
          <Typography variant="body2" color="text.secondary">
            {card.description}
          </Typography>
        </Box>
      </Stack>
      <Divider />
      <Stack
        component={RouterLink}
        to={card.to}
        direction="row"
        justifyContent="space-between"
        alignItems="center"
        sx={{
          px: { xs: 2, md: 2.5 },
          py: 1.5,
          color: "primary.main",
          textDecoration: "none",
          "&:hover": { bgcolor: (theme) => alpha(theme.palette.primary.main, 0.06) },
          ...swapSx,
        }}
      >
        <Typography variant="subtitle2" color="inherit">
          {card.actionLabel}
        </Typography>
        <ArrowRight size={16} />
      </Stack>
    </Card>
  );
}
