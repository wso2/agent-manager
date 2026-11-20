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
import { Box, Chip, Paper, Stack, Typography } from "@wso2/oxygen-ui";
import { TriangleAlert } from "@wso2/oxygen-ui-icons-react";

// Tile pieces shared by the registered and unregistered agent overview pages.

/**
 * One control, as a small titled tile like the agent's own overview page uses
 * ("AGENT ID", "EVALUATIONS"). Tiles in a row share a height.
 */
export function Tile({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Paper variant="outlined" sx={{ p: 2, height: "100%" }}>
      <Typography
        variant="overline"
        color="text.secondary"
        component="h3"
        sx={{ display: "block", lineHeight: 1.5, mb: 1 }}
      >
        {title}
      </Typography>
      {children}
    </Paper>
  );
}

/** A gap worth noticing: text with a warning icon; the colour stays on the icon. */
export function Gap({ children }: { children: ReactNode }) {
  return (
    <Stack direction="row" spacing={0.75} alignItems="center">
      <Box component="span" sx={{ color: "warning.main", display: "inline-flex" }}>
        <TriangleAlert size={16} aria-hidden />
      </Box>
      <Typography variant="body2">{children}</Typography>
    </Stack>
  );
}

/** A wrapping row of small chips. */
export function ChipRow({ labels }: { labels: string[] }) {
  return (
    <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
      {labels.map((label) => (
        <Chip key={label} size="small" variant="outlined" label={label} />
      ))}
    </Stack>
  );
}
