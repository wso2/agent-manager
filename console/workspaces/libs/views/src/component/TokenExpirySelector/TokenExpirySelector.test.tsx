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

import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { TokenExpirySelector } from './TokenExpirySelector';

function Controlled({ onChange }: { onChange: (v: string) => void }) {
  const [value, setValue] = useState('720h');
  return (
    <TokenExpirySelector
      value={value}
      onChange={(v) => {
        onChange(v);
        setValue(v);
      }}
    />
  );
}

const openMenu = () => fireEvent.mouseDown(screen.getByRole('combobox'));

describe('TokenExpirySelector', () => {
  it('shows the selected preset and emits a newly chosen one', () => {
    const onChange = vi.fn();
    renderWithProviders(<TokenExpirySelector value="2160h" onChange={onChange} />);
    expect(screen.getByRole('combobox')).toHaveTextContent('90 days');

    openMenu();
    fireEvent.click(screen.getByRole('option', { name: '30 days' }));
    expect(onChange).toHaveBeenCalledWith('720h');
  });

  it('labels a non-preset value as a custom date', () => {
    renderWithProviders(<TokenExpirySelector value="100h" onChange={() => {}} />);
    expect(screen.getByRole('combobox')).toHaveTextContent('Custom date');
  });

  it('cannot be opened when disabled', () => {
    renderWithProviders(<TokenExpirySelector value="720h" onChange={() => {}} disabled />);
    expect(screen.getByRole('combobox')).toHaveAttribute('aria-disabled', 'true');
  });

  it('applies a custom date from the popover as hours from now', async () => {
    const onChange = vi.fn();
    renderWithProviders(<Controlled onChange={onChange} />);
    openMenu();
    fireEvent.click(screen.getByRole('option', { name: 'Custom date...' }));
    expect(onChange).not.toHaveBeenCalled();

    const popover = await screen.findByText('Custom Expiry');
    const dialog = popover.closest('.MuiPopover-paper') as HTMLElement;
    fireEvent.click(within(dialog).getByRole('button', { name: 'Apply' }));

    // The draft defaults to 90 days out.
    expect(onChange).toHaveBeenCalledTimes(1);
    const hours = Number(onChange.mock.calls[0][0].replace('h', ''));
    expect(hours).toBeGreaterThanOrEqual(90 * 24);
    expect(hours).toBeLessThanOrEqual(90 * 24 + 1);
    await waitFor(() => expect(screen.queryByText('Custom Expiry')).not.toBeInTheDocument());
    // The picked date replaces the generic "Custom date" label.
    expect(screen.getByRole('combobox')).not.toHaveTextContent('Custom date');
    expect(screen.getByRole('combobox')).not.toHaveTextContent('30 days');
  });

  it('closes the popover on cancel without emitting', async () => {
    const onChange = vi.fn();
    renderWithProviders(<TokenExpirySelector value="720h" onChange={onChange} />);
    openMenu();
    fireEvent.click(screen.getByRole('option', { name: 'Custom date...' }));
    await screen.findByText('Custom Expiry');
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByText('Custom Expiry')).not.toBeInTheDocument());
    expect(onChange).not.toHaveBeenCalled();
  });
});
