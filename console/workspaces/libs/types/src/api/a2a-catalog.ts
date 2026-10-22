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

import {
  type AgentCardPathParams,
  type AgentCardSource,
  type AgentCardStatus,
} from './agent-card';
import {
  type AgentPathParams,
  type ListQuery,
  type OrgPathParams,
  type PaginationMeta,
} from './common';

export type A2ACatalogKindFilter = AgentCardSource | 'proxied';

export interface A2ACatalogEnvironment {
  name: string;
  status: AgentCardStatus;
  /** ISO date-time; omitted when the card row was never fetched. */
  fetchedAt?: string;
  proxied: boolean;
  cardDiffers: boolean;
  /** True only when q was given and this environment's card matched it. */
  matched: boolean;
}

export interface A2ACatalogAgent {
  projectName: string;
  agentName: string;
  kind: AgentCardSource;
  name: string;
  description: string;
  version: string;
  skills: string[];
  leadEnvironment: string;
  environments: A2ACatalogEnvironment[];
}

export interface A2ACatalogListResponse extends PaginationMeta {
  agents: A2ACatalogAgent[];
}

export interface A2ACatalogListQuery extends ListQuery {
  q?: string;
  kind?: A2ACatalogKindFilter;
  environment?: string;
  project?: string;
  status?: AgentCardStatus;
}

export type A2ACatalogListPathParams = OrgPathParams;
export type A2ACatalogEntryPathParams = AgentPathParams;
export type A2AConnectPathParams = AgentCardPathParams;

export type A2AConnectSecurityMode =
  | 'none'
  | 'apiKey'
  | 'oauth'
  | 'unmanaged'
  | 'unknown';

export interface A2AConnectIssuer {
  name: string;
  /** Empty when the name no longer resolves. */
  issuer: string;
}

export interface A2AConnectOAuth {
  issuers: A2AConnectIssuer[];
  audiences: string[];
  headerName: string;
  authHeaderPrefix: string;
}

export interface A2AConnectSecurity {
  mode: A2AConnectSecurityMode;
  /** X-API-Key; present only when mode is apiKey. */
  apiKeyHeader?: string;
  /** Present only when mode is oauth. */
  oauth?: A2AConnectOAuth;
}

export type A2AConnectPublicationStatus =
  | 'pending'
  | 'published'
  | 'failed'
  | 'none';

export interface A2AConnectPublication {
  /** none means a direct external agent, where the gateway isn't involved. */
  status: A2AConnectPublicationStatus;
  lastError: string;
}

export interface A2AConnectResponse {
  kind: AgentCardSource;
  proxied: boolean;
  isProduction: boolean;
  /** Empty for a direct external agent or while no gateway is mapped. */
  gatewayUrl: string;
  /**
   * The gateway's copy when the gateway fronts the agent, otherwise the card's own URL;
   * empty when neither is known.
   */
  cardUrl: string;
  /** Empty when there is none. */
  jsonRpcUrl: string;
  security: A2AConnectSecurity;
  publication: A2AConnectPublication;
}
