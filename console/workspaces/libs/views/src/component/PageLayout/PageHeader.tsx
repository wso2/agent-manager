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

import { Box, Card, PageTitle, Skeleton, Stack } from '@wso2/oxygen-ui';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { PageDocs, type DocsTarget } from '../Docs';
import { resolveEntityAvatarProps, type EntityAvatarProps } from '../EntityAvatar';
import { PageMetaList } from '../PageMeta';

/**
 * `plain` — title sitting directly on the page background (list pages, wizards).
 * `card` — title on its own outlined surface, the treatment entity detail pages
 * use (agent, project, LLM provider, MCP server) so the identity of the thing
 * being viewed reads as a distinct object above its content.
 */
export type PageHeaderVariant = 'plain' | 'card';

// Hoisted so this header — which every page renders, and which re-renders with
// whatever page state sits above it — doesn't hand emotion new style objects to
// serialize on each pass.
const PLAIN_TITLE_SX = { mb: 2 } as const;
const CARD_TITLE_SX = { mb: 0, flex: 1, minWidth: 0 } as const;
const CARD_WRAPPER_SX = { mb: 2, display: 'flex', flexDirection: 'column', gap: 1 } as const;
const CARD_SX = { p: 2.5 } as const;
const CARD_ROW_SX = { display: 'flex', alignItems: 'flex-start', gap: 2 } as const;
const CARD_ACTIONS_SX = { display: 'flex', alignItems: 'center', gap: 1, flexShrink: 0 } as const;
const CARD_SECONDARY_ACTIONS_SX = {
  display: 'flex',
  justifyContent: 'flex-end',
  gap: 1,
  mt: 1,
} as const;
const TITLE_SX = { display: 'inline-flex', alignItems: 'center', width: '100%' } as const;
const TITLE_TEXT_SX = {
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
  minWidth: 0,
  maxWidth: '70%',
} as const;
const TITLE_TAIL_SX = {
  display: 'inline-flex',
  alignItems: 'center',
  verticalAlign: 'middle',
  ml: 1,
  flexGrow: 1,
  minWidth: 0,
} as const;

export interface PageHeaderProps {
  title?: string;
  /** Renders a back button above the title. */
  backHref?: string;
  backLabel?: string;
  /** Lead line under the title. */
  description?: ReactNode;
  /** Inline content right of the title — version/type chips, inline edit affordances. */
  titleTail?: ReactNode;
  /**
   * Metadata rows stacked under the title, composed from `PageMetaItem`
   * (created at, owner, tags, ...).
   */
  meta?: ReactNode;
  /**
   * Avatar beside the title. Defaults to a `primary` letter tile built from
   * `title`; pass a spec to override (logo `src`, brand `color`, custom
   * `children`), or `null` to drop the avatar entirely.
   */
  avatar?: EntityAvatarProps | null;
  /** Primary actions — top-right of the header. */
  actions?: ReactNode;
  /**
   * The documentation behind this screen — the first entry is its primary
   * reading, the rest the related concepts and guides. Rendered as a panel
   * opened from the header, so every page carries a route into the docs
   * without each one having to lay one out.
   */
  docs?: DocsTarget | DocsTarget[];
  /**
   * Low-emphasis or destructive actions (delete, ...), pinned to the bottom-right
   * so they stay clear of the primary action. `card` variant only.
   */
  secondaryActions?: ReactNode;
  variant?: PageHeaderVariant;
  isLoading?: boolean;
}

export function PageHeader({
  title,
  backHref,
  backLabel,
  description,
  titleTail,
  meta,
  avatar,
  actions,
  docs,
  secondaryActions,
  variant = 'plain',
  isLoading = false,
}: PageHeaderProps) {
  const isCard = variant === 'card';
  // The card header gives the avatar more room than the plain one.
  const size = isCard ? 64 : 55;
  const avatarProps =
    avatar === null
      ? null
      : resolveEntityAvatarProps({
          name: title,
          color: 'primary.main',
          shape: 'circular',
          size,
          ...avatar,
        });

  const backButton = backHref ? (
    <PageTitle.BackButton component={<Link to={backHref} />}>
      {backLabel || 'Back'}
    </PageTitle.BackButton>
  ) : null;

  const actionsContent =
    actions &&
    (isLoading ? (
      <Stack direction="row" spacing={1}>
        <Skeleton variant="circular" width={32} height={32} />
        <Skeleton variant="circular" width={32} height={32} />
      </Stack>
    ) : (
      actions
    ));

  // The docs panel is static, so unlike the actions it renders while the page
  // is still loading — that's precisely when someone may want to read what the
  // screen is for.
  const docsLink = docs ? <PageDocs docs={docs} /> : null;

  const headerActions =
    docsLink || actionsContent ? (
      <Stack direction="row" spacing={1} alignItems="center">
        {docsLink}
        {actionsContent}
      </Stack>
    ) : null;

  /**
   * Oxygen's PageTitle picks its avatar/actions/back-button slots out of
   * `children` by element type, so those have to be inlined here rather than
   * extracted into components of our own. The card variant places the back
   * button and the actions itself, so it leaves those two slots empty.
   */
  const pageTitle = (
    <PageTitle sx={isCard ? CARD_TITLE_SX : PLAIN_TITLE_SX}>
      {!isCard && backButton}
      {avatarProps &&
        (isLoading ? (
          <PageTitle.Avatar
            variant={avatarProps.variant}
            sx={{ width: size, height: size, bgcolor: 'transparent' }}
          >
            <Skeleton
              variant={avatarProps.variant === 'circular' ? 'circular' : 'rounded'}
              width={size}
              height={size}
            />
          </PageTitle.Avatar>
        ) : (
          <PageTitle.Avatar {...avatarProps} />
        ))}
      <PageTitle.Header>
        {isLoading ? (
          <Skeleton variant="text" width={200} height={32} />
        ) : (
          <Box component="span" sx={TITLE_SX}>
            <Box component="span" sx={TITLE_TEXT_SX}>
              {title}
            </Box>
            {titleTail && (
              <Box component="span" sx={TITLE_TAIL_SX}>
                {titleTail}
              </Box>
            )}
          </Box>
        )}
      </PageTitle.Header>
      {(description || meta) && (
        <PageTitle.SubHeader component="div">
          {isLoading ? (
            <Skeleton variant="text" width={300} height={20} />
          ) : (
            <PageMetaList>
              {description}
              {meta}
            </PageMetaList>
          )}
        </PageTitle.SubHeader>
      )}
      {!isCard && headerActions && (
        <PageTitle.Actions>{headerActions}</PageTitle.Actions>
      )}
    </PageTitle>
  );

  if (!isCard) return pageTitle;

  // The card lays out the title block and the actions itself — PageTitle centres
  // its own actions column against the whole block, which reads as misplaced
  // next to several rows of metadata, and its internal slot classes only exist
  // in dev builds so they can't be restyled from here.
  return (
    <Box sx={CARD_WRAPPER_SX}>
      {backButton}
      <Card variant="outlined" sx={CARD_SX}>
        <Box sx={CARD_ROW_SX}>
          {pageTitle}
          {headerActions && <Box sx={CARD_ACTIONS_SX}>{headerActions}</Box>}
        </Box>
        {secondaryActions && !isLoading && (
          <Box sx={CARD_SECONDARY_ACTIONS_SX}>{secondaryActions}</Box>
        )}
      </Card>
    </Box>
  );
}
