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

import { fireEvent, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { TokenExpirySelector } from './TokenExpirySelector';

// A plain function, not a spy: the failure is what's under test, and a
// spy would record the thrown value as an error of the test itself.
const failure = vi.hoisted(() => ({ value: undefined as unknown }));
vi.mock('./tokenExpiryPresets', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./tokenExpiryPresets')>()),
  customDateToExpiresIn: () => {
    throw failure.value;
  },
}));

async function applyCustomDate() {
  fireEvent.mouseDown(screen.getByRole('combobox'));
  fireEvent.click(screen.getByRole('option', { name: 'Custom date...' }));
  const title = await screen.findByText('Custom Expiry');
  const popover = title.closest('.MuiPopover-paper') as HTMLElement;
  fireEvent.click(within(popover).getByRole('button', { name: 'Apply' }));
  return popover;
}

describe('TokenExpirySelector with a rejected custom date', () => {
  it('keeps the popover open and shows why the date was refused', async () => {
    failure.value = new Error('Expiry cannot be more than 10 years from now');
    const onChange = vi.fn();
    renderWithProviders(<TokenExpirySelector value="720h" onChange={onChange} />);

    const popover = await applyCustomDate();

    expect(within(popover).getByText('Expiry cannot be more than 10 years from now')).toBeVisible();
    expect(onChange).not.toHaveBeenCalled();
  });

  it('falls back to a generic message for a non-Error failure', async () => {
    failure.value = 'boom';
    const onChange = vi.fn();
    renderWithProviders(<TokenExpirySelector value="720h" onChange={onChange} />);

    const popover = await applyCustomDate();

    expect(within(popover).getByText('Invalid expiry date')).toBeVisible();
    expect(onChange).not.toHaveBeenCalled();
  });
});
