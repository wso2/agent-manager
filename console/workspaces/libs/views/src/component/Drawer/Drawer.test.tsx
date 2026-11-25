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

import { act, fireEvent, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { DrawerContent, DrawerHeader, DrawerWrapper, SelectionDrawer } from '.';

afterEach(() => {
  vi.useRealTimers();
});

describe('DrawerWrapper / DrawerHeader / DrawerContent', () => {
  it('renders an open drawer with its header and body, and closes from the header', () => {
    const onClose = vi.fn();
    renderWithProviders(
      <DrawerWrapper open onClose={onClose} sx={{ p: 1 }}>
        <DrawerHeader icon={<span />} title="Edit agent" onClose={onClose} meta={<span>2m ago</span>} />
        <DrawerContent>
          <p>Drawer body</p>
        </DrawerContent>
      </DrawerWrapper>,
    );
    expect(screen.getByText('Edit agent')).toBeInTheDocument();
    expect(screen.getByText('2m ago')).toBeInTheDocument();
    expect(screen.getByText('Drawer body')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Fullscreen' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /close/i }));
    expect(onClose).toHaveBeenCalled();
  });

  it('renders nothing while closed', () => {
    renderWithProviders(
      <DrawerWrapper open={false} onClose={() => {}} maxWidth={500} sx={[{ p: 1 }]}>
        <p>Hidden body</p>
      </DrawerWrapper>,
    );
    expect(screen.queryByText('Hidden body')).not.toBeInTheDocument();
  });

  it.each([
    [false, 'Fullscreen'],
    [true, 'Exit fullscreen'],
  ])('offers the fullscreen toggle (fullscreen=%s)', (isFullscreen, label) => {
    const onToggle = vi.fn();
    renderWithProviders(
      <DrawerWrapper open onClose={() => {}} fullscreen={isFullscreen} minWidth={400}>
        <DrawerHeader
          icon={<span />}
          title="T"
          onClose={() => {}}
          isFullscreen={isFullscreen}
          onToggleFullscreen={onToggle}
        />
      </DrawerWrapper>,
    );
    fireEvent.click(screen.getByRole('button', { name: label }));
    expect(onToggle).toHaveBeenCalledTimes(1);
  });
});

interface Item {
  id: string;
  name: string;
}

const items: Item[] = [
  { id: '1', name: 'Alpha' },
  { id: '2', name: 'Beta' },
];

const renderSelection = (props: Partial<Parameters<typeof SelectionDrawer<Item>>[0]> = {}) => {
  const onSelect = vi.fn();
  const onClose = vi.fn();
  renderWithProviders(
    <SelectionDrawer<Item>
      open
      onClose={onClose}
      icon={<span />}
      title="Pick a model"
      description="Choose one"
      searchPlaceholder="Search models"
      items={items}
      getItemKey={(i) => i.id}
      isItemSelected={(i) => i.id === '2'}
      matchesSearch={(i, q) => i.name.toLowerCase().includes(q)}
      onSelect={onSelect}
      renderItem={(i, selected) => `${i.name}${selected ? ' (selected)' : ''}`}
      getItemAriaLabel={(i) => `Select ${i.name}`}
      emptyState={{ title: 'No models', description: 'Add one first' }}
      searchEmptyState={{ title: 'No matches', description: 'Try another term' }}
      {...props}
    />,
  );
  return { onSelect, onClose };
};

describe('SelectionDrawer', () => {
  it('lists items, marks the selected one and selects then closes on click', () => {
    const { onSelect, onClose } = renderSelection();
    expect(screen.getByText('Choose one')).toBeInTheDocument();
    expect(screen.getByText('Beta (selected)')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Select Alpha' }));
    expect(onSelect).toHaveBeenCalledWith(items[0]);
    expect(onClose).toHaveBeenCalled();
  });

  it('filters by a debounced, lower-cased search and shows the search empty state', () => {
    vi.useFakeTimers();
    renderSelection();
    const search = screen.getByPlaceholderText('Search models');

    fireEvent.change(search, { target: { value: 'ALP' } });
    // Not applied until the debounce fires.
    expect(screen.getByText('Beta (selected)')).toBeInTheDocument();
    fireEvent.change(search, { target: { value: ' ALPH ' } });
    act(() => {
      vi.advanceTimersByTime(250);
    });
    expect(screen.getByText('Alpha')).toBeInTheDocument();
    expect(screen.queryByText('Beta (selected)')).not.toBeInTheDocument();

    fireEvent.change(search, { target: { value: 'zzz' } });
    act(() => {
      vi.advanceTimersByTime(250);
    });
    expect(screen.getByText('No matches')).toBeInTheDocument();
    expect(screen.getByText('Try another term')).toBeInTheDocument();
  });

  it('shows the empty state when there are no items', () => {
    renderSelection({ items: [] });
    expect(screen.getByText('No models')).toBeInTheDocument();
    expect(screen.getByText('Add one first')).toBeInTheDocument();
  });

  it('shows skeletons while loading', () => {
    renderSelection({ isLoading: true });
    expect(screen.queryByText('Alpha')).not.toBeInTheDocument();
    const panel = screen.getByText('Choose one').parentElement as HTMLElement;
    expect(within(panel).queryAllByRole('button', { name: /Select/ })).toHaveLength(0);
    expect(document.querySelectorAll('.MuiSkeleton-root')).toHaveLength(3);
  });
});
