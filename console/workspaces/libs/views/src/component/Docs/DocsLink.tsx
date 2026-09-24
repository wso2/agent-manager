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

import { Box, Link } from '@wso2/oxygen-ui';
import { ExternalLink } from '@wso2/oxygen-ui-icons-react';
import type { ReactNode } from 'react';
import { docsHref, resolveDocsTarget, type DocsTarget } from './docsCatalog';

export interface DocsLinkProps {
  /** The documentation page this link opens. */
  docs: DocsTarget;
  /** Link text. Defaults to "Learn more". */
  children?: ReactNode;
  /** Hides the trailing external-link glyph, for links inside running prose. */
  hideIcon?: boolean;
  variant?: 'body1' | 'body2' | 'caption';
}

const LINK_SX = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 0.5,
  fontWeight: 500,
} as const;

const ICON_SX = { display: 'inline-flex', flexShrink: 0 } as const;

/**
 * An inline "learn more" link, for explaining one field, section, or empty
 * state rather than a whole page — where {@link DocsButton} belongs instead.
 *
 * Renders its children as plain text when no `docsUrl` is configured, so the
 * sentence around it still reads correctly.
 */
export function DocsLink({
  docs,
  children,
  hideIcon = false,
  variant = 'body2',
}: DocsLinkProps) {
  const href = docsHref(docs);
  const { entry } = resolveDocsTarget(docs);
  const content = children ?? 'Learn more';

  if (!href) return <>{content}</>;

  return (
    <Link
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      variant={variant}
      underline="hover"
      title={entry.summary}
      sx={LINK_SX}
    >
      {content}
      {!hideIcon && (
        <Box component="span" sx={ICON_SX}>
          <ExternalLink size={13} />
        </Box>
      )}
    </Link>
  );
}

export default DocsLink;
