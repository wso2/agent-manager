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

import type { ReactElement } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@wso2/oxygen-ui';
import type { AgentCardResponse } from '@agent-management-platform/types';
import { describe, expect, it, vi } from 'vitest';
import { A2ACardDetails } from './A2ACardDetails';
import type { CardState } from './cardState';

vi.mock('@agent-management-platform/api-client', () => ({
  useTrack: () => ({ track: vi.fn() }),
  ConsoleAction: {},
}));

const twoMinutesAgo = () => new Date(Date.now() - 2 * 60 * 1000).toISOString();

const response = (overrides: Partial<AgentCardResponse> = {}): AgentCardResponse => ({
  source: 'platform',
  status: 'fetched',
  sourceUrl: '',
  lastError: '',
  ...overrides,
});

const baseCard = (): Record<string, unknown> => ({
  name: 'Orders',
  description: 'desc',
  skills: [{ id: 's1', name: 'order-status', tags: ['orders'] }],
  capabilities: { streaming: true },
});

const ready = (
  card: Record<string, unknown> = baseCard(),
  fetchedAt: string = twoMinutesAgo(),
): CardState => ({
  state: 'ready',
  response: response({ status: 'fetched', fetchedAt, card }),
  card,
});

const noBody = (overrides: Partial<AgentCardResponse>): CardState => ({
  state: 'noBody',
  response: response(overrides),
});

const renderDetails = (element: ReactElement) =>
  render(<ThemeProvider theme={createTheme()}>{element}</ThemeProvider>);

describe('A2ACardDetails', () => {
  it('shows the loading copy without claiming absence', () => {
    renderDetails(<A2ACardDetails state={{ state: 'loading' }} />);
    expect(screen.getByText('Loading card…')).toBeInTheDocument();
    expect(screen.queryByText(/No agent card is configured/)).not.toBeInTheDocument();
  });

  it('shows the absent copy', () => {
    renderDetails(<A2ACardDetails state={{ state: 'absent' }} />);
    expect(
      screen.getByText('No agent card is configured for this environment.'),
    ).toBeInTheDocument();
  });

  it('shows the error copy, not the absent copy', () => {
    renderDetails(<A2ACardDetails state={{ state: 'error' }} />);
    expect(screen.getByText("Couldn't load the agent card.")).toBeInTheDocument();
    expect(screen.queryByText(/No agent card is configured/)).not.toBeInTheDocument();
  });

  it('renders only the status block for a pending card without body', () => {
    renderDetails(
      <A2ACardDetails
        state={noBody({ status: 'pending' })}
        compare={<div>Compare with</div>}
      />,
    );
    expect(screen.getByText('Card pending')).toBeInTheDocument();
    expect(screen.queryByText('No skills declared.')).not.toBeInTheDocument();
    expect(screen.queryByText('✗ streaming')).not.toBeInTheDocument();
    expect(screen.queryByText('Compare with')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'View raw JSON' })).not.toBeInTheDocument();
  });

  it('shows the last error of a failed card', () => {
    renderDetails(
      <A2ACardDetails state={noBody({ status: 'failed', lastError: 'connection refused' })} />,
    );
    expect(screen.getByText('Card fetch failed')).toBeInTheDocument();
    expect(screen.getByText('connection refused')).toBeInTheDocument();
  });

  it('captions a fetched card relatively', () => {
    renderDetails(<A2ACardDetails state={ready()} />);
    expect(screen.getByText('Fetched 2 minutes ago')).toBeInTheDocument();
  });

  it('renders skills and capabilities', () => {
    renderDetails(<A2ACardDetails state={ready()} />);
    expect(screen.getByText('desc')).toBeInTheDocument();
    expect(screen.getByText('Skills')).toBeInTheDocument();
    expect(screen.getByText('order-status')).toBeInTheDocument();
    expect(screen.getByText('Capabilities')).toBeInTheDocument();
    expect(screen.getByText('✓ streaming')).toBeInTheDocument();
    expect(screen.getByText('✗ pushNotifications')).toBeInTheDocument();
    expect(screen.getByText('✗ extendedAgentCard')).toBeInTheDocument();
  });

  it('says when no skills are declared', () => {
    renderDetails(<A2ACardDetails state={ready({ ...baseCard(), skills: 'x' })} />);
    expect(screen.getByText('No skills declared.')).toBeInTheDocument();
  });

  it('toggles the raw JSON', () => {
    const { container } = renderDetails(<A2ACardDetails state={ready()} />);
    expect(container.querySelector('pre')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'View raw JSON' }));

    expect(container.querySelector('pre')?.textContent).toMatch(/order-status/);
    expect(screen.getByRole('button', { name: 'Hide raw JSON' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Hide raw JSON' }));
    expect(container.querySelector('pre')).toBeNull();
  });

  it('renders the compare slot only when ready', () => {
    const { unmount } = renderDetails(
      <A2ACardDetails state={ready()} compare={<div>cmp</div>} />,
    );
    expect(screen.getByText('cmp')).toBeInTheDocument();
    unmount();

    renderDetails(
      <A2ACardDetails state={noBody({ status: 'pending' })} compare={<div>cmp</div>} />,
    );
    expect(screen.queryByText('cmp')).not.toBeInTheDocument();
  });

  it('renders unnamed and duplicate skills without crashing', () => {
    const card = {
      ...baseCard(),
      skills: [{}, {}, { id: 'x', name: 'a' }, { id: 'x', name: 'a' }],
    };
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      expect(() => renderDetails(<A2ACardDetails state={ready(card)} />)).not.toThrow();
      expect(screen.getAllByText('a')).toHaveLength(2);
      const keyWarnings = consoleError.mock.calls.filter((call) =>
        String(call[0]).includes('same key'),
      );
      expect(keyWarnings).toHaveLength(0);
    } finally {
      consoleError.mockRestore();
    }
  });

  it('captions an invalid fetchedAt as a dash', () => {
    renderDetails(<A2ACardDetails state={ready(baseCard(), 'nope')} />);
    expect(screen.getByText('Fetched -')).toBeInTheDocument();
  });
});
