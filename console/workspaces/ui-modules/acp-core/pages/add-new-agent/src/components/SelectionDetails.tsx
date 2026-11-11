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

import type { ReactNode } from "react";
import { Card, Fade, Stack, Typography } from "@wso2/oxygen-ui";

interface SelectionDetailsProps {
  /** Shown while the option above it is the selected one. */
  open: boolean;
  /** Names the option these settings belong to ("Python configuration"). */
  title: string;
  children: ReactNode;
}

/**
 * The settings that only exist because of the card selected above, held in their own
 * outlined card so it is clear they belong to that choice and not to the form around it.
 */
export function SelectionDetails({ open, title, children }: SelectionDetailsProps) {
  // Not rendered at all when closed: a hidden sibling would still take a gap from the
  // parent stack, so the spacing under the cards would change with the selection.
  if (!open) return null;
  return (
    <Fade in>
      <Card variant="outlined" sx={{ p: 2.5 }}>
        <Stack spacing={2}>
          <Typography variant="subtitle2" color="text.secondary">
            {title}
          </Typography>
          {children}
        </Stack>
      </Card>
    </Fade>
  );
}
