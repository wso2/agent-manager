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
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { ThemeProvider, createTheme } from '@wso2/oxygen-ui';
import {
  useGetA2ACatalogEntry,
  useGetA2AConnect,
  useGetAgentCard,
  useIssueTestAgentAPIKey,
} from '@agent-management-platform/api-client';
import type {
  A2ACatalogAgent,
  A2ACatalogEnvironment,
  A2AConnectResponse,
  AgentCardResponse,
  IssueTestAgentAPIKeyResponse,
} from '@agent-management-platform/types';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { A2AAgentPage } from './A2AAgentPage';

vi.mock('@agent-management-platform/api-client', () => ({
  useGetA2ACatalogEntry: vi.fn(),
  useGetAgentCard: vi.fn(),
  useGetA2AConnect: vi.fn(),
  useIssueTestAgentAPIKey: vi.fn(),
  useListEnvironments: vi.fn(() => ({
    data: [
      { name: 'dev', displayName: 'Development' },
      { name: 'prod', displayName: 'Production', isProduction: true },
      { name: 'staging' },
    ],
  })),
  useTrack: () => ({ track: vi.fn() }),
  ConsoleAction: {},
}));

vi.mock('@agent-management-platform/shared-component', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@agent-management-platform/shared-component')>()),
  CodeBlock: ({ code }: { code: string }) => <pre data-testid="code-block">{code}</pre>,
}));

const PAGE_PATH = '/org/:orgId/a2a-agents/view/:projectName/:agentName';
const DEFAULT_URL = '/org/acme/a2a-agents/view/p1/orders';

const environment = (
  name: string,
  status: A2ACatalogEnvironment['status'],
): A2ACatalogEnvironment => ({
  name,
  status,
  fetchedAt: status === 'fetched' ? new Date().toISOString() : undefined,
  proxied: false,
  cardDiffers: false,
  matched: false,
});

const entry = (overrides: Partial<A2ACatalogAgent> = {}): A2ACatalogAgent => ({
  projectName: 'p1',
  agentName: 'orders',
  kind: 'platform',
  name: 'Orders',
  description: '',
  version: '1.0',
  skills: [],
  leadEnvironment: 'dev',
  environments: [
    environment('dev', 'fetched'),
    environment('prod', 'fetched'),
    environment('staging', 'pending'),
  ],
  ...overrides,
});

interface QueryState<T> {
  data: T | null | undefined;
  isLoading: boolean;
  isError: boolean;
}

const loadingCard: QueryState<AgentCardResponse> = {
  data: undefined,
  isLoading: true,
  isError: false,
};

const fetchedCard = (
  card: Record<string, unknown>,
  overrides: Partial<AgentCardResponse> = {},
): QueryState<AgentCardResponse> => ({
  data: {
    source: 'platform',
    status: 'fetched',
    sourceUrl: '',
    lastError: '',
    fetchedAt: new Date().toISOString(),
    card,
    ...overrides,
  },
  isLoading: false,
  isError: false,
});

const skillCard = (skill: string, extra: Record<string, unknown> = {}) =>
  fetchedCard({ skills: [{ id: skill, name: skill }], ...extra });

let cards: Record<string, QueryState<AgentCardResponse>> = {};

const connectFor = (envId: string): A2AConnectResponse => ({
  kind: 'platform',
  proxied: false,
  isProduction: envId === 'prod',
  gatewayUrl: `https://gw.example/${envId}/orders`,
  cardUrl: `https://gw.example/${envId}/orders/.well-known/agent-card.json`,
  jsonRpcUrl: `https://gw.example/${envId}/orders/rpc`,
  security: { mode: 'apiKey', apiKeyHeader: 'X-API-Key' },
  publication: { status: 'published', lastError: '' },
});

function mockEntry(state: Partial<QueryState<A2ACatalogAgent>>) {
  vi.mocked(useGetA2ACatalogEntry).mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
    ...state,
  } as unknown as ReturnType<typeof useGetA2ACatalogEntry>);
}

const issued: IssueTestAgentAPIKeyResponse = {
  status: 'ok',
  apiKey: 'k1',
  expiresAt: new Date(Date.now() + 10 * 60 * 1000).toISOString(),
};

let mutate: ReturnType<typeof vi.fn>;

const LocationProbe = () => <div data-testid="location">{useLocation().search}</div>;

