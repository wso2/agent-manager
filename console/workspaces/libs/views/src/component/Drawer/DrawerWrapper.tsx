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

import { DrawerPanel, useTheme, type SxProps, type Theme } from "@wso2/oxygen-ui";
import type { ReactNode } from "react";

/** Used when a drawer names no width of its own. */
const DEFAULT_WIDTH = 700;

export interface DrawerWrapperProps {
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  /**
   * Panel width. DrawerPanel takes a single width rather than a range, so
   * `minWidth` wins when both are given and `maxWidth` is the fallback.
   */
  minWidth?: number | string;
  maxWidth?: number | string;
  /**
   * @deprecated DrawerPanel scrolls its own body; nothing reads this.
   */
  disableScroll?: boolean;
  /** Widens the panel to the full viewport. */
  fullscreen?: boolean;
  sx?: SxProps<Theme>;
}

/**
 * Thin adapter over Oxygen's {@link DrawerPanel}, kept so the drawers already
 * written against this API pick up the panel's behaviour — focus handling,
 * Escape/backdrop close, the expand toggle and a scrollable body — without
 * each one being rewritten.
 */
export function DrawerWrapper({
  children,
  minWidth,
  maxWidth,
  fullscreen,
  sx,
  ...panelProps
}: DrawerWrapperProps) {
  const theme = useTheme();

  return (
    <DrawerPanel
      {...panelProps}
      width={minWidth ?? maxWidth ?? DEFAULT_WIDTH}
      expanded={fullscreen}
      sx={[
        // Drawers here open over other drawer-level surfaces.
        { zIndex: theme.zIndex.drawer + 2 },
        ...(Array.isArray(sx) ? sx : sx ? [sx] : []),
      ]}
    >
      {children}
    </DrawerPanel>
  );
}
