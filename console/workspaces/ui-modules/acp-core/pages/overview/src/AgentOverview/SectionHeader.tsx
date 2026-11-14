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

import { Box, Button, IconButton, Tooltip, Typography, type SxProps, type Theme } from "@wso2/oxygen-ui";
import { ChevronRight, Info } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";

interface UppercaseCaptionLabelProps {
  children: React.ReactNode;
  sx?: SxProps<Theme>;
}

/** Small bold uppercase caption used to label a field/section across the overview cards. */
export const UppercaseCaptionLabel: React.FC<UppercaseCaptionLabelProps> = ({ children, sx }) => (
  <Typography
    variant="caption"
    color="text.secondary"
    fontWeight={600}
    sx={{ textTransform: "uppercase", letterSpacing: "0.05em", ...sx }}
  >
    {children}
  </Typography>
);

interface SectionHeaderProps {
  /** Omit to draw just the "View all" link (if given) with no caption. */
  title?: string;
  /** Shown as a tooltip on a trailing info icon right after the title. */
  titleInfo?: React.ReactNode;
  /**
   * Omit when a section has nowhere to link out to (e.g. no deployment page
   * for external agents) — the "View all" button is skipped entirely.
   */
  viewAllHref?: string;
  viewAllLabel?: string;
  mb?: number;
  mt?: number;
}

/**
 * Uppercase caption + "View all" link, marking the boundary before every
 * EnvironmentCard section (Capabilities, Agent Identity, Agent Performance,
 * Recent Traces, System Metrics) that links out to its own full listing page.
 */
export const SectionHeader: React.FC<SectionHeaderProps> = ({
  title, titleInfo, viewAllHref, viewAllLabel = "View all", mb = 0.5, mt = 2,
}) => (
  <Box display="flex" justifyContent={title ? "space-between" : "flex-end"} alignItems="center" mt={mt} mb={mb}>
    {title && (
      <Box display="flex" alignItems="center" gap={0.5}>
        <UppercaseCaptionLabel>{title}</UppercaseCaptionLabel>
        {titleInfo && (
          <Tooltip title={titleInfo}>
            {/* A button, not a bare icon, so the tooltip is reachable by keyboard. */}
            <IconButton
              aria-label={`More information about ${title}`}
              size="small"
              sx={{ p: 0.25, opacity: 0.7 }}
            >
              <Info size={13} />
            </IconButton>
          </Tooltip>
        )}
      </Box>
    )}
    {viewAllHref && (
      <Button
        size="small"
        variant="text"
        endIcon={<ChevronRight size={14} />}
        component={Link}
        to={viewAllHref}
        sx={{ minWidth: 0, fontSize: "0.75rem" }}
      >
        {viewAllLabel}
      </Button>
    )}
  </Box>
);
