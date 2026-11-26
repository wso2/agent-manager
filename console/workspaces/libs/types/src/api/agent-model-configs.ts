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

import type { AgentPathParams, ListQuery, ProjectScopedDocumentMetadata } from "./common";
import type {
  CreateLLMAPIKeyRequest,
  CreateLLMAPIKeyResponse,
  LLMPolicy,
  Resilience,
  RotateLLMAPIKeyRequest,
  RotateLLMAPIKeyResponse,
} from "./llm-providers";

// -----------------------------------------------------------------------------
// Agent Model Config - Environment mappings
// -----------------------------------------------------------------------------

export interface EnvironmentVariableConfig {
  key: string;
  name: string;
}

export interface EnvProviderConfiguration {
  policies?: LLMPolicy[];
  /**
   * Overrides the proxy's request/idle timeout for this environment. When
   * omitted, the gateway's own default timeout applies.
   */
  resilience?: Resilience;
}

export interface EnvModelConfigRequest {
  providerName?: string;
  providerUuid?: string;
  proxyName?: string;
  proxyId?: string;
  mcpProxyName?: string;
  mcpProxyId?: string;
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
  providerName?: string;
  proxyUuid?: string;
  providerUuid?: string;
  proxyName?: string;
  proxyId?: string;
  mcpProxyName?: string;
  mcpProxyId?: string;
  url?: string;
  authInfo?: AuthInfo;
  policies?: LLMPolicy[];
  /**
   * Guardrails configured on the org-level LLM provider itself; applied in
   * addition to `policies`.
   */
  providerPolicies?: LLMPolicy[];
  resilience?: Resilience;
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
  envModelConfig?: Record<string, EnvProviderConfigMappings>;
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
// Documents: one binding in the default environment
// -----------------------------------------------------------------------------

export type AgentConfigKind = "AgentModelConfig" | "AgentMcpConfig";

/** Spec of a config document; providerName binds a model config, proxyName an MCP one. */
export interface AgentConfigSpec {
  description?: string;
  providerName?: string;
  proxyName?: string;
  configuration?: EnvProviderConfiguration;
  environmentVariables?: EnvironmentVariableConfig[];
}

/** How the agent reaches the binding through the gateway. */
export interface AgentConfigStatus {
  url?: string;
  authInfo?: AuthInfo;
  proxyUuid?: string;
  proxyName?: string;
  providerUuid?: string;
  providerPolicies?: LLMPolicy[];
}

export interface AgentConfigDocument {
  apiVersion: string;
  kind: AgentConfigKind;
  metadata: ProjectScopedDocumentMetadata;
  spec: AgentConfigSpec;
  status?: AgentConfigStatus;
}

export interface AgentConfigDocumentList {
  count: number;
  list: AgentConfigDocument[];
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

/** Path params with required configId for get/update/delete operations */
export interface AgentModelConfigRequiredPathParams extends AgentPathParams {
  configId: string;
}

export type GetAgentModelConfigPathParams = AgentModelConfigRequiredPathParams;
export type UpdateAgentModelConfigPathParams =
  AgentModelConfigRequiredPathParams;
export type DeleteAgentModelConfigPathParams =
  AgentModelConfigRequiredPathParams;

export type ListAgentModelConfigsQuery = ListQuery;

// -----------------------------------------------------------------------------
// Per-config LLM API keys (external agents) — keyed by configuration + environment
// -----------------------------------------------------------------------------

export interface LLMConfigAPIKeysPathParams extends AgentPathParams {
  configId: string | undefined;
  envName: string | undefined;
}

export interface LLMConfigAPIKeyPathParams extends LLMConfigAPIKeysPathParams {
  keyName: string | undefined;
}

export type ListLLMConfigAPIKeysPathParams = LLMConfigAPIKeysPathParams;
export type CreateLLMConfigAPIKeyPathParams = LLMConfigAPIKeysPathParams;
export type RotateLLMConfigAPIKeyPathParams = LLMConfigAPIKeyPathParams;
export type RevokeLLMConfigAPIKeyPathParams = LLMConfigAPIKeyPathParams;

export type CreateLLMConfigAPIKeyRequest = CreateLLMAPIKeyRequest;
export type CreateLLMConfigAPIKeyResponse = CreateLLMAPIKeyResponse;
export type RotateLLMConfigAPIKeyRequest = RotateLLMAPIKeyRequest;
export type RotateLLMConfigAPIKeyResponse = RotateLLMAPIKeyResponse;
