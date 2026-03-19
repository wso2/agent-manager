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

import { alpha, Box, Stack, Theme, Tooltip, Typography, useTheme } from "@wso2/oxygen-ui";

export function formatCpu(cores: number): string {
  return cores < 1 ? `${Math.round(cores * 1000)}m` : cores.toFixed(1);
}

export function formatMemory(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)}Ki`;
  if (bytes < 1024 * 1024 * 1024)
    return `${(bytes / (1024 * 1024)).toFixed(0)}Mi`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)}Gi`;
}

/** Parse Kubernetes-style CPU string to cores (e.g. "500m" -> 0.5, "1" -> 1) */
export function parseCpuToCores(str: string): number | undefined {
  if (!str || str === "—") return undefined;
  const m = str.match(/^([0-9]+(?:\.[0-9]+)?)\s*m?$/i);
  if (!m) return undefined;
  const val = parseFloat(m[1]);
  return str.toLowerCase().endsWith("m") ? val / 1000 : val;
}

/** Parse Kubernetes-style memory string to bytes (e.g. "512Mi" -> bytes) */
export function parseMemoryToBytes(str: string): number | undefined {
  if (!str || str === "—") return undefined;
  const m = str.match(/^([0-9]+(?:\.[0-9]+)?)\s*(Ki|Mi|Gi|Ti|Pi|Ei)?$/i);
  if (!m) return undefined;
  const val = parseFloat(m[1]);
  const unit = (m[2] ?? "").toLowerCase();
  const factors: Record<string, number> = {
    ki: 1024,
    mi: 1024 * 1024,
    gi: 1024 * 1024 * 1024,
    ti: 1024 ** 4,
    pi: 1024 ** 5,
    ei: 1024 ** 6,
  };
  return val * (factors[unit] ?? 1);
}

/** Format usage as percentage of request (e.g. current/request * 100) */
export function formatUsagePercent(
  current: number,
  request: number | undefined
): string | undefined {
  if (request === undefined || request <= 0) return undefined;
  const pct = Math.round((current / request) * 100);
  return `${pct}%`;
}

/** Get color variant for usage percentage: >90% error, >70% warning, else success */
export function getUsagePercentVariant(
  current: number,
  request: number | undefined
): "success" | "warning" | "error" | undefined {
  if (request === undefined || request <= 0) return undefined;
  const pct = (current / request) * 100;
  if (pct > 90) return "error";
  if (pct > 70) return "warning";
  return "success";
}
import type { ReactNode } from "react";

export type SecondaryVariant = "success" | "warning" | "error" | "default";

export interface ResourceMetricChipProps {
  icon: ReactNode;
  label: string;
  primaryValue: string | number;
  secondaryValue?: string | number;
  secondaryTooltip?: string;
  secondaryVariant?: SecondaryVariant;
}

function getSecondaryBadgeStyles(theme: Theme, variant?: SecondaryVariant) {
  if (!variant || variant === "default") {
    return {
      bgcolor: theme.vars?.palette?.background?.default ?? theme.palette?.background?.default,
      color: "text.secondary",
    };
  }
  const paletteMap = {
    success: theme.palette.success,
    warning: theme.palette.warning,
    error: theme.palette.error,
  };
  const palette = paletteMap[variant];
  return {
    bgcolor: alpha(palette.main, 0.2),
  };
}

export function ResourceMetricChip({
  icon,
  label,
  primaryValue,
  secondaryValue,
  secondaryTooltip,
  secondaryVariant,
}: ResourceMetricChipProps) {
  const theme = useTheme();
  const badgeStyles = getSecondaryBadgeStyles(theme, secondaryVariant);
  const secondaryBadge = (
    <Box
      component="span"
      borderRadius={0.5}
      px={0.75}
      sx={badgeStyles}
    >
      <Typography variant="caption"  fontWeight={600}>{secondaryValue ?? "—"}</Typography>
    </Box>
  );

  return (
    <Box
      bgcolor={theme.vars?.palette?.background?.default}
      border={`1px solid ${theme.vars?.palette?.divider}`}
      borderRadius={0.5}
      p={0.5}
      width="fit-content"
    >
      <Stack direction="row" gap={2} alignItems="center">
        <Tooltip title={label}>
          <Box display="flex" alignItems="center" justifyContent="center">{icon}</Box>
        </Tooltip>
        <Stack direction="row" gap={0.75} alignItems="center">
          <Typography variant="caption">{primaryValue}</Typography>
          {secondaryTooltip ? (
            <Tooltip title={secondaryTooltip}>{secondaryBadge}</Tooltip>
          ) : (
            secondaryBadge
          )}
        </Stack>
      </Stack>
    </Box>
  );
}
