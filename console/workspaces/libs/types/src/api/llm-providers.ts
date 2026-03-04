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

import type { OrgPathParams, OrgProjPathParams } from "./common";

export interface LLMProviderTemplateMetadata {
  description?: string;
  displayName?: string;
  logoUrl?: string;
  endpointUrl?: string;
  auth?: {
    type?: string;
    header?: string;
    valuePrefix?: string;
  };
  tags?: string[];
}

export interface LLMProviderTemplateAuth {
  type?: string;
  config?: Record<string, unknown>;
}

export interface LLMProviderTemplateResponse {
  id: string;
  name: string;
  orgName: string;
  metadata?: LLMProviderTemplateMetadata;
  auth?: LLMProviderTemplateAuth;
  createdAt: string;
  updatedAt: string;
}

export interface LLMProviderTemplateListResponse {
  templates: LLMProviderTemplateResponse[];
  total: number;
}

export interface CreateLLMProviderTemplateRequest {
  name: string;
  metadata?: LLMProviderTemplateMetadata;
  auth?: LLMProviderTemplateAuth;
}

export interface UpdateLLMProviderTemplateRequest {
  metadata?: LLMProviderTemplateMetadata;
  auth?: LLMProviderTemplateAuth;
}

export type ListLLMProviderTemplatesPathParams = OrgPathParams;
export type CreateLLMProviderTemplatePathParams = OrgPathParams;

export interface LLMProviderTemplatePathParams extends OrgPathParams {
  templateId: string | undefined;
}

export type GetLLMProviderTemplatePathParams = LLMProviderTemplatePathParams;
export type UpdateLLMProviderTemplatePathParams = LLMProviderTemplatePathParams;
export type DeleteLLMProviderTemplatePathParams = LLMProviderTemplatePathParams;

// -----------------------------------------------------------------------------
// Nested configuration types (mirroring OpenAPI)
// -----------------------------------------------------------------------------

export interface LLMModelProvider {
  id: string;
  name?: string;
  models?: LLMModel[];
}

export interface LLMModel {
  id: string;
  name?: string;
}

export type UpstreamAuthType = "apiKey" | "bearer" | "basic" | "none";

export interface UpstreamAuth {
  type: UpstreamAuthType;
  header?: string;
  value?: string;
}

export interface UpstreamEndpoint {
  url?: string;
  ref?: string;
  auth?: UpstreamAuth;
}

export interface UpstreamConfig {
  main?: UpstreamEndpoint;
  sandbox?: UpstreamEndpoint;
}

export type AccessControlMode = "allow" | "deny";

export interface RouteException {
  path: string;
  methods: string[];
}

export interface LLMAccessControl {
  mode: AccessControlMode;
  exceptions?: RouteException[];
}

export interface LLMPolicyPath {
  path: string;
  methods: string[];
  params?: Record<string, unknown>;
}

export interface LLMPolicy {
  name: string;
  version: string;
  paths: LLMPolicyPath[];
}

export interface RateLimitingLimitConfig {
  request?: RequestRateLimit;
  token?: TokenRateLimit;
  cost?: CostRateLimit;
}

export interface RateLimitResetWindow {
  unit?: string;
  value?: number;
}

export interface RequestRateLimit {
  enabled: boolean;
  count: number;
  reset: RateLimitResetWindow;
}

export interface TokenRateLimit {
  enabled: boolean;
  count: number;
  reset: RateLimitResetWindow;
}

export interface CostRateLimit {
  enabled: boolean;
  amount: number;
  reset: RateLimitResetWindow;
}

export interface RateLimitingResourceLimit {
  resource: string;
  limit: RateLimitingLimitConfig;
}

export interface ResourceWiseRateLimitingConfig {
  default: RateLimitingLimitConfig;
  resources: RateLimitingResourceLimit[];
}

export interface RateLimitingScopeConfig {
  global?: RateLimitingLimitConfig;
  resourceWise?: ResourceWiseRateLimitingConfig;
}

export interface LLMRateLimitingConfig {
  providerLevel?: RateLimitingScopeConfig;
  consumerLevel?: RateLimitingScopeConfig;
}

export type APIKeyLocation = "header" | "query" | "cookie";

export interface APIKeySecurity {
  enabled?: boolean;
  key?: string;
  in?: APIKeyLocation;
}

export interface SecurityConfig {
  enabled?: boolean;
  apiKey?: APIKeySecurity;
}

export interface LLMProviderConfig {
  name?: string;
  version?: string;
  context?: string;
  vhost?: string;
  template?: string;
  upstream?: UpstreamConfig;
  accessControl?: LLMAccessControl;
  rateLimiting?: LLMRateLimitingConfig;
  policies?: LLMPolicy[];
  security?: SecurityConfig;
}

