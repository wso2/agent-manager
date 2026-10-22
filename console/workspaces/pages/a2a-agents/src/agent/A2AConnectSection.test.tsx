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
import { MemoryRouter } from 'react-router-dom';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@wso2/oxygen-ui';
import { useGetA2AConnect, useIssueTestAgentAPIKey } from '@agent-management-platform/api-client';
import {
  EMPTY_A2A_CARD,
  parseA2ACard,
  type ParsedA2ACard,
} from '@agent-management-platform/shared-component';
import type {
  A2ACatalogAgent,
  A2AConnectResponse,
  IssueTestAgentAPIKeyResponse,
} from '@agent-management-platform/types';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { A2AConnectSection } from './A2AConnectSection';
import { UNMANAGED_COMMENT } from './quickStartSnippets';

const idleMutation = { mutate: vi.fn(), isPending: false, isError: false, isIdle: true };

vi.mock('@agent-management-platform/api-client', () => ({
  useGetA2AConnect: vi.fn(),
  useIssueTestAgentAPIKey: vi.fn(() => idleMutation),
  useTrack: () => ({ track: vi.fn() }),
  ConsoleAction: {},
}));

vi.mock('@agent-management-platform/shared-component', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@agent-management-platform/shared-component')>()),
  CodeBlock: ({ code }: { code: string }) => <pre data-testid="code-block">{code}</pre>,
}));

const entry = (overrides: Partial<A2ACatalogAgent> = {}): A2ACatalogAgent => ({
  projectName: 'p1',
  agentName: 'orders',
  kind: 'platform',
  name: 'Orders',
  description: '',
  version: '1.0.0',
  skills: [],
  leadEnvironment: 'dev',
  environments: [],
  ...overrides,
});

const connect = (overrides: Partial<A2AConnectResponse> = {}): A2AConnectResponse => ({
  kind: 'platform',
  proxied: false,
  isProduction: false,
  gatewayUrl: 'https://gw.example/orders',
  cardUrl: 'https://gw.example/orders/.well-known/agent-card.json',
  jsonRpcUrl: 'https://gw.example/orders/rpc',
  security: { mode: 'apiKey', apiKeyHeader: 'X-API-Key' },
  publication: { status: 'published', lastError: '' },
  ...overrides,
});

const ORIGIN_RPC = 'http://localhost:9099/rpc';

const defaultCard = (): ParsedA2ACard =>
  parseA2ACard({
    supportedInterfaces: [{ url: ORIGIN_RPC, protocolBinding: 'JSONRPC' }],
    securitySchemes: { bearer: {} },
  });

interface ConnectQueryState {
  data?: A2AConnectResponse;
  isLoading?: boolean;
  isError?: boolean;
}

function mockConnect({ data, isLoading = false, isError = false }: ConnectQueryState) {
  vi.mocked(useGetA2AConnect).mockReturnValue({
    data,
    isLoading,
    isError,
  } as unknown as ReturnType<typeof useGetA2AConnect>);
}

function mockIssuing(apiKey: string) {
  const issued: IssueTestAgentAPIKeyResponse = {
    status: 'ok',
    apiKey,
    expiresAt: new Date(Date.now() + 10 * 60 * 1000).toISOString(),
  };
  const mutation = {
    mutate: vi.fn(
      (_: unknown, opts?: { onSuccess?: (data: IssueTestAgentAPIKeyResponse) => void }) =>
        opts?.onSuccess?.(issued),
    ),
    isPending: false,
    isError: false,
    isIdle: true,
  };
  vi.mocked(useIssueTestAgentAPIKey).mockReturnValue(
    mutation as unknown as ReturnType<typeof useIssueTestAgentAPIKey>,
  );
  return mutation;
}

interface RenderOptions {
  entry?: A2ACatalogAgent;
  environment?: string;
  card?: ParsedA2ACard;
}