function renderPage(url = DEFAULT_URL) {
  const tree = (): ReactElement => (
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter
        initialEntries={[url]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route
            path={PAGE_PATH}
            element={
              <>
                <A2AAgentPage />
                <LocationProbe />
              </>
            }
          />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );
  const view = render(tree());
  return { rerender: () => view.rerender(tree()) };
}

const location = () => screen.getByTestId('location').textContent;

/** The page title, without the chips that share its heading. */
const pageTitle = () => screen.getAllByRole('heading')[0].firstChild?.firstChild?.textContent;

const chip = (label: string) =>
  screen.getByText(label, { selector: '.MuiChip-label' }).closest('.MuiChip-root') as HTMLElement;

const clickChip = (label: string) => fireEvent.click(chip(label));

/** The page's own card requests; the compare view's carry `poll: false`. */
const pageCardCalls = () =>
  vi.mocked(useGetAgentCard).mock.calls.filter(([, options]) => options?.poll === undefined);

const openConnect = () => fireEvent.click(screen.getByRole('button', { name: 'Connect' }));

const issueKey = () => fireEvent.click(screen.getByRole('button', { name: 'Issue test key' }));

const codeBlocks = () => screen.getAllByTestId('code-block').map((block) => block.textContent);

const pressEscapeOnDrawer = () =>
  fireEvent.keyDown(screen.getByRole('presentation'), { key: 'Escape' });

const drawerHeading = (environmentLabel: string) =>
  screen.queryByRole('heading', { name: `Connect · ${environmentLabel}` });

function selectCompare(label: string) {
  fireEvent.mouseDown(screen.getByRole('combobox'));
  fireEvent.click(within(screen.getByRole('listbox')).getByText(label));
}

beforeEach(() => {
  vi.clearAllMocks();
  cards = {
    dev: skillCard('order-status', { name: 'Orders' }),
    prod: skillCard('issue-refund', { name: 'Orders' }),
  };
  mockEntry({ data: entry() });
  vi.mocked(useGetAgentCard).mockImplementation(
    (params) =>
      (cards[params.envId] ?? loadingCard) as unknown as ReturnType<typeof useGetAgentCard>,
  );
  vi.mocked(useGetA2AConnect).mockImplementation(
    (params) =>
      ({
        data: connectFor(params.envId),
        isLoading: false,
        isError: false,
      }) as unknown as ReturnType<typeof useGetA2AConnect>,
  );
  mutate = vi.fn(
    (_: unknown, opts?: { onSuccess?: (data: IssueTestAgentAPIKeyResponse) => void }) =>
      opts?.onSuccess?.(issued),
  );
  vi.mocked(useIssueTestAgentAPIKey).mockReturnValue({
    mutate,
    isPending: false,
    isError: false,
    isIdle: true,
  } as unknown as ReturnType<typeof useIssueTestAgentAPIKey>);
});

describe('A2AAgentPage', () => {
  describe('shell', () => {
    it('reads the agent from the URL', () => {
      renderPage();

      expect(useGetA2ACatalogEntry).toHaveBeenCalledWith({
        orgName: 'acme',
        projName: 'p1',
        agentName: 'orders',
      });
      expect(pageTitle()).toBe('Orders');
      expect(screen.getByTestId('a2a-agent-page')).toBeInTheDocument();
    });

    it('shows no claims while loading', () => {
      mockEntry({ data: undefined, isLoading: true });
      renderPage();

      expect(screen.queryByText('platform')).not.toBeInTheDocument();
      expect(screen.queryByText('external')).not.toBeInTheDocument();
      expect(screen.queryByText('order-status')).not.toBeInTheDocument();
      expect(screen.queryByText('Loading card…')).not.toBeInTheDocument();
    });

    it('says when the agent is not in the catalogue', () => {
      mockEntry({ data: null });
      renderPage();

      expect(screen.getByText('This agent was not found in the catalogue.')).toBeInTheDocument();
      expect(pageTitle()).toBe('orders');
    });

    it('shows the entry error without data', () => {
      mockEntry({ data: undefined, isError: true });
      renderPage();

      expect(screen.getByText('Failed to load the agent. Please try again.')).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Connect' })).not.toBeInTheDocument();
      expect(screen.queryByText('Development')).not.toBeInTheDocument();
      expect(screen.queryByText('Loading card…')).not.toBeInTheDocument();
      expect(screen.queryByText('order-status')).not.toBeInTheDocument();
    });

    it('keeps the page and the issued key through a failed entry refetch', () => {
      const { rerender } = renderPage();
      openConnect();
      issueKey();
      expect(screen.getByDisplayValue('k1')).toBeInTheDocument();
      expect(codeBlocks().some((code) => code?.includes('k1'))).toBe(true);

      mockEntry({ data: entry(), isError: true });
      rerender();

      expect(screen.getByDisplayValue('k1')).toBeInTheDocument();
      expect(codeBlocks().some((code) => code?.includes('k1'))).toBe(true);
      expect(screen.getByText('order-status')).toBeInTheDocument();
      expect(
        screen.getByText("Couldn't refresh; showing the last loaded details."),
      ).toBeInTheDocument();
      expect(
        screen.queryByText('Failed to load the agent. Please try again.'),
      ).not.toBeInTheDocument();
    });

    it('opens on the lead environment', () => {
      renderPage();

      expect(chip('Development')).toHaveClass('MuiChip-colorPrimary');
      expect(chip('Production')).not.toHaveClass('MuiChip-colorPrimary');
      expect(pageCardCalls().at(-1)).toEqual([
        { orgName: 'acme', projName: 'p1', agentName: 'orders', envId: 'dev' },
        { enabled: true },
      ]);
    });

    it("opens on the URL's environment and switches from the chips", () => {
      renderPage(`${DEFAULT_URL}?env=prod&foo=1`);

      expect(screen.getByText('issue-refund')).toBeInTheDocument();

      clickChip('Development');

      expect(location()).toBe('?env=dev&foo=1');
      expect(screen.queryByText('issue-refund')).not.toBeInTheDocument();
      expect(screen.getByText('order-status')).toBeInTheDocument();
    });

    it('falls back to the lead for an unknown env', () => {
      renderPage(`${DEFAULT_URL}?env=ghost`);

      expect(screen.getByText('order-status')).toBeInTheDocument();
      expect(chip('Development')).toHaveClass('MuiChip-colorPrimary');
      expect(location()).toBe('?env=ghost');
    });

    it('labels chips with display names', () => {
      renderPage();

      expect(chip('Development')).toBeInTheDocument();
      expect(chip('Production')).toBeInTheDocument();
      expect(chip('staging')).toBeInTheDocument();
    });

    it('takes title and version from the ready card', () => {
      cards.dev = fetchedCard({ name: 'Orders Agent', version: '2.1' });
      const { rerender } = renderPage();

      expect(pageTitle()).toBe('Orders Agent');
      expect(screen.getByText('v2.1')).toBeInTheDocument();

      cards.dev = fetchedCard({}, { card: undefined });
      rerender();

      expect(pageTitle()).toBe('Orders');
      expect(screen.getByText('v1.0')).toBeInTheDocument();
    });

    it('labels the kind from the card response', () => {
      cards.dev = fetchedCard({}, { source: 'external' });
      const { rerender } = renderPage();

      expect(screen.getByText('external')).toBeInTheDocument();

      cards = {};
      rerender();

      expect(screen.getByText('platform')).toBeInTheDocument();
      expect(screen.queryByText('external')).not.toBeInTheDocument();
    });

    it('describes the project and provider', () => {
      cards.dev = fetchedCard({ provider: { organization: 'Acme' } });
      const { rerender } = renderPage();

      expect(screen.getByText('p1 · Acme')).toBeInTheDocument();

      cards.dev = fetchedCard({});
      rerender();

      expect(screen.getByText('p1')).toBeInTheDocument();
      expect(screen.queryByText('p1 · Acme')).not.toBeInTheDocument();
    });

    it('links back to the catalogue', () => {
      renderPage();

      expect(screen.getByRole('link', { name: /Back to A2A Agents/ })).toHaveAttribute(
        'href',
        '/org/acme/a2a-agents',
      );
    });

    it('waits for the entry before requesting the card', () => {
      mockEntry({ data: undefined, isLoading: true });
      renderPage();

      expect(useGetAgentCard).toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({ enabled: false }),
      );
      expect(useGetAgentCard).not.toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({ enabled: true }),
      );
    });

    it('renders only the header for an entry without environments', () => {
      mockEntry({ data: entry({ environments: [], leadEnvironment: '' }) });
      renderPage();

      expect(pageTitle()).toBe('Orders');
      expect(screen.queryByRole('button', { name: 'Connect' })).not.toBeInTheDocument();
      expect(document.querySelector('.MuiChip-clickable')).toBeNull();
      expect(useGetAgentCard).not.toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({ enabled: true }),
      );
    });
  });

  describe('connect drawer', () => {
    it('mounts the connect section on click only', () => {
      renderPage();

      expect(useGetA2AConnect).not.toHaveBeenCalled();

      openConnect();

      expect(drawerHeading('Development')).toBeInTheDocument();
      expect(useGetA2AConnect).toHaveBeenCalledWith(expect.objectContaining({ envId: 'dev' }));
    });

    it('keeps the drawer content mounted after closing', async () => {
      renderPage();
      openConnect();
      issueKey();

      pressEscapeOnDrawer();

      await waitFor(() => expect(drawerHeading('Development')).not.toBeInTheDocument());
      expect(screen.getByDisplayValue('k1')).toBeInTheDocument();

      openConnect();

      expect(drawerHeading('Development')).toBeInTheDocument();
      expect(screen.getByDisplayValue('k1')).toBeInTheDocument();
    });

    it('works while the selected card is absent', () => {
      cards.dev = { data: null, isLoading: false, isError: false };
      renderPage();

      openConnect();

      expect(screen.getByText('Gateway endpoint')).toBeInTheDocument();
      expect(screen.queryByText(/Security isn't managed/)).not.toBeInTheDocument();
    });

    it('remounts the details on an environment change', () => {
      cards.staging = skillCard('ship-order');
      renderPage();
      selectCompare('Production');
      expect(screen.getByRole('combobox')).toHaveTextContent('Production');
      openConnect();
      expect(drawerHeading('Development')).toBeInTheDocument();

      clickChip('staging');

      expect(screen.getByText('ship-order')).toBeInTheDocument();
      expect(screen.getByRole('combobox')).toHaveTextContent('None');
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(screen.queryByRole('presentation')).not.toBeInTheDocument();
    });

    it('resets the issued key on an environment change', () => {
      renderPage();
      openConnect();
      issueKey();
      expect(screen.getByDisplayValue('k1')).toBeInTheDocument();

      clickChip('staging');
      openConnect();

      expect(drawerHeading('staging')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Issue test key' })).toBeInTheDocument();
      expect(screen.queryByDisplayValue('k1')).not.toBeInTheDocument();
      expect(mutate).toHaveBeenCalledTimes(1);

      clickChip('Development');
      openConnect();

      expect(screen.getByRole('button', { name: 'Issue test key' })).toBeInTheDocument();
      expect(screen.queryByDisplayValue('k1')).not.toBeInTheDocument();
    });

    it('drops the key for a production environment', () => {
      renderPage();
      openConnect();
      issueKey();

      clickChip('Production');
      openConnect();

      expect(drawerHeading('Production')).toBeInTheDocument();
      expect(codeBlocks().some((code) => code?.includes('<YOUR_API_KEY>'))).toBe(true);
      expect(codeBlocks().some((code) => code?.includes('k1'))).toBe(false);
      expect(screen.queryByRole('button', { name: 'Issue test key' })).not.toBeInTheDocument();

      clickChip('Development');
      openConnect();

      expect(screen.queryByDisplayValue('k1')).not.toBeInTheDocument();
    });
  });

  describe('keyboard', () => {
    it('Esc closes only the compare menu', async () => {
      renderPage();
      fireEvent.mouseDown(screen.getByRole('combobox'));
      const listbox = screen.getByRole('listbox');

      fireEvent.keyDown(listbox, { key: 'Escape' });

      await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument());
      expect(location()).toBe('');
      expect(pageTitle()).toBe('Orders');
      expect(screen.getByText('order-status')).toBeInTheDocument();
    });

    it('Esc closes only the drawer', async () => {
      renderPage();
      openConnect();

      pressEscapeOnDrawer();

      await waitFor(() => expect(drawerHeading('Development')).not.toBeInTheDocument());
      expect(location()).toBe('');
      expect(pageTitle()).toBe('Orders');
    });
  });
});
