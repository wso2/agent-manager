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

import { useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Chip,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";

/**
 * Renders `children` in a section's header action slot. With no slot passed (undefined) the
 * section is used outside an OptionalSection, so they render inline instead.
 */
export function SlotPortal({
  slot,
  children,
}: {
  slot: HTMLElement | null | undefined;
  children: ReactNode;
}) {
  if (slot === undefined) return <>{children}</>;
  return slot ? createPortal(children, slot) : null;
}

interface OptionalSectionProps {
  title: string;
  /** One line on what this gives the agent. */
  description: string;
  /** Entries configured so far; the section is open only while there are some. */
  count: number;
  /** What an entry is called, for the summary chip ("variable"). */
  itemLabel: string;
  /**
   * Renders the section. It receives the header's action slot: the section portals its
   * add button(s) there so they sit in the header, and keeps its entries in the body.
   */
  children: (actionSlot: HTMLElement | null) => ReactNode;
}

/**
 * An optional part of the form. The header carries the add action and the body (the
 * entries) opens only once there is something in it, so an unused section stays one row.
 */
export function OptionalSection({
  title,
  description,
  count,
  itemLabel,
  children,
}: OptionalSectionProps) {
  const [actionSlot, setActionSlot] = useState<HTMLElement | null>(null);

  return (
    <Accordion
      expanded={count > 0}
      disableGutters
      variant="outlined"
      // Same radius and no margin whether open or closed, first or last, so a stack of
      // these stays evenly spaced and uniformly rounded.
      sx={{
        "&&": { m: 0, borderRadius: 1 },
        "&::before": { display: "none" },
      }}
    >
      {/* The header is not a toggle: the section opens itself once it has entries. */}
      <AccordionSummary
        expandIcon={null}
        component="div"
        sx={{
          cursor: "default",
          px: 2,
          py: 1,
          minHeight: 0,
          "&.Mui-expanded": { minHeight: 0 },
          "& .MuiAccordionSummary-content, & .MuiAccordionSummary-content.Mui-expanded": {
            alignItems: "center",
            gap: 2,
            m: 0,
          },
        }}
      >
        <Stack sx={{ flex: 1, minWidth: 0 }}>
          <Stack direction="row" spacing={1} alignItems="center">
            <Typography variant="subtitle1">{title}</Typography>
            {count > 0 && (
              <Chip
                size="small"
                variant="outlined"
                label={`${count} ${itemLabel}${count === 1 ? "" : "s"}`}
              />
            )}
          </Stack>
          <Typography variant="body2" color="text.secondary">
            {description}
          </Typography>
        </Stack>
        <Stack ref={setActionSlot} direction="row" spacing={1} alignItems="center" />
      </AccordionSummary>
      <AccordionDetails sx={{ px: 2, pb: 2, pt: 0 }}>{children(actionSlot)}</AccordionDetails>
    </Accordion>
  );
}
