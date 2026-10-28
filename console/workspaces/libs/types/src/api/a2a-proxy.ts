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

import { type AgentCardPathParams } from './agent-card';

export type A2AProxyStatus = 'pending' | 'published' | 'failed';

export interface A2AProxyResponse {
  status: A2AProxyStatus;
  lastError: string;
  /** Base URL on the ingress gateway, serving the card once published. Empty with no gateway. */
  gatewayUrl: string;
  /** The header the gateway adds upstream; empty when none. Non-empty means a secret is stored. */
  upstreamAuthHeader: string;
}

export interface A2AProxyUpstreamAuth {
  header: string;
  /** Omit to keep the stored secret. Never returned by the API. */
  value?: string;
}

/** Omitting upstreamAuth clears the stored credential. */
export interface SetA2AProxyRequest {
  upstreamAuth?: A2AProxyUpstreamAuth;
}

export type A2AProxyPathParams = AgentCardPathParams;
