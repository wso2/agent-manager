/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act, render, screen } from '@testing-library/react';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import {
  type SearchParamsMutation,
  useReplaceSearchParams,
} from './useReplaceSearchParams';

let update: (mutate: SearchParamsMutation) => void = () => undefined;
let navigate: ReturnType<typeof useNavigate> = () => undefined;

function Probe() {
  const [, replaceSearchParams] = useReplaceSearchParams();
  update = replaceSearchParams;
  navigate = useNavigate();
  return <output data-testid="search">{useLocation().search}</output>;
}

const search = () => screen.getByTestId('search').textContent;

const renderProbe = () =>
  render(
    <MemoryRouter
      initialEntries={['/list?a=1']}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Probe />
    </MemoryRouter>,
  );

describe('useReplaceSearchParams', () => {
  it('replaces instead of pushing and composes writes in one tick', () => {
    renderProbe();

    act(() => {
      update((p) => p.set('b', '2'));
      update((p) => p.set('c', '3'));
    });
    expect(search()).toBe('?a=1&b=2&c=3');

    act(() => navigate(-1));
    expect(search()).toBe('?a=1&b=2&c=3');
  });

  it('starts the next write from a URL changed by outside navigation', () => {
    renderProbe();

    act(() => update((p) => p.set('b', '2')));
    act(() => navigate('/list?x=9'));
    act(() => update((p) => p.set('y', '1')));

    expect(search()).toBe('?x=9&y=1');
  });
});