function renderSection({
  entry: agent = entry(),
  environment = 'dev',
  card = defaultCard(),
}: RenderOptions = {}) {
  const tree = (): ReactElement => (
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <ThemeProvider theme={createTheme()}>
        <A2AConnectSection orgId="acme" entry={agent} environment={environment} card={card} />
      </ThemeProvider>
    </MemoryRouter>
  );
  const view = render(tree());
  return { rerender: () => view.rerender(tree()) };
}

const code = () => screen.getByTestId('code-block').textContent ?? '';

describe('A2AConnectSection', () => {
  beforeEach(() => {
    vi.mocked(useGetA2AConnect).mockReset();
    idleMutation.mutate.mockReset();
    vi.mocked(useIssueTestAgentAPIKey).mockReturnValue(
      idleMutation as unknown as ReturnType<typeof useIssueTestAgentAPIKey>,
    );
  });

  it('shows a spinner while loading', () => {
    mockConnect({ isLoading: true });
    renderSection();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    expect(screen.queryByText('Endpoint')).not.toBeInTheDocument();
  });

  it('asks for the selected environment connect details', () => {
    mockConnect({ isLoading: true });
    renderSection();
    expect(useGetA2AConnect).toHaveBeenCalledWith({
      orgName: 'acme',
      projName: 'p1',
      agentName: 'orders',
      envId: 'dev',
    });
  });

  it('shows the error copy without data', () => {
    mockConnect({ isError: true });
    renderSection();
    expect(
      screen.getByText("Couldn't load connection details for this environment."),
    ).toBeInTheDocument();
    expect(screen.queryByText('Endpoint')).not.toBeInTheDocument();
  });

  it('keeps the loaded body and key on a failed refetch', () => {
    const data = connect();
    mockConnect({ data });
    mockIssuing('k1');
    const { rerender } = renderSection();
    fireEvent.click(screen.getByRole('button', { name: 'Issue test key' }));
    expect(screen.getByDisplayValue('k1')).toBeInTheDocument();
    expect(code()).toContain('k1');
    expect(
      screen.queryByText("Couldn't refresh; showing the last loaded details."),
    ).not.toBeInTheDocument();

    mockConnect({ data, isError: true });
    rerender();

    expect(screen.getByDisplayValue('https://gw.example/orders')).toBeInTheDocument();
    expect(screen.getByText(/API key required in header/)).toBeInTheDocument();
    expect(screen.getByDisplayValue('k1')).toBeInTheDocument();
    expect(code()).toContain('k1');
    expect(
      screen.getByText("Couldn't refresh; showing the last loaded details."),
    ).toBeInTheDocument();
  });

  it('describes API-key security outside production', () => {
    mockConnect({ data: connect() });
    renderSection();
    expect(screen.getByText(/API key required in header/)).toBeInTheDocument();
    expect(screen.getByText('X-API-Key', { selector: 'code' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Issue test key' })).toBeInTheDocument();
    expect(idleMutation.mutate).not.toHaveBeenCalled();
    expect(screen.queryByLabelText('Test API key')).not.toBeInTheDocument();
    expect(code()).toContain('<YOUR_API_KEY>');
  });

  it('says production keys come from the owners', () => {
    mockConnect({ data: connect({ isProduction: true }) });
    renderSection();
    expect(
      screen.getByText("Keys for production are issued by the agent's owners."),
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Issue test key' })).not.toBeInTheDocument();
    expect(code()).toContain('<YOUR_API_KEY>');
  });

  it('describes OAuth', () => {
    const oauth = {
      issuers: [
        { name: 'corp-idp', issuer: 'https://idp/x' },
        { name: 'gone-idp', issuer: '' },
      ],
      audiences: ['orders-api'],
      headerName: 'Authorization',
      authHeaderPrefix: 'Bearer',
    };
    mockConnect({ data: connect({ security: { mode: 'oauth', oauth } }) });
    const { rerender } = renderSection();

    const sentence = screen.getByText(/OAuth access token required in header/);
    expect(sentence).toHaveTextContent(
      'OAuth access token required in header Authorization with prefix Bearer',
    );
    expect(screen.getByText('Issuers:')).toBeInTheDocument();
    expect(screen.getByText('corp-idp — https://idp/x')).toBeInTheDocument();
    expect(screen.getByText('gone-idp — not found')).toBeInTheDocument();
    expect(screen.getByText('Audiences: orders-api')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Issue test key' })).not.toBeInTheDocument();
    expect(code()).toContain('Authorization');
    expect(code()).toContain('Bearer <ACCESS_TOKEN>');

    mockConnect({
      data: connect({ security: { mode: 'oauth', oauth: { ...oauth, audiences: [] } } }),
    });
    rerender();
    expect(screen.getByText('Audiences: any')).toBeInTheDocument();
  });

  it('survives OAuth without an oauth object', () => {
    mockConnect({ data: connect({ security: { mode: 'oauth' } }) });
    renderSection();
    expect(screen.getByText(/OAuth access token required in header/)).toHaveTextContent(
      'OAuth access token required in header Authorization',
    );
    expect(screen.queryByText(/with prefix/)).not.toBeInTheDocument();
    expect(screen.getByText('Issuers:')).toBeInTheDocument();
    expect(screen.getByText('Audiences: any')).toBeInTheDocument();
    expect(code()).toContain('<ACCESS_TOKEN>');
  });

  it('describes no, unknown and unmanaged security', () => {
    mockConnect({ data: connect({ security: { mode: 'none' } }) });
    const { rerender } = renderSection();
    expect(screen.getByText('No authentication required.')).toBeInTheDocument();
    expect(code()).not.toContain('X-API-Key');

    mockConnect({ data: connect({ security: { mode: 'unknown' } }) });
    rerender();
    expect(
      screen.getByText("Security settings couldn't be determined for this environment."),
    ).toBeInTheDocument();
    for (const language of ['Python', 'TypeScript', 'curl']) {
      fireEvent.click(screen.getByRole('button', { name: language }));
      expect(code()).not.toContain("does not manage this agent's security");
    }
  });

  it('describes a direct unmanaged agent', () => {
    const direct = connect({
      kind: 'external',
      gatewayUrl: '',
      cardUrl: 'http://localhost:9099/.well-known/agent-card.json',
      jsonRpcUrl: ORIGIN_RPC,
      security: { mode: 'unmanaged' },
      publication: { status: 'none', lastError: '' },
    });
    mockConnect({ data: direct });
    renderSection({ entry: entry({ kind: 'external' }) });

    expect(
      screen.getByText("Security isn't managed by the platform. The card declares: bearer."),
    ).toBeInTheDocument();
    expect(screen.queryByText('Gateway endpoint')).not.toBeInTheDocument();
    expect(screen.getAllByRole('textbox')[0]).toHaveValue(ORIGIN_RPC);
    expect(code()).not.toContain('X-API-Key');
    expect(code()).toContain(UNMANAGED_COMMENT);
  });

  it('says an unmanaged card declaring no schemes declares nothing', () => {
    mockConnect({ data: connect({ gatewayUrl: '', security: { mode: 'unmanaged' } }) });
    renderSection({ card: EMPTY_A2A_CARD });
    expect(
      screen.getByText("Security isn't managed by the platform. The card declares: nothing."),
    ).toBeInTheDocument();
  });

  it('links Manage security per kind', () => {
    mockConnect({ data: connect() });
    renderSection();
    expect(screen.getByRole('link', { name: 'Manage security' })).toHaveAttribute(
      'href',
      '/org/acme/project/p1/agents/orders/deployment',
    );

    cleanup();
    mockConnect({ data: connect({ kind: 'external' }) });
    renderSection({ entry: entry({ kind: 'external' }), environment: 'dev' });
    expect(screen.getByRole('link', { name: 'Manage security' })).toHaveAttribute(
      'href',
      '/org/acme/project/p1/agents/orders/configure?tab=a2a&env=dev',
    );
  });

  it('links Manage security before the details load', () => {
    mockConnect({ isLoading: true });
    renderSection();
    expect(screen.getByRole('link', { name: 'Manage security' })).toBeInTheDocument();
  });

  it('warns about pending or failed publication', () => {
    mockConnect({
      data: connect({ publication: { status: 'failed', lastError: 'gateway rejected the policy' } }),
    });
    const { rerender } = renderSection();
    expect(screen.getByRole('alert')).toHaveTextContent(
      'The gateway may not yet enforce these settings. gateway rejected the policy',
    );

    mockConnect({ data: connect({ publication: { status: 'pending', lastError: '' } }) });
    rerender();
    expect(screen.getByRole('alert')).toHaveTextContent(
      /^The gateway may not yet enforce these settings\.$/,
    );

    for (const status of ['published', 'none'] as const) {
      mockConnect({ data: connect({ publication: { status, lastError: 'stale' } }) });
      rerender();
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
      expect(
        screen.queryByText(/The gateway may not yet enforce these settings/),
      ).not.toBeInTheDocument();
    }
  });

  it('addresses a proxied agent at the gateway', () => {
    mockConnect({
      data: connect({
        kind: 'external',
        proxied: true,
        gatewayUrl: 'https://gw/agents/x',
        cardUrl: 'https://gw/agents/x/.well-known/agent-card.json',
        jsonRpcUrl: 'https://gw/agents/x/rpc',
      }),
    });
    renderSection({ entry: entry({ kind: 'external' }) });

    expect(screen.getAllByRole('textbox')[0]).toHaveValue('https://gw/agents/x');
    expect(screen.getByText('Declared by the card')).toBeInTheDocument();
    expect(screen.getByText(`${ORIGIN_RPC} · JSONRPC`)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'curl' }));
    expect(code()).toContain('https://gw/agents/x');
    expect(code()).not.toContain(ORIGIN_RPC);
  });

  it('replaces the quick start when there is no JSON-RPC endpoint', () => {
    mockConnect({ data: connect({ jsonRpcUrl: '' }) });
    renderSection();
    expect(screen.getByText('Quick start')).toBeInTheDocument();
    expect(
      screen.getByText('This agent declares no JSON-RPC endpoint, so there is no quick start.'),
    ).toBeInTheDocument();
    expect(screen.queryByTestId('code-block')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Python' })).not.toBeInTheDocument();
  });

  it('says when a direct card declares no endpoints', () => {
    mockConnect({ data: connect({ gatewayUrl: '', cardUrl: '', jsonRpcUrl: '' }) });
    renderSection({ card: EMPTY_A2A_CARD });
    expect(screen.getByText('The card declares no endpoints yet.')).toBeInTheDocument();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
  });

  it('shows the Agent card row only with a card URL', () => {
    mockConnect({ data: connect() });
    const { rerender } = renderSection();
    expect(screen.getByLabelText('Agent card')).toHaveValue(
      'https://gw.example/orders/.well-known/agent-card.json',
    );

    mockConnect({ data: connect({ cardUrl: '' }) });
    rerender();
    expect(screen.queryByLabelText('Agent card')).not.toBeInTheDocument();
    expect(screen.queryByText('Agent card')).not.toBeInTheDocument();
  });

  it('switches snippet languages', () => {
    mockConnect({ data: connect() });
    renderSection();
    const python = screen.getByRole('button', { name: 'Python' });
    expect(screen.getByRole('button', { name: 'TypeScript' })).toBeInTheDocument();
    expect(python).toHaveAttribute('aria-pressed', 'true');
    expect(code()).toMatch(/^# pip install a2a-sdk/);

    const curl = screen.getByRole('button', { name: 'curl' });
    fireEvent.click(curl);
    expect(curl).toHaveAttribute('aria-pressed', 'true');
    expect(code()).toMatch(/^# Fetch the agent card/);

    fireEvent.click(curl);
    expect(curl).toHaveAttribute('aria-pressed', 'true');
    expect(code()).toMatch(/^# Fetch the agent card/);
  });
});
