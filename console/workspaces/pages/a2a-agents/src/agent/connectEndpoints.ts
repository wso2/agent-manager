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

import type { A2AConnectResponse } from '@agent-management-platform/types';
import type { ParsedA2AInterface } from '@agent-management-platform/shared-component';

export const GATEWAY_ENDPOINT_LABEL = 'Gateway endpoint';
export const JSON_RPC_ENDPOINT_LABEL = 'JSON-RPC';
export const DEFAULT_ENDPOINT_LABEL = 'Endpoint';

export interface ConnectEndpoint {
  label: string;
  url: string;
}

export interface ConnectEndpoints {
  endpoints: ConnectEndpoint[];
  declared: ParsedA2AInterface[];
  cardUrl: string;
  snippetEndpoint: string;
}

export function resolveConnectEndpoints(
  connect: Pick<A2AConnectResponse, 'gatewayUrl' | 'cardUrl' | 'jsonRpcUrl'>,
  interfaces: ParsedA2AInterface[],
): ConnectEndpoints {
  const { gatewayUrl, cardUrl, jsonRpcUrl } = connect;
  if (gatewayUrl !== '') {
    const endpoints: ConnectEndpoint[] = [{ label: GATEWAY_ENDPOINT_LABEL, url: gatewayUrl }];
    if (jsonRpcUrl !== '') endpoints.push({ label: JSON_RPC_ENDPOINT_LABEL, url: jsonRpcUrl });
    return { endpoints, declared: interfaces, cardUrl, snippetEndpoint: jsonRpcUrl };
  }
  return {
    endpoints: interfaces.map(({ url, protocolBinding }) => ({
      label: protocolBinding || DEFAULT_ENDPOINT_LABEL,
      url,
    })),
    declared: [],
    cardUrl,
    snippetEndpoint: jsonRpcUrl,
  };
}
