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
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider, createTheme } from '@wso2/oxygen-ui';
import type {
  A2ACatalogAgent,
  A2ACatalogEnvironment,
} from '@agent-management-platform/types';
import { describe, expect, it } from 'vitest';
import { A2AAgentTile, TILE_TEST_ID, envStatusTestId } from './A2AAgentTile';
import type { EnvironmentLabel } from '../utils/useEnvironmentLabel';

const environment = (
  overrides: Partial<A2ACatalogEnvironment> = {},
): A2ACatalogEnvironment => ({
  name: 'dev',
  status: 'fetched',
  proxied: false,
  cardDiffers: false,
  matched: false,
  ...overrides,
});

const agent = (overrides: Partial<A2ACatalogAgent> = {}): A2ACatalogAgent => ({
  projectName: 'p1',
  agentName: 'orders',
  kind: 'platform',
  name: 'Orders',
  description: 'd',
  version: '1',
  skills: [],
  leadEnvironment: 'dev',
  environments: [environment()],
  ...overrides,
});

const identityLabel: EnvironmentLabel = (name) => name;

const renderTile = (
  entry: A2ACatalogAgent,
  {
    emphasised = false,
    environmentLabel = identityLabel,
  }: { emphasised?: boolean; environmentLabel?: EnvironmentLabel } = {},
) => {
  const ui: ReactElement = (
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <A2AAgentTile
          orgId="acme"
          agent={entry}
          emphasised={emphasised}
          environmentLabel={environmentLabel}
        />
      </MemoryRouter>
    </ThemeProvider>
  );
  return render(ui);
};

describe('A2AAgentTile', () => {
  it('links to the agent page and shows kind and project', () => {
    renderTile(agent());

    expect(screen.getByTestId(TILE_TEST_ID)).toHaveAttribute(
      'href',
      '/org/acme/a2a-agents/view/p1/orders',
    );
    expect(screen.getByRole('heading', { name: 'Orders' })).toBeInTheDocument();
    expect(screen.getByText('platform')).toBeInTheDocument();
    expect(screen.getByText('p1')).toBeInTheDocument();
  });

  it('falls back to the agent name when name is empty', () => {
    renderTile(agent({ name: '' }));

    expect(screen.getByRole('heading', { name: 'orders' })).toBeInTheDocument();
  });

  it('shows two skills and a tooltip with all of them', async () => {
    renderTile(agent({ skills: ['s1', 's2', 's3', 's4', 's5'] }));

    expect(screen.getByText('s1')).toBeInTheDocument();
    expect(screen.getByText('s2')).toBeInTheDocument();
    expect(screen.queryByText('s3')).not.toBeInTheDocument();
    const overflow = screen.getByText('+3 more');

    fireEvent.mouseOver(overflow);

    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      's1, s2, s3, s4, s5',
    );
  });

  it('shows no overflow caption for two skills and no row for none', () => {
    const { unmount } = renderTile(agent({ skills: ['s1', 's2'] }));
    expect(screen.getByText('s1')).toBeInTheDocument();
    expect(screen.getByText('s2')).toBeInTheDocument();
    expect(screen.queryByText(/more$/)).not.toBeInTheDocument();
    unmount();

    renderTile(agent({ skills: [] }));
    expect(screen.queryByTestId('a2a-agent-tile-skills')).not.toBeInTheDocument();
  });

  it.each([
    ['pending', 'card pending'],
    ['failed', 'card failed'],
  ] as const)('captions a %s lead card', (status, caption) => {
    renderTile(agent({ environments: [environment({ status })] }));

    expect(screen.getByText(caption)).toBeInTheDocument();
  });

  it('shows no caption for a fetched lead card', () => {
    renderTile(agent());

    expect(screen.queryByText(/^card (pending|failed)$/)).not.toBeInTheDocument();
  });

  it('renders environment chips, dots and labels', () => {
    renderTile(
      agent({
        environments: [
          environment({ name: 'dev', status: 'fetched', proxied: true }),
          environment({ name: 'prod', status: 'failed', cardDiffers: true }),
        ],
      }),
      { environmentLabel: (n) => (n === 'prod' ? 'Production' : n) },
    );

    expect(screen.getByText('proxied')).toBeInTheDocument();
    expect(screen.getByText('card differs')).toBeInTheDocument();
    expect(screen.getByText('Production')).toBeInTheDocument();
    expect(screen.getByText('dev')).toBeInTheDocument();
    expect(screen.getByTestId(envStatusTestId('dev'))).toHaveAttribute(
      'title',
      'fetched',
    );
    expect(screen.getByTestId(envStatusTestId('prod'))).toHaveAttribute(
      'title',
      'failed',
    );
  });

  it('marks emphasis through data-emphasised', () => {
    const { unmount } = renderTile(agent(), { emphasised: true });
    expect(screen.getByTestId(TILE_TEST_ID)).toHaveAttribute(
      'data-emphasised',
      'true',
    );
    unmount();

    renderTile(agent(), { emphasised: false });
    expect(screen.getByTestId(TILE_TEST_ID)).toHaveAttribute(
      'data-emphasised',
      'false',
    );
  });

  it('survives an entry with no environments', () => {
    renderTile(agent({ environments: [], leadEnvironment: '' }));

    expect(screen.getByRole('heading', { name: 'Orders' })).toBeInTheDocument();
    expect(screen.queryByText(/^card (pending|failed)$/)).not.toBeInTheDocument();
    expect(screen.queryByTestId(/^a2a-env-status-/)).not.toBeInTheDocument();
  });
});
