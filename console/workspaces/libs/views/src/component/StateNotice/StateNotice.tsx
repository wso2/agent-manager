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

import React from "react";
import { alpha, Box, ListingTable, Stack, Typography } from "@wso2/oxygen-ui";
import { Check, type LucideIcon } from "@wso2/oxygen-ui-icons-react";

export interface StateNoticeStep {
  label: string;
  state: "done" | "current" | "todo";
}

interface StateNoticeProps {
  icon: LucideIcon;
  title: string;
  description: React.ReactNode;
  /** `error` and `warning` tint the icon; `info` uses the primary colour. */
  tone?: "info" | "warning" | "error";
  /** A short path to follow, with where the user is on it. */
  steps?: StateNoticeStep[];
  /** Extra content under the description, such as a command to copy. */
  children?: React.ReactNode;
  actions?: React.ReactNode;
  compact?: boolean;
  /** Set false when the parent already provides the container. */
  contained?: boolean;
}

const REDUCED = "@media (prefers-reduced-motion: reduce)";

/**
 * A state panel: a haloed icon, a message and what to do next. `compact` drops the outer
 * container and shrinks it to sit inside a card.
 */
export const StateNotice: React.FC<StateNoticeProps> = ({
  icon: Icon,
  title,
  description,
  tone = "info",
  steps,
  children,
  actions,
  compact = false,
  contained = true,
}) => {
  const size = compact ? 56 : 72;
  const color = tone === "info" ? "primary.main" : `${tone}.main`;
  const body = (
      <Stack
        spacing={compact ? 2 : 3}
        alignItems="center"
        sx={{ px: 2, py: compact ? 3 : { xs: 4, md: 7 } }}
      >
        <Box
          sx={(theme) => {
            const base = theme.palette[tone === "info" ? "primary" : tone].main;
            return {
              position: "relative",
              width: size,
              height: size,
              borderRadius: "50%",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color,
              bgcolor: alpha(base, 0.12),
              border: "1px solid",
              borderColor: color,
              boxShadow: `0 0 0 8px ${alpha(base, 0.08)}`,
              "@keyframes stateNoticeHalo": {
                from: { transform: "scale(1)", opacity: 0.5 },
                to: { transform: "scale(1.45)", opacity: 0 },
              },
              "&::after": {
                content: '""',
                position: "absolute",
                inset: -1,
                borderRadius: "50%",
                border: "1px solid",
                borderColor: color,
                opacity: 0,
                animation: "stateNoticeHalo 2.6s ease-out infinite",
              },
              [REDUCED]: { "&::after": { animation: "none" } },
            };
          }}
        >
          <Icon size={compact ? 26 : 32} />
        </Box>

        <Stack spacing={1} alignItems="center" sx={{ maxWidth: compact ? 300 : 520 }}>
          <Typography variant={compact ? "h6" : "h5"} component="h2" textAlign="center">
            {title}
          </Typography>
          <Typography variant="body2" color="text.secondary" textAlign="center">
            {description}
          </Typography>
        </Stack>

        {steps && (
          <Stack
            direction={{ xs: "column", sm: "row" }}
            alignItems={{ xs: "flex-start", sm: "center" }}
            spacing={{ xs: 1.5, sm: 2 }}
            aria-label="Steps"
          >
            {steps.map((step, index) => (
              <React.Fragment key={step.label}>
                {index > 0 && (
                  <Box
                    sx={{
                      display: { xs: "none", sm: "block" },
                      width: 32,
                      height: 1,
                      bgcolor: "divider",
                    }}
                  />
                )}
                <Stack direction="row" spacing={1} alignItems="center">
                  <Box
                    sx={(theme) => ({
                      width: 24,
                      height: 24,
                      borderRadius: "50%",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      typography: "caption",
                      fontWeight: 600,
                      border: "1px solid",
                      borderColor: step.state === "todo" ? "divider" : "primary.main",
                      color: step.state === "todo" ? "text.secondary" : "primary.main",
                      bgcolor:
                        step.state === "done"
                          ? alpha(theme.palette.primary.main, 0.14)
                          : "transparent",
                    })}
                  >
                    {step.state === "done" ? <Check size={14} /> : index + 1}
                  </Box>
                  <Typography
                    variant="body2"
                    color={step.state === "current" ? "text.primary" : "text.secondary"}
                    sx={{ fontWeight: step.state === "current" ? 600 : 400 }}
                  >
                    {step.label}
                  </Typography>
                </Stack>
              </React.Fragment>
            ))}
          </Stack>
        )}

        {children}

        {actions && (
          <Stack direction="row" spacing={1.5} justifyContent="center" flexWrap="wrap" useFlexGap>
            {actions}
          </Stack>
        )}
      </Stack>
  );
  if (compact || !contained) return body;
  return <ListingTable.Container sx={{ my: 3 }}>{body}</ListingTable.Container>;
};
