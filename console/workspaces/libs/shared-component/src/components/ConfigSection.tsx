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

import { useEffect, useState, type ReactNode } from "react";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown } from "@wso2/oxygen-ui-icons-react";

export interface ConfigSectionProps {
  /** The section's heading, e.g. "Environment Variables". */
  title: string;
  /** The section's current value, shown on the right of the heading, e.g. "5 variables". */
  summary?: ReactNode;
  /** Open when first shown. Sections start closed otherwise. */
  defaultExpanded?: boolean;
  /**
   * The section holds an invalid value: it opens so the field is visible, and its heading says
   * so, so the problem can't hide in a closed section.
   */
  hasError?: boolean;
  /** Buttons for the section, e.g. Add, shown at the top of its content when open. */
  actions?: ReactNode;
  children: ReactNode;
}

/**
 * A configuration section that can be closed to just its heading and current value, so a long
 * form shows every section at a glance and opens only the ones being edited.
 */
export function ConfigSection({
  title,
  summary,
  defaultExpanded = false,
  hasError = false,
  actions,
  children,
}: ConfigSectionProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  // An invalid value opens the section, so the field showing the error is on screen.
  useEffect(() => {
    if (hasError) {
      setExpanded(true);
    }
  }, [hasError]);

  return (
    <Accordion
      expanded={expanded}
      onChange={(_, isExpanded) => setExpanded(isExpanded)}
      disableGutters
      variant="outlined"
    >
      <AccordionSummary expandIcon={<ChevronDown size={18} />}>
        <Stack
          direction="row"
          alignItems="center"
          justifyContent="space-between"
          gap={2}
          sx={{ width: "100%", pr: 1 }}
        >
          <Typography variant="subtitle1" fontWeight={600}>
            {title}
          </Typography>
          <Typography
            variant="body2"
            color={hasError ? "error" : "text.secondary"}
            noWrap
          >
            {hasError ? "Needs attention" : summary}
          </Typography>
        </Stack>
      </AccordionSummary>
      <AccordionDetails>
        <Stack spacing={2}>
          {actions && (
            <Stack direction="row" justifyContent="flex-end" gap={1}>
              {actions}
            </Stack>
          )}
          {children}
        </Stack>
      </AccordionDetails>
    </Accordion>
  );
}
