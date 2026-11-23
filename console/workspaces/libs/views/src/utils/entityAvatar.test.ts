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

import { describe, expect, it } from 'vitest';
import { getAvatarInitials, getEntityAvatarColor } from './entityAvatar';

const PALETTE = ['#5B5FEE', '#0EA5E9', '#EC4899', '#F59E0B', '#10B981', '#8B5CF6'];

describe('getEntityAvatarColor', () => {
  it.each([undefined, '', '   '])('uses the first palette color for a blank name (%j)', (name) => {
    expect(getEntityAvatarColor(name)).toBe(PALETTE[0]);
  });

  it('is deterministic and ignores case and surrounding whitespace', () => {
    const color = getEntityAvatarColor('Customer Bot');
    expect(PALETTE).toContain(color);
    expect(getEntityAvatarColor('  customer bot ')).toBe(color);
  });

  it('spreads different names across the palette', () => {
    const colors = new Set(
      ['alpha', 'beta', 'gamma', 'delta', 'epsilon', 'zeta', 'eta', 'theta'].map((n) =>
        getEntityAvatarColor(n),
      ),
    );
    expect(colors.size).toBeGreaterThan(1);
  });
});

describe('getAvatarInitials', () => {
  it.each<[string | undefined, Parameters<typeof getAvatarInitials>[1], string]>([
    ['my agent', undefined, 'MY'],
    ['1-my-agent', undefined, 'MY'],
    ['agent', { maxChars: 1 }, 'A'],
    ['agent', { maxChars: 0 }, 'A'],
    ['agent', { maxChars: 3 }, 'AGE'],
    [undefined, undefined, '??'],
    ['', { fallback: '?' }, '?'],
    ['1234', { fallback: '#' }, '#'],
  ])('%j with %j -> %s', (value, options, expected) => {
    expect(getAvatarInitials(value, options)).toBe(expected);
  });
});
