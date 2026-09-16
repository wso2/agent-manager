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

import { Box, Button, Card, Stack, Typography } from '@wso2/oxygen-ui';
import { BookOpen } from '@wso2/oxygen-ui-icons-react';
import type { ReactNode } from 'react';
import { docsHref, resolveDocsTarget, type DocsTarget } from './docsCatalog';

export interface DocsCalloutProps {
  /** The documentation page this callout points at. */
  docs: DocsTarget;
  /** Overrides the entry's own title as the callout heading. */
  title?: string;
  /** Overrides the entry's summary as the callout body. */
  description?: ReactNode;
  /** Text on the action. Defaults to "Read the docs". */
  actionLabel?: string;
}

const CARD_SX = {
  p: 2,
  // A tinted rail rather than a full alert: this is an offer of help, not a
  // condition the user has to clear.
  borderColor: 'divider',
  bgcolor: 'action.hover',
} as const;

const ICON_SX = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: 36,
  height: 36,
  borderRadius: '50%',
  bgcolor: 'background.paper',
  color: 'primary.main',
  flexShrink: 0,
} as const;

const BODY_SX = { flex: 1, minWidth: 0 } as const;
const ACTION_SX = { flexShrink: 0, alignSelf: 'center' } as const;

/**
 * A standing pointer into the documentation, for the places a bare link isn't
 * enough: an empty state a first-time user lands on with nothing to act on, or
 * a screen whose concepts have to be understood before its form makes sense.
 *
 * Renders nothing without a configured `docsUrl`.
 */
export function DocsCallout({
  docs,
  title,
  description,
  actionLabel = 'Read the docs',
}: DocsCalloutProps) {
  const href = docsHref(docs);
  if (!href) return null;

  const { entry } = resolveDocsTarget(docs);

  return (
    <Card variant="outlined" sx={CARD_SX}>
      <Stack direction="row" spacing={2} alignItems="flex-start">
        <Box sx={ICON_SX}>
          <BookOpen size={18} />
        </Box>
        <Box sx={BODY_SX}>
          <Typography variant="subtitle2" gutterBottom>
            {title ?? entry.title}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {description ?? entry.summary}
          </Typography>
        </Box>
        <Button
          component="a"
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          size="small"
          variant="outlined"
          color="primary"
          sx={ACTION_SX}
        >
          {actionLabel}
        </Button>
      </Stack>
    </Card>
  );
}

export default DocsCallout;
