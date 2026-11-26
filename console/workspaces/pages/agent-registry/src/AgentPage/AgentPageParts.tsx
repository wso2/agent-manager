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

import { useState, type ReactNode } from "react";
import {
  Box,
  Button,
  Card,
  CardContent,
  Divider,
  Grid,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Info } from "@wso2/oxygen-ui-icons-react";
import { formatRelativeTime } from "@agent-management-platform/shared-component";

// The pieces both agent pages in the registry are built from, so a managed and an
// unmanaged agent look alike: a main column of titled cards, and a properties sidebar.

/** The main column and the properties sidebar beside it; the sidebar drops below when narrow. */
export function AgentPageColumns({ main, sidebar }: { main: ReactNode; sidebar: ReactNode }) {
  return (
    <Grid container spacing={3}>
      <Grid size={{ xs: 12, lg: 8 }}>
        <Stack spacing={3}>{main}</Stack>
      </Grid>
      <Grid size={{ xs: 12, lg: 4 }}>{sidebar}</Grid>
    </Grid>
  );
}

/** One titled card of the main column, with an optional line under the title. */
export function SectionCard({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <Card variant="outlined">
      <CardContent>
        <Box sx={{ mb: 2 }}>
          <Typography variant="subtitle1">{title}</Typography>
          {hint && (
            <Typography variant="caption" color="text.secondary" component="p">
              {hint}
            </Typography>
          )}
        </Box>
        {children}
      </CardContent>
    </Card>
  );
}

/** The properties sidebar: its sections, divided. It stays in view while the page scrolls. */
export function PropertiesSidebar({ children }: { children: ReactNode }) {
  return (
    <Card
      component="aside"
      variant="outlined"
      aria-label="Properties"
      sx={{
        position: { lg: "sticky" },
        top: { lg: 24 },
        maxHeight: { lg: "calc(100vh - 48px)" },
        overflowY: { lg: "auto" },
      }}
    >
      <CardContent>
        <Stack spacing={2.5} divider={<Divider flexItem />}>
          {children}
        </Stack>
      </CardContent>
    </Card>
  );
}

/** A sidebar section's title, with an optional hint in a tooltip. */
export function SidebarHeading({ title, hint }: { title: string; hint?: string }) {
  return (
    <Stack direction="row" spacing={0.75} alignItems="center" sx={{ mb: 1.5 }}>
      <Typography variant="subtitle2">{title}</Typography>
      {hint && (
        <Tooltip title={hint}>
          <Box
            component="span"
            role="img"
            tabIndex={0}
            aria-label={hint}
            sx={{ display: "inline-flex", color: "text.secondary", cursor: "help" }}
          >
            <Info size={14} />
          </Box>
        </Tooltip>
      )}
    </Stack>
  );
}

/** A label on the left and its value on the right. */
export function Property({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack
      direction={{ xs: "column", sm: "row" }}
      spacing={{ xs: 0.25, sm: 2 }}
      justifyContent="space-between"
      alignItems={{ xs: "flex-start", sm: "baseline" }}
    >
      <Typography variant="body2" color="text.secondary" sx={{ flexShrink: 0 }}>
        {label}
      </Typography>
      <Box
        sx={{
          minWidth: 0,
          textAlign: { xs: "left", sm: "right" },
          wordBreak: "break-word",
          typography: "body2",
        }}
      >
        {children}
      </Box>
    </Stack>
  );
}

/** The date and time in the viewer's locale; empty for a missing or invalid date. */
export function exactDate(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

/** A relative time; its exact date is in a tooltip that keyboard focus also opens. */
export function TimeAgo({ iso }: { iso: string | undefined }) {
  const exact = exactDate(iso);
  return (
    <Tooltip title={exact}>
      <Box component="span" tabIndex={exact ? 0 : undefined}>
        {formatRelativeTime(iso)}
      </Box>
    </Tooltip>
  );
}

/** Lines of text shown before Show more. */
const CLAMPED_LINES = 3;

/**
 * Text cut to a few lines, with Show more and Show less. Short text, which fits, shows
 * no button (measured once rendered).
 */
export function ClampedText({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(false);
  const [overflows, setOverflows] = useState(false);
  const measure = (el: HTMLElement | null) => {
    if (el && !expanded) setOverflows(el.scrollHeight > el.clientHeight + 1);
  };
  return (
    <Box>
      <Typography
        ref={measure}
        variant="body2"
        sx={{
          whiteSpace: "pre-line",
          ...(expanded
            ? {}
            : {
                display: "-webkit-box",
                WebkitLineClamp: CLAMPED_LINES,
                WebkitBoxOrient: "vertical",
                overflow: "hidden",
              }),
        }}
      >
        {text}
      </Typography>
      {(overflows || expanded) && (
        <Button
          size="small"
          variant="text"
          sx={{ mt: 0.5, px: 0 }}
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? "Show less" : "Show more"}
        </Button>
      )}
    </Box>
  );
}
