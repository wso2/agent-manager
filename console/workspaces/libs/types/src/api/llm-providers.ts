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

import type { EnvironmentVariable, OrgPathParams, OrgProjPathParams } from "./common";

// -----------------------------------------------------------------------------
// ExtractionIdentifier — used for token/model extraction in templates
// -----------------------------------------------------------------------------

export type ExtractionIdentifierLocation = "payload" | "header" | "queryParam" | "pathParam";

export interface ExtractionIdentifier {
  location: ExtractionIdentifierLocation;
  identifier: string;
}

// -----------------------------------------------------------------------------
// LLM Provider Templates
// -----------------------------------------------------------------------------

export interface LLMProviderTemplateMetadata {
  endpointUrl?: string;
  logoUrl?: string;
  openapiSpecUrl?: string;
  auth?: LLMProviderTemplateAuth;
}

export interface LLMProviderTemplateAuth {
  type?: string;
  header?: string;
  valuePrefix?: string;
}

export interface LLMProviderTemplateResponse {
  uuid: string;
  id: string;
  name: string;
  description?: string;
  createdBy?: string;
  metadata?: LLMProviderTemplateMetadata;
  promptTokens?: ExtractionIdentifier;
  completionTokens?: ExtractionIdentifier;
  totalTokens?: ExtractionIdentifier;
  remainingTokens?: ExtractionIdentifier;
  requestModel?: ExtractionIdentifier;
  responseModel?: ExtractionIdentifier;
  createdAt: string;
  updatedAt: string;
}

export interface LLMProviderTemplateListResponse {
  templates: LLMProviderTemplateResponse[];
  total: number;
  limit: number;
  offset: number;
}

export interface CreateLLMProviderTemplateRequest {
  id: string;
  name: string;
  description?: string;
  metadata?: LLMProviderTemplateMetadata;
  promptTokens?: ExtractionIdentifier;
  completionTokens?: ExtractionIdentifier;
  totalTokens?: ExtractionIdentifier;
  remainingTokens?: ExtractionIdentifier;
  requestModel?: ExtractionIdentifier;
  responseModel?: ExtractionIdentifier;
}

export interface UpdateLLMProviderTemplateRequest {
  name?: string;
  description?: string;
  metadata?: LLMProviderTemplateMetadata;
  promptTokens?: ExtractionIdentifier;
  completionTokens?: ExtractionIdentifier;
  totalTokens?: ExtractionIdentifier;
  remainingTokens?: ExtractionIdentifier;
  requestModel?: ExtractionIdentifier;
  responseModel?: ExtractionIdentifier;
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
  description?: string;
}

export type UpstreamAuthType = "api-key" | "none";

export interface UpstreamAuth {
  type: UpstreamAuthType;
  header?: string;
  value?: string;
}

export interface Resilience {
  /**
   * Max duration for the whole request→upstream-response. "0s" disables it;
   * omit to use the gateway's default.
   */
  timeout?: string;
  /** Per-route stream idle timeout. "0s" disables it; omit to use the gateway's default. */
  idleTimeout?: string;
}

export interface UpstreamEndpoint {
  url?: string;
  ref?: string;
  auth?: UpstreamAuth;
  /**
   * Environment UUIDs this endpoint serves. UUIDs (not names) are stored so the
   * mapping survives environment renames. Used by MCP proxies that configure a
   * separate upstream per environment; empty for the classic main/sandbox shape.
   */
  environments?: string[];
}

export interface UpstreamConfig {
  main?: UpstreamEndpoint;
  sandbox?: UpstreamEndpoint;
  /**
   * Per-environment upstream endpoints. When present, `main` is derived from these
   * server-side for backwards compatibility.
   */
  endpoints?: UpstreamEndpoint[];
}

export type AccessControlMode = "allow_all" | "deny_all";

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

export interface LLMPolicyDefinition {
  name: string;
  version: string;
  displayName?: string;
  description?: string;
  parameters?: Record<string, unknown>;
  systemParameters?: Record<string, unknown>;
}

