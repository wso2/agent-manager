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

import { Avatar, type AvatarProps, type SxProps, type Theme } from '@wso2/oxygen-ui';
import type { ReactNode } from 'react';
import { getAvatarInitials, getEntityAvatarColor } from '../../utils/entityAvatar';

export type EntityAvatarShape = 'rounded' | 'circular' | 'square';

export interface EntityAvatarProps {
  /** Entity name — drives the fallback letter and, unless `color` is set, the color. */
  name?: string;
  /** Logo/image URL. Falls back to `children`, then to the name's initial. */
  src?: string;
  /** Custom content (an icon, an inline logo) shown instead of the initial. */
  children?: ReactNode;
  /**
   * Background color: a theme token (`primary.main`) or a CSS color. Defaults to
   * a deterministic color derived from `name`. Use `transparent` for logos that
   * carry their own background. A palette `.main` token (`primary.main`,
   * `success.main`, …) renders as Oxygen's `light` avatar in that color — a
   * tinted background with the color on the content.
   */
  color?: string;
  /** `rounded` (default) reads as an entity tile; `circular` as a person. */
  shape?: EntityAvatarShape;
  /** Edge length in px. Left unset, the host slot's own sizing applies. */
  size?: number;
  alt?: string;
  sx?: SxProps<Theme>;
}

/** Palette groups that ship a matching `contrastText`. */
const CONTRAST_GROUPS = ['primary', 'secondary', 'error', 'warning', 'info', 'success'];

type LightAvatarColor = NonNullable<AvatarProps['color']>;

/** Palette groups the Oxygen `light` avatar variant can be tinted with. */
const LIGHT_VARIANT_COLORS: readonly LightAvatarColor[] = [
  'primary', 'secondary', 'success', 'error', 'info', 'warning',
];

/** The `light` variant color for a `<group>.main` token, or undefined. */
function lightVariantColor(color: string | undefined): LightAvatarColor | undefined {
  const match = color?.match(/^(\w+)\.main$/);
  return LIGHT_VARIANT_COLORS.find((c) => c === match?.[1]);
}

/** Readable foreground for `color`, or undefined to inherit. */
function contrastFor(color: string): string | ((theme: Theme) => string) | undefined {
  if (color === 'transparent') return undefined;

  if (color.includes('.')) {
    const [group] = color.split('.');
    return CONTRAST_GROUPS.includes(group) ? `${group}.contrastText` : undefined;
  }

  return (theme: Theme) => {
    try {
      return theme.palette.getContrastText(color);
    } catch {
      // Named CSS colors and anything else MUI can't decompose — leave the
      // text color to be inherited rather than blowing up the whole page.
      return 'inherit';
    }
  };
}

/**
 * Maps an {@link EntityAvatarProps} spec onto plain MUI `Avatar` props, so the
 * same visual treatment can be applied to an Avatar we render ourselves
 * ({@link EntityAvatar}) *and* to slots owned by other components — e.g.
 * `PageTitle.Avatar`, which the page header has to render inline for Oxygen's
 * PageTitle to detect it as its avatar slot.
 */
export function resolveEntityAvatarProps({
  name,
  src,
  children,
  color,
  shape = 'rounded',
  size,
  alt,
  sx,
}: EntityAvatarProps): AvatarProps {
  const bgcolor = color ?? (src ? 'transparent' : getEntityAvatarColor(name));
  // Theme-colored letter/icon tiles use Oxygen's `light` variant; logos and
  // name-derived colors stay solid.
  const light = src ? undefined : lightVariantColor(color);

  return {
    src,
    alt: alt ?? name,
    variant: light ? 'light' : shape,
    ...(light && { color: light }),
    children: children ?? getAvatarInitials(name, { maxChars: 1, fallback: '?' }),
    sx: {
      // Omitted when `size` is unset so slots that size their own avatar —
      // Oxygen's `PageTitle.Avatar` — keep their dimensions.
      ...(size !== undefined && {
        width: size,
        height: size,
        fontSize: Math.round(size * 0.42),
      }),
      fontWeight: 600,
      ...(!light && { bgcolor, color: contrastFor(bgcolor) }),
      // `light` is circular by default; keep the requested shape.
      ...(light && shape === 'square' && { borderRadius: 0 }),
      ...(shape === 'rounded' && { borderRadius: (size ?? 40) >= 48 ? 2 : 1.5 }),
      // Logos are rarely square — show them whole instead of cropping to fill.
      ...(src && { '& img': { objectFit: 'contain' } }),
      ...sx,
    },
  };
}

/**
 * Letter/logo avatar for a named entity (agent, project, provider, user). Given
 * no `color`, it derives one deterministically from the name, so the same entity
 * looks the same wherever it appears — lists, cards, and page headers.
 */
export function EntityAvatar(props: EntityAvatarProps) {
  return <Avatar {...resolveEntityAvatarProps({ ...props, size: props.size ?? 40 })} />;
}
