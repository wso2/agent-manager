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

import { screen } from '@testing-library/react';
import { createTheme } from '@wso2/oxygen-ui';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { getEntityAvatarColor } from '../../utils/entityAvatar';
import { EntityAvatar, resolveEntityAvatarProps } from './EntityAvatar';

type Sx = Record<string, unknown>;
const sxOf = (props: ReturnType<typeof resolveEntityAvatarProps>) => props.sx as Sx;

describe('resolveEntityAvatarProps', () => {
  it('derives the initial, alt text and a name-based color by default', () => {
    const props = resolveEntityAvatarProps({ name: 'support bot' });
    expect(props.children).toBe('S');
    expect(props.alt).toBe('support bot');
    expect(props.variant).toBe('rounded');
    expect(sxOf(props).bgcolor).toBe(getEntityAvatarColor('support bot'));
    // Unsized avatars leave dimensions to the host slot.
    expect(sxOf(props).width).toBeUndefined();
    expect(sxOf(props).borderRadius).toBe(1.5);
  });

  it('falls back to "?" with no usable name', () => {
    expect(resolveEntityAvatarProps({}).children).toBe('?');
    expect(resolveEntityAvatarProps({ name: '123' }).children).toBe('?');
  });

  it('sizes the avatar and rounds large tiles more', () => {
    const sx = sxOf(resolveEntityAvatarProps({ name: 'a', size: 50 }));
    expect(sx).toMatchObject({ width: 50, height: 50, fontSize: 21, borderRadius: 2 });
  });

  it('uses a transparent, uncropped background for logos and no rounding for circles', () => {
    const props = resolveEntityAvatarProps({ name: 'a', src: 'logo.png', shape: 'circular', alt: 'Logo' });
    const sx = sxOf(props);
    expect(props.src).toBe('logo.png');
    expect(props.alt).toBe('Logo');
    expect(sx.bgcolor).toBe('transparent');
    expect(sx.color).toBeUndefined();
    expect(sx['& img']).toEqual({ objectFit: 'contain' });
    expect(sx.borderRadius).toBeUndefined();
  });

  it.each([
    ['primary.main', 'primary.contrastText'],
    ['error.light', 'error.contrastText'],
    ['grey.500', undefined],
  ])('picks the contrast text for theme token %s', (color, expected) => {
    expect(sxOf(resolveEntityAvatarProps({ name: 'a', color })).color).toBe(expected);
  });

  it('computes contrast for CSS colors and inherits for ones MUI cannot parse', () => {
    const theme = createTheme();
    const forColor = (color: string) =>
      (sxOf(resolveEntityAvatarProps({ name: 'a', color })).color as (t: typeof theme) => string)(theme);
    expect(forColor('#000000')).toBe(theme.palette.getContrastText('#000000'));
    expect(forColor('rebeccapurple')).toBe('inherit');
  });

  it('lets caller sx and children override the defaults', () => {
    const props = resolveEntityAvatarProps({ name: 'a', children: 'X', sx: { fontWeight: 400 } });
    expect(props.children).toBe('X');
    expect(sxOf(props).fontWeight).toBe(400);
  });
});

describe('EntityAvatar', () => {
  it('renders the initial in a 40px avatar by default', () => {
    renderWithProviders(<EntityAvatar name="Gateway" />);
    const avatar = screen.getByText('G');
    expect(avatar).toHaveStyle({ width: '40px', height: '40px' });
  });

  it('renders the logo image when given src', () => {
    renderWithProviders(<EntityAvatar name="OpenAI" src="https://example.com/logo.png" size={24} />);
    expect(screen.getByRole('img', { name: 'OpenAI' })).toHaveAttribute(
      'src',
      'https://example.com/logo.png',
    );
  });
});
