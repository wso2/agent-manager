/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { ComponentType } from "react";
import { Box, Chip, Tooltip } from "@wso2/oxygen-ui";
import { Shield, ShieldCheck, ShieldPlus } from "@wso2/oxygen-ui-icons-react";

export type IsolationTierValue = "runc" | "gvisor" | "kata";

export interface IsolationTierMeta {
  value: IsolationTierValue;
  /** Sandbox level rank: 1 (runc) → 3 (kata). */
  tier: 1 | 2 | 3;
  /**
   * Human name of the underlying runtime, e.g. "gVisor". Only surfaced at
   * environment creation — everywhere else (chips, tooltips, badges) use the
   * runtime-agnostic shortLabel/fullLabel.
   */
  runtimeLabel: string;
  /** Compact label for chips/columns, e.g. "Sandbox L2". */
  shortLabel: string;
  /** Full label for tooltips, e.g. "Sandbox Level 2". */
  fullLabel: string;
  color: "default" | "info" | "success";
  /** Theme color path for tinting the bare shield icon, e.g. "info.main". */
  iconColor: string;
  icon: ComponentType<{ size?: string | number }>;
}

const TIER_META: Record<IsolationTierValue, IsolationTierMeta> = {
  runc: {
    value: "runc",
    tier: 1,
    runtimeLabel: "runc (default)",
    shortLabel: "Sandbox L1",
    fullLabel: "Sandbox Level 1",
    color: "default",
    iconColor: "text.secondary",
    icon: Shield,
  },
  gvisor: {
    value: "gvisor",
    tier: 2,
    runtimeLabel: "gVisor",
    shortLabel: "Sandbox L2",
    fullLabel: "Sandbox Level 2",
    color: "info",
    iconColor: "info.main",
    icon: ShieldPlus,
  },
  kata: {
    value: "kata",
    tier: 3,
    runtimeLabel: "Kata Containers",
    shortLabel: "Sandbox L3",
    fullLabel: "Sandbox Level 3",
    color: "success",
    iconColor: "success.main",
    icon: ShieldCheck,
  },
};

/** Unrecognized/absent tiers resolve to runc, the platform default. */
export function getIsolationTierMeta(tier?: string): IsolationTierMeta {
  return TIER_META[tier as IsolationTierValue] ?? TIER_META.runc;
}

interface IsolationTierBadgeProps {
  tier?: string;
  size?: number;
  /** Overrides the tier's default icon color, e.g. to match a selected tab's text. */
  color?: string;
}

/**
 * Bare shield icon with a "Sandbox Level N" tooltip.
 * Used next to environment names where a chip would be too heavy.
 */
export function IsolationTierBadge({ tier, size = 18, color }: IsolationTierBadgeProps) {
  const meta = getIsolationTierMeta(tier);
  const IconComponent = meta.icon;
  return (
    <Tooltip title={meta.fullLabel}>
      <Box display="inline-flex" alignItems="center" sx={{ color: color ?? meta.iconColor }}>
        <IconComponent size={size} />
      </Box>
    </Tooltip>
  );
}

interface IsolationTierChipProps {
  tier?: string;
}

/**
 * "Sandbox LN" chip with the tier's shield icon and a "Sandbox Level N" tooltip.
 */
export function IsolationTierChip({ tier }: IsolationTierChipProps) {
  const meta = getIsolationTierMeta(tier);
  const IconComponent = meta.icon;
  return (
    <Tooltip title={meta.fullLabel}>
      <Chip
        icon={<IconComponent size={14} />}
        label={meta.shortLabel}
        size="small"
        variant="outlined"
        color={meta.color}
        sx={{ cursor: "default" }}
      />
    </Tooltip>
  );
}
