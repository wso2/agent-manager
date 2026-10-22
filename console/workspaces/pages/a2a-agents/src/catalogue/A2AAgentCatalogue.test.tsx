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

import { useEffect } from 'react';
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { ThemeProvider, createTheme } from '@wso2/oxygen-ui';
import {
  useListA2ACatalog,
  useListEnvironments,
  useListProjects,
} from '@agent-management-platform/api-client';
import type { ResourceListShellProps } from '@agent-management-platform/shared-component';
import type {
  A2ACatalogAgent,
  A2ACatalogEnvironment,
  A2ACatalogListResponse,
  Environment,
  ProjectResponse,
} from '@agent-management-platform/types';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { A2AAgentCatalogue } from './A2AAgentCatalogue';
import { A2A_FILTER_PANEL_ID } from './A2AAgentFilterBar';
import { TILE_TEST_ID } from './A2AAgentTile';

vi.mock('@agent-management-platform/api-client', () => ({
  useListA2ACatalog: vi.fn(),
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

const entry = (overrides: Partial<A2ACatalogAgent> = {}): A2ACatalogAgent => ({
  projectName: 'p1',
  agentName: 'orders',
  kind: 'platform',
  name: '',
  description: '',
  version: '',
  skills: [],
  leadEnvironment: 'dev',
  environments: [environment()],
  ...overrides,
});

const defaultAgents = (): A2ACatalogAgent[] => [
  entry({
    projectName: 'p1',
    environments: [
      environment({ name: 'dev' }),
      environment({ name: 'prod', cardDiffers: true }),
    ],
  }),
  entry({
    projectName: 'p2',
    environments: [environment({ name: 'dev', proxied: true })],
  }),
];

type ListResult = ReturnType<typeof useListA2ACatalog>;

const respondWith = (result: {
  data?: Partial<A2ACatalogListResponse>;
  isLoading?: boolean;
  error?: unknown;
}) => {
  const data = result.data && {
    agents: [],
    total: 0,
    limit: 24,
    offset: 0,
    ...result.data,
  };
  vi.mocked(useListA2ACatalog).mockReturnValue({
    data,
    isLoading: result.isLoading ?? false,
    error: result.error ?? null,
  } as unknown as ListResult);
};

const respondWithAgents = (agents: A2ACatalogAgent[], total: number) =>
  respondWith({ data: { agents, total } });

const withEnvironments = (environments: Partial<Environment>[]) =>
  vi.mocked(useListEnvironments).mockReturnValue({
    data: environments,
  } as unknown as ReturnType<typeof useListEnvironments>);

const withProjects = (projects: Partial<ProjectResponse>[]) =>
  vi.mocked(useListProjects).mockReturnValue({
    data: { projects },
  } as unknown as ReturnType<typeof useListProjects>);

let searchHistory: string[] = [];

const SearchProbe = () => {
  const location = useLocation();
  useEffect(() => {
    searchHistory.push(location.search);
  }, [location]);
  return null;
};

const currentSearch = () => searchHistory[searchHistory.length - 1];
const currentParams = () => new URLSearchParams(currentSearch());
const lastQuery = () => vi.mocked(useListA2ACatalog).mock.lastCall?.[1];

const renderCatalogue = (search = '') =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter
        initialEntries={[`/org/acme/a2a-agents${search}`]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route
            path="/org/:orgId/a2a-agents"
            element={
              <>
                <A2AAgentCatalogue orgId="acme" />
                <SearchProbe />
              </>
            }
          />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );

const searchBox = () => screen.getByPlaceholderText('Search A2A agents');
const button = (name: string) => screen.getByRole('button', { name });
const panel = () =>
  screen.queryByRole('complementary', { name: 'Advanced Filters' });
const combobox = (name: string) => screen.getByRole('combobox', { name });

const openPanel = () => fireEvent.click(button('Advanced Filters'));

const pick = (select: string, option: string) => {
  fireEvent.mouseDown(combobox(select));
  fireEvent.click(
    within(screen.getByRole('listbox')).getByRole('option', { name: option }),
  );
};

beforeEach(() => {
  searchHistory = [];
  vi.mocked(useListA2ACatalog).mockReset();
  respondWithAgents(defaultAgents(), 2);
  withEnvironments([]);
  withProjects([]);
});

describe('grid', () => {
  it('renders one tile per project/agent pair', () => {
    renderCatalogue();

    const tiles = screen.getAllByTestId(TILE_TEST_ID);
    expect(tiles).toHaveLength(2);
    expect(tiles[0]).toHaveAttribute('href', '/org/acme/a2a-agents/view/p1/orders');
    expect(tiles[1]).toHaveAttribute('href', '/org/acme/a2a-agents/view/p2/orders');
    expect(tiles[0]).toHaveTextContent('p1');
  });

  it('shows environment flags and display names', () => {
    withEnvironments([{ name: 'prod', displayName: 'Production' }]);
    renderCatalogue();

    expect(screen.getByText('card differs')).toBeInTheDocument();
    expect(screen.getByText('proxied')).toBeInTheDocument();
    expect(screen.getByText('Production')).toBeInTheDocument();
    expect(screen.queryByText('prod')).not.toBeInTheDocument();
    expect(screen.getAllByText('dev')).toHaveLength(2);
  });

  it('emphasises matched tiles only while searching', () => {
    const [first, second] = defaultAgents();
    const agents = [
      { ...first, environments: [environment({ name: 'dev', matched: true })] },
      second,
    ];
    respondWithAgents(agents, 2);
    const { unmount } = renderCatalogue('?q=x');

    let tiles = screen.getAllByTestId(TILE_TEST_ID);
    expect(tiles[0]).toHaveAttribute('data-emphasised', 'true');
    expect(tiles[1]).toHaveAttribute('data-emphasised', 'false');
    unmount();

    renderCatalogue();
    tiles = screen.getAllByTestId(TILE_TEST_ID);
    expect(tiles[0]).toHaveAttribute('data-emphasised', 'false');
    expect(tiles[1]).toHaveAttribute('data-emphasised', 'false');
  });

  it('renders four skeletons while loading', () => {
    respondWith({ data: undefined, isLoading: true });
    renderCatalogue();

    expect(screen.getAllByTestId('a2a-tile-skeleton')).toHaveLength(4);
    expect(screen.queryByTestId(TILE_TEST_ID)).not.toBeInTheDocument();
  });

  it('renders the error state', () => {
    respondWith({ error: new Error('x') });
    renderCatalogue();

    expect(screen.getByText('error')).toBeInTheDocument();
  });

  it('renders the empty and no-match states', () => {
    respondWithAgents([], 0);
    const { unmount } = renderCatalogue();
    expect(screen.getByText('No A2A agents yet')).toBeInTheDocument();
    unmount();

    renderCatalogue('?status=failed');
    expect(screen.getByText('No A2A agents match your search')).toBeInTheDocument();
  });

  it('pages only beyond 24', () => {
    const { unmount } = renderCatalogue();
    expect(
      screen.queryByRole('button', { name: 'Go to next page' }),
    ).not.toBeInTheDocument();
    unmount();

    respondWithAgents(defaultAgents(), 25);
    renderCatalogue();
    fireEvent.click(button('Go to next page'));

    expect(currentParams().get('page')).toBe('1');
    expect(lastQuery()?.offset).toBe(24);
  });

  it('keeps other params when paging', () => {
    respondWithAgents(defaultAgents(), 25);
    renderCatalogue('?kind=external');

    fireEvent.click(button('Go to next page'));

    expect(currentParams().toString()).toBe('kind=external&page=1');
  });
});

describe('page parsing and clamp', () => {
  it.each([
    ['-1', 0],
    ['1.5', 24],
    ['abc', 0],
    ['', 0],
  ])('?page=%s requests offset %i', (raw, offset) => {
    renderCatalogue(`?page=${raw}`);

    expect(lastQuery()?.offset).toBe(offset);
  });

  it('clamps to the last page when the page is empty', () => {
    respondWithAgents([], 20);
    renderCatalogue('?page=3');

    expect(currentParams().has('page')).toBe(false);
    expect(lastQuery()?.offset).toBe(0);
  });

  it('clamps to page 2 of 50', () => {
    respondWithAgents([], 50);
    renderCatalogue('?page=5');

    expect(currentParams().get('page')).toBe('2');
    expect(lastQuery()?.offset).toBe(48);
  });

  it('never clamps an empty catalogue or page 0', () => {
    respondWithAgents([], 0);
    const { unmount } = renderCatalogue('?page=2');
    expect(searchHistory).toEqual(['?page=2']);
    unmount();

    searchHistory = [];
    respondWithAgents([], 20);
    renderCatalogue();
    expect(searchHistory).toEqual(['']);
  });
});

describe('search', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  const advance = (ms: number) =>
    act(() => {
      vi.advanceTimersByTime(ms);
    });
  const settle = () => advance(300);
  const type = (value: string) =>
    fireEvent.change(searchBox(), { target: { value } });

  it('debounces typing through the real search box', () => {
    renderCatalogue();

    type('refund');
    const queriedRefund = () =>
      vi.mocked(useListA2ACatalog).mock.calls.some(([, query]) => query.q === 'refund');
    expect(queriedRefund()).toBe(false);
    advance(299);
    expect(queriedRefund()).toBe(false);

    advance(1);

    expect(vi.mocked(useListA2ACatalog).mock.lastCall?.[0]).toEqual({ orgName: 'acme' });
    expect(lastQuery()).toEqual({
      kind: undefined,
      environment: undefined,
      project: undefined,
      status: undefined,
      q: 'refund',
      limit: 24,
      offset: 0,
    });
  });

  it('seeds the box from the URL', () => {
    renderCatalogue('?q=old');

    expect(searchBox()).toHaveValue('old');
    expect(vi.mocked(useListA2ACatalog).mock.calls[0][1].q).toBe('old');
  });

  it('trims and drops the page', () => {
    renderCatalogue('?q=old&page=2');

    type('  refund  ');
    settle();

    expect(currentParams().get('q')).toBe('refund');
    expect(currentParams().has('page')).toBe(false);
    expect(lastQuery()?.q).toBe('refund');
  });

  it('removes q when cleared or whitespace', () => {
    for (const value of ['', '   ']) {
      searchHistory = [];
      const { unmount } = renderCatalogue('?q=old');

      type(value);
      settle();

      expect(currentParams().has('q')).toBe(false);
      expect(lastQuery()?.q).toBeUndefined();
      unmount();
    }
  });
});

describe('kind toggle and advanced filters', () => {
  it('applies kind immediately', () => {
    renderCatalogue('?page=1');

    fireEvent.click(button('External'));
    expect(currentParams().get('kind')).toBe('external');
    expect(currentParams().has('page')).toBe(false);
    expect(lastQuery()?.kind).toBe('external');

    fireEvent.click(button('All'));
    expect(currentParams().has('kind')).toBe(false);
    expect(lastQuery()?.kind).toBeUndefined();
  });

  it('applies status only on Filter and removes it from the chip', () => {
    renderCatalogue();

    openPanel();
    pick('Card status', 'Failed');
    expect(lastQuery()?.status).toBeUndefined();

    fireEvent.click(button('Filter'));
    expect(lastQuery()?.status).toBe('failed');
    expect(panel()).not.toBeInTheDocument();

    fireEvent.click(button('Remove Card status: Failed'));
    expect(lastQuery()?.status).toBeUndefined();
  });

  it('offers environment and project options and chips', () => {
    withEnvironments([{ name: 'dev', displayName: 'Development' }]);
    withProjects([{ name: 'p1', displayName: 'Project One' }]);
    renderCatalogue();
    openPanel();

    fireEvent.mouseDown(combobox('Environment'));
    let options = within(screen.getByRole('listbox')).getAllByRole('option');
    expect(options.map((o) => o.textContent)).toEqual(['Any', 'Development']);
    expect(options[1]).toHaveAttribute('data-value', 'dev');
    fireEvent.click(options[1]);

    fireEvent.mouseDown(combobox('Project'));
    options = within(screen.getByRole('listbox')).getAllByRole('option');
    expect(options.map((o) => o.textContent)).toEqual(['Any', 'Project One']);
    expect(options[1]).toHaveAttribute('data-value', 'p1');
    fireEvent.click(options[1]);

    fireEvent.click(button('Filter'));

    expect(currentParams().toString()).toBe('environment=dev&project=p1');
    expect(screen.getByTitle('Environment: Development')).toBeInTheDocument();
    expect(screen.getByTitle('Project: Project One')).toBeInTheDocument();
  });

  it('shows a raw chip for an unknown environment', () => {
    renderCatalogue('?environment=ghost');

    expect(screen.getByTitle('Environment: ghost')).toBeInTheDocument();
  });

  it('shows Clear all for two active filters and clears in one write', () => {
    const { unmount } = renderCatalogue('?status=failed');
    expect(screen.queryByRole('button', { name: 'Clear all' })).not.toBeInTheDocument();
    unmount();

    searchHistory = [];
    renderCatalogue('?status=failed&kind=platform');
    const writesBefore = searchHistory.length;

    fireEvent.click(button('Clear all'));

    expect(searchHistory).toHaveLength(writesBefore + 1);
    expect(currentSearch()).toBe('');
    expect(lastQuery()).toMatchObject({
      kind: undefined,
      environment: undefined,
      project: undefined,
      status: undefined,
    });
  });

  it('discards the draft on Escape and close', () => {
    renderCatalogue();

    openPanel();
    pick('Card status', 'Failed');
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(panel()).not.toBeInTheDocument();
    expect(searchHistory).toEqual(['']);

    openPanel();
    expect(combobox('Card status')).toHaveTextContent('Any');
    pick('Card status', 'Failed');
    fireEvent.click(button('Close advanced filters'));
    expect(panel()).not.toBeInTheDocument();
    expect(searchHistory).toEqual(['']);

    openPanel();
    expect(combobox('Card status')).toHaveTextContent('Any');
  });

  it('resets the draft without writing', () => {
    withEnvironments([{ name: 'dev', displayName: 'Development' }]);
    renderCatalogue('?environment=dev&status=failed');
    openPanel();
    expect(combobox('Environment')).toHaveTextContent('Development');
    expect(combobox('Card status')).toHaveTextContent('Failed');

    fireEvent.click(button('Reset'));

    expect(combobox('Environment')).toHaveTextContent('Any');
    expect(combobox('Project')).toHaveTextContent('Any');
    expect(combobox('Card status')).toHaveTextContent('Any');
    expect(searchHistory).toEqual(['?environment=dev&status=failed']);

    fireEvent.click(button('Filter'));
    expect(currentSearch()).toBe('');
  });

  it('keeps the panel open across a kind change and keeps the new kind on Filter', () => {
    renderCatalogue();
    openPanel();
    pick('Card status', 'Failed');

    fireEvent.click(button('Proxied'));
    expect(currentParams().get('kind')).toBe('proxied');
    expect(panel()).toBeInTheDocument();
    expect(combobox('Card status')).toHaveTextContent('Failed');

    fireEvent.click(button('Filter'));
    expect(currentParams().toString()).toBe('kind=proxied&status=failed');
    expect(lastQuery()).toMatchObject({ kind: 'proxied', status: 'failed' });
  });

  it('removes a chip while open and drops it from the draft', () => {
    renderCatalogue();
    openPanel();
    pick('Card status', 'Failed');
    fireEvent.click(button('Filter'));

    fireEvent.click(screen.getByTitle('Card status: Failed'));
    expect(panel()).toBeInTheDocument();
    fireEvent.click(button('Remove Card status: Failed'));

    expect(currentParams().has('status')).toBe(false);
    expect(combobox('Card status')).toHaveTextContent('Any');
    fireEvent.click(button('Filter'));
    expect(lastQuery()?.status).toBeUndefined();
  });

  it('removes page with the chip', () => {
    renderCatalogue('?kind=platform&status=failed&page=1');

    fireEvent.click(button('Remove Card status: Failed'));

    expect(currentSearch()).toBe('?kind=platform');
  });

  it('wires aria-expanded and aria-controls', () => {
    renderCatalogue();
    const filters = button('Advanced Filters');
    expect(filters).toHaveAttribute('aria-expanded', 'false');

    fireEvent.click(filters);

    expect(filters).toHaveAttribute('aria-expanded', 'true');
    expect(filters).toHaveAttribute('aria-controls', A2A_FILTER_PANEL_ID);
    expect(panel()).toHaveAttribute('id', A2A_FILTER_PANEL_ID);
  });
});
