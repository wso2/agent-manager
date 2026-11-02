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
import { Box, Form, Stack, Typography } from "@wso2/oxygen-ui";
import { IconTile } from "../IconTile";
import { RadioMark } from "../RadioMark";

export interface SelectionCardProps {
  /** The icon, about 18px; it is placed in an IconTile. */
  icon: ReactNode;
  title: string;
  /** One line under the title. */
  description: string;
  /** The full explanation, shown as the description's tooltip when it is cut short. */
  fullDescription?: string;
  selected: boolean;
  onSelect: () => void;
  disabled?: boolean;
}

/**
 * One option of a single-choice group: icon tile, title, one-line description and a radio
 * marker at the end. Put several in a role="radiogroup" grid.
 */
export function SelectionCard({
  icon,
  title,
  description,
  fullDescription,
  selected,
  onSelect,
  disabled = false,
}: SelectionCardProps) {
  return (
    <Form.CardButton
      role="radio"
      aria-checked={selected}
      selected={selected}
      disabled={disabled}
      onClick={onSelect}
      sx={{ height: "100%" }}
    >
      <Form.CardContent sx={{ width: "100%", height: "100%", py: 1.5, px: 2 }}>
        <Stack direction="row" alignItems="center" spacing={1.5} sx={{ width: "100%" }}>
          <IconTile selected={selected}>{icon}</IconTile>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 500 }}>
              {title}
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              noWrap
              title={fullDescription}
              sx={{ display: "block" }}
            >
              {description}
            </Typography>
          </Box>
          <RadioMark selected={selected} />
        </Stack>
      </Form.CardContent>
    </Form.CardButton>
  );
}

/** Responsive grid for a group of SelectionCards: one column, three from md. */
export const SELECTION_GRID_SX = {
  display: "grid",
  gap: 1.5,
  gridTemplateColumns: { xs: "1fr", md: "repeat(3, minmax(0, 1fr))" },
} as const;
