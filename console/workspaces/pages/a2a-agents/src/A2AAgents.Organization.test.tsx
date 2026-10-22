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

import { act, render, screen } from '@testing-library/react';
import {
  MemoryRouter,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from 'react-router-dom';
import { ThemeProvider, createTheme } from '@wso2/oxygen-ui';
import { useListA2ACatalog } from '@agent-management-platform/api-client';
import type { ResourceListShellProps } from '@agent-management-platform/shared-component';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { A2AAgentsOrganization } from './A2AAgents.Organization';
import { metaData } from './index';

vi.mock('@agent-management-platform/api-client', () => ({
  useListA2ACatalog: vi.fn(),
  useGetA2ACatalogEntry: vi.fn(() => ({ data: undefined, isLoading: true, isError: false })),
  useGetAgentCard: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useListEnvironments: vi.fn(() => ({ data: [] })),
  useListProjects: vi.fn(() => ({ data: { projects: [] } })),
}));

vi.mock('@agent-management-platform/shared-component', () => ({
  ResourceListShell: (p: ResourceListShellProps) => (
    <div>
      {p.toolbar}
      {p.error ? (
        <div>error</div>
      ) : p.isLoading ? (
        p.loadingPlaceholder
      ) : p.isEmpty ? (
        <div>{p.emptyState.title}</div>
      ) : p.isSearchEmpty ? (
        <div>{p.searchEmptyState?.title}</div>
      ) : (
        p.children
      )}
    </div>
  ),
}));

let currentPathname = '';
let goBack: () => void = () => undefined;

const LocationProbe = () => {
  const location = useLocation();
  const navigate = useNavigate();
  currentPathname = location.pathname;
  goBack = () => navigate(-1);
  return null;
};

const renderAt = (url: string, previous?: string) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter
        initialEntries={previous ? [previous, url] : [url]}
        initialIndex={previous ? 1 : 0}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route
            path="/org/:orgId/a2a-agents/*"
            element={<A2AAgentsOrganization />}
          />
          <Route path="*" element={null} />
        </Routes>
        <LocationProbe />
      </MemoryRouter>
    </ThemeProvider>,
  );

beforeEach(() => {
  vi.mocked(useListA2ACatalog).mockReturnValue({
    data: { agents: [], total: 0, limit: 24, offset: 0 },
    isLoading: false,
    error: null,
  } as unknown as ReturnType<typeof useListA2ACatalog>);
});

describe('A2AAgentsOrganization', () => {
  it('renders the list at the index route', () => {
    renderAt('/org/acme/a2a-agents');

    expect(
      screen.getByRole('heading', { name: 'A2A Agents' }),
    ).toBeInTheDocument();
    expect(screen.getByText('No A2A agents yet')).toBeInTheDocument();
  });

  it('renders the agent page route', () => {
    renderAt('/org/acme/a2a-agents/view/p1/orders');

    expect(screen.getByTestId('a2a-agent-page')).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: /Back to A2A Agents/ }),
    ).toHaveAttribute('href', '/org/acme/a2a-agents');
  });

  it('replaces any other sub-path with the list', () => {
    renderAt('/org/acme/a2a-agents/anything-else', '/org/acme/elsewhere');

    expect(currentPathname).toBe('/org/acme/a2a-agents');

    act(() => goBack());

    expect(currentPathname).toBe('/org/acme/elsewhere');
  });

  it('exports page metadata', () => {
    expect(metaData.title).toBe('A2A Agents');
    expect(metaData.description).toBe(
      'Organization-wide catalogue of A2A agents',
    );
    expect(metaData.path).toBe('/a2a-agents');
    expect(metaData.levels?.organization).toBe(A2AAgentsOrganization);
  });
});
