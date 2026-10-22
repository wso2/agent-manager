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

import { fireEvent, render, screen, within } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@wso2/oxygen-ui';
import { useGetAgentCard } from '@agent-management-platform/api-client';
import type { AgentCardResponse } from '@agent-management-platform/types';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { A2ACardCompare } from './A2ACardCompare';
import type { EnvironmentLabel } from '../utils/useEnvironmentLabel';

vi.mock('@agent-management-platform/api-client', () => ({
  useGetAgentCard: vi.fn(),
}));

const params = { orgName: 'acme', projName: 'p1', agentName: 'orders' };
const label: EnvironmentLabel = (name) => (name === 'prod' ? 'Production' : name);

const skill = (name: string, overrides: Record<string, unknown> = {}) => ({
  id: name,
  name,
  description: `${name} skill`,
  tags: ['orders'],
  ...overrides,
});

const devCard = { skills: [skill('order-status')] };

const fetched = (card: Record<string, unknown>): AgentCardResponse => ({
  source: 'platform',
  status: 'fetched',
  sourceUrl: '',
  lastError: '',
  card,
});

interface CompareQuery {
  data?: AgentCardResponse | null;
  isLoading?: boolean;
  isError?: boolean;
}

const mockCompareQuery = ({ data, isLoading = false, isError = false }: CompareQuery) =>
  vi.mocked(useGetAgentCard).mockReturnValue({
    data,
    isLoading,
    isError,
  } as unknown as ReturnType<typeof useGetAgentCard>);

const renderCompare = (
  baseCard: Record<string, unknown> = devCard,
  otherEnvironments: string[] = ['prod'],
) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <A2ACardCompare
        params={params}
        baseCard={baseCard}
        otherEnvironments={otherEnvironments}
        environmentLabel={label}
      />
    </ThemeProvider>,
  );

const compareSelect = () => screen.getByRole('combobox', { name: /Compare with/ });

const selectProduction = () => {
  fireEvent.mouseDown(compareSelect());
  fireEvent.click(
    within(screen.getByRole('listbox')).getByRole('option', { name: 'Production' }),
  );
};

const compareWith = (
  prodCard: Record<string, unknown>,
  baseCard: Record<string, unknown> = devCard,
) => {
  mockCompareQuery({ data: fetched(prodCard) });
  const view = renderCompare(baseCard);
  selectProduction();
  return view;
};

describe('A2ACardCompare', () => {
  beforeEach(() => {
    vi.mocked(useGetAgentCard).mockReset();
    mockCompareQuery({ data: undefined });
  });

  it('offers the other environments and none when alone', () => {
    const { unmount } = renderCompare();
    fireEvent.mouseDown(compareSelect());
    const listbox = within(screen.getByRole('listbox'));
    expect(listbox.getByRole('option', { name: 'None' })).toBeInTheDocument();
    expect(listbox.getByRole('option', { name: 'Production' })).toBeInTheDocument();
    unmount();

    const { container } = renderCompare(devCard, []);
    expect(container).toBeEmptyDOMElement();
  });

  it('queries the compare environment only once selected', () => {
    renderCompare();
    expect(vi.mocked(useGetAgentCard).mock.lastCall).toEqual([
      { ...params, envId: '' },
      { enabled: false, poll: false },
    ]);

    selectProduction();

    expect(vi.mocked(useGetAgentCard).mock.lastCall).toEqual([
      { ...params, envId: 'prod' },
      { enabled: true, poll: false },
    ]);
  });

  it('lists added skills', () => {
    compareWith({ skills: [skill('order-status'), skill('issue-refund')] });
    expect(screen.getByText('Added: issue-refund')).toBeInTheDocument();
  });

  it('lists removed and changed skills', () => {
    compareWith(
      { skills: [skill('order-status', { description: 'other description' })] },
      { skills: [skill('order-status'), skill('issue-refund')] },
    );
    expect(screen.getByText('Removed: issue-refund')).toBeInTheDocument();
    expect(screen.getByText('Changed: order-status')).toBeInTheDocument();
  });

  it('lists other differing fields', () => {
    compareWith({ ...devCard, version: '2' }, { ...devCard, version: '1' });
    expect(screen.getByText('Also differs in: version')).toBeInTheDocument();
    expect(screen.queryByText(/^Added:/)).not.toBeInTheDocument();
  });

  it('says identical cards are identical', () => {
    compareWith(
      {
        supportedInterfaces: [{ url: 'https://prod.example/a2a', protocolBinding: 'JSONRPC' }],
        skills: [skill('order-status')],
        name: 'Orders',
      },
      {
        name: 'Orders',
        skills: [skill('order-status')],
        supportedInterfaces: [{ protocolBinding: 'JSONRPC', url: 'https://dev.example/a2a' }],
      },
    );
    expect(screen.getByText('The cards are identical.')).toBeInTheDocument();
  });

  it('does not compare a card without a body', () => {
    mockCompareQuery({
      data: { source: 'platform', status: 'pending', sourceUrl: '', lastError: '' },
    });
    renderCompare();
    selectProduction();
    expect(screen.getByText('No fetched card to compare.')).toBeInTheDocument();
    expect(screen.queryByText(/^Added:/)).not.toBeInTheDocument();
  });

  it('reports a compare error and loading', () => {
    mockCompareQuery({ data: undefined, isError: true });
    const { unmount } = renderCompare();
    selectProduction();
    expect(screen.getByText("Couldn't load the card to compare.")).toBeInTheDocument();
    unmount();

    mockCompareQuery({ data: undefined, isLoading: true });
    renderCompare();
    selectProduction();
    expect(screen.getByText('Loading…')).toBeInTheDocument();
  });

  it('agrees with the fingerprint', () => {
    const cases: Array<[Record<string, unknown>, Record<string, unknown>, string]> = [
      [
        { skills: [skill('a', { inputModes: ['text'] })] },
        { skills: [skill('a', { inputModes: ['json'] })] },
        'Changed: a',
      ],
      [
        { skills: [skill('a'), skill('b')] },
        { skills: [skill('b'), skill('a')] },
        'Also differs in: skills',
      ],
      [
        { skills: [skill('a'), skill('a')] },
        { skills: [skill('a')] },
        'Also differs in: skills',
      ],
    ];
    for (const [base, other, line] of cases) {
      const { unmount } = compareWith(other, base);
      expect(screen.getByText(line)).toBeInTheDocument();
      expect(screen.queryByText('The cards are identical.')).not.toBeInTheDocument();
      unmount();
    }
  });
});
