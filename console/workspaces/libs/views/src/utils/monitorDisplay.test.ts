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
import { formatTraceWindow } from './formatTraceWindow';
import { scoreColor } from './scoreColor';

describe('scoreColor', () => {
  it.each([
    [1, '#22c55e'],
    [0.8, '#22c55e'],
    [0.79, '#f59e0b'],
    [0.6, '#f59e0b'],
    [0.59, '#ef4444'],
    [0, '#ef4444'],
  ])('colours a score of %s as %s', (score, colour) => {
    expect(scoreColor(score)).toBe(colour);
  });
});

describe('formatTraceWindow', () => {
  it('joins both ends of the window with an en dash in the local format', () => {
    const start = '2026-03-01T10:00:00Z';
    const end = '2026-03-03T14:00:00Z';
    const opts: Intl.DateTimeFormatOptions = {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    };
    expect(formatTraceWindow(start, end)).toBe(
      `${new Date(start).toLocaleString(undefined, opts)} – ${new Date(end).toLocaleString(undefined, opts)}`,
    );
  });
});
