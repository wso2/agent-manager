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
import { alpha, Box } from "@wso2/oxygen-ui";

export interface IconTileProps {
  /** The icon to show, about 18px. */
  children: ReactNode;
  /** Tints the tile with the primary colour, e.g. when the card it sits in is selected. */
  selected?: boolean;
}

/**
 * An icon in a small rounded tile: neutral by default, primary-tinted when selected.
 * Leads the selection cards so their icons read as one family.
 */
export function IconTile({ children, selected = false }: IconTileProps) {
  return (
    <Box
      aria-hidden="true"
      sx={(theme) => ({
        width: 32,
        height: 32,
        flexShrink: 0,
        borderRadius: 1,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        color: selected ? "primary.main" : "text.secondary",
        bgcolor: selected ? alpha(theme.palette.primary.main, 0.12) : "action.hover",
      })}
    >
      {children}
    </Box>
  );
}
