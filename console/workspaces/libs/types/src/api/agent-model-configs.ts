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

import type { AgentPathParams, ListQuery } from "./common";
import type { LLMPolicy } from "./llm-providers";

// -----------------------------------------------------------------------------
// Agent Model Config - Environment mappings
// -----------------------------------------------------------------------------

export interface EnvironmentVariableConfig {
  key: string;
  name: string;
}

export interface EnvProviderConfiguration {
  policies?: LLMPolicy[];
}

export interface EnvModelConfigRequest {
  providerName?: string;
  providerUuid?: string;
  configuration: EnvProviderConfiguration;
}

export type AgentModelConfigType = "llm" | "mcp" | "other";

export interface CreateAgentModelConfigRequest {
  name: string;
  description?: string;
  type: AgentModelConfigType;
  envMappings: Record<string, EnvModelConfigRequest>;
  environmentVariables?: EnvironmentVariableConfig[];
}

export interface UpdateAgentModelConfigRequest {
  name?: string;
  description?: string;
  envMappings?: Record<string, EnvModelConfigRequest>;
  environmentVariables?: EnvironmentVariableConfig[];
}

// -----------------------------------------------------------------------------
// Response types
// -----------------------------------------------------------------------------

export interface ProviderConfig {
  providerName: string;
  proxyUuid: string;
  providerUuid?: string;
  url: string;
  authInfo?: AuthInfo;
  policies?: LLMPolicy[];
  status?: string;
}

export interface AuthInfo {
  type: string;
  in: string;
  name: string;
  value?: string;
}

export interface EnvProviderConfigMappings {
  environmentName: string;
  configuration?: ProviderConfig;
}

export interface AgentModelConfigResponse {
  uuid: string;
  name: string;
  description?: string;
  agentId: string;
  type: AgentModelConfigType;
  organizationName: string;
  projectName: string;
  envMappings: Record<string, EnvProviderConfigMappings>;
  environmentVariables: EnvironmentVariableConfig[];
  createdAt: string;
  updatedAt: string;
}

export interface PaginationInfo {
  count: number;
  offset: number;
  limit: number;
}

export interface AgentModelConfigListItem {
  uuid: string;
  name: string;
  description?: string;
  agentId: string;
  type: AgentModelConfigType;
  organizationName: string;
  projectName: string;
  createdAt: string;
  updatedAt?: string;
}

export interface AgentModelConfigListResponse {
  configs: AgentModelConfigListItem[];
  pagination: PaginationInfo;
}

// -----------------------------------------------------------------------------
// Path params
// -----------------------------------------------------------------------------

export type ListAgentModelConfigsPathParams = AgentPathParams;
export type CreateAgentModelConfigPathParams = AgentPathParams;

export interface AgentModelConfigPathParams extends AgentPathParams {
  configId: string | undefined;
}

export type GetAgentModelConfigPathParams = AgentModelConfigPathParams;
export type UpdateAgentModelConfigPathParams = AgentModelConfigPathParams;
export type DeleteAgentModelConfigPathParams = AgentModelConfigPathParams;

export type ListAgentModelConfigsQuery = ListQuery;
