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

import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { JSONView } from './JSONView';

describe('JSONView', () => {
  it('shows text that is not JSON verbatim', () => {
    renderWithProviders(<JSONView json="not { json" />);
    expect(screen.getByText('not { json')).toBeInTheDocument();
  });

  it('renders primitives with their JSON spelling', () => {
    renderWithProviders(
      <JSONView json={JSON.stringify({ s: 'text', n: 4, b: false, z: null })} />,
    );
    expect(screen.getByText('s:')).toBeInTheDocument();
    expect(screen.getByText('"text"')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
    expect(screen.getByText('false')).toBeInTheDocument();
    expect(screen.getByText('null')).toBeInTheDocument();
  });

  it('renders a top-level primitive', () => {
    renderWithProviders(<JSONView json="42" />);
    expect(screen.getByText('42')).toBeInTheDocument();
  });

  it('closes empty containers inline without an expand toggle', () => {
    renderWithProviders(<JSONView json={JSON.stringify({ list: [], obj: {} })} />);
    expect(screen.getByText('list:')).toBeInTheDocument();
    expect(screen.getByText('[')).toBeInTheDocument();
    expect(screen.getByText(']')).toBeInTheDocument();
    // Only the root object has a toggle.
    expect(screen.getAllByRole('button')).toHaveLength(1);
  });

  it('collapses levels below the second, showing a count, and expands on click', async () => {
    const json = JSON.stringify({ a: { b: { c: 1, d: 2 }, items: ['x'] } });
    renderWithProviders(<JSONView json={json} />);
    // Root and "a" are open; "b" (third level) starts collapsed.
    expect(screen.getByText('2 properties')).toBeInTheDocument();
    expect(screen.queryByText('c:')).not.toBeInTheDocument();
    expect(screen.getByText('1 items')).toBeInTheDocument();

    const bRow = screen.getByText('b:').parentElement as HTMLElement;
    fireEvent.click(bRow.querySelector('button') as HTMLElement);
    expect(await screen.findByText('c:')).toBeInTheDocument();
    expect(screen.queryByText('2 properties')).not.toBeInTheDocument();

    // Collapsing the root hides everything and shows its size instead.
    fireEvent.click(screen.getAllByRole('button')[0]);
    await waitFor(() => expect(screen.queryByText('a:')).not.toBeInTheDocument());
    expect(screen.getByText('1 properties')).toBeInTheDocument();
  });
});
