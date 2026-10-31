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

import { Box, PageTitle, Skeleton, Stack } from '@wso2/oxygen-ui';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { resolveEntityAvatarProps, type EntityAvatarProps } from '../EntityAvatar';
import { PageMetaList } from '../PageMeta';

// Hoisted so this header — which every page renders, and which re-renders with
// whatever page state sits above it — doesn't hand emotion new style objects to
// serialize on each pass.
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
  /** Page actions — top-right of the header, primary action last. */
  actions?: ReactNode;
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
  isLoading = false,
}: PageHeaderProps) {
  const avatarProps =
    avatar === null
      ? null
      : resolveEntityAvatarProps({
          name: title,
          color: 'primary.main',
          shape: 'circular',
          ...avatar,
        });

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

  /**
   * Oxygen's PageTitle picks its avatar/actions/back-button slots out of
   * `children` by element type, so those have to be inlined here rather than
   * extracted into components of our own.
   */
  return (
    <PageTitle>
      {backHref && (
        <PageTitle.BackButton component={<Link to={backHref} />}>
          {backLabel || 'Back'}
        </PageTitle.BackButton>
      )}
      {avatarProps &&
        (isLoading ? (
          <PageTitle.Avatar
            variant={avatarProps.variant}
            sx={{ bgcolor: 'transparent' }}
          >
            <Skeleton
              variant={avatarProps.variant === 'circular' ? 'circular' : 'rounded'}
              width="100%"
              height="100%"
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
      {actionsContent && <PageTitle.Actions>{actionsContent}</PageTitle.Actions>}
    </PageTitle>
  );
}
