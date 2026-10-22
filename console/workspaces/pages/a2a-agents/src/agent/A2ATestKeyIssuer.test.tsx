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
import { useIssueTestAgentAPIKey } from '@agent-management-platform/api-client';
import type { IssueTestAgentAPIKeyResponse } from '@agent-management-platform/types';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { A2ATestKeyIssuer } from './A2ATestKeyIssuer';

vi.mock('@agent-management-platform/api-client', () => ({
  useIssueTestAgentAPIKey: vi.fn(),
}));

const GATEWAY_ALERT =
  'The gateway is not connected to the control plane right now. The API key has been stored but will only work once the gateway reconnects.';

const params = { orgName: 'acme', projName: 'p1', agentName: 'orders', envId: 'dev' };

const tenMinutesAhead = () => new Date(Date.now() + 10 * 60 * 1000 + 5000).toISOString();

const response = (
  overrides: Partial<IssueTestAgentAPIKeyResponse> = {},
): IssueTestAgentAPIKeyResponse => ({
  status: 'ok',
  apiKey: 'k1',
  expiresAt: tenMinutesAhead(),
  ...overrides,
});

interface MockMutationOptions {
  isPending?: boolean;
  isError?: boolean;
  result?: IssueTestAgentAPIKeyResponse | 'fail';
}

type MutateOptions = {
  onSuccess?: (data: IssueTestAgentAPIKeyResponse) => void;
  onError?: (error: Error) => void;
};

function mockMutation({
  isPending = false,
  isError = false,
  result = response(),
}: MockMutationOptions = {}) {
  const mutate = vi.fn((_: unknown, opts?: MutateOptions) => {
    if (result === 'fail') opts?.onError?.(new Error('x'));
    else opts?.onSuccess?.(result);
  });
  const mutation = { mutate, isPending, isError, isIdle: !isPending && !isError };
  vi.mocked(useIssueTestAgentAPIKey).mockReturnValue(
    mutation as unknown as ReturnType<typeof useIssueTestAgentAPIKey>,
  );
  return mutation;
}

const wrap = (element: ReactElement) => (
  <ThemeProvider theme={createTheme()}>{element}</ThemeProvider>
);

function renderIssuer() {
  const onKeyChange = vi.fn();
  const view = render(wrap(<A2ATestKeyIssuer params={params} onKeyChange={onKeyChange} />));
  const rerender = () =>
    view.rerender(wrap(<A2ATestKeyIssuer params={params} onKeyChange={onKeyChange} />));
  return { onKeyChange, rerender };
}

const lastCall = (fn: ReturnType<typeof vi.fn>) => fn.mock.calls[fn.mock.calls.length - 1]?.[0];

describe('A2ATestKeyIssuer', () => {
  beforeEach(() => {
    vi.mocked(useIssueTestAgentAPIKey).mockReset();
  });

  it('issues on click only and reports the key', () => {
    const { mutate } = mockMutation();
    const { onKeyChange } = renderIssuer();
    expect(mutate).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Issue test key' }));

    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate).toHaveBeenCalledWith(params, expect.anything());
    expect(lastCall(onKeyChange)).toBe('k1');
    expect(screen.getByLabelText('Test API key')).toHaveValue('k1');
    expect(screen.getByDisplayValue('k1')).toBeInTheDocument();
    expect(screen.getByText(/It expires in 10 minutes\./)).toBeInTheDocument();
  });

  it('reads Issuing… while the first issue is pending', () => {
    mockMutation({ isPending: true });
    renderIssuer();
    expect(screen.getByRole('button', { name: 'Issuing…' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Issue test key' })).not.toBeInTheDocument();
  });

  it('warns when the gateway is disconnected', () => {
    mockMutation({ result: response({ gatewayConnected: false }) });
    renderIssuer();
    fireEvent.click(screen.getByRole('button', { name: 'Issue test key' }));
    expect(screen.getByText(GATEWAY_ALERT)).toBeInTheDocument();
  });

  it.each([true, undefined])('does not warn when gatewayConnected is %s', (gatewayConnected) => {
    mockMutation({ result: response({ gatewayConnected }) });
    renderIssuer();
    fireEvent.click(screen.getByRole('button', { name: 'Issue test key' }));
    expect(screen.getByDisplayValue('k1')).toBeInTheDocument();
    expect(screen.queryByText(GATEWAY_ALERT)).not.toBeInTheDocument();
  });

  it('rotates and replaces the key', () => {
    const first = mockMutation();
    const { onKeyChange, rerender } = renderIssuer();
    fireEvent.click(screen.getByRole('button', { name: 'Issue test key' }));
    expect(screen.getByDisplayValue('k1')).toBeInTheDocument();

    const second = mockMutation({ result: response({ apiKey: 'k2' }) });
    rerender();
    fireEvent.click(screen.getByRole('button', { name: 'Rotate' }));

    expect(first.mutate.mock.calls.length + second.mutate.mock.calls.length).toBe(2);
    expect(second.mutate).toHaveBeenCalledWith(params, expect.anything());
    expect(screen.getByDisplayValue('k2')).toBeInTheDocument();
    expect(screen.queryByDisplayValue('k1')).not.toBeInTheDocument();
    expect(lastCall(onKeyChange)).toBe('k2');
  });

  it('keeps the previous key when a rotate fails', () => {
    mockMutation();
    const { onKeyChange, rerender } = renderIssuer();
    fireEvent.click(screen.getByRole('button', { name: 'Issue test key' }));
    const callsBeforeRotate = onKeyChange.mock.calls.length;

    const failing = mockMutation({ isError: true, result: 'fail' });
    rerender();
    fireEvent.click(screen.getByRole('button', { name: 'Rotate' }));
    rerender();

    expect(failing.mutate).toHaveBeenCalledTimes(1);
    expect(screen.getByDisplayValue('k1')).toBeInTheDocument();
    expect(screen.getByText("Couldn't issue a test key. Try again.")).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Rotate' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument();
    expect(onKeyChange.mock.calls.slice(callsBeforeRotate)).not.toContainEqual([null]);
    expect(lastCall(onKeyChange)).toBe('k1');
  });

  it('offers Retry after a failed first issue', () => {
    mockMutation({ result: 'fail' });
    const { onKeyChange, rerender } = renderIssuer();
    fireEvent.click(screen.getByRole('button', { name: 'Issue test key' }));

    mockMutation({ isError: true, result: 'fail' });
    rerender();

    expect(screen.getByText("Couldn't issue a test key. Try again.")).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Test API key')).not.toBeInTheDocument();
    expect(lastCall(onKeyChange)).toBeNull();
  });
});
