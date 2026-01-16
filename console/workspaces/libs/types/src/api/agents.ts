/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { type AgentPathParams, type RuntimeConfiguration, type ListQuery, type OrgProjPathParams, type PaginationMeta, type RepositoryConfig } from './common';

// Requests
export interface CreateAgentRequest {
  name: string;
  displayName: string;
  description?: string;
  provisioning: Provisioning;
  agentType?: AgentType;
  runtimeConfigs?: RuntimeConfiguration;
  inputInterface?: InputInterface;
}

export type InputInterfaceType = 'DEFAULT' | 'CUSTOM';

export interface InputInterface {
  type: string; // Always "HTTP" for now
  port?: number;
  schema?: {
    path: string;
  };
  basePath?: string;
}

export interface AgentType {
  type: string;
  subType: string;
}

export type ProvisioningType = 'internal' | 'external';

export interface Provisioning {
  type: ProvisioningType;
  repository?: RepositoryConfig;
}

export interface AgentResponse {
  name: string;
  displayName: string;
  description: string;
  createdAt: string; // ISO date-time
  projectName: string;
  status?: string;
  provisioning: Provisioning;
  agentType?: AgentType;
  runtimeConfigs?: RuntimeConfiguration;
  uuid?: string;
}

export interface AgentListResponse extends PaginationMeta {
  agents: AgentResponse[];
}

// Path/Query helpers
export type ListAgentsPathParams = OrgProjPathParams;
export type CreateAgentPathParams = OrgProjPathParams;
export type GetAgentPathParams = AgentPathParams;
export type DeleteAgentPathParams = AgentPathParams;
export type ListAgentsQuery = ListQuery;

// Agent Token
export interface TokenRequest {
  expires_in?: string; // Go duration format (e.g., "720h" for 30 days, "8760h" for 1 year)
}

export interface TokenResponse {
  token: string;
  expires_at: number; // Unix timestamp
  issued_at: number; // Unix timestamp
  token_type: string; // "Bearer"
}

export interface GenerateAgentTokenPathParams extends AgentPathParams {}

export interface GenerateAgentTokenQuery {
  environment?: string;
}


