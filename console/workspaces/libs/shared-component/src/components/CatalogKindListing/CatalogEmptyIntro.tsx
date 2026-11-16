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

import React, { useEffect, useState } from "react";
import { alpha, Box, Button, IconButton, ListingTable, Stack, Typography } from "@wso2/oxygen-ui";
import {
  BookOpenText,
  Bot,
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  GitBranch,
  Package,
} from "@wso2/oxygen-ui-icons-react";

const DOCS_URL =
  "https://wso2.com/agent-platform/docs/v1.0.0/concepts/agent-kind-and-catalog/";
const STEP_INTERVAL_MS = 3500;
const SLIDE = "500ms cubic-bezier(0.65, 0, 0.35, 1)";
const POP = "600ms cubic-bezier(0.34, 1.56, 0.64, 1)";
const REDUCED = "@media (prefers-reduced-motion: reduce)";

/** Ring that radiates off the active step's icon, and the autoplay fill in its dot. */
const KEYFRAMES = {
  "@keyframes catalogHalo": {
    from: { transform: "scale(1)", opacity: 0.55 },
    to: { transform: "scale(1.35)", opacity: 0 },
  },
  "@keyframes catalogProgress": {
    from: { transform: "scaleX(0)" },
    to: { transform: "scaleX(1)" },
  },
} as const;

/** The kind's lifecycle, in the order a build travels through it. */
const STAGES = [
  {
    label: "Build",
    icon: Package,
    caption: "Start from a successful agent build.",
  },
  {
    label: "Kind version",
    icon: GitBranch,
    caption: "Publish it as a version that pins the build and its config schema.",
  },
  {
    label: "Catalog",
    icon: BookOpenText,
    caption: "Add it to the catalog so the whole organization can find it.",
  },
  {
    label: "New agents",
    icon: Bot,
    caption: "Create agents from the version. No source repository needed.",
  },
] as const;

const COUNT = STAGES.length;

const prefersReducedMotion = () =>
  typeof window !== "undefined" &&
  window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;

