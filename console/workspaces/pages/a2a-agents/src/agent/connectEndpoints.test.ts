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

import { describe, expect, it } from 'vitest';
import { resolveConnectEndpoints } from './connectEndpoints';

const jsonrpc = { url: 'http://localhost:9099/rpc', protocolBinding: 'JSONRPC' };
const rest = { url: 'https://origin.example/rest', protocolBinding: 'HTTP+JSON' };
const none = { gatewayUrl: '', cardUrl: '', jsonRpcUrl: '' };

describe('resolveConnectEndpoints', () => {
  it('gateway case', () => {
    expect(resolveConnectEndpoints({ gatewayUrl: 'https://gw.example/orders', cardUrl: 'https://gw.example/orders/.well-known/agent-card.json', jsonRpcUrl: 'https://gw.example/orders/rpc' }, [jsonrpc]))
      .toEqual({ endpoints: [{ label: 'Gateway endpoint', url: 'https://gw.example/orders' }, { label: 'JSON-RPC', url: 'https://gw.example/orders/rpc' }], declared: [jsonrpc], cardUrl: 'https://gw.example/orders/.well-known/agent-card.json', snippetEndpoint: 'https://gw.example/orders/rpc' });
  });
  it('gateway case without JSON-RPC', () => {
    const r = resolveConnectEndpoints({ gatewayUrl: 'https://gw', cardUrl: '', jsonRpcUrl: '' }, [jsonrpc]);
    expect(r.endpoints).toEqual([{ label: 'Gateway endpoint', url: 'https://gw' }]);
    expect(r.snippetEndpoint).toBe('');
  });
  it('direct case', () => {
    expect(resolveConnectEndpoints({ gatewayUrl: '', cardUrl: 'https://origin.example/card', jsonRpcUrl: 'https://origin.example/rpc' }, [rest, { url: 'https://origin.example/rpc', protocolBinding: 'JSONRPC' }]))
      .toEqual({ endpoints: [{ label: 'HTTP+JSON', url: 'https://origin.example/rest' }, { label: 'JSONRPC', url: 'https://origin.example/rpc' }], declared: [], cardUrl: 'https://origin.example/card', snippetEndpoint: 'https://origin.example/rpc' });
  });
  it('never falls back to a non-JSON-RPC interface', () => {
    expect(resolveConnectEndpoints(none, [rest]).snippetEndpoint).toBe('');
  });
  it('direct case with no interfaces', () => {
    expect(resolveConnectEndpoints(none, [])).toEqual({ endpoints: [], declared: [], cardUrl: '', snippetEndpoint: '' });
  });
  it('labels a binding-less interface Endpoint', () => {
    expect(resolveConnectEndpoints(none, [{ url: 'https://o/x', protocolBinding: '' }]).endpoints[0].label).toBe('Endpoint');
  });
});
