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
import { customDateToExpiresIn, DEFAULT_TOKEN_EXPIRY, TOKEN_EXPIRY_PRESETS } from './tokenExpiryPresets';

const NOW = new Date('2026-01-01T00:00:00Z');
const hoursFromNow = (h: number) => new Date(NOW.getTime() + h * 3_600_000);

describe('customDateToExpiresIn', () => {
  it('returns whole hours from now as a Go duration, rounding up', () => {
    expect(customDateToExpiresIn(hoursFromNow(24), NOW)).toBe('24h');
    expect(customDateToExpiresIn(hoursFromNow(1.2), NOW)).toBe('2h');
  });

  it.each([0, -1])('rejects a date that is not in the future (%i h)', (h) => {
    expect(() => customDateToExpiresIn(hoursFromNow(h), NOW)).toThrow(
      'Expiry date must be in the future',
    );
  });

  it('accepts exactly ten years and rejects anything beyond', () => {
    expect(customDateToExpiresIn(hoursFromNow(87_600), NOW)).toBe('87600h');
    expect(() => customDateToExpiresIn(hoursFromNow(87_601), NOW)).toThrow(
      'Expiry cannot be more than 10 years from now',
    );
  });

  it('defaults to a preset that exists', () => {
    expect(TOKEN_EXPIRY_PRESETS.map((p) => p.value)).toContain(DEFAULT_TOKEN_EXPIRY);
  });
});
