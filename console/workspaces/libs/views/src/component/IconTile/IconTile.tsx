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

import type { ReactNode } from "react";
import { alpha, Box, type SxProps, type Theme } from "@wso2/oxygen-ui";

const SIZES = {
  sm: { box: 32, radius: 1 },
  md: { box: 40, radius: 1.5 },
} as const;

export interface IconTileProps {
  /** The icon to show; size it to roughly half the tile (18 in "sm", 20 in "md"). */
  children: ReactNode;
  /** Tints the tile with the primary colour, e.g. when the card it sits in is selected. */
  selected?: boolean;
  size?: keyof typeof SIZES;
  sx?: SxProps<Theme>;
}

/**
 * An icon in a small rounded tile: neutral by default, primary-tinted when selected.
 * Leads selection cards and list rows so their icons read as one family.
 */
export function IconTile({ children, selected = false, size = "sm", sx }: IconTileProps) {
  const { box, radius } = SIZES[size];
  return (
    <Box
      aria-hidden="true"
      sx={[
        (theme) => ({
          width: box,
          height: box,
          flexShrink: 0,
          borderRadius: radius,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          color: selected ? "primary.main" : "text.secondary",
          bgcolor: selected ? alpha(theme.palette.primary.main, 0.12) : "action.hover",
        }),
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
    >
      {children}
    </Box>
  );
}
