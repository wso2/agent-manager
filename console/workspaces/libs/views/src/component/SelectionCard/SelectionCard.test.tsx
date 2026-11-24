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

import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { SelectionIndicator } from '../SelectionIndicator';
import { SelectionCard, selectionGridSx } from './SelectionCard';

describe('SelectionCard', () => {
  it('is an unchecked radio that reports a selection', () => {
    const onSelect = vi.fn();
    renderWithProviders(
      <SelectionCard
        icon={<svg data-testid="icon" />}
        title="Chat agent"
        description="Conversational"
        fullDescription="A conversational agent with memory"
        selected={false}
        onSelect={onSelect}
      />,
    );
    const radio = screen.getByRole('radio', { name: /Chat agent/ });
    expect(radio).toHaveAttribute('aria-checked', 'false');
    expect(screen.getByText('Conversational')).toHaveAttribute(
      'title',
      'A conversational agent with memory',
    );
    expect(screen.getByTestId('icon')).toBeInTheDocument();
    fireEvent.click(radio);
    expect(onSelect).toHaveBeenCalledTimes(1);
  });

  it('shows the checked state with a check mark', () => {
    renderWithProviders(
      <SelectionCard icon={null} title="API" description="d" selected onSelect={() => {}} />,
    );
    const radio = screen.getByRole('radio', { name: /API/ });
    expect(radio).toHaveAttribute('aria-checked', 'true');
    expect(radio.querySelector('svg')).toBeInTheDocument();
  });

  it('builds a grid with the requested column count', () => {
    expect(selectionGridSx().gridTemplateColumns.md).toBe('repeat(3, minmax(0, 1fr))');
    expect(selectionGridSx(2).gridTemplateColumns.md).toBe('repeat(2, minmax(0, 1fr))');
  });
});

describe('SelectionIndicator', () => {
  it.each([true, false])('renders an icon (selected=%s)', (selected) => {
    const { container } = renderWithProviders(<SelectionIndicator selected={selected} />);
    expect(container.querySelector('.MuiAvatar-root svg')).toBeInTheDocument();
  });

  it('switches icon when selected', () => {
    const { container, rerender } = renderWithProviders(<SelectionIndicator selected={false} />);
    const unselected = container.querySelector('svg')?.innerHTML;
    rerender(<SelectionIndicator selected />);
    expect(container.querySelector('svg')?.innerHTML).not.toBe(unselected);
  });
});
