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

import type { ReactNode } from "react";
import { Box, Button, Card, Divider, Typography, type SxProps, type Theme } from "@wso2/oxygen-ui";
import { ChevronRight } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";

interface OverviewSectionCardProps {
  /** Uppercase caption shown in the section's own header row. */
  title: string;
  /** Rendered right after the title, e.g. a value with an edit icon. */
  titleAdornment?: ReactNode;
  /** Omit when there's nowhere to link out to — the action button is skipped entirely. */
  actionHref?: string;
  actionLabel?: string;
  /** Extra action rendered in the header row, to the left of the actionHref button. */
  headerAction?: ReactNode;
  /**
   * "card" (default) wraps the content in an outlined Card; "plain" keeps the
   * same header row but wraps the content in a bare Box — no border/padding
   * chrome — for sections that shouldn't read as their own card.
   */
  variant?: "card" | "plain";
  sx?: SxProps<Theme>;
  children: ReactNode;
}

/**
 * Section with a built-in header row (uppercase title + a "View all"-style
 * action link in the top-right corner), used for the standalone sections on an
 * agent's overview page (e.g. Invoke URL, Capabilities). Bakes in the
 * Card/header-row boilerplate so call sites only supply the body; pass
 * variant="plain" to drop the card chrome and render into a plain Box instead.
 */
export const OverviewSectionCard: React.FC<OverviewSectionCardProps> = ({
  title, titleAdornment, actionHref, actionLabel = "View all", headerAction, variant = "card", sx, children,
}) => {
  const content = (
    <>
      <Box display="flex" justifyContent="space-between" pb={1} alignItems="center">
        <Typography
          variant="caption"
          color="text.secondary"
          fontWeight={600}
          sx={{ textTransform: "uppercase", letterSpacing: "0.05em" }}
        >
          {title}
        </Typography>
        {titleAdornment && (
          <Box display="flex" alignItems="center" gap={0.5} sx={{ ml: 1.5, mr: 1, flex: 1, minWidth: 0 }}>
            {titleAdornment}
          </Box>
        )}
        <Box display="flex" alignItems="center" gap={0.5}>
          {headerAction}
          {headerAction && actionHref && (
            <Divider orientation="vertical" flexItem sx={{ my: 0.5 }} />
          )}
          {actionHref && (
            <Button
              size="small"
              variant="text"
              endIcon={<ChevronRight size={14} />}
              component={Link}
              to={actionHref}
              sx={{ minWidth: 0, fontSize: "0.75rem" }}
            >
              {actionLabel}
            </Button>
          )}
        </Box>
      </Box>
      {children}
    </>
  );

  if (variant === "plain") {
    return <Box sx={{ m: 1, ...sx }}>{content}</Box>;
  }

  return (
    <Card variant="outlined" sx={{ px: 2, my: 0.5, pt: 0.5, pb: 3, ...sx }}>
      {content}
    </Card>
  );
};
