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
import { Chip, Typography, useTheme } from "@wso2/oxygen-ui";

/**
 * Returns a color based on score thresholds:
 *   >= 0.8 → green, >= 0.6 → amber, < 0.6 → red
 */
export function scoreColor(score: number): string {
  if (score >= 0.8) return "#22c55e";
  if (score >= 0.6) return "#f59e0b";
  return "#ef4444";
}

interface ScoreChipProps {
  /** Score value in 0–1 range */
  score: number;
  /** "chip" renders a colored Chip badge, "text" renders colored Typography */
  variant?: "chip" | "text";
  /** Number of decimal places for the percentage (default: 1) */
  decimals?: number;
}

const ScoreChip: React.FC<ScoreChipProps> = ({
  score,
  variant = "chip",
  decimals = 1,
}) => {
  const theme = useTheme();
  const isDark = theme.palette.mode === "dark";
  const color = scoreColor(score);
  const label = `${(score * 100).toFixed(decimals)}%`;

  if (variant === "text") {
    return (
      <Typography
        variant="caption"
        component="span"
        sx={{ color, fontWeight: 600 }}
      >
        {label}
      </Typography>
    );
  }

  return (
    <Chip
      label={label}
      size="small"
      sx={{
        backgroundColor: `${color}${isDark ? "40" : "18"}`,
        color,
        fontWeight: 600,
        fontSize: "0.75rem",
      }}
    />
  );
};

export default ScoreChip;
