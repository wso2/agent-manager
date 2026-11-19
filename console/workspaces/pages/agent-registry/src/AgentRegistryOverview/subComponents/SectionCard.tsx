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
import {
  Box,
  Card,
  CardContent,
  CardHeader,
  Divider,
  Grid,
  Typography,
} from "@wso2/oxygen-ui";

/** Outlined, titled card for one section of an overview page. */
export function SectionCard({
  title,
  subheader,
  children,
}: {
  title: string;
  /** One line under the title, e.g. where the data comes from. */
  subheader?: string;
  children: ReactNode;
}) {
  return (
    <Card variant="outlined">
      <CardHeader
        title={title}
        subheader={subheader}
        slotProps={{ subheader: { variant: "body2", color: "text.secondary" } }}
      />
      <Divider />
      <CardContent>{children}</CardContent>
    </Card>
  );
}

/** A grid of label/value pairs. */
export function SummaryGrid({ children }: { children: ReactNode }) {
  return (
    <Grid container spacing={2.5}>
      {children}
    </Grid>
  );
}

/** One label and its value in a SummaryGrid; `wide` takes the full row. */
export function SummaryItem({
  label,
  wide = false,
  children,
}: {
  label: string;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <Grid size={wide ? { xs: 12 } : { xs: 12, sm: 6, lg: 4 }}>
      <Typography variant="caption" color="text.secondary" component="div">
        {label}
      </Typography>
      <Box sx={{ mt: 0.5, minWidth: 0 }}>{children}</Box>
    </Grid>
  );
}

/** One label and its value, for layouts that place fields themselves. */
export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Box>
      <Typography variant="caption" color="text.secondary" component="div">
        {label}
      </Typography>
      <Box sx={{ mt: 0.5, minWidth: 0 }}>{children}</Box>
    </Box>
  );
}

/** Plain value text; "None" (greyed) when empty. */
export function ValueText({ children }: { children?: ReactNode }) {
  const empty = children === undefined || children === null || children === "";
  return (
    <Typography
      variant="body2"
      color={empty ? "text.secondary" : "text.primary"}
      sx={{ overflowWrap: "anywhere" }}
    >
      {empty ? "None" : children}
    </Typography>
  );
}
