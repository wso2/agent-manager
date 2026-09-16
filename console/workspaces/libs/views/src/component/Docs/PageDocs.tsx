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

import {
  Box,
  Button,
  ButtonBase,
  Divider,
  Popover,
  Stack,
  Typography,
} from '@wso2/oxygen-ui';
import { BookOpen, ChevronDown, ExternalLink, FileText } from '@wso2/oxygen-ui-icons-react';
import { useState, type MouseEvent } from 'react';
import {
  docsHref,
  docsHomeHref,
  resolveDocsTarget,
  type DocsTarget,
} from './docsCatalog';

export interface PageDocsProps {
  /**
   * The documentation for this screen. The first entry is the page's primary
   * reading; the rest are the related concepts and guides worth knowing about.
   */
  docs: DocsTarget | DocsTarget[];
  /** Label on the trigger. Defaults to "Documentation". */
  label?: string;
  /** Heading inside the panel. Defaults to "Documentation". */
  title?: string;
  /** Lead line under the panel heading. */
  subtitle?: string;
}

const TRIGGER_SX = {
  color: 'text.secondary',
  flexShrink: 0,
  whiteSpace: 'nowrap',
  '&:hover': { color: 'primary.main', bgcolor: 'action.hover' },
} as const;

const PAPER_SX = {
  width: 400,
  maxWidth: '92vw',
  mt: 1,
  borderRadius: 2,
  overflow: 'hidden',
} as const;

const HEADER_SX = { px: 2, pt: 1.75, pb: 1.5 } as const;
const LIST_SX = { py: 0.5, maxHeight: 420, overflowY: 'auto' } as const;

const ITEM_SX = {
  display: 'block',
  width: '100%',
  textAlign: 'left',
  px: 2,
  py: 1.25,
  transition: 'background-color 120ms ease',
  '&:hover': { bgcolor: 'action.hover' },
  '&:focus-visible': { bgcolor: 'action.hover', outline: 'none' },
} as const;

const ITEM_ICON_SX = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: 30,
  height: 30,
  borderRadius: 1,
  flexShrink: 0,
  mt: 0.25,
} as const;

const PRIMARY_ICON_SX = {
  ...ITEM_ICON_SX,
  bgcolor: 'primary.main',
  color: 'primary.contrastText',
} as const;

const RELATED_ICON_SX = {
  ...ITEM_ICON_SX,
  bgcolor: 'action.selected',
  color: 'text.secondary',
} as const;

const ITEM_BODY_SX = { flex: 1, minWidth: 0 } as const;

const SUMMARY_SX = {
  display: '-webkit-box',
  WebkitLineClamp: 2,
  WebkitBoxOrient: 'vertical',
  overflow: 'hidden',
} as const;

const EXTERNAL_SX = { color: 'text.disabled', flexShrink: 0, mt: 0.75 } as const;
const SECTION_LABEL_SX = { px: 2, pt: 1.25, pb: 0.5, display: 'block' } as const;
const FOOTER_SX = { px: 1, py: 0.75 } as const;

interface DocsMenuItemProps {
  href: string;
  title: string;
  summary: string;
  isPrimary: boolean;
  onNavigate: () => void;
}

function DocsMenuItem({ href, title, summary, isPrimary, onNavigate }: DocsMenuItemProps) {
  return (
    <ButtonBase
      component="a"
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      onClick={onNavigate}
      sx={ITEM_SX}
    >
      <Stack direction="row" spacing={1.5} alignItems="flex-start">
        <Box sx={isPrimary ? PRIMARY_ICON_SX : RELATED_ICON_SX}>
          {isPrimary ? <BookOpen size={16} /> : <FileText size={15} />}
        </Box>
        <Box sx={ITEM_BODY_SX}>
          <Typography variant="body2" fontWeight={600} color="text.primary">
            {title}
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={SUMMARY_SX}>
            {summary}
          </Typography>
        </Box>
        <Box sx={EXTERNAL_SX}>
          <ExternalLink size={13} />
        </Box>
      </Stack>
    </ButtonBase>
  );
}

/**
 * The per-page documentation panel.
 *
 * Every screen in the console names the documentation that explains it, and
 * this is where that lands: a trigger in the page header opening a panel with
 * the page's primary reading, the related concepts and guides behind it, and a
 * way into the documentation site as a whole. A bare link would answer "where
 * are the docs"; this answers "what should I read to understand this screen",
 * which is the question someone on an unfamiliar page actually has.
 *
 * Renders nothing when the deployment has no `docsUrl` configured.
 */
export function PageDocs({
  docs,
  label = 'Documentation',
  title = 'Documentation',
  subtitle = 'Guides and concepts behind this page.',
}: PageDocsProps) {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);

  const targets = Array.isArray(docs) ? docs : [docs];
  const entries = targets
    .map((target) => {
      const { entry, label: itemLabel } = resolveDocsTarget(target);
      return { href: docsHref(target), entry, itemLabel };
    })
    .filter((item): item is typeof item & { href: string } => Boolean(item.href));

  const homeHref = docsHomeHref();
  if (entries.length === 0 || !homeHref) return null;

  const [primary, ...related] = entries;

  return (
    <>
      <Button
        size="small"
        variant="text"
        color="inherit"
        startIcon={<BookOpen size={16} />}
        endIcon={<ChevronDown size={14} />}
        sx={TRIGGER_SX}
        aria-haspopup="dialog"
        aria-expanded={anchorEl ? true : undefined}
        onClick={(event: MouseEvent<HTMLElement>) => setAnchorEl(event.currentTarget)}
      >
        {label}
      </Button>

      <Popover
        open={Boolean(anchorEl)}
        anchorEl={anchorEl}
        onClose={() => setAnchorEl(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        slotProps={{ paper: { variant: 'outlined', sx: PAPER_SX } }}
      >
        <Box sx={HEADER_SX}>
          <Typography variant="subtitle2">{title}</Typography>
          <Typography variant="caption" color="text.secondary">
            {subtitle}
          </Typography>
        </Box>
        <Divider />

        <Box sx={LIST_SX}>
          <DocsMenuItem
            href={primary.href}
            title={primary.entry.title}
            summary={primary.entry.summary}
            isPrimary
            onNavigate={() => setAnchorEl(null)}
          />

          {related.length > 0 && (
            <>
              <Typography variant="overline" color="text.disabled" sx={SECTION_LABEL_SX}>
                Related
              </Typography>
              {related.map((item) => (
                <DocsMenuItem
                  key={item.entry.path}
                  href={item.href}
                  title={item.entry.title}
                  summary={item.entry.summary}
                  isPrimary={false}
                  onNavigate={() => setAnchorEl(null)}
                />
              ))}
            </>
          )}
        </Box>

        <Divider />
        <Box sx={FOOTER_SX}>
          <Button
            component="a"
            href={homeHref}
            target="_blank"
            rel="noopener noreferrer"
            size="small"
            fullWidth
            color="primary"
            endIcon={<ExternalLink size={13} />}
            onClick={() => setAnchorEl(null)}
          >
            Browse all documentation
          </Button>
        </Box>
      </Popover>
    </>
  );
}

export default PageDocs;