/** Empty catalog: a title, a stepped walkthrough carousel, and the docs link. */
export const CatalogEmptyIntro: React.FC = () => {
  const [active, setActive] = useState(0);
  const [paused, setPaused] = useState(false);

  useEffect(() => {
    if (paused || prefersReducedMotion()) return undefined;
    const timer = setTimeout(() => setActive((index) => (index + 1) % COUNT), STEP_INTERVAL_MS);
    return () => clearTimeout(timer);
  }, [paused, active]);

  const go = (delta: number) => setActive((index) => (index + delta + COUNT) % COUNT);

  return (
    // Same container as the other empty states, so the catalog reads as one of them.
    <ListingTable.Container
      sx={{ my: 3 }}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <Stack spacing={3} alignItems="center" sx={{ px: 2, py: { xs: 4, md: 6 } }}>
        <Stack
          direction="row"
          alignItems="center"
          spacing={1}
          role="group"
          aria-roledescription="carousel"
          aria-label="How agent kinds work"
          sx={{ width: "100%", maxWidth: 560 }}
        >
          <IconButton size="small" aria-label="Previous step" onClick={() => go(-1)}>
            <ChevronLeft size={20} />
          </IconButton>

          {/* Vertical padding keeps the icon's glow ring inside the clipped track. */}
          <Box sx={{ flex: 1, minWidth: 0, overflow: "hidden", py: 1.5 }} aria-live="polite">
            <Box
              sx={{
                display: "flex",
                transform: `translateX(-${active * 100}%)`,
                transition: `transform ${SLIDE}`,
                "@media (prefers-reduced-motion: reduce)": { transition: "none" },
              }}
            >
              {STAGES.map((stage, index) => {
                const Icon = stage.icon;
                const current = index === active;
                return (
                  <Stack
                    key={stage.label}
                    direction="row"
                    alignItems="center"
                    spacing={2.5}
                    role="group"
                    aria-roledescription="slide"
                    aria-label={`Step ${index + 1} of ${COUNT}`}
                    aria-hidden={index !== active}
                    sx={{
                      flex: "0 0 100%",
                      minWidth: 0,
                      px: 1.5,
                      opacity: current ? 1 : 0.2,
                      transition: `opacity ${SLIDE}`,
                      [REDUCED]: { transition: "none" },
                    }}
                  >
                    <Box
                      sx={(theme) => ({
                        width: 56,
                        height: 56,
                        flexShrink: 0,
                        borderRadius: "50%",
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        color: "primary.main",
                        bgcolor: alpha(theme.palette.primary.main, 0.14),
                        border: "1px solid",
                        borderColor: "primary.main",
                        boxShadow: `0 0 0 6px ${alpha(theme.palette.primary.main, 0.1)}`,
                        position: "relative",
                        ...KEYFRAMES,
                        // The icon pops in as its slide arrives, then a ring keeps radiating.
                        transform: current ? "scale(1)" : "scale(0.6)",
                        transition: `transform ${POP}`,
                        "&::after": {
                          content: '""',
                          position: "absolute",
                          inset: -1,
                          borderRadius: "50%",
                          border: "1px solid",
                          borderColor: "primary.main",
                          opacity: 0,
                          animation: current ? "catalogHalo 2.4s ease-out 0.5s infinite" : "none",
                        },
                        [REDUCED]: { transition: "none", "&::after": { animation: "none" } },
                      })}
                    >
                      <Icon size={26} />
                    </Box>
                    <Stack spacing={0.25} sx={{ minWidth: 0, minHeight: 72, justifyContent: "center" }}>
                      {[
                        <Typography key="n" variant="caption" color="text.secondary">
                          Step {index + 1} of {COUNT}
                        </Typography>,
                        <Typography key="t" variant="subtitle1">
                          {stage.label}
                        </Typography>,
                        <Typography key="c" variant="body2" color="text.secondary">
                          {stage.caption}
                        </Typography>,
                      ].map((line, row) => (
                        // Lines settle in one after another once the slide is current.
                        <Box
                          key={line.key}
                          sx={{
                            opacity: current ? 1 : 0,
                            transform: current ? "none" : "translateX(20px)",
                            transition: `opacity 450ms ease ${current ? 250 + row * 110 : 0}ms, transform 450ms ease ${current ? 250 + row * 110 : 0}ms`,
                            [REDUCED]: { transition: "none" },
                          }}
                        >
                          {line}
                        </Box>
                      ))}
                    </Stack>
                  </Stack>
                );
              })}
            </Box>
          </Box>

          <IconButton size="small" aria-label="Next step" onClick={() => go(1)}>
            <ChevronRight size={20} />
          </IconButton>
        </Stack>

        <Stack direction="row" spacing={1} role="tablist" aria-label="Choose a step">
          {STAGES.map((stage, index) => (
            <Box
              key={stage.label}
              component="button"
              type="button"
              role="tab"
              aria-selected={index === active}
              aria-label={`Step ${index + 1}: ${stage.label}`}
              onClick={() => setActive(index)}
              sx={(theme) => ({
                position: "relative",
                overflow: "hidden",
                width: index === active ? 28 : 8,
                height: 8,
                p: 0,
                border: 0,
                borderRadius: 4,
                cursor: "pointer",
                bgcolor: index === active ? alpha(theme.palette.primary.main, 0.25) : "divider",
                transition: "width 300ms ease, background-color 300ms ease",
                ...KEYFRAMES,
                [REDUCED]: { transition: "none" },
                "&:focus-visible": {
                  outline: "2px solid",
                  outlineColor: "primary.main",
                  outlineOffset: 2,
                },
              })}
            >
              {index === active && (
                // Fills over the time left on this step; restarts whenever the timer does.
                <Box
                  key={`${active}-${paused}`}
                  sx={{
                    position: "absolute",
                    inset: 0,
                    bgcolor: "primary.main",
                    transformOrigin: "left",
                    animation: paused
                      ? "none"
                      : `catalogProgress ${STEP_INTERVAL_MS}ms linear forwards`,
                    [REDUCED]: { animation: "none" },
                  }}
                />
              )}
            </Box>
          ))}
        </Stack>

        <Typography variant="h6" component="h2" textAlign="center">
          No agent kinds yet
        </Typography>

        <Typography
          variant="body2"
          color="text.secondary"
          textAlign="center"
          sx={{ maxWidth: 480 }}
        >
          Agent kinds are reusable, versioned templates for built agents. Publish one and it
          shows up here for everyone in the organization.
        </Typography>

        <Button
          variant="outlined"
          size="small"
          component="a"
          href={DOCS_URL}
          target="_blank"
          rel="noopener noreferrer"
          endIcon={<ExternalLink size={14} />}
        >
          How agent kinds work
        </Button>
      </Stack>
    </ListingTable.Container>
  );
};

export default CatalogEmptyIntro;