export interface Artifact {
  uuid?: string;
  name?: string;
  displayName?: string;
  description?: string;
  status?: string;
}

// -----------------------------------------------------------------------------
// LLM providers (Create/Update/List/Response)
// -----------------------------------------------------------------------------

export interface CreateLLMProviderRequest {
  description?: string;
  /**
   * Handle of the template being used (e.g., "openai").
   */
  templateHandle: string;
  /**
   * Custom OpenAPI specification that can override the template.
   */
  openapi?: string;
  /**
   * Optional custom model list that can override the template.
   */
  modelList?: LLMModelProvider[];
  /**
   * Full configuration for the provider.
   */
  configuration: LLMProviderConfig;
  /**
   * Optional list of gateway UUIDs this provider should be attached to.
   */
  gateways?: string[];
}

export interface UpdateLLMProviderRequest {
  description?: string;
  templateHandle?: string;
  openapi?: string;
  modelList?: LLMModelProvider[];
  configuration?: LLMProviderConfig;
  gateways?: string[];
}

export interface LLMProviderResponse {
  uuid: string;
  description?: string;
  createdBy?: string;
  templateHandle: string;
  openapi?: string;
  modelProviders?: LLMModelProvider[];
  status: string;
  configuration: LLMProviderConfig;
  artifact?: Artifact;
  gateways?: string[];
  inCatalog: boolean;
}

export interface LLMProviderListResponse {
  providers: LLMProviderResponse[];
  total: number;
  limit: number;
  offset: number;
}

export interface UpdateLLMProviderCatalogRequest {
  inCatalog: boolean;
}

export type ListLLMProvidersPathParams = OrgPathParams;
export type CreateLLMProviderPathParams = OrgPathParams;

export interface LLMProviderPathParams extends OrgPathParams {
  /**
   * Provider UUID (maps to `{id}` in the path).
   */
  providerId: string | undefined;
}

export type GetLLMProviderPathParams = LLMProviderPathParams;
export type UpdateLLMProviderPathParams = LLMProviderPathParams;
export type DeleteLLMProviderPathParams = LLMProviderPathParams;
export type UpdateLLMProviderCatalogPathParams = LLMProviderPathParams;
export type ListLLMProviderProxiesPathParams = LLMProviderPathParams;

// -----------------------------------------------------------------------------
// LLM proxies
// -----------------------------------------------------------------------------

export interface LLMProxyConfig {
  name?: string;
  version?: string;
  context?: string;
  vhost?: string;
  provider?: string;
  policies?: LLMPolicy[];
  security?: SecurityConfig;
}

export interface CreateLLMProxyRequest {
  description?: string;
  providerUuid: string;
  openapi?: string;
  configuration: LLMProxyConfig;
}

export interface UpdateLLMProxyRequest {
  description?: string;
  providerUuid?: string;
  openapi?: string;
  configuration?: LLMProxyConfig;
}

export interface LLMProxyResponse {
  uuid: string;
  projectId: string;
  providerUuid: string;
  status: string;
  description?: string;
  createdBy?: string;
  openapi?: string;
  configuration: LLMProxyConfig;
  artifact?: Artifact;
}

export interface LLMProxyListResponse {
  proxies: LLMProxyResponse[];
  total: number;
  limit: number;
  offset: number;
}

export type ListLLMProxiesPathParams = OrgProjPathParams;
export type CreateLLMProxyPathParams = OrgProjPathParams;

export interface LLMProxyPathParams extends OrgProjPathParams {
  proxyId: string | undefined;
}

export type GetLLMProxyPathParams = LLMProxyPathParams;
export type UpdateLLMProxyPathParams = LLMProxyPathParams;
export type DeleteLLMProxyPathParams = LLMProxyPathParams;

// -----------------------------------------------------------------------------
// LLM deployments (kept simple – spec-compatible but minimal)
// -----------------------------------------------------------------------------

export interface LLMDeploymentResponse {
  id: string;
  providerId: string;
  environmentId: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface LLMDeploymentListResponse {
  deployments: LLMDeploymentResponse[];
  total: number;
}

export interface CreateLLMDeploymentRequest {
  environmentId: string;
}

export type ListLLMDeploymentsPathParams = LLMProviderPathParams;
export type CreateLLMDeploymentPathParams = LLMProviderPathParams;
export type UndeployLLMProviderPathParams = LLMProviderPathParams;
export type RestoreLLMDeploymentPathParams = LLMProviderPathParams;

export interface LLMDeploymentPathParams extends LLMProviderPathParams {
  deploymentId: string | undefined;
}

export type GetLLMDeploymentPathParams = LLMDeploymentPathParams;
export type DeleteLLMDeploymentPathParams = LLMDeploymentPathParams;