export interface LLMPolicyAvailabilityResponse {
  count: number;
  list: LLMPolicyDefinition[];
}

export type ListAvailableLLMPoliciesPathParams = OrgPathParams;

export interface RateLimitingLimitConfig {
  request?: RequestRateLimit;
  token?: TokenRateLimit;
  cost?: CostRateLimit;
}

export interface RateLimitResetWindow {
  duration: number;
  unit: string;
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

export type APIKeyLocation = "header" | "query";

export interface APIKeySecurity {
  enabled?: boolean;
  key?: string;
  in?: APIKeyLocation;
}

/**
 * Agent Identity security — callers present a JWT issued by the environment's
 * IdP. v1 pins the issuer to the ThunderKeyManager key manager.
 */
export interface IdentitySecurity {
  enabled?: boolean;
}

/**
 * Exactly one variant is active: apiKey or identity (both omitted /
 * enabled=false means no security).
 */
export interface SecurityConfig {
  enabled?: boolean;
  apiKey?: APIKeySecurity;
  identity?: IdentitySecurity;
}

/**
 * @deprecated Use the flat fields on CreateLLMProviderRequest / LLMProviderResponse instead.
 * Kept for backward compatibility with form state in UI components.
 */
export interface LLMProviderConfig {
  name?: string;
  version?: string;
  context?: string;
  vhost?: string;
  template?: string;
  upstream?: UpstreamConfig;
  resilience?: Resilience;
  accessControl?: LLMAccessControl;
  rateLimiting?: LLMRateLimitingConfig;
  policies?: LLMPolicy[];
  security?: SecurityConfig;
}

// -----------------------------------------------------------------------------
// LLM providers (Create/Update/List/Response)
// -----------------------------------------------------------------------------

export interface CreateLLMProviderRequest {
  id: string;
  name: string;
  version: string;
  context: string;
  template: string;
  upstream: UpstreamConfig;
  resilience?: Resilience;
  description?: string;
  accessControl?: LLMAccessControl;
  policies?: LLMPolicy[];
  openapi?: string;
  modelProviders?: LLMModelProvider[];
  rateLimiting?: LLMRateLimitingConfig;
  security?: SecurityConfig;
  gateways?: string[];
}

export interface UpdateLLMProviderRequest {
  name?: string;
  description?: string;
  version?: string;
  context?: string;
  template?: string;
  upstream?: UpstreamConfig;
  resilience?: Resilience;
  accessControl?: LLMAccessControl;
  policies?: LLMPolicy[];
  openapi?: string;
  modelProviders?: LLMModelProvider[];
  rateLimiting?: LLMRateLimitingConfig;
  security?: SecurityConfig;
  gateways?: string[];
}

export interface LLMProviderListItem {
  uuid: string;
  id: string;
  name: string;
  template: string;
  status: "pending" | "deployed" | "failed";
  createdBy?: string;
  gateways?: string[];
  createdAt?: string;
  updatedAt?: string;
}

export interface LLMProviderResponse {
  uuid: string;
  id: string;
  name: string;
  version: string;
  context: string;
  template: string;
  upstream: UpstreamConfig;
  resilience?: Resilience;
  status: "pending" | "deployed" | "failed";
  description?: string;
  createdBy?: string;
  accessControl?: LLMAccessControl;
  policies?: LLMPolicy[];
  openapi?: string;
  modelProviders?: LLMModelProvider[];
  rateLimiting?: LLMRateLimitingConfig;
  security?: SecurityConfig;
  gateways?: string[];
  inCatalog?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface LLMProviderListResponse {
  providers: LLMProviderListItem[];
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
export type GenerateEvaluatorPathParams = LLMProviderPathParams;

/**
 * One-shot evaluator authoring request. Nothing here is persisted by the backend —
 * the provider and model are chosen per generation and forgotten afterwards.
 */
export interface GenerateEvaluatorRequest {
  /** Model identifier, typed by the user and passed to the upstream verbatim. */
  model: string;
  evaluatorType: "code" | "llm_judge";
  level: "trace" | "agent" | "llm";
  /** Natural-language description of what the evaluator should measure. */
  instructions: string;
  displayName?: string;
  description?: string;
}

export interface GenerateEvaluatorResponse {
  /** Generated Python source (`code`) or prompt template (`llm_judge`). */
  code: string;
}

// -----------------------------------------------------------------------------
// LLM proxies
// -----------------------------------------------------------------------------

export interface LLMProxyConfig {
  name?: string;
  version?: string;
  context?: string;
  vhost?: string;
  provider?: string;
  resilience?: Resilience;
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
// LLM deployments
// -----------------------------------------------------------------------------

export interface LLMDeploymentResponse {
  agentName: string;
  projectName: string;
  imageId: string;
  environment: string;
  /** Gateway UUID where the provider is deployed (returned by API) */
  gatewayId?: string;
}

export type LLMDeploymentListResponse = LLMDeploymentResponse[];

/** @deprecated Use DeployLLMProviderRequest for LLM provider deployments */
export interface CreateLLMDeploymentRequest {
  imageId: string;
  env?: EnvironmentVariable[];
  enableAutoInstrumentation?: boolean;
}

/** Request body for deploying an LLM provider to a gateway */
export interface DeployLLMProviderRequest {
  name: string;
  gatewayId: string;
  base?: string;
}

export interface UndeployLLMProviderQuery {
  deploymentId: string;
  gatewayId: string;
}

export interface RestoreLLMDeploymentQuery {
  deploymentId: string;
  gatewayId: string;
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

// -----------------------------------------------------------------------------
// LLM API keys (provider)
// -----------------------------------------------------------------------------

export interface CreateLLMAPIKeyRequest {
  name?: string;
  displayName?: string;
  expiresAt?: string;
}

export interface CreateLLMAPIKeyResponse {
  status: string;
  message: string;
  keyId?: string;
  apiKey?: string;
}

export interface RotateLLMAPIKeyRequest {
  displayName?: string;
  expiresAt?: string;
}

export interface RotateLLMAPIKeyResponse {
  status: string;
  message: string;
  keyId?: string;
  apiKey?: string;
}

export interface LLMProviderAPIKeyPathParams extends LLMProviderPathParams {
  keyName: string | undefined;
}

export type CreateLLMProviderAPIKeyPathParams = LLMProviderPathParams;
export type RotateLLMProviderAPIKeyPathParams = LLMProviderAPIKeyPathParams;
export type RevokeLLMProviderAPIKeyPathParams = LLMProviderAPIKeyPathParams;
export type ListLLMProviderAPIKeysPathParams = LLMProviderPathParams;

// -----------------------------------------------------------------------------
// LLM API keys (proxy)
// -----------------------------------------------------------------------------

export interface LLMProxyAPIKeyPathParams extends LLMProxyPathParams {
  keyName: string | undefined;
}

export type CreateLLMProxyAPIKeyPathParams = LLMProxyPathParams;
export type RotateLLMProxyAPIKeyPathParams = LLMProxyAPIKeyPathParams;
export type RevokeLLMProxyAPIKeyPathParams = LLMProxyAPIKeyPathParams;
export type ListLLMProxyAPIKeysPathParams = LLMProxyPathParams;

// -----------------------------------------------------------------------------
// LLM provider consumers
// -----------------------------------------------------------------------------

export type LLMProviderConsumerType = "agent" | "monitor";

export interface LLMProviderConsumerItem {
  proxyId: string;
  proxyName: string;
  projectName: string;
  consumerType: LLMProviderConsumerType;
  consumerName: string;
}

export interface LLMProviderConsumerListResponse {
  consumers: LLMProviderConsumerItem[];
  total: number;
}

export type ListLLMProviderConsumersPathParams = LLMProviderPathParams;
