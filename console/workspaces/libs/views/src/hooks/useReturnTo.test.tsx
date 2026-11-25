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

import type { ReactNode } from 'react';
import { renderHook } from '@testing-library/react';
import { MemoryRouter, type MemoryRouterProps } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { returnToState, useReturnTo } from './useReturnTo';

type Entry = NonNullable<MemoryRouterProps['initialEntries']>[number];

const renderAt = (entry: Entry) =>
  renderHook(() => useReturnTo('/fallback'), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <MemoryRouter
        initialEntries={[entry]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >{children}</MemoryRouter>
    ),
  });

describe('returnToState', () => {
  it('records the current path and query string', () => {
    expect(
      returnToState({
        pathname: '/org/a/projects',
        search: '?page=2',
        hash: '#x',
        state: null,
        key: 'k',
      }),
    ).toEqual({ from: '/org/a/projects?page=2' });
  });
});

describe('useReturnTo', () => {
  it('returns the page that opened the current one', () => {
    const { result } = renderAt({ pathname: '/edit', state: { from: '/list?q=1' } });
    expect(result.current).toBe('/list?q=1');
  });

  it.each([
    ['no router state', undefined],
    ['state without from', {}],
    ['a non-string from', { from: 42 }],
    ['a relative path', { from: 'list' }],
    ['a protocol-relative URL', { from: '//evil.example/x' }],
    ['an absolute URL', { from: 'https://evil.example/x' }],
    ['a backslash path', { from: '/\\evil.example' }],
  ])('falls back when given %s', (_label, state) => {
    const { result } = renderAt({ pathname: '/edit', state });
    expect(result.current).toBe('/fallback');
  });
});
