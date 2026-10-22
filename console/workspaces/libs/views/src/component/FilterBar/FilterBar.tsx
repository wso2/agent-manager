/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { type ReactNode } from "react";
import { Box, Stack } from "@wso2/oxygen-ui";

export interface FilterBarProps {
  /** Fixed on the right of the first line, e.g. search and the Filters button. */
  actions: ReactNode;
  /** Toggles, chips and Clear all, inline on the left. */
  children?: ReactNode;
}

/**
 * Actions float right on the first line; children that don't fit wrap below,
 * across the full width, so the actions never move.
 */
export const FilterBar: React.FC<FilterBarProps> = ({ actions, children }) => (
  <Box
    sx={{
      display: "flow-root",
      mb: 1,
      "& > *": { display: "inline-flex", verticalAlign: "middle", mr: 1, mb: 1 },
    }}
  >
    <Stack
      direction="row"
      spacing={1}
      useFlexGap
      flexWrap="wrap"
      alignItems="center"
      sx={{ float: "right", maxWidth: "100%", minWidth: 0, "&&": { mr: 0, ml: 1 } }}
    >
      {actions}
    </Stack>
    {children}
  </Box>
);
