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

import { act, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { TextInput } from './TextInput';

afterEach(() => {
  vi.useRealTimers();
});

describe('TextInput', () => {
  it('labels the field and forwards typing', () => {
    const typed: string[] = [];
    const onChange = vi.fn((e: { target: { value: string } }) => typed.push(e.target.value));
    renderWithProviders(
      <TextInput label="Agent name" value="" onChange={onChange} required labelAction={<span>?</span>} />,
    );
    const input = screen.getByRole('textbox', { name: /Agent name/ });
    fireEvent.change(input, { target: { value: 'bot' } });
    expect(typed).toEqual(['bot']);
    expect(input).toBeRequired();
    expect(screen.getByText('?')).toBeInTheDocument();
  });

  it('shows helper/error text', () => {
    renderWithProviders(
      <TextInput label="Name" value="x" error helperText="Name is taken" onChange={() => {}} />,
    );
    expect(screen.getByText('Name is taken')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Name' })).toHaveAttribute('aria-invalid', 'true');
  });

  it('caps the length and shows a live counter beside the helper text', () => {
    renderWithProviders(
      <TextInput label="Desc" value="hello" maxLength={10} helperText="Short" onChange={() => {}} />,
    );
    expect(screen.getByRole('textbox', { name: 'Desc' })).toHaveAttribute('maxlength', '10');
    expect(screen.getByText('5/10')).toBeInTheDocument();
    expect(screen.getByText('Short')).toBeInTheDocument();
  });

  it.each([
    ['hideCharacterCount', { hideCharacterCount: true }],
    ['disabled', { disabled: true }],
  ])('keeps the cap but hides the counter when %s', (_label, props) => {
    renderWithProviders(
      <TextInput label="Desc" value="hey" maxLength={10} onChange={() => {}} {...props} />,
    );
    expect(screen.getByRole('textbox', { name: 'Desc' })).toHaveAttribute('maxlength', '10');
    expect(screen.queryByText('3/10')).not.toBeInTheDocument();
  });

  it('counts zero for a non-string value', () => {
    renderWithProviders(<TextInput label="N" value={5} maxLength={4} onChange={() => {}} />);
    expect(screen.getByText('0/4')).toBeInTheDocument();
  });

  it('toggles password visibility', () => {
    renderWithProviders(
      <TextInput label="Secret" type="password" showPasswordToggle value="s3cret" onChange={() => {}} />,
    );
    const input = screen.getByLabelText('Secret');
    expect(input).toHaveAttribute('type', 'password');

    fireEvent.click(screen.getByRole('button', { name: 'Show password' }));
    expect(input).toHaveAttribute('type', 'text');

    fireEvent.click(screen.getByRole('button', { name: 'Hide password' }));
    expect(input).toHaveAttribute('type', 'password');
  });

  it('copies the value and briefly confirms it', async () => {
    vi.useFakeTimers();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    renderWithProviders(
      <TextInput label="Key" value="abc123" copyable copyTooltipText="Copy key" onChange={() => {}} />,
    );
    const button = screen.getByRole('button', { name: 'Copy key' });
    await act(async () => {
      fireEvent.click(button);
    });
    expect(writeText).toHaveBeenCalledWith('abc123');
    expect(screen.getByRole('button', { name: 'Copied!' })).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.getByRole('button', { name: 'Copy key' })).toBeInTheDocument();
  });

  it('uses the default tooltip and swallows clipboard failures', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'));
    Object.assign(navigator, { clipboard: { writeText } });
    renderWithProviders(<TextInput label="Key" value="abc" copyable onChange={() => {}} />);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
    });
    expect(writeText).toHaveBeenCalledWith('abc');
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument();
  });

  it('offers no copy button for an empty value', () => {
    renderWithProviders(<TextInput label="Key" value="" copyable onChange={() => {}} />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
