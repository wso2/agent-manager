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

import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useTimeRangeParams } from './useTimeRangeParams';

const START = '2026-03-01T10:00:00.000Z';
const END = '2026-03-03T14:00:00.000Z';

function setup(query: string) {
  const setSearchParams = vi.fn();
  const { result } = renderHook(() =>
    useTimeRangeParams(new URLSearchParams(query), setSearchParams),
  );
  return { result, setSearchParams };
}

describe('useTimeRangeParams', () => {
  it('reads a valid custom range from the URL', () => {
    const { result } = setup(`startTime=${START}&endTime=${END}`);
    expect(result.current).toMatchObject({
      customStartTime: START,
      customEndTime: END,
      hasCustomRange: true,
    });
  });

  it.each([
    ['no range', 'timeRange=1h'],
    ['only a start', `startTime=${START}`],
    ['an unparseable end', `startTime=${START}&endTime=soon`],
    ['a start after the end', `startTime=${END}&endTime=${START}`],
  ])('ignores %s', (_, query) => {
    const { result } = setup(query);
    expect(result.current).toMatchObject({
      customStartTime: undefined,
      customEndTime: undefined,
      hasCustomRange: false,
    });
  });

  it('applies a custom range in place of a preset, keeping other params', () => {
    const { result, setSearchParams } = setup('timeRange=1h&agent=a1');
    act(() => result.current.handleCustomRangeApply(START, END));
    const next: URLSearchParams = setSearchParams.mock.calls[0][0];
    expect(Object.fromEntries(next)).toEqual({ agent: 'a1', startTime: START, endTime: END });
  });

  it('clears the custom range, keeping other params', () => {
    const { result, setSearchParams } = setup(`startTime=${START}&endTime=${END}&agent=a1`);
    act(() => result.current.handleCustomRangeClear());
    const next: URLSearchParams = setSearchParams.mock.calls[0][0];
    expect(Object.fromEntries(next)).toEqual({ agent: 'a1' });
  });
});
