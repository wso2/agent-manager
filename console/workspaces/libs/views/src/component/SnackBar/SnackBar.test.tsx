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

import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test-utils/render';
import { SnackBarProvider, useSnackBar, type SnackBarMessage } from './SnackBar';

type Push = Omit<SnackBarMessage, 'id'>;

let pushSnackBar: ((message: Push) => void) | undefined;
let pending: Push[] = [];

function Pusher({ messages }: { messages: Push[] }) {
  pushSnackBar = useSnackBar().pushSnackBar;
  pending = messages;
  return null;
}

const renderPusher = (messages: Push[]) =>
  renderWithProviders(
    <SnackBarProvider>
      <Pusher messages={messages} />
    </SnackBarProvider>,
  );

// Pushed through the hook rather than by clicking a button: a click is a
// click-away that closes the snackbars already open. Ids come from
// Date.now(), so each push needs its own millisecond.
const push = (index: number) => {
  act(() => {
    vi.advanceTimersByTime(1);
    pushSnackBar?.(pending[index]);
  });
};

describe('SnackBarProvider', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows a typed message as an alert and hides it after its duration', () => {
    renderPusher([{ message: 'Agent saved', type: 'success', duration: 1000 }]);
    push(0);
    expect(screen.getByRole('alert')).toHaveTextContent('Agent saved');

    act(() => {
      vi.advanceTimersByTime(1600);
    });
    expect(screen.queryByText('Agent saved')).not.toBeInTheDocument();
  });

  it('shows an untyped message as plain snackbar text', () => {
    renderPusher([{ message: 'Plain note' }]);
    push(0);
    expect(screen.getByText('Plain note')).toBeInTheDocument();
    expect(document.querySelector('.MuiAlert-root')).toBeNull();
  });

  it('closes on Escape and calls the caller\'s onClose', () => {
    const onClose = vi.fn();
    renderPusher([{ message: 'Sticky', duration: 60_000, onClose }]);
    push(0);
    expect(screen.getByText('Sticky')).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledWith(expect.anything(), 'escapeKeyDown');
    expect(screen.queryByText('Sticky')).not.toBeInTheDocument();
  });

  it('dismisses a timed message early and drops its pending timer', () => {
    renderPusher([{ message: 'Timed', duration: 5000 }]);
    push(0);
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByText('Timed')).not.toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    expect(screen.queryByText('Timed')).not.toBeInTheDocument();
  });

  it('replaces a repeated message instead of stacking it', () => {
    renderPusher([{ message: 'Same', duration: 60_000 }]);
    push(0);
    push(0);
    expect(screen.getAllByText('Same')).toHaveLength(1);
  });

  it('shows at most three messages, always including the newest', () => {
    renderPusher(['one', 'two', 'three', 'four'].map((message) => ({ message, duration: 60_000 })));
    [0, 1, 2].forEach(push);
    ['one', 'two', 'three'].forEach((m) => expect(screen.getByText(m)).toBeInTheDocument());

    push(3);
    const shown = ['one', 'two', 'three', 'four'].filter((m) => screen.queryByText(m));
    expect(shown).toHaveLength(3);
    expect(shown).toContain('four');
  });

  it('clears pending timers on unmount', () => {
    const clearSpy = vi.spyOn(globalThis, 'clearTimeout');
    const { unmount } = renderPusher([{ message: 'Bye' }]);
    push(0);
    clearSpy.mockClear();
    unmount();
    expect(clearSpy).toHaveBeenCalled();
    clearSpy.mockRestore();
  });
});

describe('useSnackBar', () => {
  it('throws outside the provider', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    function Orphan() {
      useSnackBar();
      return null;
    }
    expect(() => render(<Orphan />)).toThrow('useSnackBar must be used within SnackBarProvider');
    errorSpy.mockRestore();
  });
});
